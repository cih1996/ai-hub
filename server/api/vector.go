package api

import (
	"ai-hub/server/core"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
)

// waitVectorReady waits for the vector engine to become ready during bootstrap.
// Returns true if ready; returns false and writes 503 response if not.
func waitVectorReady(c *gin.Context) bool {
	if core.Vector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine not initialized"})
		return false
	}
	if core.Vector.IsReady() {
		return true
	}
	if core.Vector.IsDisabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine disabled"})
		return false
	}
	// Engine is bootstrapping — wait up to 60s
	log.Printf("[vector-api] engine not ready, waiting for bootstrap (request: %s)", c.Request.URL.Path)
	if core.Vector.WaitReady(60 * time.Second) {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine not ready (timeout waiting for bootstrap)"})
	return false
}

// isValidScope returns true if scope is one of the allowed forms:
//   - "knowledge" or "memory" (global)
//   - "<groupname>/knowledge", "<groupname>/memory", or "<groupname>/rules" (team-level)
//
// groupname supports Unicode letters (including CJK/Chinese), digits, spaces,
// hyphens and underscores. Path traversal sequences are rejected.
func isValidScope(scope string) bool {
	if scope == "knowledge" || scope == "memory" {
		return true
	}
	idx := strings.LastIndex(scope, "/")
	if idx <= 0 {
		return false
	}
	suffix := scope[idx+1:]
	if suffix != "knowledge" && suffix != "memory" && suffix != "rules" {
		return false
	}
	prefix := scope[:idx]
	// Reject path traversal and null bytes
	if strings.Contains(prefix, "..") || strings.Contains(prefix, "\x00") || strings.Contains(prefix, "/") {
		return false
	}
	// Allow Unicode letters (covers Chinese, Japanese, etc.), digits, spaces, hyphens, underscores
	for _, ch := range prefix {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '-' && ch != '_' && ch != ' ' {
			return false
		}
	}
	return len(strings.TrimSpace(prefix)) > 0
}

// --- Vector MCP tool handlers ---
// These are HTTP endpoints that Claude CLI calls via MCP configuration.

