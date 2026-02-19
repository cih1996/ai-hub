package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// TemplateVars returns all available template variables with current values.
func TemplateVars() map[string]string {
	home, _ := os.UserHomeDir()
	aiHubBase := filepath.Join(home, ".ai-hub")
	bjLoc := time.FixedZone("CST", 8*3600)
	now := time.Now()

	return map[string]string{
		"HOME_DIR":      home,
		"CLAUDE_DIR":    aiHubBase,
		"MEMORY_DIR":    filepath.Join(aiHubBase, "memory"),
		"KNOWLEDGE_DIR": filepath.Join(aiHubBase, "knowledge"),
		"RULES_DIR":     filepath.Join(aiHubBase, "rules"),
		"OS":            runtime.GOOS,
		"PORT":          hubPort,
		"DATE":          now.Format("2006-01-02"),
		"DATETIME":      now.Format("2006-01-02 15:04:05"),
		"TIME_BEIJING":  now.In(bjLoc).Format("2006-01-02 15:04:05"),
	}
}

// RenderTemplate replaces {{VAR}} placeholders in content with actual values.
func RenderTemplate(content string) string {
	vars := TemplateVars()
	for k, v := range vars {
		content = strings.ReplaceAll(content, "{{"+k+"}}", v)
	}
	return content
}

var templateDir string
var hubPort string

// InitTemplates sets the rules storage directory (was templates/).
func InitTemplates(dataDir string) {
	templateDir = filepath.Join(dataDir, "rules")
	os.MkdirAll(templateDir, 0755)
}

// SetPort stores the server port for template rendering and env injection.
func SetPort(port int) {
	hubPort = fmt.Sprintf("%d", port)
}

// GetPort returns the stored server port string.
func GetPort() string {
	return hubPort
}

// TemplateDir returns the base rules directory.
func TemplateDir() string {
	return templateDir
}

// BuildSystemPrompt reads all rule files from ~/.ai-hub/rules/,
// concatenates and renders them, returning the full system prompt.
func BuildSystemPrompt() string {
	var parts []string

	// 1. Main rule: CLAUDE.md
	mainRule := filepath.Join(templateDir, "CLAUDE.md")
	if data, err := os.ReadFile(mainRule); err == nil {
		parts = append(parts, string(data))
	}

	// 2. Sub-rules: rules/*.md (sorted by name)
	rulesDir := filepath.Join(templateDir, "rules")
	if entries, err := os.ReadDir(rulesDir); err == nil {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			if data, err := os.ReadFile(filepath.Join(rulesDir, name)); err == nil {
				parts = append(parts, string(data))
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	combined := strings.Join(parts, "\n\n---\n\n")
	return RenderTemplate(combined)
}

// RenderAllTemplates is kept for backward compatibility but now is a no-op.
// System prompt is built on-the-fly via BuildSystemPrompt().
func RenderAllTemplates() error {
	return nil
}
