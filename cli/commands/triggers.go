package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// RunTriggers executes the triggers command
func RunTriggers(c *client.Client, args []string) int {
	if len(args) == 0 {
		printTriggersHelp()
		return 1
	}

	switch args[0] {
	case "list":
		return triggersList(c, args[1:])
	case "create":
		return triggersCreate(c, args[1:])
	case "update":
		return triggersUpdate(c, args[1:])
	case "delete":
		return triggersDelete(c, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown triggers subcommand: %s\n", args[0])
		printTriggersHelp()
		return 1
	}
}

func triggersList(c *client.Client, args []string) int {
	path := "/triggers"
	// Optional --session filter
	for i := 0; i < len(args); i++ {
		if args[i] == "--session" && i+1 < len(args) {
			i++
			path = fmt.Sprintf("/triggers?session_id=%s", args[i])
		}
	}

	respData, err := c.GET(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var triggers []struct {
		ID          int64  `json:"id"`
		SessionID   int64  `json:"session_id"`
		Content     string `json:"content"`
		TriggerTime string `json:"trigger_time"`
		MaxFires    int    `json:"max_fires"`
		Enabled     bool   `json:"enabled"`
		FiredCount  int    `json:"fired_count"`
		Status      string `json:"status"`
		NextFireAt  string `json:"next_fire_at"`
	}
	if err := json.Unmarshal(respData, &triggers); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(triggers) == 0 {
		fmt.Println("No triggers found.")
		return 0
	}

	fmt.Printf("%d triggers:\n\n", len(triggers))
	for _, t := range triggers {
		enabled := "on"
		if !t.Enabled {
			enabled = "off"
		}
		fmt.Printf("#%-4d [%s] session=%d  %s\n", t.ID, t.Status, t.SessionID, enabled)
		fmt.Printf("      时间: %s  触发: %d/%d\n", t.TriggerTime, t.FiredCount, t.MaxFires)
		fmt.Printf("      指令: %s\n", TruncatePreview(t.Content, 100))
		if t.NextFireAt != "" {
			fmt.Printf("      下次: %s\n", FormatTime(t.NextFireAt))
		}
		fmt.Println("---")
	}
	return 0
}

func triggersCreate(c *client.Client, args []string) int {
	var sessionID, content, triggerTime string
	maxFires := -1

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 < len(args) {
				i++
				sessionID = args[i]
			}
		case "--content":
			if i+1 < len(args) {
				i++
				content = args[i]
			}
		case "--time":
			if i+1 < len(args) {
				i++
				triggerTime = args[i]
			}
		case "--max-fires":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &maxFires)
			}
		}
	}

	// Fallback to env
	if sessionID == "" {
		sessionID = os.Getenv("AI_HUB_SESSION_ID")
	}

	if sessionID == "" || content == "" || triggerTime == "" {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub triggers create --session <id> --time \"09:00:00\" --content \"指令\" [--max-fires -1]\n")
		return 1
	}

	sid, err := strconv.ParseInt(sessionID, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid session ID: %s\n", sessionID)
		return 1
	}

	body := map[string]interface{}{
		"session_id":   sid,
		"content":      content,
		"trigger_time": triggerTime,
		"max_fires":    maxFires,
	}

	respData, err := c.POST("/triggers", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Printf("Trigger #%d created.\n", resp.ID)
	return 0
}

func triggersUpdate(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub triggers update <id> [--content \"新指令\"] [--time \"10:00:00\"] [--max-fires 5] [--enabled true/false]\n")
		return 1
	}

	triggerID := args[0]
	body := map[string]interface{}{}

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--content":
			if i+1 < len(args) {
				i++
				body["content"] = args[i]
			}
		case "--time":
			if i+1 < len(args) {
				i++
				body["trigger_time"] = args[i]
			}
		case "--max-fires":
			if i+1 < len(args) {
				i++
				var v int
				fmt.Sscanf(args[i], "%d", &v)
				body["max_fires"] = v
			}
		case "--enabled":
			if i+1 < len(args) {
				i++
				body["enabled"] = args[i] == "true"
			}
		}
	}

	if len(body) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one field to update is required\n")
		return 1
	}

	_, err := c.PUT(fmt.Sprintf("/triggers/%s", triggerID), body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Trigger #%s updated.\n", triggerID)
	return 0
}

func triggersDelete(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub triggers delete <id>\n")
		return 1
	}

	triggerID := args[0]
	_, err := c.DELETE(fmt.Sprintf("/triggers/%s", triggerID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Trigger #%s deleted.\n", triggerID)
	return 0
}

func printTriggersHelp() {
	fmt.Fprintf(os.Stderr, `Usage: ai-hub triggers <subcommand> [args]

Manage scheduled triggers.

Subcommands:
  list [--session <id>]               List triggers
  create --session <id> --time "09:00:00" --content "指令" [--max-fires -1]
  update <id> [--content "新指令"] [--time "10:00:00"] [--enabled true/false]
  delete <id>                         Delete a trigger

Examples:
  ai-hub triggers list
  ai-hub triggers list --session 25
  ai-hub triggers create --session 25 --time "09:00:00" --content "早报" --max-fires -1
  ai-hub triggers update 1 --content "新指令"
  ai-hub triggers delete 1
`)
}
