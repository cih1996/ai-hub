package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// RunSend executes the send command
func RunSend(c *client.Client, args []string) int {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub send <session_id> "消息内容" [flags]

Send a message to a session. Use session_id=0 to create a new session.

Flags:
  --group <name>       Group name for new session
  --work-dir <path>    Working directory for new session
  --remote <url>       Send to remote AI Hub instance (default port: 9527)

Examples:
  ai-hub send 25 "你好"
  ai-hub send 0 "初始化" --group "团队A" --work-dir "/path/to/project"
  ai-hub send 23 "跨系统协作" --remote http://192.168.1.100
  ai-hub send 23 "跨系统协作" --remote http://192.168.1.100:8080
`)
		return 1
	}

	sessionID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid session ID: %s\n", args[0])
		return 1
	}

	content := args[1]

	// Parse optional flags
	var groupName, workDir, remoteURL string
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--group":
			if i+1 < len(args) {
				i++
				groupName = args[i]
			}
		case "--work-dir":
			if i+1 < len(args) {
				i++
				workDir = args[i]
			}
		case "--remote":
			if i+1 < len(args) {
				i++
				remoteURL = args[i]
			}
		}
	}

	// Use remote client if --remote is specified
	targetClient := c
	if remoteURL != "" {
		targetClient = client.NewClientWithURL(remoteURL)
		fmt.Printf("Sending to remote: %s\n", targetClient.BaseURL)
	}

	body := map[string]interface{}{
		"session_id": sessionID,
		"content":    content,
	}
	if groupName != "" {
		body["group_name"] = groupName
	}
	if workDir != "" {
		body["work_dir"] = workDir
	}

	respData, err := targetClient.POST("/chat/send", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		SessionID int64  `json:"session_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if remoteURL != "" {
		fmt.Printf("Remote Session #%d: %s\n", resp.SessionID, resp.Status)
	} else {
		fmt.Printf("Session #%d: %s\n", resp.SessionID, resp.Status)
	}
	return 0
}
