package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// RunWrite executes the write command
func RunWrite(c *client.Client, globalGroup string, args []string) int {
	filename, flagArgs := SplitQueryAndFlags(args)

	var scope, group, content string
	fs := flag.NewFlagSet("write", flag.ExitOnError)
	fs.StringVar(&scope, "scope", "memory", "Scope: memory (default)")
	fs.StringVar(&group, "group", globalGroup, "Group name")
	fs.StringVar(&content, "content", "", "Content to write (or use stdin)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub write <filename> --scope <type> [flags]

Write a file to memory store.

Content can be provided via:
  --content "text"     Inline content
  echo "text" | ai-hub write <filename> --scope memory   Pipe from stdin

Flags:
  --scope <type>       Scope: memory (default)
  --group <name>       Group name (optional)
  --content <text>     Content to write (or pipe via stdin)

Examples:
  ai-hub write "my-note.md" --scope memory --content "# My Note"
  echo "hello" | ai-hub write "note.md" --scope memory
  cat file.md | ai-hub write "doc.md" --scope memory --group "MyTeam"
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
		scope = "memory"
	}
	if !ValidateScope(scope) {
		fmt.Fprintf(os.Stderr, "Error: --scope must be 'memory'\n")
		return 1
	}

	// Read content from stdin if not provided via flag
	if content == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				return 1
			}
			content = string(data)
		}
	}
	if content == "" {
		fmt.Fprintf(os.Stderr, "Error: content is required (use --content or pipe via stdin)\n")
		return 1
	}

	fullScope := BuildScope(scope, group)
	reqBody := map[string]interface{}{
		"file_name": filename,
		"content":   content,
		"scope":     fullScope,
	}

	respData, err := c.POST("/vector/write", reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		OK       bool   `json:"ok"`
		FileName string `json:"file_name"`
		Scope    string `json:"scope"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Printf("Written: %s (scope: %s)\n", resp.FileName, resp.Scope)
	return 0
}
