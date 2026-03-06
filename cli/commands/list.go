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
func RunList(c *client.Client, args []string) int {
	var level string
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.StringVar(&level, "level", "", "Level: session, team, or global (required)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub list --level <level>

List memory files at the specified level.

Flags:
`)
		PrintLevelUsage()
		fmt.Fprintf(os.Stderr, `
Examples:
  ai-hub list --level session
  ai-hub list --level team
  ai-hub list --level global
`)
	}

	if err := fs.Parse(args); err != nil {
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

	params := url.Values{}
	params.Set("scope", scope)
	path := "/vector/list_files?" + params.Encode()
	respData, err := c.GET(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Files []struct {
			FileName  string `json:"file_name"`
			Preview   string `json:"preview"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
		} `json:"files"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if resp.Total == 0 {
		fmt.Printf("No files found (level=%s)\n", level)
		return 0
	}

	fmt.Printf("%d files (level=%s):\n\n", resp.Total, level)
	for i, f := range resp.Files {
		fmt.Printf("%d. %s\n", i+1, f.FileName)
		fmt.Printf("   预览: %s\n", TruncatePreview(f.Preview, 100))
		fmt.Printf("   创建: %s  更新: %s\n", FormatTime(f.CreatedAt), FormatTime(f.UpdatedAt))
		fmt.Println("---")
	}
	return 0
}
