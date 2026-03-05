package mem

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MemoryRecord is the core structured memory entity
type MemoryRecord struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Scope     string                 `json:"scope"`
	Title     string                 `json:"title"`
	Tags      []string               `json:"tags"`
	Status    string                 `json:"status"`
	Risk      string                 `json:"risk,omitempty"`
	Content   map[string]interface{} `json:"content"`
	Evidence  []Evidence             `json:"evidence"`
	Stats     Stats                  `json:"stats"`
	Version   int                    `json:"version"`
	ParentID  string                 `json:"parent_id,omitempty"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
}

type Evidence struct {
	Kind string `json:"kind"` // user|tool|file|log
	Ref  string `json:"ref"`
	Ts   string `json:"ts,omitempty"`
}

type Stats struct {
	Impressions int    `json:"impressions"`
	Uses        int    `json:"uses"`
	Success     int    `json:"success"`
	Fail        int    `json:"fail"`
	LastUsedAt  string `json:"last_used_at,omitempty"`
}

// Valid memory types
var ValidTypes = []string{
	"env_fact", "preference", "procedure",
	"diagnostic", "anti_pattern", "decision",
}

// ValidStatuses for memory records
var ValidStatuses = []string{"active", "deprecated", "invalid"}

// ValidRisks for memory records
var ValidRisks = []string{"low", "medium", "high"}

// GenerateID creates a unique memory ID
func GenerateID() string {
	now := time.Now()
	return fmt.Sprintf("mem_%s_%04d", now.Format("20060102"), now.UnixMilli()%10000)
}

// NowISO returns current time in ISO8601 format
func NowISO() string {
	return time.Now().Format(time.RFC3339)
}

// IsValidType checks if a type is valid
func IsValidType(t string) bool {
	for _, v := range ValidTypes {
		if v == t {
			return true
		}
	}
	return false
}

// RenderMarkdown converts a MemoryRecord to readable Markdown
func RenderMarkdown(r *MemoryRecord) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# %s\n\n", r.Title))
	b.WriteString(fmt.Sprintf("> Type: `%s` | Status: `%s` | Version: v%d\n", r.Type, r.Status, r.Version))
	if r.Risk != "" {
		b.WriteString(fmt.Sprintf("> Risk: `%s`\n", r.Risk))
	}
	if len(r.Tags) > 0 {
		b.WriteString(fmt.Sprintf("> Tags: %s\n", strings.Join(r.Tags, ", ")))
	}
	b.WriteString(fmt.Sprintf("> ID: `%s`\n", r.ID))
	b.WriteString("\n")

	// Render content based on type
	switch r.Type {
	case "env_fact":
		renderEnvFact(&b, r.Content)
	case "preference":
		renderPreference(&b, r.Content)
	case "procedure":
		renderProcedure(&b, r.Content)
	case "diagnostic":
		renderDiagnostic(&b, r.Content)
	case "anti_pattern":
		renderAntiPattern(&b, r.Content)
	case "decision":
		renderDecision(&b, r.Content)
	default:
		// Fallback: dump as JSON
		data, _ := json.MarshalIndent(r.Content, "", "  ")
		b.WriteString("## Content\n\n```json\n")
		b.Write(data)
		b.WriteString("\n```\n\n")
	}

	// Evidence
	if len(r.Evidence) > 0 {
		b.WriteString("## Evidence\n\n")
		for _, e := range r.Evidence {
			ts := ""
			if e.Ts != "" {
				ts = fmt.Sprintf(" (%s)", e.Ts)
			}
			b.WriteString(fmt.Sprintf("- [%s] %s%s\n", e.Kind, e.Ref, ts))
		}
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(fmt.Sprintf("---\nCreated: %s | Updated: %s\n", r.CreatedAt, r.UpdatedAt))

	return b.String()
}

