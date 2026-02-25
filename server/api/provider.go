package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
)

// Provider handlers

func ListProviders(c *gin.Context) {
	list, err := store.ListProviders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.Provider{}
	}
	c.JSON(http.StatusOK, list)
}

func CreateProvider(c *gin.Context) {
	var p model.Provider
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := store.CreateProvider(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, p)
}

func UpdateProvider(c *gin.Context) {
	id := c.Param("id")
	existing, err := store.GetProvider(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}
	if err := c.ShouldBindJSON(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.ID = id
	if err := store.UpdateProvider(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func DeleteProvider(c *gin.Context) {
	id := c.Param("id")
	if err := store.DeleteProvider(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetClaudeAuthStatus GET /api/v1/claude/auth-status
// Calls `claude auth status` and parses the output to return login state.
func GetClaudeAuthStatus(c *gin.Context) {
	out, err := exec.Command("claude", "auth", "status").CombinedOutput()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"logged_in":   false,
			"auth_method": "",
			"error":       strings.TrimSpace(string(out)),
		})
		return
	}
	output := string(out)
	loggedIn := strings.Contains(output, "loggedIn") && strings.Contains(output, "true")
	var authMethod, email string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "authMethod") || strings.Contains(line, "authMethod") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				authMethod = strings.TrimSpace(parts[1])
			}
			// Also try colon-separated
			parts = strings.SplitN(line, ":", 2)
			if len(parts) == 2 && authMethod == "" {
				authMethod = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "email") || strings.Contains(line, "email") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				email = strings.TrimSpace(parts[1])
			}
			parts = strings.SplitN(line, ":", 2)
			if len(parts) == 2 && email == "" {
				email = strings.TrimSpace(parts[1])
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"logged_in":   loggedIn,
		"auth_method": authMethod,
		"email":       email,
		"raw":         strings.TrimSpace(output),
	})
}
