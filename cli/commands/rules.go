package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// RunRules executes the rules command
func RunRules(c *client.Client, args []string) int {
	if len(args) == 0 {
		printRulesHelp()
		return 1
	}

	switch args[0] {
	case "get":
		return rulesGet(c, args[1:])
	case "set":
		return rulesSet(c, args[1:])
	case "delete":
		return rulesDelete(c, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown rules subcommand: %s\n", args[0])
		printRulesHelp()
		return 1
	}
}

func rulesGet(c *client.Client, args []string) int {
	sessionID := os.Getenv("AI_HUB_SESSION_ID")
	if len(args) > 0 {
		sessionID = args[0]
	}
	if sessionID == "" {
		fmt.Fprintf(os.Stderr, "Error: session ID required (argument or AI_HUB_SESSION_ID)\n")
		return 1
	}

	respData, err := c.GET(fmt.Sprintf("/session-rules/%s", sessionID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		SessionID int64  `json:"session_id"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if resp.Content == "" {
		fmt.Printf("Session #%s: no rules set\n", sessionID)
		return 0
	}

	fmt.Print(resp.Content)
	if resp.Content[len(resp.Content)-1] != '\n' {
		fmt.Println()
	}
	return 0
}

func rulesSet(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub rules set <session_id> --content \"内容\"\n")
		return 1
	}

	sessionID := args[0]
	if _, err := strconv.ParseInt(sessionID, 10, 64); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid session ID: %s\n", sessionID)
		return 1
	}

	var content string
	for i := 1; i < len(args); i++ {
		if args[i] == "--content" && i+1 < len(args) {
			i++
			content = args[i]
		}
	}

	if content == "" {
		fmt.Fprintf(os.Stderr, "Error: --content is required\n")
		return 1
	}

	body := map[string]string{"content": content}
	_, err := c.PUT(fmt.Sprintf("/session-rules/%s", sessionID), body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Session #%s: rules updated\n", sessionID)
	return 0
}

func rulesDelete(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub rules delete <session_id>\n")
		return 1
	}

	sessionID := args[0]
	_, err := c.DELETE(fmt.Sprintf("/session-rules/%s", sessionID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Session #%s: rules deleted\n", sessionID)
	return 0
}

func printRulesHelp() {
	fmt.Fprintf(os.Stderr, `Usage: ai-hub rules <subcommand> [args]

Manage session-level rules.

Subcommands:
  get [session_id]                    Read rules (default: current session)
  set <session_id> --content "内容"   Write rules
  delete <session_id>                 Delete rules

Examples:
  ai-hub rules get
  ai-hub rules get 25
  ai-hub rules set 25 --content "你是技术维护工程师"
  ai-hub rules delete 25
`)
}
