package main

import (
	"ai-hub/server/api"
	"ai-hub/server/core"
	"ai-hub/server/store"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

//go:embed web/dist/*
var frontendFS embed.FS

func main() {
	port := flag.Int("port", 8080, "server port")
	dataDir := flag.String("data", "", "data directory (default: ~/.ai-hub)")
	flag.Parse()

	if *dataDir == "" {
		home, _ := os.UserHomeDir()
		*dataDir = filepath.Join(home, ".ai-hub")
	}

	// Init database
	if err := store.Init(*dataDir); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	defer store.Close()

	// Check dependencies and auto-install claude CLI
	core.Deps.CheckAll()
	core.Deps.AutoInstallClaude()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	r.Use(gin.Recovery())

	// API routes
	v1 := r.Group("/api/v1")
	{
		// Providers
		v1.GET("/providers", api.ListProviders)
		v1.POST("/providers", api.CreateProvider)
		v1.PUT("/providers/:id", api.UpdateProvider)
		v1.DELETE("/providers/:id", api.DeleteProvider)

		// Sessions
		v1.GET("/sessions", api.ListSessions)
		v1.POST("/sessions", api.CreateSession)
		v1.GET("/sessions/:id", api.GetSession)
		v1.PUT("/sessions/:id", api.UpdateSession)
		v1.DELETE("/sessions/:id", api.DeleteSession)
		v1.GET("/sessions/:id/messages", api.GetMessages)

		// Chat
		v1.POST("/chat/send", api.SendChat)

		// Status & deps
		v1.GET("/status", api.GetStatus)
		v1.POST("/status/retry-install", api.RetryInstall)
	}

	// WebSocket
	r.GET("/ws/chat", api.HandleChat)

	// Serve frontend
	distFS, err := fs.Sub(frontendFS, "web/dist")
	if err != nil {
		log.Fatalf("Failed to load frontend: %v", err)
	}
	r.NoRoute(func(c *gin.Context) {
		// Try to serve static file first
		path := c.Request.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		f, err := distFS.Open(path[1:]) // strip leading /
		if err == nil {
			f.Close()
			c.FileFromFS(path, http.FS(distFS))
			return
		}
		// SPA fallback: serve index.html for all non-API routes
		c.FileFromFS("/index.html", http.FS(distFS))
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("AI Hub running at http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
