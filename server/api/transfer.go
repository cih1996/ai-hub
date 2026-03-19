package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ChunkSize 分块大小 2MB
const ChunkSize = 2 * 1024 * 1024

// TransferRecord 传输记录
type TransferRecord struct {
	ID             string    `json:"id"`
	FileName       string    `json:"filename"`
	FileSize       int64     `json:"file_size"`
	ChunkSize      int64     `json:"chunk_size"`
	TotalChunks    int       `json:"total_chunks"`
	UploadedChunks int       `json:"uploaded_chunks"`
	Status         string    `json:"status"` // pending/uploading/completed/failed
	SavePath       string    `json:"save_path"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

var (
	transferStore = make(map[string]*TransferRecord)
	transferMu    sync.RWMutex
)

// getTransfersDir 获取传输目录
func getTransfersDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ai-hub", "transfers")
}

// TransferInit POST /api/v1/transfer/upload — 初始化上传
func TransferInit(c *gin.Context) {
	var req struct {
		FileName string `json:"filename" binding:"required"`
		FileSize int64  `json:"file_size" binding:"required"`
		SavePath string `json:"save_path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id := uuid.New().String()
	totalChunks := int(req.FileSize / ChunkSize)
	if req.FileSize%ChunkSize != 0 {
		totalChunks++
	}
	if totalChunks == 0 {
		totalChunks = 1
	}

	savePath := req.SavePath
	if savePath == "" {
		savePath = filepath.Join(getTransfersDir(), req.FileName)
	}

	// 创建分块临时目录
	chunkDir := filepath.Join(getTransfersDir(), "chunks", id)
	if err := os.MkdirAll(chunkDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建临时目录失败: " + err.Error()})
		return
	}

	record := &TransferRecord{
		ID:             id,
		FileName:       req.FileName,
		FileSize:       req.FileSize,
		ChunkSize:      ChunkSize,
		TotalChunks:    totalChunks,
		UploadedChunks: 0,
		Status:         "pending",
		SavePath:       savePath,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	transferMu.Lock()
	transferStore[id] = record
	transferMu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"id":           id,
		"chunk_size":   ChunkSize,
		"total_chunks": totalChunks,
	})
}

// TransferChunk PUT /api/v1/transfer/upload/:id/chunk — 上传分块
func TransferChunk(c *gin.Context) {
	id := c.Param("id")
	indexStr := c.Query("index")
	if indexStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 index 参数"})
		return
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "index 参数无效"})
		return
	}

	transferMu.RLock()
	record, ok := transferStore[id]
	transferMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "传输记录不存在"})
		return
	}

	if index < 0 || index >= record.TotalChunks {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("index 超出范围 [0, %d)", record.TotalChunks)})
		return
	}

	// 读取 multipart chunk
	file, _, err := c.Request.FormFile("chunk")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取分块数据失败: " + err.Error()})
		return
	}
	defer file.Close()

	// 写入分块文件
	chunkPath := filepath.Join(getTransfersDir(), "chunks", id, fmt.Sprintf("%06d", index))
	out, err := os.Create(chunkPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入分块失败: " + err.Error()})
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入分块失败: " + err.Error()})
		return
	}

	// 更新记录
	transferMu.Lock()
	record.UploadedChunks++
	record.Status = "uploading"
	record.UpdatedAt = time.Now()
	transferMu.Unlock()

	progress := float64(record.UploadedChunks) / float64(record.TotalChunks) * 100

	c.JSON(http.StatusOK, gin.H{
		"uploaded_chunks": record.UploadedChunks,
		"total_chunks":    record.TotalChunks,
		"progress":        fmt.Sprintf("%.1f%%", progress),
	})
}

// TransferComplete POST /api/v1/transfer/upload/:id/complete — 完成上传，合并分块
func TransferComplete(c *gin.Context) {
	id := c.Param("id")

	transferMu.RLock()
	record, ok := transferStore[id]
	transferMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "传输记录不存在"})
		return
	}

	if record.UploadedChunks < record.TotalChunks {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("分块未上传完成: %d/%d", record.UploadedChunks, record.TotalChunks),
		})
		return
	}

	// 确保保存目录存在
	saveDir := filepath.Dir(record.SavePath)
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		transferMu.Lock()
		record.Status = "failed"
		record.UpdatedAt = time.Now()
		transferMu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建保存目录失败: " + err.Error()})
		return
	}

	// 合并分块
	chunkDir := filepath.Join(getTransfersDir(), "chunks", id)
	entries, err := os.ReadDir(chunkDir)
	if err != nil {
		transferMu.Lock()
		record.Status = "failed"
		record.UpdatedAt = time.Now()
		transferMu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取分块目录失败: " + err.Error()})
		return
	}

	// 按文件名排序（已用 %06d 格式化）
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	outFile, err := os.Create(record.SavePath)
	if err != nil {
		transferMu.Lock()
		record.Status = "failed"
		record.UpdatedAt = time.Now()
		transferMu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建目标文件失败: " + err.Error()})
		return
	}
	defer outFile.Close()

	for _, entry := range entries {
		chunkPath := filepath.Join(chunkDir, entry.Name())
		chunkFile, err := os.Open(chunkPath)
		if err != nil {
			transferMu.Lock()
			record.Status = "failed"
			record.UpdatedAt = time.Now()
			transferMu.Unlock()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取分块失败: " + err.Error()})
			return
		}
		io.Copy(outFile, chunkFile)
		chunkFile.Close()
	}

	// 清理分块目录
	os.RemoveAll(chunkDir)

	transferMu.Lock()
	record.Status = "completed"
	record.UpdatedAt = time.Now()
	transferMu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"id":        record.ID,
		"filename":  record.FileName,
		"file_size": record.FileSize,
		"save_path": record.SavePath,
		"status":    record.Status,
	})
}

// TransferStatus GET /api/v1/transfer/:id — 查询传输状态
func TransferStatus(c *gin.Context) {
	id := c.Param("id")

	transferMu.RLock()
	record, ok := transferStore[id]
	transferMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "传输记录不存在"})
		return
	}

	c.JSON(http.StatusOK, record)
}

// TransferList GET /api/v1/transfer/list — 列出所有传输记录
func TransferList(c *gin.Context) {
	transferMu.RLock()
	defer transferMu.RUnlock()

	list := make([]*TransferRecord, 0, len(transferStore))
	for _, r := range transferStore {
		list = append(list, r)
	}

	// 按创建时间倒序
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	c.JSON(http.StatusOK, list)
}

// TransferDownload GET /api/v1/transfer/download/:id — 下载文件（支持 Range 断点续传）
func TransferDownload(c *gin.Context) {
	id := c.Param("id")

	transferMu.RLock()
	record, ok := transferStore[id]
	transferMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "传输记录不存在"})
		return
	}

	if record.Status != "completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件尚未上传完成"})
		return
	}

	// 检查文件是否存在
	if _, err := os.Stat(record.SavePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, record.FileName))
	c.File(record.SavePath)
}

// TransferDelete DELETE /api/v1/transfer/:id — 删除传输记录和文件
func TransferDelete(c *gin.Context) {
	id := c.Param("id")

	transferMu.Lock()
	record, ok := transferStore[id]
	if !ok {
		transferMu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "传输记录不存在"})
		return
	}
	delete(transferStore, id)
	transferMu.Unlock()

	// 清理文件
	if record.Status == "completed" {
		os.Remove(record.SavePath)
	}
	// 清理可能残留的分块目录
	chunkDir := filepath.Join(getTransfersDir(), "chunks", id)
	os.RemoveAll(chunkDir)

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
