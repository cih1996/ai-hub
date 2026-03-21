package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetChangelog GET /api/v1/changelog?file_name=xxx&scope=xxx&limit=20
func GetChangelog(c *gin.Context) {
	fileName := c.Query("file_name")
	scope := c.Query("scope")
	if fileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_name required"})
		return
	}
	if scope == "" {
		scope = "memory" // default scope
	}
	limit := 20
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	list, err := store.ListChangelog(fileName, scope, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.MemoryChangelog{}
	}
	c.JSON(http.StatusOK, gin.H{"changelog": list, "file_name": fileName, "scope": scope})
}

// RollbackChangelog POST /api/v1/changelog/rollback
// Rolls back a memory file to a specific version.
func RollbackChangelog(c *gin.Context) {
	var req struct {
		FileName string `json:"file_name"`
		Scope    string `json:"scope"`
		Version  int    `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.FileName == "" || req.Version <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_name and version (>0) required"})
		return
	}
	if req.Scope == "" {
		req.Scope = "memory"
	}

	// Get the target version
	cl, err := store.GetChangelogByVersion(req.FileName, req.Scope, req.Version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}

	// Write the old content back to the file
	dir := core.ScopeDir(req.Scope)
	path := filepath.Join(dir, req.FileName)
	os.MkdirAll(dir, 0755)
	if err := os.WriteFile(path, []byte(cl.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write file failed: " + err.Error()})
		return
	}

	// Sync to vector
	core.SyncFileToVector(req.Scope, path, 0)

	// Record the rollback as a new changelog entry
	rollbackCl := &model.MemoryChangelog{
		FileName:   req.FileName,
		Scope:      req.Scope,
		ChangeType: "update",
		SessionID:  0,
		Diff:       "rollback to version " + strconv.Itoa(req.Version),
		Schema:     cl.Schema,
		Content:    cl.Content,
	}
	store.AddChangelog(rollbackCl)

	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"rolled_back_to":  req.Version,
		"new_version":     rollbackCl.Version,
	})
}
