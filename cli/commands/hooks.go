package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// RunHooks executes the hooks command
func RunHooks(c *client.Client, args []string) int {
	if len(args) == 0 {
		printHooksHelp()
		return 1
	}

	switch args[0] {
	case "list":
		return hooksList(c, args[1:])
	case "create":
		return hooksCreate(c, args[1:])
	case "delete":
		return hooksDelete(c, args[1:])
	case "enable":
		return hooksToggle(c, args[1:], true)
	case "disable":
		return hooksToggle(c, args[1:], false)
	default:
		fmt.Fprintf(os.Stderr, "Unknown hooks subcommand: %s\n", args[0])
		printHooksHelp()
		return 1
	}
}

func hooksList(c *client.Client, args []string) int {
	path := "/hooks"
	// Optional --event filter
	for i := 0; i < len(args); i++ {
		if args[i] == "--event" && i+1 < len(args) {
			i++
			path = fmt.Sprintf("/hooks?event=%s", args[i])
		}
	}

	respData, err := c.GET(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var hooks []struct {
		ID            int64  `json:"id"`
		Event         string `json:"event"`
		Condition     string `json:"condition"`
		TargetSession int64  `json:"target_session"`
		Payload       string `json:"payload"`
		Enabled       bool   `json:"enabled"`
		FiredCount    int    `json:"fired_count"`
	}
	if err := json.Unmarshal(respData, &hooks); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(hooks) == 0 {
		fmt.Println("No hooks found.")
		return 0
	}

	fmt.Printf("%d hooks:\n\n", len(hooks))
	for _, h := range hooks {
		enabled := "on"
		if !h.Enabled {
			enabled = "off"
		}
		fmt.Printf("#%-4d [%s] %s  -> session %d\n", h.ID, enabled, h.Event, h.TargetSession)
		if h.Condition != "" {
			fmt.Printf("      条件: %s\n", h.Condition)
		}
		fmt.Printf("      载荷: %s\n", TruncatePreview(h.Payload, 100))
		fmt.Printf("      触发: %d 次\n", h.FiredCount)
		fmt.Println("---")
	}
	return 0
}

func hooksCreate(c *client.Client, args []string) int {
	var event, condition, payload string
	var targetSession int64

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--event":
			if i+1 < len(args) {
				i++
				event = args[i]
			}
		case "--condition":
			if i+1 < len(args) {
				i++
				condition = args[i]
			}
		case "--target-session":
			if i+1 < len(args) {
				i++
				targetSession, _ = strconv.ParseInt(args[i], 10, 64)
			}
		case "--payload":
			if i+1 < len(args) {
				i++
				payload = args[i]
			}
		}
	}

	if event == "" || targetSession == 0 || payload == "" {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub hooks create --event <type> --target-session <id> --payload <msg> [--condition <cond>]\n")
		return 1
	}

	body := map[string]interface{}{
		"event":          event,
		"condition":      condition,
		"target_session": targetSession,
		"payload":        payload,
	}

	respData, err := c.POST("/hooks", body)
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

	fmt.Printf("Hook #%d created.\n", resp.ID)
	return 0
}

func hooksDelete(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub hooks delete <id>\n")
		return 1
	}

	hookID := args[0]
	_, err := c.DELETE(fmt.Sprintf("/hooks/%s", hookID))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Hook #%s deleted.\n", hookID)
	return 0
}

func hooksToggle(c *client.Client, args []string, enable bool) int {
	if len(args) < 1 {
		action := "enable"
		if !enable {
			action = "disable"
		}
		fmt.Fprintf(os.Stderr, "Usage: ai-hub hooks %s <id>\n", action)
		return 1
	}

	hookID := args[0]
	action := "enable"
	if !enable {
		action = "disable"
	}

	_, err := c.POST(fmt.Sprintf("/hooks/%s/%s", hookID, action), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	if enable {
		fmt.Printf("Hook #%s enabled.\n", hookID)
	} else {
		fmt.Printf("Hook #%s disabled.\n", hookID)
	}
	return 0
}

func printHooksHelp() {
	fmt.Fprintf(os.Stderr, `Usage: ai-hub hooks <subcommand> [args]

Manage event hooks.

Subcommands:
  list [--event <type>]               List hooks
  create --event <type> --target-session <id> --payload <msg> [--condition <cond>]
  delete <id>                         Delete a hook
  enable <id>                         Enable a hook
  disable <id>                        Disable a hook

Event types:
  session.created    New session created
  message.received   Message received (can filter with condition)
  message.count      Session message count exceeds threshold
  session.error      Error recorded in session

Condition formats:
  content_match:pattern1|pattern2     Match if content contains pattern
  count_gt:N                          Match if message count > N

Examples:
  ai-hub hooks list
  ai-hub hooks create --event "message.received" --condition "content_match:我说过了|不是这样" --target-session 999 --payload "会话 {source_session_id} 用户纠正"
  ai-hub hooks enable 1
  ai-hub hooks disable 1
  ai-hub hooks delete 1
`)
}
