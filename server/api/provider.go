package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
)

// Provider handlers

func normalizeProviderInput(p *model.Provider) {
	if p == nil {
		return
	}
	p.ModelID = strings.TrimSpace(p.ModelID)
	// Claude subscription OAuth mode cannot set model explicitly.
	if p.AuthMode == "oauth" {
		p.ModelID = ""
	}
}

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
	normalizeProviderInput(&p)
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
	normalizeProviderInput(existing)
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
	cmd := exec.Command("claude", "auth", "status")
	// Filter out CLAUDECODE env var to avoid nested session detection
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"logged_in":   false,
			"auth_method": "",
			"error":       strings.TrimSpace(string(out)),
		})
		return
	}
	// claude auth status outputs JSON: {"loggedIn":true,"authMethod":"oauth_token",...}
	var parsed struct {
		LoggedIn   bool   `json:"loggedIn"`
		AuthMethod string `json:"authMethod"`
		Email      string `json:"email"`
	}
	output := strings.TrimSpace(string(out))
	if json.Unmarshal([]byte(output), &parsed) == nil {
		c.JSON(http.StatusOK, gin.H{
			"logged_in":   parsed.LoggedIn,
			"auth_method": parsed.AuthMethod,
			"email":       parsed.Email,
			"raw":         output,
		})
		return
	}
	// Fallback: return raw output
	c.JSON(http.StatusOK, gin.H{
		"logged_in":   strings.Contains(output, "true"),
		"auth_method": "",
		"email":       "",
		"raw":         output,
	})
}
