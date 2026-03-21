package api

import (
	"ai-hub/server/store"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetSessionHealth GET /api/v1/sessions/:id/health
func GetSessionHealth(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	score, updatedAt, correctionCount, driftCount, err := store.GetSessionHealth(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id":       id,
		"health_score":     score,
		"health_updated_at": updatedAt,
		"correction_count": correctionCount,
		"drift_count":      driftCount,
	})
}

// UpdateSessionHealth PUT /api/v1/sessions/:id/health
func UpdateSessionHealth(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	// Verify session exists
	if _, err := store.GetSession(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var req struct {
		Score     *string `json:"health_score"`     // green | yellow | red
		IncrField *string `json:"incr"`             // correction | drift
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Handle score update
	if req.Score != nil {
		validScores := map[string]bool{"green": true, "yellow": true, "red": true, "": true}
		if !validScores[*req.Score] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid health_score. Valid: green, yellow, red, or empty string"})
			return
		}
		if err := store.UpdateSessionHealth(id, *req.Score); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Handle increment
	if req.IncrField != nil {
		switch *req.IncrField {
		case "correction":
			if err := store.IncrementCorrectionCount(id); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		case "drift":
			if err := store.IncrementDriftCount(id); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid incr field. Valid: correction, drift"})
			return
		}
	}

	// Return updated health info
	score, updatedAt, correctionCount, driftCount, _ := store.GetSessionHealth(id)
	c.JSON(http.StatusOK, gin.H{
		"ok":               true,
		"session_id":       id,
		"health_score":     score,
		"health_updated_at": updatedAt,
		"correction_count": correctionCount,
		"drift_count":      driftCount,
	})
}
