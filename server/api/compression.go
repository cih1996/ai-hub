package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetCompressionSettings(c *gin.Context) {
	settings, err := store.GetCompressionSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func PutCompressionSettings(c *gin.Context) {
	var settings model.CompressionSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := store.UpsertCompressionSettings(&settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings)
}
