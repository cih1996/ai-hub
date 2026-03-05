package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// SearchFlags holds search command flags
type SearchFlags struct {
	Scope     string
	Group     string
	Top       int
	Threshold float64
}

// RunSearch executes the search command
func RunSearch(c *client.Client, globalGroup string, args []string) int {
	query, flagArgs := SplitQueryAndFlags(args)

	flags := &SearchFlags{}
	fs := flag.NewFlagSet("search", flag.ExitOnError)

	fs.StringVar(&flags.Scope, "scope", "", "Scope: knowledge or memory (required)")
	fs.StringVar(&flags.Group, "group", globalGroup, "Group name (inherits from global --group)")
	fs.IntVar(&flags.Top, "top", 5, "Number of results to return")
	fs.Float64Var(&flags.Threshold, "threshold", 0.7, "Similarity threshold (0.0-1.0)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: ai-hub search <query> [flags]

Search knowledge or memory files by semantic similarity.

Flags:
  --scope <type>       Scope: knowledge or memory (required)
  --group <name>       Group name (optional, inherits from global --group)
  --top <n>            Number of results to return (default: 5)
  --threshold <float>  Similarity threshold 0.0-1.0 (default: 0.7)

Examples:
  ai-hub search "向量搜索" --scope knowledge --group "AI Hub维护团队"
  ai-hub search "BUG修复" --scope memory --top 3
`)
	}

	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}

	// Validate query
	if query == "" {
		fmt.Fprintf(os.Stderr, "Error: query is required\n\n")
		fs.Usage()
		return 1
	}

	// Validate scope
	if flags.Scope == "" {
		fmt.Fprintf(os.Stderr, "Error: --scope is required (knowledge or memory)\n\n")
		fs.Usage()
		return 1
	}
	if flags.Scope != "knowledge" && flags.Scope != "memory" {
		fmt.Fprintf(os.Stderr, "Error: --scope must be 'knowledge' or 'memory'\n")
		return 1
	}

	// Build full scope with group prefix if provided
	fullScope := flags.Scope
	if flags.Group != "" {
		fullScope = flags.Group + "/" + flags.Scope
	}

	// Call API
	reqBody := map[string]interface{}{
		"scope": fullScope,
		"query": query,
		"top_k": flags.Top,
	}

	respData, err := c.POST("/vector/search", reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Parse response
	var resp struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(respData, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	// Filter by threshold and display results
	filtered := 0
	for i, result := range resp.Results {
		// API returns "similarity" not "score"
		similarity, _ := result["similarity"].(float64)
		if similarity < flags.Threshold {
			continue
		}

		filtered++
		// API returns "id" (filename) and "document" (full content)
		filename, _ := result["id"].(string)
		document, _ := result["document"].(string)

		// Extract path from metadata if available
		path := ""
		if metadata, ok := result["metadata"].(map[string]interface{}); ok {
			if filePath, ok := metadata["file_path"].(string); ok {
				path = filePath
			}
		}

		// Truncate content to 200 chars
		preview := document
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}

		fmt.Printf("%d. %s (score: %.3f)\n", i+1, filename, similarity)
		if path != "" {
			fmt.Printf("   Path: %s\n", path)
		}
		fmt.Printf("   Preview: %s\n", preview)
		if i < len(resp.Results)-1 {
			fmt.Println("   " + strings.Repeat("-", 60))
		}
	}

	if filtered == 0 {
		fmt.Printf("No results found above threshold %.2f\n", flags.Threshold)
	}

	return 0
}
