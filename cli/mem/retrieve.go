package mem

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

// RunRetrieveImpl executes the mem retrieve command
func RunRetrieveImpl(c *client.Client, group string, args []string) int {
	input, err := readInput(args)
	if err != nil {
		// Fallback: try --query flag
		return runRetrieveFlags(c, group, args)
	}

	var rArgs RetrieveArgs
	if err := json.Unmarshal(input, &rArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		return 1
	}
	return doRetrieve(c, group, &rArgs)
}

// RetrieveArgs is the input schema for mem retrieve
type RetrieveArgs struct {
	Query  string   `json:"query"`
	K      int      `json:"k,omitempty"`
	Types  []string `json:"types,omitempty"`
	Status []string `json:"status,omitempty"`
}

func runRetrieveFlags(c *client.Client, group string, args []string) int {
	rArgs := &RetrieveArgs{K: 8, Status: []string{"active"}}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--query":
			if i+1 < len(args) {
				i++
				rArgs.Query = args[i]
			}
		case "--k":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &rArgs.K)
			}
		case "--types":
			if i+1 < len(args) {
				i++
				rArgs.Types = strings.Split(args[i], ",")
			}
		case "--status":
			if i+1 < len(args) {
				i++
				rArgs.Status = strings.Split(args[i], ",")
			}
		default:
			if !strings.HasPrefix(args[i], "-") && rArgs.Query == "" {
				rArgs.Query = args[i]
			}
		}
	}

	if rArgs.Query == "" {
		fmt.Fprintf(os.Stderr, "Error: --query is required\n")
		fmt.Fprintf(os.Stderr, "\nUsage: ai-hub mem retrieve --query <text> [--k N] [--types type1,type2] [--status active,deprecated]\n")
		return 1
	}
	return doRetrieve(c, group, rArgs)
}

func doRetrieve(c *client.Client, group string, rArgs *RetrieveArgs) int {
	if rArgs.K <= 0 {
		rArgs.K = 8
	}
	if len(rArgs.Status) == 0 {
		rArgs.Status = []string{"active"}
	}

	vectorScope := "memory"
	if group != "" {
		vectorScope = group + "/memory"
	}

	// Fetch more candidates than needed for post-filtering
	fetchK := rArgs.K * 3
	if fetchK < 20 {
		fetchK = 20
	}

	reqBody := map[string]interface{}{
		"scope": vectorScope,
		"query": rArgs.Query,
		"top_k": fetchK,
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

	// Filter and rerank
	type scored struct {
		result    map[string]interface{}
		semantic  float64
		statScore float64
		final     float64
		breakdown map[string]float64
	}

	var candidates []scored
	statusSet := toSet(rArgs.Status)
	typeSet := toSet(rArgs.Types)

	for _, r := range resp.Results {
		meta, _ := r["metadata"].(map[string]interface{})
		if meta == nil {
			continue
		}

		// Filter by status
		memStatus, _ := meta["mem_status"].(string)
		if memStatus == "" {
			memStatus = "active"
		}
		if !statusSet[memStatus] {
			continue
		}

		// Filter by type
		memType, _ := meta["mem_type"].(string)
		if len(typeSet) > 0 && !typeSet[memType] {
			continue
		}

		similarity, _ := r["similarity"].(float64)

		// Statistical reranking
		uses := toFloat(meta["hit_count"])
		successRate := 0.5 // default neutral
		memSuccess := toFloat(meta["mem_success"])
		memUses := toFloat(meta["mem_uses"])
		if memUses > 0 {
			successRate = (memSuccess + 1) / (memUses + 2) // Laplace smoothing
		}

		popularity := math.Log(1 + uses + memUses)

		// Scope priority
		scopePriority := 0.5
		memScope, _ := meta["mem_scope"].(string)
		switch memScope {
		case "session":
			scopePriority = 1.0
		case "workspace":
			scopePriority = 0.8
		case "tenant":
			scopePriority = 0.5
		case "global":
			scopePriority = 0.3
		}

		breakdown := map[string]float64{
			"semantic":     similarity,
			"scope":        scopePriority,
			"success_rate": successRate,
			"popularity":   popularity / 5.0, // normalize
		}

		final := 0.50*similarity +
			0.20*scopePriority +
			0.15*successRate +
			0.15*(popularity/5.0)

		candidates = append(candidates, scored{
			result:    r,
			semantic:  similarity,
			statScore: successRate,
			final:     final,
			breakdown: breakdown,
		})
	}

	// Sort by final score descending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].final > candidates[i].final {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Truncate to K
	if len(candidates) > rArgs.K {
		candidates = candidates[:rArgs.K]
	}

	// Build output
	results := make([]map[string]interface{}, 0, len(candidates))
	for _, c := range candidates {
		meta, _ := c.result["metadata"].(map[string]interface{})
		doc, _ := c.result["document"].(string)

		// Build snippet (first 150 chars of document)
		snippet := doc
		if len(snippet) > 150 {
			snippet = snippet[:150] + "..."
		}

		entry := map[string]interface{}{
			"id":              metaStr(meta, "mem_id"),
			"type":            metaStr(meta, "mem_type"),
			"title":           metaStr(meta, "mem_title"),
			"status":          metaStr(meta, "mem_status"),
			"version":         meta["mem_version"],
			"snippet":         snippet,
			"score":           round(c.final, 4),
			"score_breakdown": c.breakdown,
		}
		results = append(results, entry)
	}

	output := map[string]interface{}{
		"ok":   true,
		"data": map[string]interface{}{"results": results, "total": len(results)},
	}
	outJSON, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(outJSON))
	return 0
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		if item != "" {
			s[item] = true
		}
	}
	return s
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func metaStr(meta map[string]interface{}, key string) string {
	if v, ok := meta[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func round(f float64, decimals int) float64 {
	pow := math.Pow(10, float64(decimals))
	return math.Round(f*pow) / pow
}
