package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
)

// RunList executes the list command
func RunList(c *client.Client, globalGroup string, sessionID int64, args []string) int {
	var scope, group, level string
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.StringVar(&scope, "scope", "", "Scope: knowledge or memory (required)")
	fs.StringVar(&group, "group", globalGroup, "Group name")
	fs.StringVar(&level, "level", "all", "Level filter: session, team, global, all")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub list --scope <type> [flags]

List files in knowledge or memory store.

Flags:
  --scope <type>       Scope: knowledge or memory (required)
  --group <name>       Group name (optional)
  --level <level>      Level: session, team, global, all (default: all)

Examples:
  ai-hub list --scope knowledge
  ai-hub list --scope memory --level session
  ai-hub list --scope knowledge --group "AI Hub 维护团队" --level team
`)
	}

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if scope == "" {
		fmt.Fprintf(os.Stderr, "Error: --scope is required (knowledge or memory)\n\n")
		fs.Usage()
		return 1
	}
	if !ValidateScope(scope) {
		fmt.Fprintf(os.Stderr, "Error: --scope must be 'knowledge' or 'memory'\n")
		return 1
	}

	// Use rich list endpoint with level support
	params := url.Values{}
	params.Set("type", scope)
	params.Set("level", level)
	if sessionID > 0 {
		params.Set("session_id", fmt.Sprintf("%d", sessionID))
	}
	if group != "" {
		// If explicit group but no session, build explicit scope
		params.Set("scope", BuildScope(scope, group))
	}

	path := "/vector/list_files?" + params.Encode()
	respData, err := c.GET(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Files []struct {
			FileName string `json:"file_name"`
			Origin   string `json:"origin"`
			Type     string `json:"type"`
			Scope    string `json:"scope"`
		} `json:"files"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if resp.Total == 0 {
		fmt.Printf("No files found (level=%s, type=%s)\n", level, scope)
		return 0
	}

	fmt.Printf("%d files (level=%s, type=%s):\n", resp.Total, level, scope)
	for i, f := range resp.Files {
		fmt.Printf("  %d. [%s] %s  (%s)\n", i+1, f.Origin, f.FileName, f.Scope)
	}
	return 0
}
