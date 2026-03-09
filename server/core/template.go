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
	aiHubBase := GetDataDir()
	bjLoc := time.FixedZone("CST", 8*3600)
	now := time.Now()

	return map[string]string{
		"HOME_DIR":      home,
		"CLAUDE_DIR":    aiHubBase,
		"MEMORY_DIR":    filepath.Join(aiHubBase, "memory"),
		"RULES_DIR":     filepath.Join(aiHubBase, "rules"),
		"OS":            runtime.GOOS,
		"PORT":          hubPort,
		"DATE":          now.Format("2006-01-02"),
		"DATETIME":      now.Format("2006-01-02 15:04:05"),
		"TIME_BEIJING":  now.In(bjLoc).Format("2006-01-02 15:04:05"),
		// Runtime-injected vars (populated per session at stream build time).
		"AI_HUB_SESSION_ID":           "",
		"AI_HUB_PORT":                 hubPort,
		"AI_HUB_GROUP_NAME":           "",
		"AI_HUB_SESSION_MESSAGES_API": "",
	}
}

// TemplateVarsWithExtra returns template vars merged with extra runtime variables.
// Extra keys override defaults when duplicated.
func TemplateVarsWithExtra(extra map[string]string) map[string]string {
	vars := TemplateVars()
	for k, v := range extra {
		if strings.TrimSpace(k) == "" {
			continue
		}
		vars[k] = v
	}
	return vars
}

// RenderTemplate replaces {{VAR}} placeholders in content with actual values.
func RenderTemplate(content string) string {
	return RenderTemplateWithVars(content, nil)
}

// RenderTemplateWithVars replaces {{VAR}} placeholders in content with actual values,
// merged with optional extra runtime variables.
func RenderTemplateWithVars(content string, extra map[string]string) string {
	vars := TemplateVarsWithExtra(extra)
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

// ScopeDir returns the filesystem directory for the given vector scope.
// Global scopes (e.g. "memory") resolve to <data-dir>/memory.
// Team scopes (e.g. "团队名/memory") resolve to <data-dir>/teams/团队名/memory.
// Session scopes (e.g. "团队名/sessions/21/memory") resolve to <data-dir>/teams/团队名/sessions/21/memory.
func ScopeDir(scope string) string {
	baseDir := GetDataDir()
	parts := strings.Split(scope, "/")
	// Session-level: <group>/sessions/<id>/<suffix>
	if len(parts) == 4 && parts[1] == "sessions" {
		return filepath.Join(baseDir, "teams", parts[0], "sessions", parts[2], parts[3])
	}
	// Team-level: <group>/<suffix>
	if len(parts) == 2 {
		return filepath.Join(baseDir, "teams", parts[0], parts[1])
	}
	// Global
	return filepath.Join(baseDir, scope)
}

// TeamDir returns the base directory for a team's resources.
func TeamDir(groupName string) string {
	return filepath.Join(GetDataDir(), "teams", groupName)
}

// BuildTeamRules reads all *.md files from ~/.ai-hub/teams/<groupName>/rules/ (flat),
// concatenates them sorted by name, renders template variables, and returns
// the combined content. Returns "" when groupName is empty or no files found.
func BuildTeamRules(groupName string) string {
	return BuildTeamRulesWithVars(groupName, nil)
}

// BuildTeamRulesWithVars reads all *.md files from ~/.ai-hub/teams/<groupName>/rules/ (flat),
// concatenates them sorted by name, renders template variables + extra vars, and returns
// the combined content. Returns "" when groupName is empty or no files found.
func BuildTeamRulesWithVars(groupName string, extra map[string]string) string {
	if groupName == "" {
		return ""
	}
	teamRulesDir := filepath.Join(TeamDir(groupName), "rules")
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
	return RenderTemplateWithVars(combined, extra)
}

// BuildSystemPrompt reads all *.md files from ~/.ai-hub/rules/ (flat),
// concatenates them sorted by name, appends Skills summary, renders variables,
// and returns the full system prompt.
func BuildSystemPrompt() string {
	return BuildSystemPromptWithVars(nil)
}

// BuildSystemPromptWithVars reads all *.md files from ~/.ai-hub/rules/ (flat),
// concatenates them sorted by name, appends Skills summary, renders template variables
// with optional extra vars, and returns the full system prompt.
func BuildSystemPromptWithVars(extra map[string]string) string {
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
	return RenderTemplateWithVars(combined, extra)
}

// buildSkillsSummary scans <data-dir>/skills/*/SKILL.md, extracts YAML
// frontmatter (name + description), and returns a summary block.
// ai-hub-core's CLI usage is already injected via CLI-REFERENCE.md in rules/,
// so it does not prompt "Read SKILL.md". Other skills still prompt Read.
func buildSkillsSummary() string {
	skillsDir := filepath.Join(GetDataDir(), "skills")
	dirs, err := os.ReadDir(skillsDir)
	if err != nil {
		return ""
	}
	var coreLines []string
	var otherLines []string
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
		if d.Name() == "ai-hub-core" {
			coreLines = append(coreLines, fmt.Sprintf("- %s（%s）：%s", name, d.Name(), desc))
		} else {
			otherLines = append(otherLines, fmt.Sprintf("- %s（%s）：%s", name, d.Name(), desc))
		}
	}
	allLines := append(coreLines, otherLines...)
	if len(allLines) == 0 {
		return ""
	}
	var sb strings.Builder
	if len(coreLines) > 0 {
		sb.WriteString("可用技能：\n")
		sb.WriteString(strings.Join(coreLines, "\n"))
	}
	if len(otherLines) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("可用技能（触发后 Read ~/.ai-hub/skills/<目录名>/SKILL.md 获取完整操作手册）：\n")
		sb.WriteString(strings.Join(otherLines, "\n"))
	}
	return sb.String()
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
