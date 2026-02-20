package api

import (
	"ai-hub/server/core"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

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
	if req.Scope != "knowledge" && req.Scope != "memory" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope must be 'knowledge' or 'memory'"})
		return
	}
	if req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if !core.Vector.IsReady() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine not ready"})
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
	if !core.Vector.IsReady() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine not ready"})
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
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
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
	c.JSON(http.StatusOK, gin.H{"file_name": req.FileName, "content": string(data)})
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
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
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
	// Trigger vector sync
	core.SyncFileToVector(scope, path)
	c.JSON(http.StatusOK, gin.H{"ok": true, "file_name": req.FileName})
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
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
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
	if !core.Vector.IsReady() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "vector engine not ready"})
		return
	}
	stats, err := core.Vector.Stats(scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// VectorStatus returns vector engine status
// GET /api/v1/vector/status
func VectorStatus(c *gin.Context) {
	c.JSON(http.StatusOK, core.Vector.Status())
}
