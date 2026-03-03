package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetCompressSettings handles GET /api/v1/settings/compress
func GetCompressSettings(c *gin.Context) {
	cfg, err := store.GetCompressSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateCompressSettings handles PUT /api/v1/settings/compress
func UpdateCompressSettings(c *gin.Context) {
	var req model.CompressSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// Validate mode
	if req.Mode != "auto" && req.Mode != "intelligent" && req.Mode != "simple" {
		req.Mode = "auto"
	}
	// Validate threshold
	if req.Threshold <= 0 {
		req.Threshold = 80000
	}
	if req.Threshold > 500000 {
		req.Threshold = 500000
	}

	if err := store.SaveCompressSettings(&req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
