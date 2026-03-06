package commands

import (
	"ai-hub/cli/client"
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// RunDelete executes the delete command
func RunDelete(c *client.Client, args []string) int {
	filename, flagArgs := SplitQueryAndFlags(args)

	var level string
	var force bool
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	fs.StringVar(&level, "level", "", "Level: session, team, or global (required)")
	fs.BoolVar(&force, "force", false, "Skip confirmation prompt")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub delete <filename> --level <level> [flags]

Delete a memory file.

Flags:
`)
		PrintLevelUsage()
		fmt.Fprintf(os.Stderr, `  --force             Skip confirmation prompt

Examples:
  ai-hub delete "old-note.md" --level session --force
  ai-hub delete "temp.md" --level team
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

	if !force {
		fmt.Printf("Delete '%s' from %s? [y/N] ", filename, level)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Cancelled")
			return 0
		}
	}

	reqBody := map[string]interface{}{
		"scope":     scope,
		"file_name": filename,
	}

	respData, err := c.POST("/vector/delete", reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp map[string]interface{}
	json.Unmarshal(respData, &resp)
	fmt.Printf("Deleted: %s (level=%s)\n", filename, level)
	return 0
}
