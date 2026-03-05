package mem

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// AddArgs is the input schema for mem add
type AddArgs struct {
	Type     string                 `json:"type"`
	Scope    string                 `json:"scope,omitempty"`
	Title    string                 `json:"title"`
	Tags     []string               `json:"tags,omitempty"`
	Risk     string                 `json:"risk,omitempty"`
	Content  map[string]interface{} `json:"content"`
	Evidence []Evidence             `json:"evidence"`
}

// RunAdd executes the mem add command
func RunAdd(c *client.Client, group string, args []string) int {
	// Read JSON from stdin or --in flag
	input, err := readInput(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var addArgs AddArgs
	if err := json.Unmarshal(input, &addArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		return 1
	}

	// Validate
	if !IsValidType(addArgs.Type) {
		fmt.Fprintf(os.Stderr, "Error: invalid type '%s'. Must be one of: %s\n",
			addArgs.Type, strings.Join(ValidTypes, ", "))
		return 1
	}
	if addArgs.Title == "" {
		fmt.Fprintf(os.Stderr, "Error: title is required\n")
		return 1
	}
	if len(addArgs.Title) > 80 {
		fmt.Fprintf(os.Stderr, "Error: title must be <= 80 characters\n")
		return 1
	}
	if addArgs.Content == nil {
		fmt.Fprintf(os.Stderr, "Error: content is required\n")
		return 1
	}
	if len(addArgs.Evidence) == 0 {
		fmt.Fprintf(os.Stderr, "Error: at least one evidence entry is required\n")
		return 1
	}

	// Build MemoryRecord
	now := NowISO()
	record := &MemoryRecord{
		ID:        GenerateID(),
		Type:      addArgs.Type,
		Scope:     addArgs.Scope,
		Title:     addArgs.Title,
		Tags:      addArgs.Tags,
		Status:    "active",
		Risk:      addArgs.Risk,
		Content:   addArgs.Content,
		Evidence:  addArgs.Evidence,
		Stats:     Stats{},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if record.Scope == "" {
		record.Scope = "workspace"
	}
	if record.Risk == "" {
		record.Risk = "low"
	}

	// Render Markdown
	markdown := RenderMarkdown(record)

	// Build filename from title
	fileName := sanitizeFileName(record.Title) + ".md"

	// Determine vector scope
	vectorScope := "memory"
	if group != "" {
		vectorScope = group + "/memory"
	}

	// Build extra_metadata for vector storage
	extraMeta := map[string]interface{}{
		"mem_id":      record.ID,
		"mem_type":    record.Type,
		"mem_scope":   record.Scope,
		"mem_status":  record.Status,
		"mem_risk":    record.Risk,
		"mem_version": record.Version,
		"mem_tags":    TagsToJSON(record.Tags),
		"mem_evidence": EvidenceToJSON(record.Evidence),
		"mem_content":  ContentToJSON(record.Content),
		"mem_title":    record.Title,
		"mem_parent_id": "",
	}

	// Write via API
	reqBody := map[string]interface{}{
		"file_name":      fileName,
		"content":        markdown,
		"scope":          vectorScope,
		"extra_metadata": extraMeta,
	}

	respData, err := c.POST("/vector/write", reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Output response envelope
	output := map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"id":        record.ID,
			"file_name": fileName,
			"scope":     vectorScope,
			"type":      record.Type,
			"version":   record.Version,
			"status":    record.Status,
		},
	}
	_ = respData
	outJSON, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(outJSON))
	return 0
}

// readInput reads JSON from --in flag or stdin
func readInput(args []string) ([]byte, error) {
	for i, arg := range args {
		if arg == "--in" && i+1 < len(args) {
			return []byte(args[i+1]), nil
		}
	}
	// Try stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return io.ReadAll(os.Stdin)
	}
	return nil, fmt.Errorf("JSON input required via stdin or --in flag")
}

// sanitizeFileName converts a title to a safe filename
func sanitizeFileName(title string) string {
	// Replace unsafe chars with hyphens
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "-",
		"?", "-", "\"", "-", "<", "-", ">", "-",
		"|", "-", " ", "-",
	)
	name := replacer.Replace(title)
	// Collapse multiple hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}
