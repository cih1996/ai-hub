package mem

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FeedbackArgs is the input schema for mem feedback
type FeedbackArgs struct {
	ID     string `json:"id"`
	Result string `json:"result"` // success|fail
}

// RunFeedbackImpl executes the mem feedback command
func RunFeedbackImpl(c *client.Client, group string, args []string) int {
	var fArgs FeedbackArgs

	// Try JSON input first
	input, err := readInput(args)
	if err == nil {
		if err := json.Unmarshal(input, &fArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
			return 1
		}
	} else {
		// Parse flags
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--id":
				if i+1 < len(args) {
					i++
					fArgs.ID = args[i]
				}
			case "--result":
				if i+1 < len(args) {
					i++
					fArgs.Result = args[i]
				}
			}
		}
	}

	if fArgs.ID == "" {
		fmt.Fprintf(os.Stderr, "Error: --id is required\n")
		return 1
	}
	if fArgs.Result != "success" && fArgs.Result != "fail" {
		fmt.Fprintf(os.Stderr, "Error: --result must be 'success' or 'fail'\n")
		return 1
	}

	vectorScope := "memory"
	if group != "" {
		vectorScope = group + "/memory"
	}

	// Find the doc by mem_id — we need to search for it
	// Use the vector search to find the doc, then update metadata
	fileName, err := findDocByMemID(c, vectorScope, fArgs.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Build metadata updates
	updates := map[string]interface{}{
		"mem_last_used_at": NowISO(),
	}

	// We need to get current stats first, then increment
	docResp, err := c.POST("/vector/get_doc", map[string]interface{}{
		"scope":  vectorScope,
		"doc_id": fileName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting doc: %v\n", err)
		return 1
	}

	var doc struct {
		Metadata map[string]interface{} `json:"metadata"`
	}
	json.Unmarshal(docResp, &doc)

	currentUses := toFloat(doc.Metadata["mem_uses"])
	currentSuccess := toFloat(doc.Metadata["mem_success"])
	currentFail := toFloat(doc.Metadata["mem_fail"])

	updates["mem_uses"] = int(currentUses) + 1
	if fArgs.Result == "success" {
		updates["mem_success"] = int(currentSuccess) + 1
		updates["mem_fail"] = int(currentFail)
	} else {
		updates["mem_success"] = int(currentSuccess)
		updates["mem_fail"] = int(currentFail) + 1
	}

	_, err = c.POST("/vector/update_metadata", map[string]interface{}{
		"scope":    vectorScope,
		"doc_id":   fileName,
		"metadata": updates,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating metadata: %v\n", err)
		return 1
	}

	output := map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"id":      fArgs.ID,
			"result":  fArgs.Result,
			"uses":    int(currentUses) + 1,
			"success": updates["mem_success"],
			"fail":    updates["mem_fail"],
		},
	}
	outJSON, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(outJSON))
	return 0
}

// findDocByMemID searches for a document by its mem_id in metadata
func findDocByMemID(c *client.Client, scope, memID string) (string, error) {
	// Search with the mem_id as query to find the doc
	respData, err := c.POST("/vector/search", map[string]interface{}{
		"scope": scope,
		"query": memID,
		"top_k": 20,
	})
	if err != nil {
		return "", err
	}

	var resp struct {
		Results []map[string]interface{} `json:"results"`
	}
	json.Unmarshal(respData, &resp)

	for _, r := range resp.Results {
		meta, _ := r["metadata"].(map[string]interface{})
		if meta != nil {
			if id, ok := meta["mem_id"].(string); ok && id == memID {
				docID, _ := r["id"].(string)
				return docID, nil
			}
		}
	}

	// Fallback: try converting mem_id to filename pattern
	// mem_id format: mem_YYYYMMDD_XXXX, but filename is title-based
	// We need a broader search
	return "", fmt.Errorf("memory record not found: %s (try searching with a broader query)", memID)
}

// findDocByFileName is a helper to find doc by filename prefix
func findDocByFileName(c *client.Client, scope, prefix string) (string, error) {
	respData, err := c.GET("/vector/list?scope=" + strings.ReplaceAll(scope, " ", "%20"))
	if err != nil {
		return "", err
	}

	var files []string
	json.Unmarshal(respData, &files)

	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			return f, nil
		}
	}
	return "", fmt.Errorf("file not found with prefix: %s", prefix)
}
