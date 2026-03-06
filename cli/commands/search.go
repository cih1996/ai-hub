package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// RunSearch executes the search command
func RunSearch(c *client.Client, args []string) int {
	query, flagArgs := SplitQueryAndFlags(args)

	var level string
	var top int
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	fs.StringVar(&level, "level", "", "Level: session, team, or global (required)")
	fs.IntVar(&top, "top", 10, "Number of results to return")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub search <query> --level <level> [flags]

Search memory files by semantic similarity.

Flags:
`)
		PrintLevelUsage()
		fmt.Fprintf(os.Stderr, `  --top <n>           Number of results (default: 10)

Examples:
  ai-hub search "BUG修复" --level session
  ai-hub search "部署流程" --level team --top 5
`)
	}

	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}

	if query == "" {
		fmt.Fprintf(os.Stderr, "Error: query is required\n\n")
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

	sessionID := os.Getenv("AI_HUB_SESSION_ID")

	reqBody := map[string]interface{}{
		"scope": scope,
		"query": query,
		"top_k": top,
	}
	if sessionID != "" {
		reqBody["session_id"], _ = json.Number(sessionID).Int64()
	}

	respData, err := c.POST("/vector/search", reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var resp struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(resp.Results) == 0 {
		fmt.Println("No results found")
		return 0
	}

	fmt.Printf("%d results (level=%s):\n\n", len(resp.Results), level)
	for i, r := range resp.Results {
		filename, _ := r["id"].(string)
		similarity, _ := r["similarity"].(float64)
		document, _ := r["document"].(string)
		createdAt, _ := r["created_at"].(string)
		updatedAt, _ := r["updated_at"].(string)

		fmt.Printf("%d. %s (相似度: %.3f)\n", i+1, filename, similarity)
		fmt.Printf("   预览: %s\n", TruncatePreview(document, 100))
		fmt.Printf("   创建: %s  更新: %s\n", FormatTime(createdAt), FormatTime(updatedAt))
		fmt.Println("---")
	}
	return 0
}
