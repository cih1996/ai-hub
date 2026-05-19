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
	stats, err := store.GetSessionTokenStats(id, 0)
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

// GetDailyTokenUsage GET /api/v1/token-usage/daily?start=&end=
func GetDailyTokenUsage(c *gin.Context) {
	start := c.Query("start")
	end := c.Query("end")
	list, err := store.GetDailyTokenUsage(start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []store.DailyTokenUsage{}
	}
	c.JSON(http.StatusOK, list)
}

// GetTokenUsageRanking GET /api/v1/token-usage/ranking?start=&end=&limit=10
func GetTokenUsageRanking(c *gin.Context) {
	start := c.Query("start")
	end := c.Query("end")
	limit := 10
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	list, err := store.GetTokenUsageRanking(start, end, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []store.SessionTokenRanking{}
	}
	c.JSON(http.StatusOK, list)
}

// GetHourlyTokenUsage GET /api/v1/token-usage/hourly?start=&end=&session_id=
func GetHourlyTokenUsage(c *gin.Context) {
	start := c.Query("start")
	end := c.Query("end")
	var sessionID int64
	if sid := c.Query("session_id"); sid != "" {
		if id, err := strconv.ParseInt(sid, 10, 64); err == nil {
			sessionID = id
		}
	}
	list, err := store.GetHourlyTokenUsage(start, end, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []store.HourlyTokenUsage{}
	}
	c.JSON(http.StatusOK, list)
}

// GetSessionContextUsage returns context usage based on actual HTTP request body size (bytes).
// GET /api/v1/sessions/:id/context-usage
func GetSessionContextUsage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	bodySize, _ := store.GetLatestRequestBodySize(id)

	session, _ := store.GetSession(id)
	var provider *model.Provider
	if session != nil {
		provider, _ = store.GetProvider(session.ProviderID)
		if provider == nil {
			provider, _ = store.GetDefaultProvider()
		}
	}

	settings, _ := store.GetCompressionSettings()
	if settings == nil {
		settings = &model.CompressionSettings{}
	}

	providerMaxSize := 0
	thresholdPercent := 0
	thresholdBytes := 0
	compressionEnabled := false
	displayPct := 0.0

	if provider != nil {
		providerMaxSize = providerMaxRequestBodyBytes(provider) // bytes, fallback to default when provider is unset
	}
	if settings.Enabled && settings.ThresholdPercent > 0 {
		compressionEnabled = true
		thresholdPercent = settings.ThresholdPercent
		if providerMaxSize > 0 {
			thresholdBytes = providerMaxSize * thresholdPercent / 100
			if thresholdBytes > 0 {
				displayPct = float64(bodySize) * 100 / float64(thresholdBytes)
				if displayPct > 100 {
					displayPct = 100
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"estimated_tokens":    bodySize,      // bytes (field name kept for frontend compat)
		"provider_max_tokens": providerMaxSize, // bytes
		"threshold_percent":   thresholdPercent,
		"threshold_tokens":    thresholdBytes, // bytes
		"display_percent":     displayPct,
		"compression_enabled": compressionEnabled,
	})
}
