package api

import (
	"ai-hub/server/core"
	"ai-hub/server/store"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListInjectionRoutes handles GET /api/v1/injection-router
func ListInjectionRoutes(c *gin.Context) {
	routes, err := store.ListInjectionRoutes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if routes == nil {
		routes = []store.InjectionRoute{}
	}
	c.JSON(http.StatusOK, gin.H{
		"routes":     routes,
		"categories": core.AllCategories,
		"fixed":      core.FixedCategories,
		"conditional": core.ConditionalCategories,
	})
}

// CreateInjectionRoute handles POST /api/v1/injection-router
func CreateInjectionRoute(c *gin.Context) {
	var req struct {
		Keywords         string `json:"keywords" binding:"required"`
		InjectCategories string `json:"inject_categories" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}
	route, err := store.CreateInjectionRoute(req.Keywords, req.InjectCategories)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, route)
}

// UpdateInjectionRoute handles PUT /api/v1/injection-router/:id
func UpdateInjectionRoute(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	// Read-then-merge pattern: read existing first
	existing, err := store.GetInjectionRoute(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	var req struct {
		Keywords         *string `json:"keywords"`
		InjectCategories *string `json:"inject_categories"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}

	keywords := existing.Keywords
	categories := existing.InjectCategories
	if req.Keywords != nil {
		keywords = *req.Keywords
	}
	if req.InjectCategories != nil {
		categories = *req.InjectCategories
	}

	if err := store.UpdateInjectionRoute(id, keywords, categories); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteInjectionRoute handles DELETE /api/v1/injection-router/:id
func DeleteInjectionRoute(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := store.DeleteInjectionRoute(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GetStructuredMemory handles GET /api/v1/structured-memory/:category
func GetStructuredMemory(c *gin.Context) {
	category := c.Param("category")
	// Validate category
	valid := false
	for _, cat := range core.AllCategories {
		if cat == category {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category: " + category})
		return
	}
	content := core.ReadStructuredMemory(category)
	c.JSON(http.StatusOK, gin.H{
		"category": category,
		"label":    core.CategoryLabels[category],
		"content":  content,
	})
}

// PutStructuredMemory handles PUT /api/v1/structured-memory/:category
func PutStructuredMemory(c *gin.Context) {
	category := c.Param("category")
	// Validate category
	valid := false
	for _, cat := range core.AllCategories {
		if cat == category {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category: " + category})
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}
	if err := core.WriteStructuredMemory(category, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListStructuredMemory handles GET /api/v1/structured-memory
func ListStructuredMemory(c *gin.Context) {
	type CategoryInfo struct {
		Category string `json:"category"`
		Label    string `json:"label"`
		HasData  bool   `json:"has_data"`
		Fixed    bool   `json:"fixed"`
	}

	var categories []CategoryInfo
	fixedSet := make(map[string]bool)
	for _, cat := range core.FixedCategories {
		fixedSet[cat] = true
	}

	for _, cat := range core.AllCategories {
		content := core.ReadStructuredMemory(cat)
		categories = append(categories, CategoryInfo{
			Category: cat,
			Label:    core.CategoryLabels[cat],
			HasData:  content != "",
			Fixed:    fixedSet[cat],
		})
	}
	c.JSON(http.StatusOK, gin.H{"categories": categories})
}
