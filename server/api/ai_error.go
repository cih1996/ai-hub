package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetSessionErrors returns error/warning records for a session.
// GET /api/v1/sessions/:id/errors?level=error|warning
func GetSessionErrors(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	level := c.Query("level")
	errors, err := store.GetSessionErrors(id, level)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if errors == nil {
		errors = []model.AIError{}
	}
	c.JSON(http.StatusOK, gin.H{"errors": errors})
}

// GetErrorStats returns error/warning counts aggregated by session.
// GET /api/v1/stats/errors?session_id=123
func GetErrorStats(c *gin.Context) {
	var sessionID int64
	if s := c.Query("session_id"); s != "" {
		sessionID, _ = strconv.ParseInt(s, 10, 64)
	}
	stats, err := store.GetErrorStats(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if stats == nil {
		stats = []model.ErrorStat{}
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}
