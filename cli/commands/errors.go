package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// RunErrors executes the errors command
// Usage: ai-hub errors [session_id] [--all] [--level error|warning] [--context <msg_id>] [--lines N]
func RunErrors(c *client.Client, args []string) int {
	var sessionID int64
	var level string
	var showAll bool
	var contextMsgID int64
	var lines int = 2 // default context lines

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all":
			showAll = true
		case "--level":
			if i+1 < len(args) {
				i++
				level = args[i]
			}
		case "--context":
			if i+1 < len(args) {
				i++
				id, err := strconv.ParseInt(args[i], 10, 64)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: invalid message ID for --context: %s\n", args[i])
					return 1
				}
				contextMsgID = id
			}
		case "--lines":
			if i+1 < len(args) {
				i++
				n, err := strconv.Atoi(args[i])
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: invalid number for --lines: %s\n", args[i])
					return 1
				}
				lines = n
			}
		default:
			if !strings.HasPrefix(args[i], "-") && sessionID == 0 {
				id, err := strconv.ParseInt(args[i], 10, 64)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: invalid session ID: %s\n", args[i])
					return 1
				}
				sessionID = id
			}
		}
	}

	// --context requires session_id
	if contextMsgID > 0 {
		if sessionID == 0 {
			fmt.Fprintf(os.Stderr, "Error: --context requires session_id\n")
			fmt.Fprintf(os.Stderr, "Usage: ai-hub errors <session_id> --context <message_id> [--lines N]\n")
			return 1
		}
		return showErrorContext(c, sessionID, contextMsgID, lines)
	}

	// If --all or no session_id, show stats overview
	if showAll || sessionID == 0 {
		return showErrorStats(c, sessionID)
	}
	return showSessionErrors(c, sessionID, level)
}

func showErrorContext(c *client.Client, sessionID, msgID int64, lines int) int {
	// First verify the message has an error
	errData, err := c.GET(fmt.Sprintf("/sessions/%d/errors", sessionID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var errResp struct {
		Errors []struct {
			ID        int64  `json:"id"`
			MessageID int64  `json:"message_id"`
			Level     string `json:"level"`
			Summary   string `json:"summary"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(errData, &errResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	// Find the error for this message
	var foundError *struct {
		ID        int64  `json:"id"`
		MessageID int64  `json:"message_id"`
		Level     string `json:"level"`
		Summary   string `json:"summary"`
	}
	for i := range errResp.Errors {
		if errResp.Errors[i].MessageID == msgID {
			foundError = &errResp.Errors[i]
			break
		}
	}

	if foundError != nil {
		icon := "❌"
		if foundError.Level == "warning" {
			icon = "⚠️"
		}
		fmt.Printf("%s Error in message #%d: %s\n\n", icon, msgID, foundError.Summary)
	}

	// Show message with context
	fmt.Printf("=== Context (±%d messages) ===\n\n", lines)
	return ShowMessageWithContextPublic(c, sessionID, msgID, lines)
}

func showErrorStats(c *client.Client, sessionID int64) int {
	path := "/stats/errors"
	if sessionID > 0 {
		path += fmt.Sprintf("?session_id=%d", sessionID)
	}
	data, err := c.Request("GET", path, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	var resp struct {
		Stats []struct {
			SessionID    int64  `json:"session_id"`
			SessionTitle string `json:"session_title"`
			ErrorCount   int    `json:"error_count"`
			WarningCount int    `json:"warning_count"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}
	if len(resp.Stats) == 0 {
		fmt.Println("No errors recorded.")
		return 0
	}
	fmt.Printf("%-8s %-30s %6s %8s\n", "Session", "Title", "Errors", "Warnings")
	fmt.Println(strings.Repeat("-", 56))
	for _, s := range resp.Stats {
		title := s.SessionTitle
		if len(title) > 28 {
			title = title[:28] + ".."
		}
		fmt.Printf("%-8d %-30s %6d %8d\n", s.SessionID, title, s.ErrorCount, s.WarningCount)
	}
	return 0
}

func showSessionErrors(c *client.Client, sessionID int64, level string) int {
	path := fmt.Sprintf("/sessions/%d/errors", sessionID)
	if level != "" {
		path += "?level=" + level
	}
	data, err := c.Request("GET", path, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	var resp struct {
		Errors []struct {
			ID        int64  `json:"id"`
			MessageID int64  `json:"message_id"`
			Level     string `json:"level"`
			Summary   string `json:"summary"`
			CreatedAt string `json:"created_at"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}
	if len(resp.Errors) == 0 {
		fmt.Printf("No errors for session %d.\n", sessionID)
		return 0
	}

	fmt.Printf("Session #%d errors:\n\n", sessionID)
	for _, e := range resp.Errors {
		icon := "❌"
		if e.Level == "warning" {
			icon = "⚠️"
		}
		fmt.Printf("%s [msg#%d] %s\n", icon, e.MessageID, e.Summary)
		fmt.Printf("   %s\n", FormatTime(e.CreatedAt))
		fmt.Printf("   查看上下文: ai-hub errors %d --context %d\n\n", sessionID, e.MessageID)
	}
	return 0
}
