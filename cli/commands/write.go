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
func RunWrite(c *client.Client, args []string) int {
	filename, flagArgs := SplitQueryAndFlags(args)

	var level, content, schema string
	fs := flag.NewFlagSet("write", flag.ExitOnError)
	fs.StringVar(&level, "level", "", "Level: session, team, or global (required)")
	fs.StringVar(&content, "content", "", "Content to write (or use stdin)")
	fs.StringVar(&schema, "schema", "", "Schema name for validation before write (optional)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub write <filename> --level <level> [flags]

Write a memory file.

Flags:
`)
		PrintLevelUsage()
		fmt.Fprintf(os.Stderr, `  --content <text>    Content to write (or pipe via stdin)
  --schema <name>     Schema name for validation before write (optional)

Examples:
  ai-hub write "note.md" --level session --content "# My Note"
  echo "hello" | ai-hub write "note.md" --level team
  ai-hub write "config.json" --level team --schema my-schema --content '{"title":"test"}'
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

	if level == "" {
		fmt.Fprintf(os.Stderr, "Error: --level is required (session / team / global)\n\n")
		fs.Usage()
		return 1
	}

	scope, errMsg := LevelToScope(level)
	if errMsg != "" {
		fmt.Fprintf(os.Stderr, "%s\n", errMsg)
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
			content = strings.TrimRight(string(data), "\n")
		}
	}

	if content == "" {
		fmt.Fprintf(os.Stderr, "Error: content is required (use --content or pipe via stdin)\n")
		return 1
	}

	reqBody := map[string]interface{}{
		"scope":     scope,
		"file_name": filename,
		"content":   content,
	}

	// Add schema if specified
	if schema != "" {
		reqBody["schema"] = schema
	}

	respData, err := c.POST("/vector/write", reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp map[string]interface{}
	json.Unmarshal(respData, &resp)
	fmt.Printf("Written: %s (level=%s)\n", filename, level)
	return 0
}
