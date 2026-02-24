package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetMessageTokenUsage GET /api/v1/token-usage/message/:id
func GetMessageTokenUsage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}
	t, err := store.GetTokenUsageByMessage(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, t)
}

// GetSessionTokenUsage GET /api/v1/token-usage/session/:id
func GetSessionTokenUsage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	stats, err := store.GetSessionTokenStats(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	list, _ := store.GetSessionTokenUsageList(id)
	if list == nil {
		list = []model.TokenUsage{}
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats, "records": list})
}

// GetSystemTokenUsage GET /api/v1/token-usage/system
func GetSystemTokenUsage(c *gin.Context) {
	start := c.Query("start")
	end := c.Query("end")
	stats, err := store.GetSystemTokenStats(start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}
