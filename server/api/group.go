package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ListGroups returns all groups (from groups table + unique group_names from sessions)
func ListGroups(c *gin.Context) {
	groups, err := store.ListGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if groups == nil {
		groups = []model.Group{}
	}

	// Build a map of existing groups
	groupMap := make(map[string]*model.Group)
	for i := range groups {
		groupMap[groups[i].Name] = &groups[i]
	}

	// Get all unique group_names from sessions
	sessionGroupNames, _ := store.ListUniqueGroupNames()

	// Add session count for each group
	type GroupWithCount struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		Icon         string `json:"icon"`
		Description  string `json:"description"`
		SessionCount int    `json:"session_count"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
	}

	result := make([]GroupWithCount, 0)

	// First add groups from groups table
	for _, g := range groups {
		count, _ := store.CountSessionsByGroup(g.Name)
		result = append(result, GroupWithCount{
			ID:           g.ID,
			Name:         g.Name,
			Icon:         g.Icon,
			Description:  g.Description,
			SessionCount: count,
			CreatedAt:    g.CreatedAt,
			UpdatedAt:    g.UpdatedAt,
		})
	}

	// Then add groups that only exist in sessions (not in groups table)
	for _, name := range sessionGroupNames {
		if _, exists := groupMap[name]; !exists {
			count, _ := store.CountSessionsByGroup(name)
			result = append(result, GroupWithCount{
				ID:           0, // No ID since not in groups table
				Name:         name,
				Icon:         "", // Default icon
				Description:  "",
				SessionCount: count,
				CreatedAt:    "",
				UpdatedAt:    "",
			})
		}
	}

	c.JSON(http.StatusOK, result)
}

// GetGroup returns a group by name
func GetGroup(c *gin.Context) {
	name := c.Param("name")
	group, err := store.GetGroupByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}

	// Get session count
	count, _ := store.CountSessionsByGroup(name)

	// Get sessions in this group
	sessions, _ := store.ListSessionsByGroup(name)

	c.JSON(http.StatusOK, gin.H{
		"id":            group.ID,
		"name":          group.Name,
		"description":   group.Description,
		"session_count": count,
		"sessions":      sessions,
		"created_at":    group.CreatedAt,
		"updated_at":    group.UpdatedAt,
	})
}

// CreateGroup creates a new group
func CreateGroup(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if group already exists
	if _, err := store.GetGroupByName(req.Name); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "group already exists"})
		return
	}

	group, err := store.CreateGroup(req.Name, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, group)
}

// DeleteGroup deletes a group by name
func DeleteGroup(c *gin.Context) {
	name := c.Param("name")

	// Check if group exists
	if _, err := store.GetGroupByName(name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}

	// Check if group has sessions
	count, _ := store.CountSessionsByGroup(name)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "group has sessions, cannot delete"})
		return
	}

	if err := store.DeleteGroup(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "group deleted"})
}

// UpdateGroup updates a group's icon and description
// If the group doesn't exist but has sessions, create it automatically
func UpdateGroup(c *gin.Context) {
	name := c.Param("name")

	var req struct {
		Icon        *string `json:"icon"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if group exists
	group, err := store.GetGroupByName(name)
	if err != nil {
		// Group doesn't exist in groups table, check if it has sessions
		count, _ := store.CountSessionsByGroup(name)
		if count == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
			return
		}
		// Auto-create the group record
		group, err = store.CreateGroup(name, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create group: " + err.Error()})
			return
		}
	}

	// Merge with existing values
	icon := group.Icon
	description := group.Description
	if req.Icon != nil {
		icon = *req.Icon
	}
	if req.Description != nil {
		description = *req.Description
	}

	if err := store.UpdateGroup(name, icon, description); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "group updated"})
}

// UpdateSessionGroup moves a session to a different group
func UpdateSessionGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	var req struct {
		GroupName string `json:"group_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if session exists
	session, err := store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Update session group
	if err := store.UpdateSessionGroup(id, req.GroupName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "session moved",
		"session_id": id,
		"old_group":  session.GroupName,
		"new_group":  req.GroupName,
	})
}

// ListAvatars returns available avatar filenames (uploaded first, then defaults)
func ListAvatars(c *gin.Context) {
	result := []string{}

	// 1. Get uploaded avatars first (from ~/.ai-hub/avatars/)
	homeDir, _ := os.UserHomeDir()
	uploadDir := filepath.Join(homeDir, ".ai-hub", "avatars")
	if entries, err := os.ReadDir(uploadDir); err == nil {
		// Sort by modification time (newest first)
		type fileInfo struct {
			name    string
			modTime time.Time
		}
		files := make([]fileInfo, 0)
		for _, entry := range entries {
			if !entry.IsDir() {
				name := entry.Name()
				ext := strings.ToLower(filepath.Ext(name))
				if ext == ".svg" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" {
					info, _ := entry.Info()
					files = append(files, fileInfo{name: "custom/" + name, modTime: info.ModTime()})
				}
			}
		}
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.After(files[j].modTime)
		})
		for _, f := range files {
			result = append(result, f.name)
		}
	}

	// 2. Add default avatars
	for i := 0; i < 50; i++ {
		result = append(result, "avatar"+strconv.Itoa(i+1)+".svg")
	}

	c.JSON(http.StatusOK, result)
}

// UploadAvatar handles avatar file upload
func UploadAvatar(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowedExts := map[string]bool{".svg": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}
	if !allowedExts[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type. Allowed: svg, png, jpg, jpeg, gif, webp"})
		return
	}

	// Create upload directory
	homeDir, _ := os.UserHomeDir()
	uploadDir := filepath.Join(homeDir, ".ai-hub", "avatars")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	// Generate unique filename
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	filePath := filepath.Join(uploadDir, filename)

	// Save file
	out, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"icon": "custom/" + filename,
		"url":  "/avatars/custom/" + filename,
	})
}
