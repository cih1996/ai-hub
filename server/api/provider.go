package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Provider handlers

func ListProviders(c *gin.Context) {
	list, err := store.ListProviders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.Provider{}
	}
	c.JSON(http.StatusOK, list)
}

func CreateProvider(c *gin.Context) {
	var p model.Provider
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := store.CreateProvider(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, p)
}

func UpdateProvider(c *gin.Context) {
	id := c.Param("id")
	existing, err := store.GetProvider(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}
	if err := c.ShouldBindJSON(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.ID = id
	if err := store.UpdateProvider(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func DeleteProvider(c *gin.Context) {
	id := c.Param("id")
	if err := store.DeleteProvider(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