// SearchVector performs semantic search with scope in request body
// POST /api/v1/vector/search
func SearchVector(c *gin.Context) {
	var req struct {
		Scope string `json:"scope"`
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isValidScope(req.Scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope must be 'knowledge', 'memory', or '<groupname>/knowledge', '<groupname>/memory'"})
		return
	}
	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if !waitVectorReady(c) {
		return
	}
	results, err := core.Vector.Search(req.Scope, req.Query, req.TopK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// SearchKnowledge performs semantic search on knowledge files
// POST /api/v1/vector/search_knowledge
func SearchKnowledge(c *gin.Context) {
	vectorSearch(c, "knowledge")
}

// SearchMemory performs semantic search on memory files
// POST /api/v1/vector/search_memory
func SearchMemory(c *gin.Context) {
	vectorSearch(c, "memory")
}

func vectorSearch(c *gin.Context, scope string) {
	var req struct {
		Query string `json:"query"`
		TopK  int    `json:"top_k"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if !waitVectorReady(c) {
		return
	}
	results, err := core.Vector.Search(scope, req.Query, req.TopK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// ReadKnowledge reads a knowledge file's full content
// POST /api/v1/vector/read_knowledge
func ReadKnowledge(c *gin.Context) {
	vectorRead(c, "knowledge")
}

// ReadMemory reads a memory file's full content
// POST /api/v1/vector/read_memory
func ReadMemory(c *gin.Context) {
	vectorRead(c, "memory")
}

func vectorRead(c *gin.Context, scope string) {
	var req struct {
		FileName string `json:"file_name"`
		Scope    string `json:"scope"` // optional: overrides the fixed scope parameter
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Use request-body scope if provided and valid (allows team scoping via universal endpoints)
	if req.Scope != "" {
		if !isValidScope(req.Scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		scope = req.Scope
	}
	if req.FileName == "" || !validatePath(req.FileName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid file_name is required"})
		return
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ai-hub", scope)
	path := filepath.Join(dir, req.FileName)
	if !strings.HasPrefix(path, dir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"file_name": req.FileName, "content": string(data), "scope": scope})
}

// WriteKnowledge writes/updates a knowledge file
// POST /api/v1/vector/write_knowledge
func WriteKnowledge(c *gin.Context) {
	vectorWrite(c, "knowledge")
}

// WriteMemory writes/updates a memory file
// POST /api/v1/vector/write_memory
func WriteMemory(c *gin.Context) {
	vectorWrite(c, "memory")
}

func vectorWrite(c *gin.Context, scope string) {
	var req struct {
		FileName string `json:"file_name"`
		Content  string `json:"content"`
		Scope    string `json:"scope"` // optional: overrides the fixed scope parameter
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scope != "" {
		if !isValidScope(req.Scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		scope = req.Scope
	}
	if req.FileName == "" || !validatePath(req.FileName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid file_name is required"})
		return
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ai-hub", scope)
	path := filepath.Join(dir, req.FileName)
	if !strings.HasPrefix(path, dir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	os.MkdirAll(dir, 0755)
	if err := os.WriteFile(path, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Trigger vector sync (also registers the dir for watching if new group scope)
	core.SyncFileToVector(scope, path)
	c.JSON(http.StatusOK, gin.H{"ok": true, "file_name": req.FileName, "scope": scope})
}

// DeleteKnowledge deletes a knowledge file
// POST /api/v1/vector/delete_knowledge
func DeleteKnowledge(c *gin.Context) {
	vectorDelete(c, "knowledge")
}

// DeleteMemory deletes a memory file
// POST /api/v1/vector/delete_memory
func DeleteMemory(c *gin.Context) {
	vectorDelete(c, "memory")
}

func vectorDelete(c *gin.Context, scope string) {
	var req struct {
		FileName string `json:"file_name"`
		Scope    string `json:"scope"` // optional: overrides the fixed scope parameter
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Scope != "" {
		if !isValidScope(req.Scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
			return
		}
		scope = req.Scope
	}
	if req.FileName == "" || !validatePath(req.FileName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid file_name is required"})
		return
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ai-hub", scope)
	path := filepath.Join(dir, req.FileName)
	if !strings.HasPrefix(path, dir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	os.Remove(path)
	// Clean vector record
	if core.Vector != nil {
		core.Vector.Delete(scope, req.FileName)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "file_name": req.FileName})
}

// StatsVector returns vector hit statistics
// GET /api/v1/vector/stats?scope=knowledge
func StatsVector(c *gin.Context) {
	scope := c.DefaultQuery("scope", "knowledge")
	if !isValidScope(scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if !waitVectorReady(c) {
		return
	}
	stats, err := core.Vector.Stats(scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// RestartVector restarts the vector engine
// POST /api/v1/vector/restart
func RestartVector(c *gin.Context) {
	if core.Vector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine not initialized"})
		return
	}
	go core.Vector.Restart()
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "vector engine restarting"})
}

// VectorStatus returns vector engine status
// GET /api/v1/vector/status
func VectorStatus(c *gin.Context) {
	c.JSON(http.StatusOK, core.Vector.Status())
}

// VectorHealth returns a simplified health check for frontend banner
// GET /api/v1/vector/health
func VectorHealth(c *gin.Context) {
	status := core.Vector.Status()
	ready, _ := status["ready"].(bool)
	disabled, _ := status["disabled"].(bool)
	errMsg, _ := status["error"].(string)

	health := gin.H{
		"ready":    ready,
		"disabled": disabled,
	}
	if errMsg != "" {
		health["error"] = errMsg
	}

	// Check Python availability
	if !ready {
		if _, err := exec.LookPath("python3"); err != nil {
			health["fix_hint"] = "python3_missing"
		} else {
			health["fix_hint"] = "engine_not_ready"
		}
	}

	c.JSON(http.StatusOK, health)
}

// ListVectorFiles lists .md files in a vector scope directory (filesystem only, no engine required).
// GET /api/v1/vector/list?scope=<scope>
func ListVectorFiles(c *gin.Context) {
	scope := c.Query("scope")
	if scope == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope is required"})
		return
	}
	if !isValidScope(scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ai-hub", scope)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, []string{})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			files = append(files, e.Name())
		}
	}
	if files == nil {
		files = []string{}
	}
	c.JSON(http.StatusOK, files)
}

// ReadVector reads a single file from any valid scope (filesystem only, no engine required).
// POST /api/v1/vector/read
func ReadVector(c *gin.Context) {
	var req struct {
		FileName string `json:"file_name"`
		Scope    string `json:"scope"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isValidScope(req.Scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope"})
		return
	}
	if req.FileName == "" || !validatePath(req.FileName) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid file_name is required"})
		return
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ai-hub", req.Scope)
	path := filepath.Join(dir, req.FileName)
	if !strings.HasPrefix(path, dir) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"file_name": req.FileName, "content": string(data), "scope": req.Scope})
}
