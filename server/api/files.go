package api

import (
	"ai-hub/server/core"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type FileInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type FileContentRequest struct {
	Scope   string `json:"scope"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func claudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func scopeDir(base, scope string) string {
	switch scope {
	case "knowledge":
		return filepath.Join(base, "knowledge")
	case "memory":
		return filepath.Join(base, "memory")
	case "rules":
		return filepath.Join(base, "rules")
	default:
		return ""
	}
}

func validatePath(p string) bool {
	return !strings.Contains(p, "..") && !strings.Contains(p, "~")
}

// resolvePaths returns (templatePath, claudePath, ok).
func resolvePaths(scope, p string) (string, string, bool) {
	if !validatePath(p) {
		return "", "", false
	}
	tplBase := core.TemplateDir()
	clBase := claudeDir()

	if scope == "rules" && p == "CLAUDE.md" {
		return filepath.Join(tplBase, "CLAUDE.md"), filepath.Join(clBase, "CLAUDE.md"), true
	}
	tplFull := filepath.Join(tplBase, p)
	clFull := filepath.Join(clBase, p)
	if !strings.HasPrefix(tplFull, tplBase) || !strings.HasPrefix(clFull, clBase) {
		return "", "", false
	}
	return tplFull, clFull, true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ListFiles(c *gin.Context) {
	scope := c.Query("scope")
	if scope == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope is required"})
		return
	}

	tplBase := core.TemplateDir()
	clBase := claudeDir()
	var files []FileInfo
	seen := map[string]bool{}

	if scope == "rules" {
		// Always include CLAUDE.md (built-in)
		exists := fileExists(filepath.Join(clBase, "CLAUDE.md")) || fileExists(filepath.Join(tplBase, "CLAUDE.md"))
		files = append(files, FileInfo{Name: "CLAUDE.md", Path: "CLAUDE.md", Exists: exists})
		seen["CLAUDE.md"] = true

		// Scan templates/rules/ and claude/rules/
		for _, base := range []string{filepath.Join(tplBase, "rules"), filepath.Join(clBase, "rules")} {
			os.MkdirAll(base, 0755)
			entries, _ := os.ReadDir(base)
			for _, e := range entries {
				p := "rules/" + e.Name()
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !seen[p] {
					seen[p] = true
					files = append(files, FileInfo{Name: e.Name(), Path: p, Exists: true})
				}
			}
		}
	} else {
		dir := scopeDir(tplBase, scope)
		clDir := scopeDir(clBase, scope)
		if dir == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		for _, base := range []string{dir, clDir} {
			os.MkdirAll(base, 0755)
			entries, _ := os.ReadDir(base)
			for _, e := range entries {
				p := scope + "/" + e.Name()
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !seen[p] {
					seen[p] = true
					files = append(files, FileInfo{Name: e.Name(), Path: p, Exists: true})
				}
			}
		}
	}

	if files == nil {
		files = []FileInfo{}
	}
	c.JSON(http.StatusOK, files)
}

func ReadFile(c *gin.Context) {
	scope := c.Query("scope")
	p := c.Query("path")
	if scope == "" || p == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope and path are required"})
		return
	}
	tplPath, clPath, ok := resolvePaths(scope, p)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	// Prefer template source; fall back to claude dir
	data, err := os.ReadFile(tplPath)
	if err != nil {
		data, err = os.ReadFile(clPath)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"content": ""})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": string(data)})
}

func WriteFile(c *gin.Context) {
	var req FileContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tplPath, clPath, ok := resolvePaths(req.Scope, req.Path)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	// Save template source
	os.MkdirAll(filepath.Dir(tplPath), 0755)
	if err := os.WriteFile(tplPath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Render to claude dir
	rendered := core.RenderTemplate(req.Content)
	os.MkdirAll(filepath.Dir(clPath), 0755)
	os.WriteFile(clPath, []byte(rendered), 0644)

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func CreateFile(c *gin.Context) {
	var req FileContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scope == "" || req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope and path are required"})
		return
	}
	tplPath, clPath, ok := resolvePaths(req.Scope, req.Path)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	if fileExists(tplPath) || fileExists(clPath) {
		c.JSON(http.StatusConflict, gin.H{"error": "file already exists"})
		return
	}
	os.MkdirAll(filepath.Dir(tplPath), 0755)
	if err := os.WriteFile(tplPath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Render to claude dir
	rendered := core.RenderTemplate(req.Content)
	os.MkdirAll(filepath.Dir(clPath), 0755)
	os.WriteFile(clPath, []byte(rendered), 0644)

	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

func DeleteFile(c *gin.Context) {
	scope := c.Query("scope")
	p := c.Query("path")
	if scope == "" || p == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope and path are required"})
		return
	}
	tplPath, clPath, ok := resolvePaths(scope, p)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	os.Remove(tplPath)
	os.Remove(clPath)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetTemplateVars returns available template variables with current values.
func GetTemplateVars(c *gin.Context) {
	type VarInfo struct {
		Name  string `json:"name"`
		Desc  string `json:"desc"`
		Value string `json:"value"`
	}
	vars := core.TemplateVars()
	descs := map[string]string{
		"HOME_DIR":      "用户主目录",
		"CLAUDE_DIR":    "Claude 配置目录",
		"MEMORY_DIR":    "记忆文件目录",
		"KNOWLEDGE_DIR": "知识库文件目录",
		"RULES_DIR":     "规则文件目录",
		"OS":            "操作系统",
		"DATE":          "当前日期",
		"DATETIME":      "当前本地时间",
		"TIME_BEIJING":  "当前北京时间",
	}
	order := []string{"HOME_DIR", "CLAUDE_DIR", "MEMORY_DIR", "KNOWLEDGE_DIR", "RULES_DIR", "OS", "DATE", "DATETIME", "TIME_BEIJING"}
	result := make([]VarInfo, 0, len(order))
	for _, k := range order {
		result = append(result, VarInfo{Name: k, Desc: descs[k], Value: vars[k]})
	}
	c.JSON(http.StatusOK, result)
}
