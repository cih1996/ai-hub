package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// ListTriggers GET /api/v1/triggers?session_id=X (optional filter)
func ListTriggers(c *gin.Context) {
	sidStr := c.Query("session_id")
	if sidStr != "" {
		sid, err := strconv.ParseInt(sidStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
			return
		}
		list, err := store.ListTriggersBySession(sid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if list == nil {
			list = []model.Trigger{}
		}
		c.JSON(http.StatusOK, list)
		return
	}
	list, err := store.ListTriggers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.Trigger{}
	}
	c.JSON(http.StatusOK, list)
}

// CreateTrigger POST /api/v1/triggers
func CreateTrigger(c *gin.Context) {
	var t model.Trigger
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if t.SessionID == 0 || t.Content == "" || t.TriggerTime == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id, content, trigger_time required"})
		return
	}
	// 验证会话存在
	if _, err := store.GetSession(t.SessionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session not found"})
		return
	}
	if t.MaxFires == 0 {
		t.MaxFires = 1
	}
	t.Enabled = true
	t.Status = "active"
	now := time.Now().In(time.FixedZone("CST", 8*3600))
	t.NextFireAt = core.CalcNextFireAt(&t, now)
	if err := store.CreateTrigger(&t); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// UpdateTrigger PUT /api/v1/triggers/:id
// 使用指针类型做 partial update，只更新请求中实际传了的字段
func UpdateTrigger(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	existing, err := store.GetTrigger(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trigger not found"})
		return
	}
	var req struct {
		Content     *string `json:"content"`
		TriggerTime *string `json:"trigger_time"`
		MaxFires    *int    `json:"max_fires"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Content != nil {
		existing.Content = *req.Content
	}
	if req.TriggerTime != nil {
		existing.TriggerTime = *req.TriggerTime
	}
	if req.MaxFires != nil {
		existing.MaxFires = *req.MaxFires
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if !existing.Enabled {
		existing.Status = "disabled"
	} else if existing.Status == "disabled" || existing.Status == "failed" {
		existing.Status = "active"
	}
	now := time.Now().In(time.FixedZone("CST", 8*3600))
	existing.NextFireAt = core.CalcNextFireAt(existing, now)
	if err := store.UpdateTrigger(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

// DeleteTrigger DELETE /api/v1/triggers/:id
func DeleteTrigger(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := store.DeleteTrigger(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
