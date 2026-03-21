package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListSchemas returns all schema definitions.
// GET /api/v1/schemas
func ListSchemas(c *gin.Context) {
	list, err := store.ListSchemas()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.Schema{}
	}
	c.JSON(http.StatusOK, list)
}

// GetSchema returns a single schema by name.
// GET /api/v1/schemas/:name
func GetSchema(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	s, err := store.GetSchema(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "schema not found"})
		return
	}
	c.JSON(http.StatusOK, s)
}

// CreateSchema creates a new schema definition.
// POST /api/v1/schemas
func CreateSchema(c *gin.Context) {
	var req struct {
		Name       string          `json:"name"`
		Definition json.RawMessage `json:"definition"`
		Writers    json.RawMessage `json:"writers"` // optional: JSON array of session IDs, e.g. [21,23]
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if len(req.Definition) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "definition is required"})
		return
	}

	// Validate that definition is valid JSON
	var parsed interface{}
	if err := json.Unmarshal(req.Definition, &parsed); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "definition must be valid JSON: " + err.Error()})
		return
	}

	// Validate writers if provided
	writers := ""
	if len(req.Writers) > 0 {
		var writerIDs []int64
		if err := json.Unmarshal(req.Writers, &writerIDs); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "writers must be a JSON array of session IDs: " + err.Error()})
			return
		}
		writers = string(req.Writers)
	}

	// Check for duplicate name
	existing, err := store.GetSchema(req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "schema with name '" + req.Name + "' already exists"})
		return
	}

	s := &model.Schema{
		Name:       req.Name,
		Definition: string(req.Definition),
		Writers:    writers,
	}
	if err := store.CreateSchema(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, s)
}

// DeleteSchema removes a schema by name.
// DELETE /api/v1/schemas/:name
func DeleteSchema(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	// Check if schema exists
	existing, err := store.GetSchema(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "schema not found"})
		return
	}

	if err := store.DeleteSchema(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": name})
}

// UpdateSchema updates a schema definition by name (read-then-merge).
// PUT /api/v1/schemas/:name
func UpdateSchema(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	// Read existing schema first (read-then-merge pattern per API convention)
	existing, err := store.GetSchema(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "schema not found"})
		return
	}

	var req struct {
		Definition json.RawMessage `json:"definition"`
		Writers    json.RawMessage `json:"writers"` // optional: JSON array of session IDs
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Merge: only update fields if provided
	definition := existing.Definition
	if len(req.Definition) > 0 {
		var parsed interface{}
		if err := json.Unmarshal(req.Definition, &parsed); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "definition must be valid JSON: " + err.Error()})
			return
		}
		definition = string(req.Definition)
	}

	writers := existing.Writers
	if len(req.Writers) > 0 {
		var writerIDs []int64
		if err := json.Unmarshal(req.Writers, &writerIDs); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "writers must be a JSON array of session IDs: " + err.Error()})
			return
		}
		writers = string(req.Writers)
	}

	updated, err := store.UpdateSchema(name, definition, writers)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}
