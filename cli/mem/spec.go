package mem

import (
	"encoding/json"
	"fmt"
	"os"
)

// RunSpecImpl outputs JSON Schema for a subcommand
func RunSpecImpl(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: ai-hub mem spec <command>\nCommands: add, retrieve, feedback, revise, deprecate\n")
		return 1
	}

	cmd := args[0]
	schema, ok := specSchemas[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nAvailable: add, retrieve, feedback, revise, deprecate\n", cmd)
		return 1
	}

	outJSON, _ := json.MarshalIndent(schema, "", "  ")
	fmt.Println(string(outJSON))
	return 0
}

var specSchemas = map[string]interface{}{
	"add": map[string]interface{}{
		"command":     "mem.add",
		"description": "Write a new structured memory record",
		"args_schema": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"type", "title", "content", "evidence"},
			"properties": map[string]interface{}{
				"type": map[string]interface{}{
					"type": "string",
					"enum": ValidTypes,
				},
				"title": map[string]interface{}{
					"type":      "string",
					"maxLength": 80,
				},
				"scope": map[string]interface{}{
					"type":    "string",
					"enum":    []string{"session", "workspace", "tenant", "global"},
					"default": "workspace",
				},
				"tags": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
				"risk": map[string]interface{}{
					"type":    "string",
					"enum":    ValidRisks,
					"default": "low",
				},
				"content": map[string]interface{}{
					"type":        "object",
					"description": "Content structure depends on type. Use mem spec add for details.",
				},
				"evidence": map[string]interface{}{
					"type":     "array",
					"minItems": 1,
					"items": map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"kind", "ref"},
						"properties": map[string]interface{}{
							"kind": map[string]interface{}{"type": "string", "enum": []string{"user", "tool", "file", "log"}},
							"ref":  map[string]interface{}{"type": "string"},
							"ts":   map[string]interface{}{"type": "string", "format": "date-time"},
						},
					},
				},
			},
		},
		"content_schemas": map[string]interface{}{
			"env_fact":     map[string]interface{}{"required": []string{"key", "value"}, "properties": map[string]interface{}{"key": "string", "value": "string", "note": "string (optional, <=120)"}},
			"preference":   map[string]interface{}{"required": []string{"key", "value"}, "properties": map[string]interface{}{"key": "string", "value": "string", "priority": "int (optional)"}},
			"procedure":    map[string]interface{}{"required": []string{"summary", "steps"}, "properties": map[string]interface{}{"summary": "string (<=200)", "steps": "[{do, expect}]", "prechecks": "[string]", "rollback": "[string]"}},
			"diagnostic":   map[string]interface{}{"required": []string{"summary", "checks"}, "properties": map[string]interface{}{"summary": "string (<=200)", "checks": "[{check, if_yes, if_no}]"}},
			"anti_pattern": map[string]interface{}{"required": []string{"pattern", "symptom", "cause", "fix"}, "properties": map[string]interface{}{"pattern": "string (<=120)", "symptom": "string (<=120)", "cause": "string (<=120)", "fix": "string (<=120)"}},
			"decision":     map[string]interface{}{"required": []string{"summary"}, "properties": map[string]interface{}{"summary": "string (<=200)", "rationale": "[string]", "impact": "[string]"}},
		},
	},
	"retrieve": map[string]interface{}{
		"command":     "mem.retrieve",
		"description": "Search memories with semantic + statistical reranking",
		"args_schema": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"query"},
			"properties": map[string]interface{}{
				"query":  map[string]interface{}{"type": "string"},
				"k":      map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 20, "default": 8},
				"types":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string", "enum": ValidTypes}},
				"status": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string", "enum": ValidStatuses}, "default": []string{"active"}},
			},
		},
	},
	"feedback": map[string]interface{}{
		"command":     "mem.feedback",
		"description": "Report success/fail for a memory record",
		"args_schema": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"id", "result"},
			"properties": map[string]interface{}{
				"id":     map[string]interface{}{"type": "string"},
				"result": map[string]interface{}{"type": "string", "enum": []string{"success", "fail"}},
			},
		},
	},
	"revise": map[string]interface{}{
		"command":     "mem.revise",
		"description": "Create a new version of an existing memory (old version deprecated)",
		"args_schema": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"id"},
			"properties": map[string]interface{}{
				"id":          map[string]interface{}{"type": "string"},
				"new_title":   map[string]interface{}{"type": "string", "maxLength": 80},
				"new_content": map[string]interface{}{"type": "object"},
				"new_tags":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			},
		},
	},
	"deprecate": map[string]interface{}{
		"command":     "mem.deprecate",
		"description": "Mark a memory as deprecated (not deleted, just deprioritized)",
		"args_schema": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"id"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{"type": "string"},
			},
		},
	},
}
