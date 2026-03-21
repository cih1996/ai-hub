package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// RunChangelog executes the changelog command
// Usage: ai-hub changelog <file_name> [--scope <scope>] [--limit 10] [--rollback <version>]
func RunChangelog(c *client.Client, args []string) int {
	if len(args) == 0 {
		printChangelogHelp()
		return 1
	}

	fileName := args[0]
	var scope string
	var limit int
	var rollbackVersion int

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--scope":
			if i+1 < len(args) {
				i++
				scope = args[i]
			}
		case "--limit":
			if i+1 < len(args) {
				i++
				limit, _ = strconv.Atoi(args[i])
			}
		case "--rollback":
			if i+1 < len(args) {
				i++
				rollbackVersion, _ = strconv.Atoi(args[i])
			}
		}
	}

	// Rollback mode
	if rollbackVersion > 0 {
		return changelogRollback(c, fileName, scope, rollbackVersion)
	}

	// List mode (default)
	return changelogList(c, fileName, scope, limit)
}

func changelogList(c *client.Client, fileName, scope string, limit int) int {
	path := fmt.Sprintf("/changelog?file_name=%s", fileName)
	if scope != "" {
		path += fmt.Sprintf("&scope=%s", scope)
	}
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}

	respData, err := c.GET(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Changelog []struct {
			ID         int64  `json:"id"`
			FileName   string `json:"file_name"`
			Scope      string `json:"scope"`
			ChangeType string `json:"change_type"`
			SessionID  int64  `json:"session_id"`
			Diff       string `json:"diff"`
			Schema     string `json:"schema"`
			Version    int    `json:"version"`
			CreatedAt  string `json:"created_at"`
		} `json:"changelog"`
		FileName string `json:"file_name"`
		Scope    string `json:"scope"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(resp.Changelog) == 0 {
		fmt.Printf("No changelog for %s (scope: %s)\n", resp.FileName, resp.Scope)
		return 0
	}

	fmt.Printf("Changelog for %s (scope: %s):\n\n", resp.FileName, resp.Scope)
	for _, cl := range resp.Changelog {
		typeIcon := "+"
		switch cl.ChangeType {
		case "update":
			typeIcon = "~"
		case "delete":
			typeIcon = "-"
		}
		schema := ""
		if cl.Schema != "" {
			schema = fmt.Sprintf(" [%s]", cl.Schema)
		}
		fmt.Printf("v%-3d [%s] %s  session=%d%s\n", cl.Version, typeIcon, FormatTime(cl.CreatedAt), cl.SessionID, schema)
		if cl.Diff != "" {
			fmt.Printf("     %s\n", TruncatePreview(cl.Diff, 200))
		}
		fmt.Println("---")
	}
	return 0
}

func changelogRollback(c *client.Client, fileName, scope string, version int) int {
	body := map[string]interface{}{
		"file_name": fileName,
		"scope":     scope,
		"version":   version,
	}

	respData, err := c.POST("/changelog/rollback", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		OK           bool `json:"ok"`
		RolledBackTo int  `json:"rolled_back_to"`
		NewVersion   int  `json:"new_version"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Printf("Rolled back %s to version %d (new version: %d)\n", fileName, resp.RolledBackTo, resp.NewVersion)
	return 0
}

func printChangelogHelp() {
	fmt.Fprintf(os.Stderr, `Usage: ai-hub changelog <file_name> [options]

View and manage memory file change history.

Options:
  --scope <scope>         Memory scope (default: memory)
  --limit <n>             Limit results (default: 20)
  --rollback <version>    Rollback to a specific version

Examples:
  ai-hub changelog "项目.md" --scope memory
  ai-hub changelog "项目.md" --scope memory --limit 5
  ai-hub changelog "项目.md" --scope memory --rollback 3
`)
}