func str(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func renderEnvFact(b *strings.Builder, c map[string]interface{}) {
	b.WriteString("## Fact\n\n")
	b.WriteString(fmt.Sprintf("- **Key**: `%s`\n", str(c, "key")))
	b.WriteString(fmt.Sprintf("- **Value**: `%s`\n", str(c, "value")))
	if note := str(c, "note"); note != "" {
		b.WriteString(fmt.Sprintf("- **Note**: %s\n", note))
	}
	b.WriteString("\n")
}

func renderPreference(b *strings.Builder, c map[string]interface{}) {
	b.WriteString("## Preference\n\n")
	b.WriteString(fmt.Sprintf("- **Key**: `%s`\n", str(c, "key")))
	b.WriteString(fmt.Sprintf("- **Value**: %s\n", str(c, "value")))
	if p := str(c, "priority"); p != "" {
		b.WriteString(fmt.Sprintf("- **Priority**: %s\n", p))
	}
	b.WriteString("\n")
}

func renderProcedure(b *strings.Builder, c map[string]interface{}) {
	if summary := str(c, "summary"); summary != "" {
		b.WriteString(fmt.Sprintf("## Summary\n\n%s\n\n", summary))
	}
	if steps, ok := c["steps"].([]interface{}); ok && len(steps) > 0 {
		b.WriteString("## Steps\n\n")
		for i, s := range steps {
			if step, ok := s.(map[string]interface{}); ok {
				b.WriteString(fmt.Sprintf("%d. **Do**: %s\n", i+1, str(step, "do")))
				if expect := str(step, "expect"); expect != "" {
					b.WriteString(fmt.Sprintf("   **Expect**: %s\n", expect))
				}
			}
		}
		b.WriteString("\n")
	}
}

func renderDiagnostic(b *strings.Builder, c map[string]interface{}) {
	if summary := str(c, "summary"); summary != "" {
		b.WriteString(fmt.Sprintf("## Summary\n\n%s\n\n", summary))
	}
	if checks, ok := c["checks"].([]interface{}); ok && len(checks) > 0 {
		b.WriteString("## Checks\n\n")
		for i, ch := range checks {
			if check, ok := ch.(map[string]interface{}); ok {
				b.WriteString(fmt.Sprintf("%d. **Check**: %s\n", i+1, str(check, "check")))
				b.WriteString(fmt.Sprintf("   - Yes → %s\n", str(check, "if_yes")))
				b.WriteString(fmt.Sprintf("   - No → %s\n", str(check, "if_no")))
			}
		}
		b.WriteString("\n")
	}
}

func renderAntiPattern(b *strings.Builder, c map[string]interface{}) {
	b.WriteString("## Anti-Pattern\n\n")
	b.WriteString(fmt.Sprintf("- **Pattern**: %s\n", str(c, "pattern")))
	b.WriteString(fmt.Sprintf("- **Symptom**: %s\n", str(c, "symptom")))
	b.WriteString(fmt.Sprintf("- **Cause**: %s\n", str(c, "cause")))
	b.WriteString(fmt.Sprintf("- **Fix**: %s\n", str(c, "fix")))
	b.WriteString("\n")
}

func renderDecision(b *strings.Builder, c map[string]interface{}) {
	if summary := str(c, "summary"); summary != "" {
		b.WriteString(fmt.Sprintf("## Summary\n\n%s\n\n", summary))
	}
	if rationale, ok := c["rationale"].([]interface{}); ok && len(rationale) > 0 {
		b.WriteString("## Rationale\n\n")
		for _, r := range rationale {
			b.WriteString(fmt.Sprintf("- %v\n", r))
		}
		b.WriteString("\n")
	}
	if impact, ok := c["impact"].([]interface{}); ok && len(impact) > 0 {
		b.WriteString("## Impact\n\n")
		for _, i := range impact {
			b.WriteString(fmt.Sprintf("- %v\n", i))
		}
		b.WriteString("\n")
	}
}

// TagsToJSON serializes tags array to JSON string for ChromaDB metadata
func TagsToJSON(tags []string) string {
	data, _ := json.Marshal(tags)
	return string(data)
}

// EvidenceToJSON serializes evidence array to JSON string for ChromaDB metadata
func EvidenceToJSON(evidence []Evidence) string {
	data, _ := json.Marshal(evidence)
	return string(data)
}

// ContentToJSON serializes content map to JSON string for ChromaDB metadata
func ContentToJSON(content map[string]interface{}) string {
	data, _ := json.Marshal(content)
	return string(data)
}
