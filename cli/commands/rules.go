package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	case "list":
		return rulesList(c, args[1:])
	case "--help", "-h", "help":
		printRulesHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown rules subcommand: %s\n", args[0])
		printRulesHelp()
		return 1
	}
}

// parseRulesArgs parses --level and filename from args
func parseRulesArgs(args []string) (level, filename, content string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--level":
			if i+1 < len(args) {
				i++
				level = args[i]
			}
		case "--content":
			if i+1 < len(args) {
				i++
				content = args[i]
			}
		default:
			if filename == "" && !strings.HasPrefix(args[i], "-") {
				filename = args[i]
			}
		}
	}
	return
}

// getDataDir returns the AI Hub data directory
func getDataDir() string {
	if dir := os.Getenv("AI_HUB_DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ai-hub")
}

// getRulesDir returns the rules directory for the given level
func getRulesDir(level string) (string, error) {
	dataDir := getDataDir()
	groupName := os.Getenv("AI_HUB_GROUP_NAME")

	switch level {
	case "global":
		return filepath.Join(dataDir, "rules"), nil
	case "team":
		if groupName == "" {
			return "", fmt.Errorf("AI_HUB_GROUP_NAME required for team level")
		}
		return filepath.Join(dataDir, "teams", groupName, "rules"), nil
	default:
		return "", fmt.Errorf("invalid level: %s (use global or team)", level)
	}
}

func rulesList(c *client.Client, args []string) int {
	level, _, _ := parseRulesArgs(args)
	if level == "" {
		fmt.Fprintf(os.Stderr, "Error: --level is required (global or team)\n")
		return 1
	}

	rulesDir, err := getRulesDir(level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("No rules found at %s\n", rulesDir)
			return 0
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		fmt.Printf("No rules found at %s\n", rulesDir)
		return 0
	}

	fmt.Printf("Rules (%s level, %d files):\n", level, len(files))
	for _, f := range files {
		fmt.Printf("  %s\n", f)
	}
	return 0
}

func rulesGet(c *client.Client, args []string) int {
	level, filename, _ := parseRulesArgs(args)

	// If no level specified, try session-level (backward compatible)
	if level == "" {
		return rulesGetSession(c, args)
	}

	if filename == "" {
		fmt.Fprintf(os.Stderr, "Error: filename required for %s level\n", level)
		return 1
	}

	rulesDir, err := getRulesDir(level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	filePath := filepath.Join(rulesDir, filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: file not found: %s\n", filePath)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return 1
	}

	fmt.Print(string(data))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Println()
	}
	return 0
}

// rulesGetSession handles session-level rules (backward compatible)
func rulesGetSession(c *client.Client, args []string) int {
	sessionID := os.Getenv("AI_HUB_SESSION_ID")
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sessionID = args[0]
	}
	if sessionID == "" {
		fmt.Fprintf(os.Stderr, "Error: session ID required (argument or AI_HUB_SESSION_ID)\n")
		fmt.Fprintf(os.Stderr, "Hint: use --level global or --level team for file-based rules\n")
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
	level, filename, content := parseRulesArgs(args)

	// If no level specified, try session-level (backward compatible)
	if level == "" {
		return rulesSetSession(c, args)
	}

	if filename == "" {
		fmt.Fprintf(os.Stderr, "Error: filename required for %s level\n", level)
		return 1
	}
	if content == "" {
		fmt.Fprintf(os.Stderr, "Error: --content is required\n")
		return 1
	}

	rulesDir, err := getRulesDir(level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Ensure directory exists
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		return 1
	}

	filePath := filepath.Join(rulesDir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		return 1
	}

	fmt.Printf("Rules written: %s\n", filePath)
	return 0
}

// rulesSetSession handles session-level rules (backward compatible)
func rulesSetSession(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub rules set <session_id> --content \"内容\"\n")
		fmt.Fprintf(os.Stderr, "   or: ai-hub rules set <filename.md> --level <global|team> --content \"内容\"\n")
		return 1
	}

	sessionID := args[0]
	if _, err := strconv.ParseInt(sessionID, 10, 64); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid session ID: %s\n", sessionID)
		fmt.Fprintf(os.Stderr, "Hint: use --level global or --level team for file-based rules\n")
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
	level, filename, _ := parseRulesArgs(args)

	// If no level specified, try session-level (backward compatible)
	if level == "" {
		return rulesDeleteSession(c, args)
	}

	if filename == "" {
		fmt.Fprintf(os.Stderr, "Error: filename required for %s level\n", level)
		return 1
	}

	rulesDir, err := getRulesDir(level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	filePath := filepath.Join(rulesDir, filename)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: file not found: %s\n", filePath)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return 1
	}

	fmt.Printf("Rules deleted: %s\n", filePath)
	return 0
}

// rulesDeleteSession handles session-level rules (backward compatible)
func rulesDeleteSession(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub rules delete <session_id>\n")
		fmt.Fprintf(os.Stderr, "   or: ai-hub rules delete <filename.md> --level <global|team>\n")
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

Manage rules at different levels (session, team, global).

Subcommands:
  list --level <global|team>                          List rule files
  get [session_id]                                    Read session rules
  get <filename.md> --level <global|team>             Read rule file
  set <session_id> --content "内容"                   Write session rules
  set <filename.md> --level <global|team> --content "内容"  Write rule file
  delete <session_id>                                 Delete session rules
  delete <filename.md> --level <global|team>          Delete rule file

Levels:
  session  - Database-stored, per-session rules (default, no --level needed)
  team     - File-based, requires AI_HUB_GROUP_NAME env
  global   - File-based, applies to all sessions

Examples:
  # Session-level (backward compatible)
  ai-hub rules get
  ai-hub rules get 25
  ai-hub rules set 25 --content "你是技术维护工程师"
  ai-hub rules delete 25

  # Team-level
  ai-hub rules list --level team
  ai-hub rules get team-rules.md --level team
  ai-hub rules set team-rules.md --level team --content "团队规则内容"
  ai-hub rules delete team-rules.md --level team

  # Global-level
  ai-hub rules list --level global
  ai-hub rules get CLAUDE.md --level global
  ai-hub rules set custom.md --level global --content "全局规则内容"
  ai-hub rules delete custom.md --level global
`)
}
