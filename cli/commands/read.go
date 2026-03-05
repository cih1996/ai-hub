package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// RunRead executes the read command
func RunRead(c *client.Client, globalGroup string, args []string) int {
	filename, flagArgs := SplitQueryAndFlags(args)

	var scope, group string
	fs := flag.NewFlagSet("read", flag.ExitOnError)
	fs.StringVar(&scope, "scope", "", "Scope: knowledge or memory (required)")
	fs.StringVar(&group, "group", globalGroup, "Group name")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub read <filename> --scope <type> [flags]

Read a file from knowledge or memory store.

Flags:
  --scope <type>       Scope: knowledge or memory (required)
  --group <name>       Group name (optional)

Examples:
  ai-hub read "my-note.md" --scope knowledge
  ai-hub read "bug-fix.md" --scope memory --group "AI Hub 维护团队"
`)
	}

	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}

	if filename == "" {
		fmt.Fprintf(os.Stderr, "Error: filename is required\n\n")
		fs.Usage()
		return 1
	}
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
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
	reqBody := map[string]interface{}{
		"file_name": filename,
		"scope":     fullScope,
	}

	respData, err := c.POST("/vector/read", reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		FileName string `json:"file_name"`
		Content  string `json:"content"`
		Scope    string `json:"scope"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Print(resp.Content)
	return 0
}
