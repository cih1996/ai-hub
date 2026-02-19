package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

func sessionRulesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ai-hub", "session-rules")
}

func sessionRulesPath(id int64) string {
	return filepath.Join(sessionRulesDir(), fmt.Sprintf("%d.md", id))
}

// ReadSessionRules returns the content of a session's rules file.
// Returns ("", nil) if the file does not exist.
func ReadSessionRules(id int64) (string, error) {
	data, err := os.ReadFile(sessionRulesPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// GetSessionRules handles GET /api/v1/session-rules/:id
func GetSessionRules(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	content, err := ReadSessionRules(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"session_id": id, "content": content})
}

// PutSessionRules handles PUT /api/v1/session-rules/:id
func PutSessionRules(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body failed"})
		return
	}
	dir := sessionRulesDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create dir failed: " + err.Error()})
		return
	}
	if err := os.WriteFile(sessionRulesPath(id), body, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteSessionRules handles DELETE /api/v1/session-rules/:id
func DeleteSessionRules(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	path := sessionRulesPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
