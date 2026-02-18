package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

type McpServerInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Command string `json:"command"`
	Enabled bool   `json:"enabled"`
}

type ToggleMcpRequest struct {
	Name   string `json:"name"`
	Enable bool   `json:"enable"`
}

// mcpConfigPaths returns candidate paths for MCP config, ordered by priority.
func mcpConfigPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".claude", "settings.json"),
	}
}

// detectMcpConfigPath finds which file actually contains mcpServers.
// Falls back to the first candidate if none contains it.
func detectMcpConfigPath() string {
	for _, p := range mcpConfigPaths() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var obj map[string]interface{}
		if json.Unmarshal(data, &obj) != nil {
			continue
		}
		if _, ok := obj["mcpServers"]; ok {
			return p
		}
		if _, ok := obj["disabledMcpServers"]; ok {
			return p
		}
	}
	return mcpConfigPaths()[0]
}

func readMcpConfig() (string, map[string]interface{}, error) {
	path := detectMcpConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return path, map[string]interface{}{}, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return path, nil, err
	}
	return path, obj, nil
}

func writeMcpConfig(path string, obj map[string]interface{}) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func parseMcpServer(name string, raw interface{}) McpServerInfo {
	info := McpServerInfo{Name: name}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return info
	}
	if t, ok := m["type"].(string); ok {
		info.Type = t
	}
	if u, ok := m["url"].(string); ok {
		info.URL = u
		if info.Type == "" {
			info.Type = "http"
		}
	}
	if cmd, ok := m["command"].(string); ok {
		info.Command = cmd
		if info.Type == "" {
			info.Type = "stdio"
		}
	}
	if info.Type == "" {
		info.Type = "stdio"
	}
	return info
}

func ListMcp(c *gin.Context) {
	_, obj, err := readMcpConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var servers []McpServerInfo

	if enabled, ok := obj["mcpServers"].(map[string]interface{}); ok {
		for name, raw := range enabled {
			s := parseMcpServer(name, raw)
			s.Enabled = true
			servers = append(servers, s)
		}
	}
	if disabled, ok := obj["disabledMcpServers"].(map[string]interface{}); ok {
		for name, raw := range disabled {
			s := parseMcpServer(name, raw)
			s.Enabled = false
			servers = append(servers, s)
		}
	}
	if servers == nil {
		servers = []McpServerInfo{}
	}
	c.JSON(http.StatusOK, servers)
}

func ToggleMcp(c *gin.Context) {
	var req ToggleMcpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	path, obj, err := readMcpConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	enabled, _ := obj["mcpServers"].(map[string]interface{})
	if enabled == nil {
		enabled = map[string]interface{}{}
	}
	disabled, _ := obj["disabledMcpServers"].(map[string]interface{})
	if disabled == nil {
		disabled = map[string]interface{}{}
	}

	if req.Enable {
		// Move from disabled to enabled
		if entry, ok := disabled[req.Name]; ok {
			enabled[req.Name] = entry
			delete(disabled, req.Name)
		}
	} else {
		// Move from enabled to disabled
		if entry, ok := enabled[req.Name]; ok {
			disabled[req.Name] = entry
			delete(enabled, req.Name)
		}
	}

	obj["mcpServers"] = enabled
	obj["disabledMcpServers"] = disabled

	if err := writeMcpConfig(path, obj); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}


