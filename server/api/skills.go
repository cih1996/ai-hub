package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

var dataDir string

func InitDataDir(dir string) {
	dataDir = dir
}

type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	Enabled     bool   `json:"enabled"`
}

type ToggleSkillRequest struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Enable bool   `json:"enable"`
}

func parseSkillFrontmatter(path string) (name, desc string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return "", ""
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return "", ""
	}
	fm := content[3 : 3+end]
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, "\"'")
		} else if strings.HasPrefix(line, "description:") {
			desc = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			desc = strings.Trim(desc, "\"'")
		}
	}
	return
}

func disabledSkillPath(name, source string) string {
	base := filepath.Join(dataDir, "disabled", "skills", source)
	return filepath.Join(base, name)
}

func disabledCommandPath(name string) string {
	return filepath.Join(dataDir, "disabled", "commands", name+".md")
}

func isSkillDisabled(name, source string) bool {
	if source == "command" {
		_, err := os.Stat(disabledCommandPath(name))
		return err == nil
	}
	_, err := os.Stat(disabledSkillPath(name, source))
	return err == nil
}

func scanUserSkills() []SkillInfo {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".claude", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var skills []SkillInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}
		name, desc := parseSkillFrontmatter(skillFile)
		if name == "" {
			name = e.Name()
		}
		skills = append(skills, SkillInfo{
			Name:        name,
			Description: desc,
			Source:      "user",
			Path:        skillFile,
			Enabled:     !isSkillDisabled(e.Name(), "user"),
		})
	}
	return skills
}

func scanPluginSkills() []SkillInfo {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".claude", "plugins", "marketplaces")
	marketplaces, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var skills []SkillInfo
	for _, m := range marketplaces {
		if !m.IsDir() {
			continue
		}
		pluginsDir := filepath.Join(base, m.Name(), "plugins")
		plugins, err := os.ReadDir(pluginsDir)
		if err != nil {
			continue
		}
		for _, p := range plugins {
			if !p.IsDir() {
				continue
			}
			skillsDir := filepath.Join(pluginsDir, p.Name(), "skills")
			skillEntries, err := os.ReadDir(skillsDir)
			if err != nil {
				continue
			}
			for _, s := range skillEntries {
				if !s.IsDir() {
					continue
				}
				skillFile := filepath.Join(skillsDir, s.Name(), "SKILL.md")
				if _, err := os.Stat(skillFile); err != nil {
					continue
				}
				name, desc := parseSkillFrontmatter(skillFile)
				if name == "" {
					name = s.Name()
				}
				skills = append(skills, SkillInfo{
					Name:        name,
					Description: desc,
					Source:      "plugin",
					Path:        skillFile,
					Enabled:     !isSkillDisabled(s.Name(), "plugin"),
				})
			}
		}
	}
	return skills
}

func scanCommands() []SkillInfo {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".claude", "commands")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var skills []SkillInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		skills = append(skills, SkillInfo{
			Name:        name,
			Description: "斜杠命令 /" + name,
			Source:      "command",
			Path:        filepath.Join(dir, e.Name()),
			Enabled:     !isSkillDisabled(name, "command"),
		})
	}
	return skills
}

func ListSkills(c *gin.Context) {
	var all []SkillInfo
	all = append(all, scanUserSkills()...)
	all = append(all, scanPluginSkills()...)
	all = append(all, scanCommands()...)
	if all == nil {
		all = []SkillInfo{}
	}
	c.JSON(http.StatusOK, all)
}

func ToggleSkill(c *gin.Context) {
	var req ToggleSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Source == "command" {
		disPath := disabledCommandPath(req.Name)
		home, _ := os.UserHomeDir()
		origPath := filepath.Join(home, ".claude", "commands", req.Name+".md")
		if req.Enable {
			// Move back from disabled
			if _, err := os.Stat(disPath); err == nil {
				os.MkdirAll(filepath.Dir(origPath), 0755)
				os.Rename(disPath, origPath)
			}
		} else {
			// Move to disabled
			if _, err := os.Stat(origPath); err == nil {
				os.MkdirAll(filepath.Dir(disPath), 0755)
				os.Rename(origPath, disPath)
			}
		}
	} else {
		disPath := disabledSkillPath(req.Name, req.Source)
		// Find original path
		home, _ := os.UserHomeDir()
		var origDir string
		if req.Source == "user" {
			origDir = filepath.Join(home, ".claude", "skills", req.Name)
		} else {
			origDir = findPluginSkillDir(req.Name)
		}
		if req.Enable {
			if _, err := os.Stat(disPath); err == nil && origDir != "" {
				os.MkdirAll(filepath.Dir(origDir), 0755)
				os.Rename(disPath, origDir)
			}
		} else {
			if origDir != "" {
				if _, err := os.Stat(origDir); err == nil {
					os.MkdirAll(filepath.Dir(disPath), 0755)
					os.Rename(origDir, disPath)
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func findPluginSkillDir(name string) string {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".claude", "plugins", "marketplaces")
	marketplaces, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, m := range marketplaces {
		if !m.IsDir() {
			continue
		}
		pluginsDir := filepath.Join(base, m.Name(), "plugins")
		plugins, _ := os.ReadDir(pluginsDir)
		for _, p := range plugins {
			if !p.IsDir() {
				continue
			}
			skillDir := filepath.Join(pluginsDir, p.Name(), "skills", name)
			if _, err := os.Stat(skillDir); err == nil {
				return skillDir
			}
		}
	}
	return ""
}



