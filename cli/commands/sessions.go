package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// RunSessions executes the sessions command
// Usage: ai-hub sessions [id] [messages] [--with-errors]
func RunSessions(c *client.Client, args []string) int {
	// Check for --with-errors flag
	var withErrors bool
	var filteredArgs []string
	for _, arg := range args {
		if arg == "--with-errors" {
			withErrors = true
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}
	args = filteredArgs

	if len(args) == 0 {
		return listSessions(c, withErrors)
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

func listSessions(c *client.Client, withErrors bool) int {
	// Get sessions
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

	// Get error stats for all sessions
	errorStats := make(map[int64]struct {
		Errors   int
		Warnings int
	})

	statsData, err := c.GET("/stats/errors")
	if err == nil {
		var statsResp struct {
			Stats []struct {
				SessionID    int64 `json:"session_id"`
				ErrorCount   int   `json:"error_count"`
				WarningCount int   `json:"warning_count"`
			} `json:"stats"`
		}
		if json.Unmarshal(statsData, &statsResp) == nil {
			for _, s := range statsResp.Stats {
				errorStats[s.SessionID] = struct {
					Errors   int
					Warnings int
				}{s.ErrorCount, s.WarningCount}
			}
		}
	}

	// Filter sessions if --with-errors
	if withErrors {
		var filtered []struct {
			ID           int64  `json:"id"`
			Title        string `json:"title"`
			GroupName    string `json:"group_name"`
			WorkDir      string `json:"work_dir"`
			Streaming    bool   `json:"streaming"`
			ProcessAlive bool   `json:"process_alive"`
			ProcessState string `json:"process_state"`
			UpdatedAt    string `json:"updated_at"`
		}
		for _, s := range sessions {
			stats := errorStats[s.ID]
			if stats.Errors > 0 || stats.Warnings > 0 {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
		if len(sessions) == 0 {
			fmt.Println("No sessions with errors found.")
			return 0
		}
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

		// Get error stats for this session
		stats := errorStats[s.ID]
		errStr := ""
		if stats.Errors > 0 || stats.Warnings > 0 {
			errStr = fmt.Sprintf("  E:%d W:%d", stats.Errors, stats.Warnings)
		}

		fmt.Printf("#%-4d [%s] %s%s\n", s.ID, status, s.Title, errStr)
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
	var page, pageSize, nth, fromID int
	var search string
	var countOnly bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &limit)
			}
		case "--page":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &page)
			}
		case "--size":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &pageSize)
			}
		case "--nth":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &nth)
			}
		case "--from":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &fromID)
			}
		case "--count":
			countOnly = true
		case "--search":
			if i+1 < len(args) {
				i++
				search = args[i]
			}
		}
	}

	// --count: just show total
	if countOnly {
		respData, err := c.GET(fmt.Sprintf("/sessions/%d/messages?limit=1", id))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		var resp struct {
			Total int64 `json:"total"`
		}
		json.Unmarshal(respData, &resp)
		fmt.Printf("Session #%d: %d messages\n", id, resp.Total)
		return 0
	}

	// --nth N: show message #N with context
	if nth > 0 {
		// First get the message at position nth using offset
		respData, err := c.GET(fmt.Sprintf("/sessions/%d/messages?offset=%d&limit=1", id, nth-1))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
		var resp struct {
			Messages []struct {
				ID int64 `json:"id"`
			} `json:"messages"`
		}
		json.Unmarshal(respData, &resp)
		if len(resp.Messages) == 0 {
			fmt.Fprintf(os.Stderr, "Message #%d not found in session #%d\n", nth, id)
			return 1
		}
		msgID := resp.Messages[0].ID
		return showMessageWithContext(c, id, msgID, 2)
	}

	// --from <msg_id>: show from specific message ID with context
	if fromID > 0 {
		return showMessageWithContext(c, id, int64(fromID), 2)
	}

	// Build query path
	var path string
	if search != "" {
		path = fmt.Sprintf("/sessions/%d/messages?search=%s&limit=%d", id, search, limit)
	} else if page > 0 {
		if pageSize <= 0 {
			pageSize = 20
		}
		path = fmt.Sprintf("/sessions/%d/messages?page=%d&page_size=%d", id, page, pageSize)
	} else {
		path = fmt.Sprintf("/sessions/%d/messages?limit=%d", id, limit)
	}

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
		HasMore  bool  `json:"has_more"`
		Total    int64 `json:"total"`
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(resp.Messages) == 0 {
		fmt.Printf("No messages in session #%d\n", id)
		return 0
	}

	if resp.Total > 0 {
		fmt.Printf("Session #%d — %d messages total\n\n", id, resp.Total)
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
		if page > 0 {
			fmt.Printf("(第 %d 页，使用 --page %d 查看下一页)\n", page, page+1)
		} else {
			fmt.Println("(还有更多消息，使用 --page 或 --limit 调整)")
		}
	}
	return 0
}

func showMessageWithContext(c *client.Client, sessionID, msgID int64, lines int) int {
	respData, err := c.GET(fmt.Sprintf("/sessions/%d/messages/%d?context=%d", sessionID, msgID, lines))
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
		TargetID int64 `json:"target_id"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}
	if len(resp.Messages) == 0 {
		fmt.Printf("Message #%d not found\n", msgID)
		return 1
	}
	for _, m := range resp.Messages {
		role := "👤"
		if m.Role == "assistant" {
			role = "🤖"
		}
		marker := "  "
		if m.ID == resp.TargetID {
			marker = "▶ "
		}
		fmt.Printf("%s%s [#%d] %s\n", marker, role, m.ID, FormatTime(m.CreatedAt))
		preview := 200
		if m.ID == resp.TargetID {
			preview = 500
		}
		fmt.Printf("   %s\n\n", TruncatePreview(m.Content, preview))
	}
	return 0
}

// ShowMessageWithContextPublic is exported for use by errors command
func ShowMessageWithContextPublic(c *client.Client, sessionID, msgID int64, lines int) int {
	return showMessageWithContext(c, sessionID, msgID, lines)
}
