package api

import (
	"ai-hub/server/core"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetStatus(c *gin.Context) {
	status := core.Deps.GetStatus()
	c.JSON(http.StatusOK, status)
}

func RetryInstall(c *gin.Context) {
	core.Deps.RetryInstall()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
