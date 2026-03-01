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

// BuildTeamRules reads all *.md files from ~/.ai-hub/<groupName>/rules/ (flat),
// concatenates them sorted by name, renders template variables, and returns
// the combined content. Returns "" when groupName is empty or no files found.
func BuildTeamRules(groupName string) string {
	if groupName == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	teamRulesDir := filepath.Join(home, ".ai-hub", groupName, "rules")
	entries, err := os.ReadDir(teamRulesDir)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	var parts []string
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join(teamRulesDir, name)); err == nil {
			parts = append(parts, string(data))
		}
	}
	combined := strings.Join(parts, "\n\n---\n\n")
	return RenderTemplate(combined)
}

// BuildSystemPrompt reads all *.md files from ~/.ai-hub/rules/ (flat),
// concatenates them sorted by name, appends Skills summary, renders variables,
// and returns the full system prompt.
func BuildSystemPrompt() string {
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	var parts []string
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join(templateDir, name)); err == nil {
			parts = append(parts, string(data))
		}
	}

	// Append Skills summary
	if summary := buildSkillsSummary(); summary != "" {
		parts = append(parts, summary)
	}

	combined := strings.Join(parts, "\n\n---\n\n")
	return RenderTemplate(combined)
}

// buildSkillsSummary scans ~/.ai-hub/skills/*/SKILL.md, extracts YAML
// frontmatter (name + description), and returns a summary block.
func buildSkillsSummary() string {
	home, _ := os.UserHomeDir()
	skillsDir := filepath.Join(home, ".ai-hub", "skills")
	dirs, err := os.ReadDir(skillsDir)
	if err != nil {
		return ""
	}
	var lines []string
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		path := filepath.Join(skillsDir, d.Name(), "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name, desc := parseSkillYAML(string(data))
		if name == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s（%s）：%s", name, d.Name(), desc))
	}
	if len(lines) == 0 {
		return ""
	}
	header := "可用技能（触发后 Read ~/.ai-hub/skills/<目录名>/SKILL.md 获取完整操作手册）："
	return header + "\n" + strings.Join(lines, "\n")
}

// parseSkillYAML extracts name and description from YAML frontmatter.
func parseSkillYAML(content string) (string, string) {
	if !strings.HasPrefix(content, "---") {
		return "", ""
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return "", ""
	}
	header := content[3 : 3+end]
	var name, desc string
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			name = strings.Trim(strings.TrimPrefix(line, "name:"), " \"")
		} else if strings.HasPrefix(line, "description:") {
			desc = strings.Trim(strings.TrimPrefix(line, "description:"), " \"")
		}
	}
	return name, desc
}

// RenderAllTemplates is kept for backward compatibility but now is a no-op.
// System prompt is built on-the-fly via BuildSystemPrompt().
func RenderAllTemplates() error {
	return nil
}
