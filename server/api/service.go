package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ListServices returns all services with live status.
func ListServices(c *gin.Context) {
	services, err := store.ListServices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Refresh status from ServiceManager
	if core.ServiceMgr != nil {
		for i := range services {
			services[i].Status = core.ServiceMgr.CheckAlive(&services[i])
		}
	}
	if services == nil {
		services = []model.Service{}
	}
	c.JSON(http.StatusOK, services)
}

// CreateService creates a new service.
func CreateService(c *gin.Context) {
	var s model.Service
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if s.Name == "" || s.Command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and command are required"})
		return
	}
	// Auto-assign log path
	home, _ := os.UserHomeDir()
	safeName := strings.ReplaceAll(s.Name, "/", "-")
	s.LogPath = fmt.Sprintf("%s/.ai-hub/logs/service-%s.log", home, safeName)

	if err := store.CreateService(&s); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			c.JSON(http.StatusConflict, gin.H{"error": "service name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

// GetService returns a single service.
func GetService(c *gin.Context) {
	svc, err := resolveService(c)
	if err != nil {
		return // already responded
	}
	if core.ServiceMgr != nil {
		svc.Status = core.ServiceMgr.CheckAlive(svc)
	}
	c.JSON(http.StatusOK, svc)
}

// UpdateService updates service configuration (not status/pid).
func UpdateService(c *gin.Context) {
	svc, err := resolveService(c)
	if err != nil {
		return
	}
	var req struct {
		Name      *string `json:"name"`
		Command   *string `json:"command"`
		WorkDir   *string `json:"work_dir"`
		Port      *int    `json:"port"`
		AutoStart *bool   `json:"auto_start"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Merge fields (read-then-merge pattern)
	if req.Name != nil {
		svc.Name = *req.Name
		home, _ := os.UserHomeDir()
		safeName := strings.ReplaceAll(svc.Name, "/", "-")
		svc.LogPath = fmt.Sprintf("%s/.ai-hub/logs/service-%s.log", home, safeName)
	}
	if req.Command != nil {
		svc.Command = *req.Command
	}
	if req.WorkDir != nil {
		svc.WorkDir = *req.WorkDir
	}
	if req.Port != nil {
		svc.Port = *req.Port
	}
	if req.AutoStart != nil {
		svc.AutoStart = *req.AutoStart
	}
	if err := store.UpdateService(svc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, svc)
}

// DeleteService stops and deletes a service.
func DeleteService(c *gin.Context) {
	svc, err := resolveService(c)
	if err != nil {
		return
	}
	// Stop first if running
	if core.ServiceMgr != nil && svc.PID > 0 {
		core.ServiceMgr.Stop(svc)
	}
	if err := store.DeleteService(svc.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// StartService starts a service.
func StartService(c *gin.Context) {
	svc, err := resolveService(c)
	if err != nil {
		return
	}
	if core.ServiceMgr == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "service manager not initialized"})
		return
	}
	if err := core.ServiceMgr.Start(svc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, svc)
}

// StopService stops a service.
func StopService(c *gin.Context) {
	svc, err := resolveService(c)
	if err != nil {
		return
	}
	if core.ServiceMgr == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "service manager not initialized"})
		return
	}
	if err := core.ServiceMgr.Stop(svc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, svc)
}

// RestartService restarts a service.
func RestartService(c *gin.Context) {
	svc, err := resolveService(c)
	if err != nil {
		return
	}
	if core.ServiceMgr == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "service manager not initialized"})
		return
	}
	if err := core.ServiceMgr.Restart(svc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, svc)
}

// GetServiceLogs returns the last N lines of a service log.
func GetServiceLogs(c *gin.Context) {
	svc, err := resolveService(c)
	if err != nil {
		return
	}
	lines := 100
	if l := c.Query("lines"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			lines = v
		}
	}
	content, err := tailFile(svc.LogPath, lines)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"logs": "", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": content})
}

// resolveService parses :id param and fetches the service.
func resolveService(c *gin.Context) (*model.Service, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service id"})
		return nil, err
	}
	svc, err := store.GetService(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
		return nil, err
	}
	return svc, nil
}

// tailFile reads the last N lines from a file efficiently.
func tailFile(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Seek to end and read backwards
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := stat.Size()
	if size == 0 {
		return "", nil
	}

	// For small files, just read all
	if size < 64*1024 {
		scanner := bufio.NewScanner(f)
		var allLines []string
		for scanner.Scan() {
			allLines = append(allLines, scanner.Text())
		}
		start := 0
		if len(allLines) > n {
			start = len(allLines) - n
		}
		return strings.Join(allLines[start:], "\n"), nil
	}

	// For large files, read from end
	bufSize := int64(n * 512) // estimate ~512 bytes per line
	if bufSize > size {
		bufSize = size
	}
	offset := size - bufSize
	if offset < 0 {
		offset = 0
	}
	f.Seek(offset, io.SeekStart)
	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	// Skip first partial line if we didn't start from beginning
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}
	return strings.Join(lines[start:], "\n"), nil
}
