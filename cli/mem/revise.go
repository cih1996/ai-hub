package mem

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"os"
)

// ReviseArgs is the input schema for mem revise
type ReviseArgs struct {
	ID         string                 `json:"id"`
	NewTitle   string                 `json:"new_title,omitempty"`
	NewContent map[string]interface{} `json:"new_content,omitempty"`
	NewTags    []string               `json:"new_tags,omitempty"`
}

// RunReviseImpl executes the mem revise command
func RunReviseImpl(c *client.Client, group string, args []string) int {
	input, err := readInput(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var rArgs ReviseArgs
	if err := json.Unmarshal(input, &rArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		return 1
	}

	if rArgs.ID == "" {
		fmt.Fprintf(os.Stderr, "Error: id is required\n")
		return 1
	}

	vectorScope := "memory"
	if group != "" {
		vectorScope = group + "/memory"
	}

	// Find the original doc
	fileName, err := findDocByMemID(c, vectorScope, rArgs.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Get original doc metadata
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

	// Read original file content to get full record
	readResp, err := c.POST("/vector/read", map[string]interface{}{
		"scope":     vectorScope,
		"file_name": fileName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		return 1
	}
	var readData struct {
		Content string `json:"content"`
	}
	json.Unmarshal(readResp, &readData)

	// Deprecate old version
	c.POST("/vector/update_metadata", map[string]interface{}{
		"scope":    vectorScope,
		"doc_id":   fileName,
		"metadata": map[string]interface{}{"mem_status": "deprecated"},
	})

	// Build new record from old metadata
	oldVersion := int(toFloat(doc.Metadata["mem_version"]))
	newVersion := oldVersion + 1

	title := metaStr(doc.Metadata, "mem_title")
	if rArgs.NewTitle != "" {
		title = rArgs.NewTitle
	}

	memType := metaStr(doc.Metadata, "mem_type")
	content := rArgs.NewContent
	if content == nil {
		// Parse old content from metadata
		contentStr := metaStr(doc.Metadata, "mem_content")
		if contentStr != "" {
			json.Unmarshal([]byte(contentStr), &content)
		}
		if content == nil {
			content = map[string]interface{}{}
		}
	}

	tags := rArgs.NewTags
	if tags == nil {
		tagsStr := metaStr(doc.Metadata, "mem_tags")
		if tagsStr != "" {
			json.Unmarshal([]byte(tagsStr), &tags)
		}
	}

	now := NowISO()
	newRecord := &MemoryRecord{
		ID:        GenerateID(),
		Type:      memType,
		Scope:     metaStr(doc.Metadata, "mem_scope"),
		Title:     title,
		Tags:      tags,
		Status:    "active",
		Risk:      metaStr(doc.Metadata, "mem_risk"),
		Content:   content,
		Evidence:  []Evidence{{Kind: "revision", Ref: "revised from " + rArgs.ID}},
		Stats:     Stats{},
		Version:   newVersion,
		ParentID:  rArgs.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	markdown := RenderMarkdown(newRecord)
	newFileName := sanitizeFileName(title) + ".md"

	extraMeta := map[string]interface{}{
		"mem_id":        newRecord.ID,
		"mem_type":      newRecord.Type,
		"mem_scope":     newRecord.Scope,
		"mem_status":    "active",
		"mem_risk":      newRecord.Risk,
		"mem_version":   newVersion,
		"mem_tags":      TagsToJSON(tags),
		"mem_evidence":  EvidenceToJSON(newRecord.Evidence),
		"mem_content":   ContentToJSON(content),
		"mem_title":     title,
		"mem_parent_id": rArgs.ID,
	}

	_, err = c.POST("/vector/write", map[string]interface{}{
		"file_name":      newFileName,
		"content":        markdown,
		"scope":          vectorScope,
		"extra_metadata": extraMeta,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing new version: %v\n", err)
		return 1
	}

	output := map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"new_id":      newRecord.ID,
			"old_id":      rArgs.ID,
			"version":     newVersion,
			"file_name":   newFileName,
			"old_status":  "deprecated",
			"new_status":  "active",
		},
	}
	outJSON, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(outJSON))
	return 0
}

// DeprecateArgs is the input schema for mem deprecate
type DeprecateArgs struct {
	ID string `json:"id"`
}

// RunDeprecateImpl executes the mem deprecate command
func RunDeprecateImpl(c *client.Client, group string, args []string) int {
	var dArgs DeprecateArgs

	input, err := readInput(args)
	if err == nil {
		json.Unmarshal(input, &dArgs)
	} else {
		for i := 0; i < len(args); i++ {
			if args[i] == "--id" && i+1 < len(args) {
				i++
				dArgs.ID = args[i]
			}
		}
	}

	if dArgs.ID == "" {
		fmt.Fprintf(os.Stderr, "Error: --id is required\n")
		return 1
	}

	vectorScope := "memory"
	if group != "" {
		vectorScope = group + "/memory"
	}

	fileName, err := findDocByMemID(c, vectorScope, dArgs.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	_, err = c.POST("/vector/update_metadata", map[string]interface{}{
		"scope":    vectorScope,
		"doc_id":   fileName,
		"metadata": map[string]interface{}{"mem_status": "deprecated"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	output := map[string]interface{}{
		"ok":   true,
		"data": map[string]interface{}{"id": dArgs.ID, "status": "deprecated"},
	}
	outJSON, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(outJSON))
	return 0
}
