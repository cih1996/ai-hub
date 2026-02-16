package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// TemplateVars returns all available template variables with current values.
func TemplateVars() map[string]string {
	home, _ := os.UserHomeDir()
	claudeBase := filepath.Join(home, ".claude")
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now()

	return map[string]string{
		"HOME_DIR":      home,
		"CLAUDE_DIR":    claudeBase,
		"MEMORY_DIR":    filepath.Join(claudeBase, "memory"),
		"KNOWLEDGE_DIR": filepath.Join(claudeBase, "knowledge"),
		"RULES_DIR":     filepath.Join(claudeBase, "rules"),
		"OS":            runtime.GOOS,
		"DATE":          now.Format("2006-01-02"),
		"DATETIME":      now.Format("2006-01-02 15:04:05"),
		"TIME_BEIJING":  now.In(loc).Format("2006-01-02 15:04:05"),
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

// InitTemplates sets the template storage directory.
func InitTemplates(dataDir string) {
	templateDir = filepath.Join(dataDir, "templates")
	os.MkdirAll(templateDir, 0755)
}

// TemplateDir returns the base template directory.
func TemplateDir() string {
	return templateDir
}

// RenderAllTemplates renders all template files to ~/.claude/.
func RenderAllTemplates() error {
	home, _ := os.UserHomeDir()
	claudeBase := filepath.Join(home, ".claude")

	return filepath.Walk(templateDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		rel, _ := filepath.Rel(templateDir, path)
		target := filepath.Join(claudeBase, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rendered := RenderTemplate(string(data))
		os.MkdirAll(filepath.Dir(target), 0755)
		return os.WriteFile(target, []byte(rendered), 0644)
	})
}
