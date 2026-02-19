package api

import (
	"ai-hub/server/core"
	"embed"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

var defaultTemplatesFS embed.FS

func SetDefaultTemplatesFS(fs embed.FS) {
	defaultTemplatesFS = fs
}

// GetDefaultFile returns the built-in default template content.
// GET /api/v1/files/default?path=CLAUDE.md
func GetDefaultFile(c *gin.Context) {
	p := c.Query("path")
	if p == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	data, err := defaultTemplatesFS.ReadFile("claude/" + p)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "default template not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": string(data)})
}

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

func aiHubDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ai-hub")
}

func scopeDir(base, scope string) string {
	switch scope {
	case "knowledge":
		return filepath.Join(base, "knowledge")
	case "memory":
		return filepath.Join(base, "memory")
	case "rules":
		return filepath.Join(base, "rules")
	case "notes":
		return filepath.Join(base, "notes")
	default:
		return ""
	}
}

func validatePath(p string) bool {
	return !strings.Contains(p, "..") && !strings.Contains(p, "~")
}

// resolvePaths returns (templatePath, dataPath, ok).
// Both now point into ~/.ai-hub/ hierarchy.
func resolvePaths(scope, p string) (string, string, bool) {
	if !validatePath(p) {
		return "", "", false
	}
	tplBase := core.TemplateDir()
	dataBase := aiHubDir()

	if scope == "rules" && p == "CLAUDE.md" {
		path := filepath.Join(tplBase, "CLAUDE.md")
		return path, path, true
	}
	tplFull := filepath.Join(tplBase, p)
	dataFull := filepath.Join(dataBase, p)
	if !strings.HasPrefix(tplFull, tplBase) || !strings.HasPrefix(dataFull, dataBase) {
		return "", "", false
	}
	return tplFull, dataFull, true
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
	dataBase := aiHubDir()
	var files []FileInfo
	seen := map[string]bool{}

	if scope == "rules" {
		// Always include CLAUDE.md (built-in)
		exists := fileExists(filepath.Join(tplBase, "CLAUDE.md"))
		files = append(files, FileInfo{Name: "CLAUDE.md", Path: "CLAUDE.md", Exists: exists})
		seen["CLAUDE.md"] = true

		// Scan rules/ under the rules directory
		rulesDir := filepath.Join(tplBase, "rules")
		os.MkdirAll(rulesDir, 0755)
		entries, _ := os.ReadDir(rulesDir)
		for _, e := range entries {
			p := "rules/" + e.Name()
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !seen[p] {
				seen[p] = true
				files = append(files, FileInfo{Name: e.Name(), Path: p, Exists: true})
			}
		}
	} else {
		dir := scopeDir(dataBase, scope)
		if dir == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		os.MkdirAll(dir, 0755)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			p := scope + "/" + e.Name()
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !seen[p] {
				seen[p] = true
				files = append(files, FileInfo{Name: e.Name(), Path: p, Exists: true})
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
	tplPath, dataPath, ok := resolvePaths(scope, p)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	// rules scope reads from tplPath, others from dataPath
	primaryPath := tplPath
	fallbackPath := dataPath
	if scope != "rules" {
		primaryPath = dataPath
		fallbackPath = tplPath
	}
	data, err := os.ReadFile(primaryPath)
	if err != nil && fallbackPath != primaryPath {
		data, err = os.ReadFile(fallbackPath)
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
	tplPath, dataPath, ok := resolvePaths(req.Scope, req.Path)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	// rules scope writes to tplPath (rules dir), others write to dataPath (ai-hub dir)
	writePath := tplPath
	if req.Scope != "rules" {
		writePath = dataPath
	}
	os.MkdirAll(filepath.Dir(writePath), 0755)
	if err := os.WriteFile(writePath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Trigger vector sync for knowledge/memory
	if req.Scope == "knowledge" || req.Scope == "memory" {
		core.SyncFileToVector(req.Scope, writePath)
	}

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
	tplPath, dataPath, ok := resolvePaths(req.Scope, req.Path)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	writePath := tplPath
	if req.Scope != "rules" {
		writePath = dataPath
	}
	if fileExists(writePath) {
		c.JSON(http.StatusConflict, gin.H{"error": "file already exists"})
		return
	}
	os.MkdirAll(filepath.Dir(writePath), 0755)
	if err := os.WriteFile(writePath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Trigger vector sync for knowledge/memory
	if req.Scope == "knowledge" || req.Scope == "memory" {
		core.SyncFileToVector(req.Scope, writePath)
	}

	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

func DeleteFile(c *gin.Context) {
	scope := c.Query("scope")
	p := c.Query("path")
	if scope == "" || p == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope and path are required"})
		return
	}
	tplPath, dataPath, ok := resolvePaths(scope, p)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	os.Remove(tplPath)
	if dataPath != tplPath {
		os.Remove(dataPath)
	}

	// Clean vector record for knowledge/memory
	if scope == "knowledge" || scope == "memory" {
		docID := filepath.Base(tplPath)
		core.Vector.Delete(scope, docID)
	}

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
		"CLAUDE_DIR":    "AI Hub 数据目录",
		"MEMORY_DIR":    "记忆文件目录",
		"KNOWLEDGE_DIR": "知识库文件目录",
		"RULES_DIR":     "规则文件目录",
		"OS":            "操作系统",
		"PORT":          "服务运行端口",
		"DATE":          "当前日期",
		"DATETIME":      "当前本地时间",
		"TIME_BEIJING":  "当前北京时间",
	}
	order := []string{"HOME_DIR", "CLAUDE_DIR", "MEMORY_DIR", "KNOWLEDGE_DIR", "RULES_DIR", "OS", "PORT", "DATE", "DATETIME", "TIME_BEIJING"}
	result := make([]VarInfo, 0, len(order))
	for _, k := range order {
		result = append(result, VarInfo{Name: k, Desc: descs[k], Value: vars[k]})
	}
	c.JSON(http.StatusOK, result)
}

// ---- Project-level rules API (operates on {workDir}/.claude/) ----

func validateProjectPath(p string) bool {
	return !strings.Contains(p, "..") && !strings.Contains(p, "~")
}

// ListProjectRules lists CLAUDE.md + rules/*.md under {workDir}/.claude/
func ListProjectRules(c *gin.Context) {
	workDir := c.Query("work_dir")
	if workDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "work_dir is required"})
		return
	}
	base := filepath.Join(workDir, ".claude")
	var files []FileInfo

	// CLAUDE.md
	claudeMd := filepath.Join(base, "CLAUDE.md")
	files = append(files, FileInfo{Name: "CLAUDE.md", Path: "CLAUDE.md", Exists: fileExists(claudeMd)})

	// rules/*.md
	rulesDir := filepath.Join(base, "rules")
	os.MkdirAll(rulesDir, 0755)
	entries, _ := os.ReadDir(rulesDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			p := "rules/" + e.Name()
			files = append(files, FileInfo{Name: e.Name(), Path: p, Exists: true})
		}
	}
	c.JSON(http.StatusOK, files)
}

// ReadProjectRule reads a rule file from {workDir}/.claude/{path}
func ReadProjectRule(c *gin.Context) {
	workDir := c.Query("work_dir")
	p := c.Query("path")
	if workDir == "" || p == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "work_dir and path are required"})
		return
	}
	if !validateProjectPath(p) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	full := filepath.Join(workDir, ".claude", p)
	if !strings.HasPrefix(full, filepath.Join(workDir, ".claude")) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"content": ""})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": string(data)})
}

// WriteProjectRule writes a rule file to {workDir}/.claude/{path}
func WriteProjectRule(c *gin.Context) {
	var req struct {
		WorkDir string `json:"work_dir"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.WorkDir == "" || req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "work_dir and path are required"})
		return
	}
	if !validateProjectPath(req.Path) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	full := filepath.Join(req.WorkDir, ".claude", req.Path)
	if !strings.HasPrefix(full, filepath.Join(req.WorkDir, ".claude")) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	os.MkdirAll(filepath.Dir(full), 0755)
	if err := os.WriteFile(full, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
