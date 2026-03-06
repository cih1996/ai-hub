package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// RunEdit executes the edit command (read → find/replace → write)
func RunEdit(c *client.Client, args []string) int {
	filename, flagArgs := SplitQueryAndFlags(args)

	var level, oldText, newText string
	fs := flag.NewFlagSet("edit", flag.ExitOnError)
	fs.StringVar(&level, "level", "", "Level: session, team, or global (required)")
	fs.StringVar(&oldText, "old", "", "Text to find and replace (required)")
	fs.StringVar(&newText, "new", "", "Replacement text (required)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub edit <filename> --level <level> --old "旧文本" --new "新文本"

Edit a memory file by finding and replacing text.

Flags:
`)
		PrintLevelUsage()
		fmt.Fprintf(os.Stderr, `  --old <text>        Text to find (required)
  --new <text>        Replacement text (required)

Examples:
  ai-hub edit "note.md" --level session --old "旧内容" --new "新内容"
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
	if oldText == "" {
		fmt.Fprintf(os.Stderr, "Error: --old is required\n")
		return 1
	}

	scope, errMsg := LevelToScope(level)
	if errMsg != "" {
		fmt.Fprintf(os.Stderr, "%s\n", errMsg)
		return 1
	}

	// Step 1: Read current content
	readBody := map[string]interface{}{
		"scope":     scope,
		"file_name": filename,
	}
	respData, err := c.POST("/vector/read", readBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		return 1
	}

	var readResp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(respData, &readResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	// Step 2: Find and replace
	if !strings.Contains(readResp.Content, oldText) {
		fmt.Fprintf(os.Stderr, "Error: old text not found in '%s'\n", filename)
		return 1
	}

	newContent := strings.Replace(readResp.Content, oldText, newText, 1)

	// Step 3: Write back
	writeBody := map[string]interface{}{
		"scope":     scope,
		"file_name": filename,
		"content":   newContent,
	}
	_, err = c.POST("/vector/write", writeBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		return 1
	}

	// Step 4: Show diff
	fmt.Printf("Edited: %s (level=%s)\n", filename, level)
	fmt.Println("--- old")
	fmt.Println("+++ new")
	// Show context around the change
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	for _, l := range oldLines {
		fmt.Printf("- %s\n", l)
	}
	for _, l := range newLines {
		fmt.Printf("+ %s\n", l)
	}

	return 0
}
