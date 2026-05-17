package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

type scopedSearchFile struct {
	FileName  string `json:"file_name"`
	Preview   string `json:"preview"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Scope     string `json:"scope"`
	Origin    string `json:"origin"`
}

type searchResult struct {
	FileName  string
	Level     string
	Snippet   string
	CreatedAt string
	UpdatedAt string
	Score     int
}

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

Search memory files by filename and content text matching.
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
	if top <= 0 {
		top = 10
	}

	scopes, errMsg := scopesForSearch(level)
	if errMsg != "" {
		fmt.Fprintf(os.Stderr, "%s\n", errMsg)
		return 1
	}

	queryLower := strings.ToLower(query)
	var results []searchResult
	for _, scope := range scopes {
		params := url.Values{}
		params.Set("scope", scope)
		params.Set("type", "memory")
		respData, err := c.GET("/files/scoped/list?" + params.Encode())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing files in %s: %v\n", scope, err)
			return 1
		}

		var resp struct {
			Files []scopedSearchFile `json:"files"`
		}
		if err := json.Unmarshal(respData, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
			return 1
		}

		for _, file := range resp.Files {
			readResp, err := c.POST("/files/scoped/read", map[string]interface{}{
				"scope":     file.Scope,
				"file_name": file.FileName,
				"type":      "memory",
			})
			if err != nil {
				continue
			}

			var contentResp struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(readResp, &contentResp); err != nil {
				continue
			}

			filenameLower := strings.ToLower(file.FileName)
			contentLower := strings.ToLower(contentResp.Content)
			previewLower := strings.ToLower(file.Preview)
			if !strings.Contains(filenameLower, queryLower) &&
				!strings.Contains(contentLower, queryLower) &&
				!strings.Contains(previewLower, queryLower) {
				continue
			}

			score := strings.Count(contentLower, queryLower)
			score += strings.Count(previewLower, queryLower)
			if strings.Contains(filenameLower, queryLower) {
				score += 3
			}
			score += scopePriority(file.Origin)

			results = append(results, searchResult{
				FileName:  file.FileName,
				Level:     file.Origin,
				Snippet:   buildSnippet(contentResp.Content, query),
				CreatedAt: file.CreatedAt,
				UpdatedAt: file.UpdatedAt,
				Score:     score,
			})
		}
	}

	if len(results) == 0 {
		fmt.Println("No results found")
		return 0
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].UpdatedAt > results[j].UpdatedAt
	})
	if len(results) > top {
		results = results[:top]
	}

	levelLabel := level
	if levelLabel == "" {
		levelLabel = "all"
	}
	fmt.Printf("%d results (level=%s):\n\n", len(results), levelLabel)
	for i, r := range results {
		fmt.Printf("%d. %s [%s]\n", i+1, r.FileName, r.Level)
		fmt.Printf("   片段: %s\n", TruncatePreview(r.Snippet, 120))
		fmt.Printf("   创建: %s  更新: %s\n", FormatTime(r.CreatedAt), FormatTime(r.UpdatedAt))
		fmt.Println("---")
	}
	return 0
}

func scopesForSearch(level string) ([]string, string) {
	if level != "" {
		scope, errMsg := LevelToScope(level)
		if errMsg != "" {
			return nil, errMsg
		}
		return []string{scope}, ""
	}

	group := os.Getenv("AI_HUB_GROUP_NAME")
	sessionID := os.Getenv("AI_HUB_SESSION_ID")
	if group == "" {
		group = "_standalone"
	}

	var scopes []string
	if sessionID != "" {
		scopes = append(scopes, group+"/sessions/"+sessionID+"/memory")
	}
	if group != "" {
		scopes = append(scopes, group+"/memory")
	}
	scopes = append(scopes, "global")
	return uniqueStrings(scopes), ""
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func scopePriority(origin string) int {
	switch origin {
	case "session":
		return 3
	case "team":
		return 2
	default:
		return 1
	}
}

func buildSnippet(content string, query string) string {
	content = strings.ReplaceAll(content, "\n", " ")
	if content == "" {
		return ""
	}
	contentRunes := []rune(content)
	lowerContent := strings.ToLower(content)
	lowerQuery := strings.ToLower(query)
	idx := strings.Index(lowerContent, lowerQuery)
	if idx < 0 {
		return TruncatePreview(content, 120)
	}
	startIdx := utf8.RuneCountInString(lowerContent[:idx])
	endIdx := startIdx + utf8.RuneCountInString(query)

	start := startIdx - 40
	if start < 0 {
		start = 0
	}
	end := endIdx + 80
	if end > len(contentRunes) {
		end = len(contentRunes)
	}
	snippet := strings.TrimSpace(string(contentRunes[start:end]))
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(contentRunes) {
		snippet += "..."
	}
	return snippet
}
