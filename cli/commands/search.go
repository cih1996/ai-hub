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
	fs.StringVar(&level, "level", "", "Level: session, team, or global (optional, searches all if omitted)")
	fs.IntVar(&top, "top", 10, "Number of results to return")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub search <query> [--level <level>] [flags]

Search memory files by semantic similarity and keyword matching.
When --level is omitted, searches session + team + global and merges results.

Flags:
`)
		fmt.Fprintf(os.Stderr, `  --level <level>     Optional. session / team / global (omit for all)
`)
		fmt.Fprintf(os.Stderr, `  --top <n>           Number of results (default: 10)

Examples:
  ai-hub search "BUG修复"
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

	sessionID := os.Getenv("AI_HUB_SESSION_ID")

	reqBody := map[string]interface{}{
		"query": query,
		"top_k": top,
	}
	if sessionID != "" {
		reqBody["session_id"], _ = json.Number(sessionID).Int64()
	}

	// If level specified, resolve to explicit scope
	if level != "" {
		scope, errMsg := LevelToScope(level)
		if errMsg != "" {
			fmt.Fprintf(os.Stderr, "%s\n", errMsg)
			return 1
		}
		reqBody["scope"] = scope
	}
	// If level is empty, don't set scope — backend will do three-layer merge

	respData, err := c.POST("/vector/search_memory", reqBody)
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

	levelLabel := level
	if levelLabel == "" {
		levelLabel = "all"
	}
	fmt.Printf("%d results (level=%s):\n\n", len(resp.Results), levelLabel)
	for i, r := range resp.Results {
		filename, _ := r["id"].(string)
		origin, _ := r["level"].(string)
		if origin == "" {
			origin, _ = r["origin"].(string)
		}
		hitCount := toInt(r["hit_count"])
		readCount := toInt(r["read_count"])
		snippet, _ := r["snippet"].(string)
		createdAt, _ := r["created_at"].(string)
		updatedAt, _ := r["updated_at"].(string)

		// If no snippet from keyword match, use document preview
		if snippet == "" {
			if doc, ok := r["document"].(string); ok {
				snippet = TruncatePreview(doc, 100)
			}
		}

		fmt.Printf("%d. %s [%s] (命中:%d 阅读:%d)\n", i+1, filename, origin, hitCount, readCount)
		fmt.Printf("   片段: %s\n", TruncatePreview(snippet, 120))
		fmt.Printf("   创建: %s  更新: %s\n", FormatTime(createdAt), FormatTime(updatedAt))
		fmt.Println("---")
	}
	return 0
}

// toInt extracts an int from interface{} (handles float64, json.Number, int, etc.)
func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
