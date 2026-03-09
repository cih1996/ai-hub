package api

import (
	"ai-hub/server/core"
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

var shutdownServer func()

// SetShutdownFunc sets the function to call for graceful shutdown
func SetShutdownFunc(fn func()) {
	shutdownServer = fn
}

// Shutdown gracefully shuts down the server
// POST /api/v1/shutdown
func Shutdown(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "Server shutting down..."})

	// Shutdown in background after response is sent
	go func() {
		time.Sleep(100 * time.Millisecond)
		if shutdownServer != nil {
			shutdownServer()
		} else {
			// Fallback: send SIGTERM to self
			p, _ := os.FindProcess(os.Getpid())
			p.Signal(syscall.SIGTERM)
		}
	}()
}

// ReloadVector reloads the vector engine
// POST /api/v1/reload/vector
func ReloadVector(c *gin.Context) {
	if core.Vector == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Vector engine not initialized"})
		return
	}

	// Reinitialize vector engine
	go func() {
		core.Vector.Reload()
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Vector engine reload initiated"})
}

// ReloadConfig reloads configuration (placeholder)
// POST /api/v1/reload/config
func ReloadConfig(c *gin.Context) {
	// Currently no hot-reloadable config
	// This is a placeholder for future use
	c.JSON(http.StatusOK, gin.H{"message": "Configuration reloaded (no changes)"})
}

// ReloadSkills reloads skill definitions
// POST /api/v1/reload/skills
func ReloadSkills(c *gin.Context) {
	// Skills are read from filesystem on each request
	// This endpoint can be used to clear any caches if added later
	c.JSON(http.StatusOK, gin.H{"message": "Skills reloaded"})
}

// SetupGracefulShutdown sets up signal handlers for graceful shutdown
func SetupGracefulShutdown(srv *http.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			os.Exit(1)
		}
	}()

	SetShutdownFunc(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	})
}
