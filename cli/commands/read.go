package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// RunRead executes the read command
func RunRead(c *client.Client, args []string) int {
	filename, flagArgs := SplitQueryAndFlags(args)

	var level string
	fs := flag.NewFlagSet("read", flag.ExitOnError)
	fs.StringVar(&level, "level", "", "Level: session, team, or global (required)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub read <filename> --level <level>

Read a memory file's full content.

Flags:
`)
		PrintLevelUsage()
		fmt.Fprintf(os.Stderr, `
Examples:
  ai-hub read "my-note.md" --level session
  ai-hub read "team-sop.md" --level team
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

	reqBody := map[string]interface{}{
		"scope":     scope,
		"file_name": filename,
	}

	respData, err := c.POST("/vector/read", reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	fmt.Print(resp.Content)
	return 0
}
