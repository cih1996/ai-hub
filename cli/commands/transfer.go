package commands

import (
	"ai-hub/cli/client"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// RunTransfer executes the transfer command
func RunTransfer(c *client.Client, args []string) int {
	if len(args) == 0 {
		printTransferHelp()
		return 0
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "send":
		return runTransferSend(c, subArgs)
	case "pull":
		return runTransferPull(c, subArgs)
	case "list":
		return runTransferList(c, subArgs)
	case "status":
		return runTransferStatus(c, subArgs)
	case "delete":
		return runTransferDelete(c, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown transfer subcommand: %s\n", sub)
		printTransferHelp()
		return 1
	}
}

func printTransferHelp() {
	fmt.Println(`AI Hub Transfer - Cross-machine file transfer

Usage:
  ai-hub transfer <subcommand> [flags]

Subcommands:
  send     Upload a file to remote
  pull     Download a file from remote
  list     List transfer records
  status   Check transfer status
  delete   Delete transfer record and file

Examples:
  ai-hub transfer send --file ./data.zip --remote http://192.168.1.100
  ai-hub transfer pull --remote http://192.168.1.100 --id <transfer_id> --save ./data.zip
  ai-hub transfer list --remote http://192.168.1.100
  ai-hub transfer status <transfer_id> --remote http://192.168.1.100
  ai-hub transfer delete <transfer_id> --remote http://192.168.1.100`)
}

// resolveBaseURL builds the API base URL from --remote flag or local client
func resolveBaseURL(c *client.Client, remoteURL string) string {
	if remoteURL != "" {
		url := strings.TrimSuffix(remoteURL, "/")
		// Add default port if not specified
		afterScheme := url[strings.Index(url, "://")+3:]
		if !strings.Contains(afterScheme, ":") {
			url += ":9527"
		}
		return url + "/api/v1"
	}
	return c.BaseURL
}

func runTransferSend(c *client.Client, args []string) int {
	var filePath, remoteURL, savePath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file":
			if i+1 < len(args) {
				i++
				filePath = args[i]
			}
		case "--remote":
			if i+1 < len(args) {
				i++
				remoteURL = args[i]
			}
		case "--save":
			if i+1 < len(args) {
				i++
				savePath = args[i]
			}
		}
	}

	if filePath == "" {
		fmt.Fprintf(os.Stderr, "Error: --file is required\n")
		return 1
	}

	// 检查文件存在
	info, err := os.Stat(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: file not found: %s\n", filePath)
		return 1
	}
	fileSize := info.Size()
	fileName := filepath.Base(filePath)
	baseURL := resolveBaseURL(c, remoteURL)

	httpClient := &http.Client{Timeout: 0} // no timeout for large files

	// Step 1: 初始化上传
	initBody := map[string]interface{}{
		"filename":  fileName,
		"file_size": fileSize,
	}
	if savePath != "" {
		initBody["save_path"] = savePath
	}

	bodyBytes, _ := json.Marshal(initBody)
	resp, err := httpClient.Post(baseURL+"/transfer/upload", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: init upload failed: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	var initResp struct {
		ID          string `json:"id"`
		ChunkSize   int64  `json:"chunk_size"`
		TotalChunks int    `json:"total_chunks"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: parse init response: %v\n", err)
		return 1
	}
	if initResp.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", initResp.Error)
		return 1
	}

	fmt.Printf("Transfer ID: %s\n", initResp.ID)
	fmt.Printf("File: %s (%s)\n", fileName, formatSize(fileSize))
	fmt.Printf("Chunks: %d x %s\n", initResp.TotalChunks, formatSize(initResp.ChunkSize))

	// Step 2: 分块上传
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: open file: %v\n", err)
		return 1
	}
	defer file.Close()

	buf := make([]byte, initResp.ChunkSize)
	startTime := time.Now()

	for i := 0; i < initResp.TotalChunks; i++ {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Error: read chunk %d: %v\n", i, err)
			return 1
		}

		// 构建 multipart 请求
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("chunk", fmt.Sprintf("chunk_%06d", i))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: create form: %v\n", err)
			return 1
		}
		part.Write(buf[:n])
		writer.Close()

		url := fmt.Sprintf("%s/transfer/upload/%s/chunk?index=%d", baseURL, initResp.ID, i)
		req, _ := http.NewRequest("PUT", url, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		chunkResp, err := httpClient.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: upload chunk %d: %v\n", i, err)
			return 1
		}
		chunkResp.Body.Close()

		if chunkResp.StatusCode != 200 {
			fmt.Fprintf(os.Stderr, "Error: upload chunk %d: HTTP %d\n", i, chunkResp.StatusCode)
			return 1
		}

		// 进度
		progress := float64(i+1) / float64(initResp.TotalChunks) * 100
		elapsed := time.Since(startTime).Seconds()
		speed := float64((int64(i+1) * initResp.ChunkSize)) / elapsed
		fmt.Printf("\r  Uploading: %d/%d (%.1f%%) - %s/s", i+1, initResp.TotalChunks, progress, formatSize(int64(speed)))
	}
	fmt.Println()

	// Step 3: 完成上传
	completeResp, err := httpClient.Post(baseURL+"/transfer/upload/"+initResp.ID+"/complete", "application/json", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: complete upload: %v\n", err)
		return 1
	}
	defer completeResp.Body.Close()

	var completeResult struct {
		ID       string `json:"id"`
		FileName string `json:"filename"`
		FileSize int64  `json:"file_size"`
		SavePath string `json:"save_path"`
		Status   string `json:"status"`
		Error    string `json:"error"`
	}
	json.NewDecoder(completeResp.Body).Decode(&completeResult)
	if completeResult.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", completeResult.Error)
		return 1
	}

	elapsed := time.Since(startTime)
	fmt.Printf("Completed in %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Saved to: %s\n", completeResult.SavePath)

	// 输出 JSON 摘要（AI 友好）
	summary, _ := json.Marshal(map[string]interface{}{
		"id":        completeResult.ID,
		"filename":  completeResult.FileName,
		"file_size": completeResult.FileSize,
		"save_path": completeResult.SavePath,
		"status":    completeResult.Status,
		"duration":  elapsed.String(),
	})
	fmt.Printf("JSON: %s\n", string(summary))

	return 0
}

func runTransferPull(c *client.Client, args []string) int {
	var remoteURL, transferID, savePath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--remote":
			if i+1 < len(args) {
				i++
				remoteURL = args[i]
			}
		case "--id":
			if i+1 < len(args) {
				i++
				transferID = args[i]
			}
		case "--save":
			if i+1 < len(args) {
				i++
				savePath = args[i]
			}
		}
	}

	if transferID == "" {
		fmt.Fprintf(os.Stderr, "Error: --id is required\n")
		return 1
	}

	baseURL := resolveBaseURL(c, remoteURL)
	httpClient := &http.Client{Timeout: 0}

	// Step 1: 获取文件信息
	infoResp, err := httpClient.Get(baseURL + "/transfer/status/" + transferID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: get transfer info: %v\n", err)
		return 1
	}
	defer infoResp.Body.Close()

	var info struct {
		ID       string `json:"id"`
		FileName string `json:"filename"`
		FileSize int64  `json:"file_size"`
		Status   string `json:"status"`
		Error    string `json:"error"`
	}
	json.NewDecoder(infoResp.Body).Decode(&info)
	if info.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", info.Error)
		return 1
	}
	if info.Status != "completed" {
		fmt.Fprintf(os.Stderr, "Error: file not ready, status: %s\n", info.Status)
		return 1
	}

	// 确定保存路径
	if savePath == "" {
		savePath = "./" + info.FileName
	}

	fmt.Printf("Downloading: %s (%s)\n", info.FileName, formatSize(info.FileSize))

	// Step 2: 下载文件（支持断点续传）
	var startOffset int64
	outFlags := os.O_CREATE | os.O_WRONLY

	// 检查是否有部分下载的文件
	if existInfo, err := os.Stat(savePath); err == nil {
		startOffset = existInfo.Size()
		if startOffset > 0 && startOffset < info.FileSize {
			fmt.Printf("Resuming from %s\n", formatSize(startOffset))
			outFlags |= os.O_APPEND
		} else if startOffset >= info.FileSize {
			fmt.Printf("File already downloaded: %s\n", savePath)
			return 0
		} else {
			outFlags |= os.O_TRUNC
		}
	} else {
		outFlags |= os.O_TRUNC
	}

	req, _ := http.NewRequest("GET", baseURL+"/transfer/download/"+transferID, nil)
	if startOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
	}

	startTime := time.Now()
	dlResp, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: download: %v\n", err)
		return 1
	}
	defer dlResp.Body.Close()

	outFile, err := os.OpenFile(savePath, outFlags, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: create file: %v\n", err)
		return 1
	}
	defer outFile.Close()

	// 带进度的复制
	buf := make([]byte, 32*1024)
	var downloaded int64

	for {
		n, readErr := dlResp.Body.Read(buf)
		if n > 0 {
			outFile.Write(buf[:n])
			downloaded += int64(n)
			progress := float64(startOffset+downloaded) / float64(info.FileSize) * 100
			elapsed := time.Since(startTime).Seconds()
			speed := float64(downloaded) / elapsed
			fmt.Printf("\r  Downloading: %s/%s (%.1f%%) - %s/s",
				formatSize(startOffset+downloaded), formatSize(info.FileSize), progress, formatSize(int64(speed)))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "\nError: download: %v\n", readErr)
			return 1
		}
	}
	fmt.Println()

	elapsed := time.Since(startTime)
	fmt.Printf("Saved to: %s (%s)\n", savePath, elapsed.Round(time.Millisecond))

	// JSON 摘要
	summary, _ := json.Marshal(map[string]interface{}{
		"id":        info.ID,
		"filename":  info.FileName,
		"file_size": info.FileSize,
		"save_path": savePath,
		"duration":  elapsed.String(),
	})
	fmt.Printf("JSON: %s\n", string(summary))

	return 0
}

func runTransferList(c *client.Client, args []string) int {
	var remoteURL string
	for i := 0; i < len(args); i++ {
		if args[i] == "--remote" && i+1 < len(args) {
			i++
			remoteURL = args[i]
		}
	}

	baseURL := resolveBaseURL(c, remoteURL)
	httpClient := &http.Client{Timeout: 30 * time.Second}

	resp, err := httpClient.Get(baseURL + "/transfer/list")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	var records []struct {
		ID             string `json:"id"`
		FileName       string `json:"filename"`
		FileSize       int64  `json:"file_size"`
		UploadedChunks int    `json:"uploaded_chunks"`
		TotalChunks    int    `json:"total_chunks"`
		Status         string `json:"status"`
		SavePath       string `json:"save_path"`
		CreatedAt      string `json:"created_at"`
	}
	json.NewDecoder(resp.Body).Decode(&records)

	if len(records) == 0 {
		fmt.Println("No transfer records")
		return 0
	}

	fmt.Printf("%-36s  %-20s  %10s  %-10s  %s\n", "ID", "File", "Size", "Status", "Progress")
	fmt.Println(strings.Repeat("-", 100))
	for _, r := range records {
		progress := fmt.Sprintf("%d/%d", r.UploadedChunks, r.TotalChunks)
		if r.Status == "completed" {
			progress = "done"
		}
		name := r.FileName
		if len(name) > 20 {
			name = name[:17] + "..."
		}
		fmt.Printf("%-36s  %-20s  %10s  %-10s  %s\n", r.ID, name, formatSize(r.FileSize), r.Status, progress)
	}

	return 0
}

func runTransferStatus(c *client.Client, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub transfer status <transfer_id> [--remote <url>]\n")
		return 1
	}

	transferID := args[0]
	var remoteURL string
	for i := 1; i < len(args); i++ {
		if args[i] == "--remote" && i+1 < len(args) {
			i++
			remoteURL = args[i]
		}
	}

	baseURL := resolveBaseURL(c, remoteURL)
	httpClient := &http.Client{Timeout: 30 * time.Second}

	resp, err := httpClient.Get(baseURL + "/transfer/status/" + transferID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Error: HTTP %d: %s\n", resp.StatusCode, string(body))
		return 1
	}

	// Pretty print
	var out bytes.Buffer
	json.Indent(&out, body, "", "  ")
	fmt.Println(out.String())

	return 0
}

func runTransferDelete(c *client.Client, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub transfer delete <transfer_id> [--remote <url>]\n")
		return 1
	}

	transferID := args[0]
	var remoteURL string
	for i := 1; i < len(args); i++ {
		if args[i] == "--remote" && i+1 < len(args) {
			i++
			remoteURL = args[i]
		}
	}

	baseURL := resolveBaseURL(c, remoteURL)
	httpClient := &http.Client{Timeout: 30 * time.Second}

	req, _ := http.NewRequest("DELETE", baseURL+"/transfer/delete/"+transferID, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Error: HTTP %d: %s\n", resp.StatusCode, string(body))
		return 1
	}

	fmt.Printf("Deleted: %s\n", transferID)
	return 0
}

// formatSize 格式化文件大小
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return strconv.FormatFloat(float64(bytes)/float64(GB), 'f', 2, 64) + " GB"
	case bytes >= MB:
		return strconv.FormatFloat(float64(bytes)/float64(MB), 'f', 2, 64) + " MB"
	case bytes >= KB:
		return strconv.FormatFloat(float64(bytes)/float64(KB), 'f', 1, 64) + " KB"
	default:
		return strconv.FormatInt(bytes, 10) + " B"
	}
}
