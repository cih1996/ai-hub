package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// RunMount handles the mount command
func RunMount(c *client.Client, args []string) int {
	if len(args) == 0 {
		printMountHelp()
		return 0
	}

	subCmd := args[0]
	subArgs := args[1:]

	switch subCmd {
	case "list":
		return runMountList(c)
	case "remove":
		return runMountRemove(c, subArgs)
	case "--help", "-h":
		printMountHelp()
		return 0
	default:
		// 默认是挂载操作：mount <path> --alias <alias>
		return runMountCreate(c, args)
	}
}

func runMountList(c *client.Client) int {
	resp, err := c.GET("/mounts")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var mounts []struct {
		ID        int64  `json:"id"`
		Alias     string `json:"alias"`
		LocalPath string `json:"local_path"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(resp, &mounts); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(mounts) == 0 {
		fmt.Println("No mounts configured")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ALIAS\tLOCAL PATH\tACCESS URL")
	for _, m := range mounts {
		// 从 BaseURL 提取端口
		port := extractPort(c.BaseURL)
		accessURL := fmt.Sprintf("http://localhost:%s/static/%s/", port, m.Alias)
		fmt.Fprintf(w, "%s\t%s\t%s\n", m.Alias, m.LocalPath, accessURL)
	}
	w.Flush()
	return 0
}

func runMountCreate(c *client.Client, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: local path is required\n")
		printMountHelp()
		return 1
	}

	localPath := args[0]
	alias := ""

	// 解析 --alias 参数
	for i := 1; i < len(args); i++ {
		if args[i] == "--alias" && i+1 < len(args) {
			alias = args[i+1]
			i++
		}
	}

	if alias == "" {
		fmt.Fprintf(os.Stderr, "Error: --alias is required\n")
		printMountHelp()
		return 1
	}

	body := map[string]string{
		"alias":      alias,
		"local_path": localPath,
	}

	resp, err := c.POST("/mounts", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var result struct {
		ID        int64  `json:"id"`
		Alias     string `json:"alias"`
		LocalPath string `json:"local_path"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", result.Error)
		return 1
	}

	port := extractPort(c.BaseURL)
	fmt.Printf("Mount created: %s -> %s\n", result.Alias, result.LocalPath)
	fmt.Printf("Access URL: http://localhost:%s/static/%s/\n", port, result.Alias)
	return 0
}

func runMountRemove(c *client.Client, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: alias is required\n")
		return 1
	}

	alias := args[0]

	resp, err := c.DELETE("/mounts/" + alias)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var result struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	json.Unmarshal(resp, &result)

	if result.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", result.Error)
		return 1
	}

	fmt.Printf("Mount removed: %s\n", alias)
	return 0
}

// extractPort 从 BaseURL 中提取端口号
func extractPort(baseURL string) string {
	// BaseURL 格式: http://localhost:9527/api/v1
	parts := strings.Split(baseURL, ":")
	if len(parts) >= 3 {
		portPart := parts[2] // "9527/api/v1"
		if idx := strings.Index(portPart, "/"); idx > 0 {
			return portPart[:idx]
		}
		return portPart
	}
	return "9527"
}

func printMountHelp() {
	help := `Mount - Static file serving

Usage:
  ai-hub mount <local_path> --alias <alias>   Mount a local directory
  ai-hub mount list                           List all mounts
  ai-hub mount remove <alias>                 Remove a mount

Examples:
  ai-hub mount ~/Pictures --alias media
  ai-hub mount /tmp/screenshots --alias shots
  ai-hub mount list
  ai-hub mount remove media

After mounting, files are accessible at:
  http://localhost:<port>/static/<alias>/<filename>

Use in AI responses:
  <img src="http://localhost:9527/static/media/test.png">`

	fmt.Println(strings.TrimSpace(help))
}
