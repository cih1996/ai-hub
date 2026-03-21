package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListHooks GET /api/v1/hooks?event=xxx (optional filter)
func ListHooks(c *gin.Context) {
	event := c.Query("event")
	var list []model.Hook
	var err error
	if event != "" {
		list, err = store.ListHooksByEvent(event)
	} else {
		list, err = store.ListHooks()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.Hook{}
	}
	c.JSON(http.StatusOK, list)
}

// GetHook GET /api/v1/hooks/:id
func GetHook(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	h, err := store.GetHook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "hook not found"})
		return
	}
	c.JSON(http.StatusOK, h)
}

// CreateHook POST /api/v1/hooks
func CreateHook(c *gin.Context) {
	var h model.Hook
	if err := c.ShouldBindJSON(&h); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Validate required fields
	if h.Event == "" || h.TargetSession == 0 || h.Payload == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "event, target_session, payload required"})
		return
	}
	// Validate event type
	validEvents := map[string]bool{
		"session.created":  true,
		"message.received": true,
		"message.count":    true,
		"session.error":    true,
	}
	if !validEvents[h.Event] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event type. Valid: session.created, message.received, message.count, session.error"})
		return
	}
	// Validate target session exists
	if _, err := store.GetSession(h.TargetSession); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target session not found"})
		return
	}
	h.Enabled = true
	if err := store.CreateHook(&h); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h)
}

// UpdateHook PUT /api/v1/hooks/:id (partial update)
func UpdateHook(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	existing, err := store.GetHook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "hook not found"})
		return
	}
	var req struct {
		Event         *string `json:"event"`
		Condition     *string `json:"condition"`
		TargetSession *int64  `json:"target_session"`
		Payload       *string `json:"payload"`
		Enabled       *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Event != nil {
		existing.Event = *req.Event
	}
	if req.Condition != nil {
		existing.Condition = *req.Condition
	}
	if req.TargetSession != nil {
		existing.TargetSession = *req.TargetSession
	}
	if req.Payload != nil {
		existing.Payload = *req.Payload
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if err := store.UpdateHook(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

// DeleteHook DELETE /api/v1/hooks/:id
func DeleteHook(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := store.DeleteHook(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// EnableHook POST /api/v1/hooks/:id/enable
func EnableHook(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	h, err := store.GetHook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "hook not found"})
		return
	}
	h.Enabled = true
	if err := store.UpdateHook(h); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": true})
}

// DisableHook POST /api/v1/hooks/:id/disable
func DisableHook(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	h, err := store.GetHook(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "hook not found"})
		return
	}
	h.Enabled = false
	if err := store.UpdateHook(h); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": false})
}
