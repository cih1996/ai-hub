package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// RunDelete executes the delete command
func RunDelete(c *client.Client, globalGroup string, args []string) int {
	filename, flagArgs := SplitQueryAndFlags(args)

	var scope, group string
	var force bool
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	fs.StringVar(&scope, "scope", "", "Scope: knowledge or memory (required)")
	fs.StringVar(&group, "group", globalGroup, "Group name")
	fs.BoolVar(&force, "force", false, "Skip confirmation prompt")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub delete <filename> --scope <type> [flags]

Delete a file from knowledge or memory store.

Flags:
  --scope <type>       Scope: knowledge or memory (required)
  --group <name>       Group name (optional)
  --force              Skip confirmation prompt

Examples:
  ai-hub delete "old-note.md" --scope knowledge --force
  ai-hub delete "temp.md" --scope memory --group "MyTeam"
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

	// Confirmation prompt unless --force
	if !force {
		fmt.Printf("Delete %s from %s? [y/N] ", filename, fullScope)
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Cancelled.")
			return 0
		}
	}

	reqBody := map[string]interface{}{
		"file_name": filename,
		"scope":     fullScope,
	}

	respData, err := c.POST("/vector/delete", reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		OK       bool   `json:"ok"`
		FileName string `json:"file_name"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Printf("Deleted: %s (scope: %s)\n", resp.FileName, fullScope)
	return 0
}
