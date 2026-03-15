package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListGroups returns all groups
func ListGroups(c *gin.Context) {
	groups, err := store.ListGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if groups == nil {
		groups = []model.Group{}
	}

	// Add session count for each group
	type GroupWithCount struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		SessionCount int    `json:"session_count"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
	}

	result := make([]GroupWithCount, 0, len(groups))
	for _, g := range groups {
		count, _ := store.CountSessionsByGroup(g.Name)
		result = append(result, GroupWithCount{
			ID:           g.ID,
			Name:         g.Name,
			Description:  g.Description,
			SessionCount: count,
			CreatedAt:    g.CreatedAt,
			UpdatedAt:    g.UpdatedAt,
		})
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
func UpdateGroup(c *gin.Context) {
	name := c.Param("name")

	// Check if group exists
	group, err := store.GetGroupByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}

	var req struct {
		Icon        *string `json:"icon"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
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
		"message":        "session moved",
		"session_id":     id,
		"old_group":      session.GroupName,
		"new_group":      req.GroupName,
	})
}
