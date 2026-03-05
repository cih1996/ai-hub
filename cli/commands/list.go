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
func RunList(c *client.Client, globalGroup string, args []string) int {
	var scope, group string
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.StringVar(&scope, "scope", "", "Scope: knowledge or memory (required)")
	fs.StringVar(&group, "group", globalGroup, "Group name")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub list --scope <type> [flags]

List files in knowledge or memory store.

Flags:
  --scope <type>       Scope: knowledge or memory (required)
  --group <name>       Group name (optional)

Examples:
  ai-hub list --scope knowledge
  ai-hub list --scope memory --group "AI Hub 维护团队"
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

	fullScope := BuildScope(scope, group)
	path := "/vector/list?scope=" + url.QueryEscape(fullScope)

	respData, err := c.GET(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var files []string
	if err := json.Unmarshal(respData, &files); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(files) == 0 {
		fmt.Printf("No files in %s\n", fullScope)
		return 0
	}

	fmt.Printf("%s (%d files):\n", fullScope, len(files))
	for i, f := range files {
		fmt.Printf("  %d. %s\n", i+1, f)
	}
	return 0
}
