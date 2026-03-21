package commands

import (
	"ai-hub/cli/client"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// RunSchemas executes the schemas command
func RunSchemas(c *client.Client, args []string) int {
	if len(args) == 0 {
		return schemasList(c)
	}

	switch args[0] {
	case "list":
		return schemasList(c)
	case "get":
		return schemasGet(c, args[1:])
	case "create":
		return schemasCreate(c, args[1:])
	case "delete":
		return schemasDelete(c, args[1:])
	case "--help":
		printSchemasHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown schemas subcommand: %s\n", args[0])
		printSchemasHelp()
		return 1
	}
}

func schemasList(c *client.Client) int {
	respData, err := c.GET("/schemas")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var schemas []struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Definition string `json:"definition"`
		CreatedAt  string `json:"created_at"`
		UpdatedAt  string `json:"updated_at"`
	}
	if err := json.Unmarshal(respData, &schemas); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	if len(schemas) == 0 {
		fmt.Println("No schemas found.")
		return 0
	}

	fmt.Printf("%d schemas:\n\n", len(schemas))
	for _, s := range schemas {
		// Try to extract description or type from definition
		preview := s.Definition
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		fmt.Printf("  %s\n    %s\n    created: %s\n\n", s.Name, preview, FormatTime(s.CreatedAt))
	}
	return 0
}

func schemasGet(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub schemas get <name>\n")
		return 1
	}

	name := args[0]
	respData, err := c.GET(fmt.Sprintf("/schemas/%s", name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	var schema struct {
		Name       string `json:"name"`
		Definition string `json:"definition"`
		CreatedAt  string `json:"created_at"`
		UpdatedAt  string `json:"updated_at"`
	}
	if err := json.Unmarshal(respData, &schema); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		return 1
	}

	// Pretty-print the definition JSON
	var raw interface{}
	if err := json.Unmarshal([]byte(schema.Definition), &raw); err == nil {
		pretty, err := json.MarshalIndent(raw, "", "  ")
		if err == nil {
			fmt.Printf("Schema: %s\nCreated: %s\nUpdated: %s\n\nDefinition:\n%s\n",
				schema.Name, FormatTime(schema.CreatedAt), FormatTime(schema.UpdatedAt), string(pretty))
			return 0
		}
	}

	// Fallback: print raw
	fmt.Printf("Schema: %s\nCreated: %s\n\nDefinition:\n%s\n",
		schema.Name, FormatTime(schema.CreatedAt), schema.Definition)
	return 0
}

func schemasCreate(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub schemas create <name> --definition '<json>'\n")
		printSchemasHelp()
		return 1
	}

	name := args[0]
	var definition string

	// Parse flags
	for i := 1; i < len(args); i++ {
		if (args[i] == "--definition" || args[i] == "--def") && i+1 < len(args) {
			i++
			definition = args[i]
		}
	}

	// Read from stdin if not provided via flag
	if definition == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				return 1
			}
			definition = strings.TrimRight(string(data), "\n")
		}
	}

	if definition == "" {
		fmt.Fprintf(os.Stderr, "Error: --definition is required (or pipe JSON via stdin)\n")
		return 1
	}

	// Validate JSON
	var parsed json.RawMessage
	if err := json.Unmarshal([]byte(definition), &parsed); err != nil {
		fmt.Fprintf(os.Stderr, "Error: definition must be valid JSON: %v\n", err)
		return 1
	}

	body := map[string]interface{}{
		"name":       name,
		"definition": parsed,
	}
	_, err := c.POST("/schemas", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Schema '%s' created.\n", name)
	return 0
}

func schemasDelete(c *client.Client, args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub schemas delete <name>\n")
		return 1
	}

	name := args[0]
	_, err := c.DELETE(fmt.Sprintf("/schemas/%s", name))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	fmt.Printf("Schema '%s' deleted.\n", name)
	return 0
}

func printSchemasHelp() {
	fmt.Fprintf(os.Stderr, `Usage: ai-hub schemas [subcommand] [args]

Manage JSON Schema definitions for structured memory validation.

Subcommands:
  list                                   List all schemas (default)
  get <name>                             Show schema definition
  create <name> --definition '<json>'    Create a new schema
  delete <name>                          Delete a schema

Create also accepts definition from stdin:
  cat schema.json | ai-hub schemas create my-schema

Examples:
  ai-hub schemas list
  ai-hub schemas get my-schema
  ai-hub schemas create my-schema --definition '{"type":"object","required":["title"],"properties":{"title":{"type":"string"}}}'
  ai-hub schemas delete my-schema
`)
}
