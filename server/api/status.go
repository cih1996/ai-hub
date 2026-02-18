package api

import (
	"ai-hub/server/core"
	"net/http"

	"github.com/gin-gonic/gin"
)

var appVersion = "dev"

func SetVersion(v string) { appVersion = v }

func GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": appVersion})
}

func GetStatus(c *gin.Context) {
	status := core.Deps.GetStatus()
	c.JSON(http.StatusOK, status)
}

func RetryInstall(c *gin.Context) {
	core.Deps.RetryInstall()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
