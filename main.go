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
	"mime"
	"os"
	"path"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

//go:embed all:web/dist
var frontendFS embed.FS

//go:embed skills/*
var builtinSkillsFS embed.FS

//go:embed claude/*
var claudeRulesFS embed.FS

var (
	Version = "dev"
	BuildAt = ""
)

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

	// Init template system
	core.InitTemplates(*dataDir)
	core.SetPort(*port)

	// Init API data dir (for skills disable path)
	api.InitDataDir(*dataDir)

	// Install built-in skills to ~/.claude/skills/
	installBuiltinSkills()

	// Install default CLAUDE.md rules (skip if already exists)
	installClaudeRules()

	// Render templates to ~/.claude/ on startup
	core.RenderAllTemplates()

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

		// Files (manage page)
		v1.GET("/files", api.ListFiles)
		v1.GET("/files/content", api.ReadFile)
		v1.PUT("/files/content", api.WriteFile)
		v1.POST("/files", api.CreateFile)
		v1.DELETE("/files", api.DeleteFile)
		v1.GET("/files/variables", api.GetTemplateVars)

		// Project-level rules
		v1.GET("/project-rules", api.ListProjectRules)
		v1.GET("/project-rules/content", api.ReadProjectRule)
		v1.PUT("/project-rules/content", api.WriteProjectRule)

		// Status & deps
		v1.GET("/status", api.GetStatus)
		v1.POST("/status/retry-install", api.RetryInstall)

		// Skills
		v1.GET("/skills", api.ListSkills)
		v1.POST("/skills/toggle", api.ToggleSkill)

		// MCP
		v1.GET("/mcp", api.ListMcp)
		v1.POST("/mcp/toggle", api.ToggleMcp)

		// Triggers
		v1.GET("/triggers", api.ListTriggers)
		v1.POST("/triggers", api.CreateTrigger)
		v1.PUT("/triggers/:id", api.UpdateTrigger)
		v1.DELETE("/triggers/:id", api.DeleteTrigger)
	}

	// WebSocket
	r.GET("/ws/chat", api.HandleChat)

	// Serve frontend
	distFS, err := fs.Sub(frontendFS, "web/dist")
	if err != nil {
		log.Fatalf("Failed to load frontend: %v", err)
	}
	// Pre-read index.html for SPA fallback (avoid http.FileServer redirect loop)
	indexHTML, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		log.Fatalf("Failed to read index.html: %v", err)
	}
	r.NoRoute(func(c *gin.Context) {
		urlPath := c.Request.URL.Path
		// Try serving static file directly
		if urlPath != "/" && urlPath != "/index.html" {
			name := urlPath[1:] // strip leading /
			if data, err := fs.ReadFile(distFS, name); err == nil {
				ctype := mime.TypeByExtension(path.Ext(name))
				if ctype == "" {
					ctype = "application/octet-stream"
				}
				c.Data(200, ctype, data)
				return
			}
		}
		// SPA fallback
		c.Data(200, "text/html; charset=utf-8", indexHTML)
	})

	// Start trigger scheduler
	core.StartTriggerLoop(*port)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("AI Hub running at http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

// installBuiltinSkills copies embedded skills/* to ~/.claude/skills/ on every startup.
func installBuiltinSkills() {
	home, _ := os.UserHomeDir()
	targetBase := filepath.Join(home, ".claude", "skills")

	fs.WalkDir(builtinSkillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		// Strip the "skills/" prefix to get relative path
		rel, _ := filepath.Rel("skills", path)
		if rel == "." {
			return nil
		}
		target := filepath.Join(targetBase, rel)

		if d.IsDir() {
			os.MkdirAll(target, 0755)
			return nil
		}
		data, err := builtinSkillsFS.ReadFile(path)
		if err != nil {
			return nil
		}
		os.MkdirAll(filepath.Dir(target), 0755)
		os.WriteFile(target, data, 0644)
		return nil
	})
	log.Printf("[skills] built-in skills installed to %s", targetBase)
}

// installClaudeRules copies embedded claude/* to the templates directory (~/.ai-hub/templates/)
// only if the target template does not exist. The template system (RenderAllTemplates) will
// render these to ~/.claude/ with fresh variables on every chat message.
func installClaudeRules() {
	home, _ := os.UserHomeDir()
	targetBase := filepath.Join(home, ".ai-hub", "templates")
	log.Printf("[rules] template dir: %s", targetBase)

	count := 0
	fs.WalkDir(claudeRulesFS, "claude", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("claude", p)
		target := filepath.Join(targetBase, rel)
		log.Printf("[rules] embed path=%s rel=%s target=%s", p, rel, target)

		// Skip if template source already exists
		if _, err := os.Stat(target); err == nil {
			log.Printf("[rules] skip (exists): %s", target)
			return nil
		}

		data, err := claudeRulesFS.ReadFile(p)
		if err != nil {
			log.Printf("[rules] read embed error: %v", err)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			log.Printf("[rules] mkdir error: %v", err)
			return nil
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			log.Printf("[rules] write error: %v", err)
			return nil
		}
		log.Printf("[rules] installed template %s (%d bytes)", target, len(data))
		count++
		return nil
	})
	log.Printf("[rules] done, installed %d template(s)", count)
}
