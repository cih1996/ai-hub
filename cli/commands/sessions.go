package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// RunSessions executes the sessions command
func RunSessions(c *client.Client, args []string) int {
	if len(args) == 0 {
		return listSessions(c)
	}

	// Parse session ID
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid session ID: %s\n", args[0])
		return 1
	}

	// Check for "messages" subcommand
	if len(args) > 1 && args[1] == "messages" {
		return sessionMessages(c, id, args[2:])
	}

	return sessionDetail(c, id)
}

func listSessions(c *client.Client) int {
	respData, err := c.GET("/sessions")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var sessions []struct {
		ID           int64  `json:"id"`
		Title        string `json:"title"`
		GroupName    string `json:"group_name"`
		WorkDir      string `json:"work_dir"`
		Streaming    bool   `json:"streaming"`
		ProcessAlive bool   `json:"process_alive"`
		ProcessState string `json:"process_state"`
		UpdatedAt    string `json:"updated_at"`
	}
	if err := json.Unmarshal(respData, &sessions); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return 0
	}

	fmt.Printf("%d sessions:\n\n", len(sessions))
	for _, s := range sessions {
		status := "idle"
		if s.Streaming {
			status = "streaming"
		} else if s.ProcessAlive {
			status = "alive"
		}
		group := s.GroupName
		if group == "" {
			group = "-"
		}
		fmt.Printf("#%-4d [%s] %s\n", s.ID, status, s.Title)
		fmt.Printf("      团队: %s  更新: %s\n", group, FormatTime(s.UpdatedAt))
		fmt.Println("---")
	}
	return 0
}

func sessionDetail(c *client.Client, id int64) int {
	respData, err := c.GET(fmt.Sprintf("/sessions/%d", id))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var s struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		GroupName string `json:"group_name"`
		WorkDir   string `json:"work_dir"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(respData, &s); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Printf("Session #%d\n", s.ID)
	fmt.Printf("  标题: %s\n", s.Title)
	fmt.Printf("  团队: %s\n", s.GroupName)
	fmt.Printf("  目录: %s\n", s.WorkDir)
	fmt.Printf("  创建: %s\n", FormatTime(s.CreatedAt))
	fmt.Printf("  更新: %s\n", FormatTime(s.UpdatedAt))
	return 0
}

func sessionMessages(c *client.Client, id int64, args []string) int {
	limit := 20
	for i := 0; i < len(args); i++ {
		if args[i] == "--limit" && i+1 < len(args) {
			i++
			fmt.Sscanf(args[i], "%d", &limit)
		}
	}

	path := fmt.Sprintf("/sessions/%d/messages?limit=%d", id, limit)
	respData, err := c.GET(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Messages []struct {
			ID        int64  `json:"id"`
			Role      string `json:"role"`
			Content   string `json:"content"`
			CreatedAt string `json:"created_at"`
		} `json:"messages"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(resp.Messages) == 0 {
		fmt.Printf("No messages in session #%d\n", id)
		return 0
	}

	for _, m := range resp.Messages {
		role := "👤"
		if m.Role == "assistant" {
			role = "🤖"
		}
		fmt.Printf("%s [#%d] %s\n", role, m.ID, FormatTime(m.CreatedAt))
		fmt.Printf("   %s\n\n", TruncatePreview(m.Content, 200))
	}
	if resp.HasMore {
		fmt.Println("(还有更多消息，使用 --limit 调整)")
	}
	return 0
}
