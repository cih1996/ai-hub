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
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	"github.com/gin-gonic/gin"
)

//go:embed all:web/dist
var frontendFS embed.FS

//go:embed skills/*
var builtinSkillsFS embed.FS

//go:embed claude/*
var claudeRulesFS embed.FS

//go:embed vector-engine/*
var vectorEngineFS embed.FS

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

	// Setup log file: ~/.ai-hub/logs/ai-hub.log (truncate on startup)
	logDir := filepath.Join(*dataDir, "logs")
	os.MkdirAll(logDir, 0755)
	logFile, err := os.OpenFile(filepath.Join(logDir, "ai-hub.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags)

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

	// Init persistent process pool
	core.InitPool(core.NewClaudeCodeClient())
	defer core.Pool.ShutdownPool()

	// Register process state change callback for WS broadcast
	core.Pool.OnStateChange = func(hubSessionID int64, alive bool, state string) {
		api.BroadcastProcessState(hubSessionID, alive, state)
	}

	// Pass version to API layer
	api.SetVersion(Version)

	// Init API data dir (for skills disable path)
	api.InitDataDir(*dataDir)

	// Pass embedded default templates to API for "restore default" feature
	api.SetDefaultTemplatesFS(claudeRulesFS)

	// Install built-in skills to ~/.ai-hub/skills/
	installBuiltinSkills(*dataDir)

	// Migrate rules/rules/ → rules/ (flatten legacy nested structure)
	migrateNestedRules(*dataDir)

	// Install default rules (skip if already exists)
	installClaudeRules(*dataDir)

	// No longer render templates to ~/.claude/ — system prompt is built on-the-fly

	// Clean up legacy ~/.claude/rules/ and ~/.claude/skills/ (migrated to ~/.ai-hub/ since v1.17.0)
	cleanLegacyClaudeDirs()

	// Install vector engine scripts and start engine
	vectorScriptDir := installVectorEngine()
	core.InitVectorEngine(vectorScriptDir)
	defer func() {
		if core.Vector != nil {
			core.Vector.Stop()
		}
	}()

	// Start vector file watcher
	vectorWatcher := core.StartVectorWatcher()
	defer vectorWatcher.Stop()

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
		v1.GET("/claude/auth-status", api.GetClaudeAuthStatus)

		// Sessions
		v1.GET("/sessions", api.ListSessions)
		v1.POST("/sessions", api.CreateSession)
		v1.GET("/sessions/:id", api.GetSession)
		v1.PUT("/sessions/:id", api.UpdateSession)
		v1.DELETE("/sessions/:id", api.DeleteSession)
		v1.GET("/sessions/:id/messages", api.GetMessages)
		v1.POST("/sessions/:id/compress", api.CompressSession)

		// Session rules
		v1.GET("/session-rules/:id", api.GetSessionRules)
		v1.PUT("/session-rules/:id", api.PutSessionRules)
		v1.DELETE("/session-rules/:id", api.DeleteSessionRules)

		// Chat
		v1.POST("/chat/send", api.SendChat)

		// Files (manage page)
		v1.GET("/files", api.ListFiles)
		v1.GET("/files/content", api.ReadFile)
		v1.PUT("/files/content", api.WriteFile)
		v1.POST("/files", api.CreateFile)
		v1.DELETE("/files", api.DeleteFile)
		v1.GET("/files/variables", api.GetTemplateVars)
		v1.GET("/files/default", api.GetDefaultFile)

		// Project-level rules
		v1.GET("/project-rules", api.ListProjectRules)
		v1.GET("/project-rules/content", api.ReadProjectRule)
		v1.PUT("/project-rules/content", api.WriteProjectRule)

		// Status & deps
		v1.GET("/status", api.GetStatus)
		v1.POST("/status/retry-install", api.RetryInstall)
		v1.GET("/version", api.GetVersion)

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

		// Channels (IM gateway)
		v1.GET("/channels", api.ListChannels)
		v1.POST("/channels", api.CreateChannel)
		v1.PUT("/channels/:id", api.UpdateChannel)
		v1.DELETE("/channels/:id", api.DeleteChannel)

		// Webhooks (IM platform callbacks)
		v1.POST("/webhook/feishu", api.HandleFeishuWebhook)
		v1.POST("/webhook/qq", api.HandleQQWebhook)

		// Vector engine (Skill tools)
		v1.POST("/vector/search", api.SearchVector)
		v1.POST("/vector/search_knowledge", api.SearchKnowledge)
		v1.POST("/vector/search_memory", api.SearchMemory)
		v1.POST("/vector/read_knowledge", api.ReadKnowledge)
		v1.POST("/vector/read_memory", api.ReadMemory)
		v1.POST("/vector/write_knowledge", api.WriteKnowledge)
		v1.POST("/vector/write_memory", api.WriteMemory)
		v1.POST("/vector/delete_knowledge", api.DeleteKnowledge)
		v1.POST("/vector/delete_memory", api.DeleteMemory)
		v1.GET("/vector/stats", api.StatsVector)
		v1.GET("/vector/status", api.VectorStatus)
		v1.GET("/vector/health", api.VectorHealth)
		v1.POST("/vector/restart", api.RestartVector)

		// Token usage
		v1.GET("/token-usage/message/:id", api.GetMessageTokenUsage)
		v1.GET("/token-usage/session/:id", api.GetSessionTokenUsage)
		v1.GET("/token-usage/system", api.GetSystemTokenUsage)
		v1.GET("/token-usage/daily", api.GetDailyTokenUsage)
		v1.GET("/token-usage/ranking", api.GetTokenUsageRanking)
		v1.GET("/token-usage/hourly", api.GetHourlyTokenUsage)

		// Anthropic API reverse proxy for precise token metering (Issue #72)
		v1.Any("/proxy/anthropic/*path", api.HandleAnthropicProxy)
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

	// Start QQ WebSocket client connections for enabled channels
	api.LogQQDedupConfig()
	api.QQWSMgr.StartAll()

	// Signal handling: ensure cleanup on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Println("[main] shutting down...")
		api.QQWSMgr.Shutdown()
		if core.Vector != nil {
			core.Vector.Stop()
		}
		vectorWatcher.Stop()
		core.Pool.ShutdownPool()
		store.Close()
		os.Exit(0)
	}()

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("AI Hub running at http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

// installBuiltinSkills copies embedded skills/* to ~/.ai-hub/skills/ on every startup.
func installBuiltinSkills(dataDir string) {
	targetBase := filepath.Join(dataDir, "skills")

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

// installVectorEngine extracts embedded vector-engine/* to ~/.ai-hub/vector-engine/scripts/
// and returns the script directory path.
func installVectorEngine() string {
	home, _ := os.UserHomeDir()
	targetBase := filepath.Join(home, ".ai-hub", "vector-engine", "scripts")

	fs.WalkDir(vectorEngineFS, "vector-engine", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel("vector-engine", p)
		if rel == "." {
			return nil
		}
		target := filepath.Join(targetBase, rel)
		if d.IsDir() {
			os.MkdirAll(target, 0755)
			return nil
		}
		data, err := vectorEngineFS.ReadFile(p)
		if err != nil {
			return nil
		}
		os.MkdirAll(filepath.Dir(target), 0755)
		os.WriteFile(target, data, 0644)
		return nil
	})
	log.Printf("[vector] scripts installed to %s", targetBase)
	return targetBase
}

// migrateNestedRules moves files from rules/rules/ to rules/ (flatten).
func migrateNestedRules(dataDir string) {
	nested := filepath.Join(dataDir, "rules", "rules")
	if _, err := os.Stat(nested); os.IsNotExist(err) {
		return
	}
	entries, err := os.ReadDir(nested)
	if err != nil {
		return
	}
	target := filepath.Join(dataDir, "rules")
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		src := filepath.Join(nested, e.Name())
		dst := filepath.Join(target, e.Name())
		if _, err := os.Stat(dst); err == nil {
			log.Printf("[rules] migrate skip (exists): %s", dst)
			continue
		}
		if err := os.Rename(src, dst); err == nil {
			log.Printf("[rules] migrated: %s → %s", src, dst)
		}
	}
	// Remove empty nested dir
	os.Remove(nested)
	log.Println("[rules] migrated rules/rules/ → rules/")
}

// cleanLegacyClaudeDirs removes residual rules/ and skills/ directories under ~/.claude/
// that were created by versions prior to v1.17.0 (data migrated to ~/.ai-hub/).
func cleanLegacyClaudeDirs() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	for _, sub := range []string{"rules", "skills"} {
		dir := filepath.Join(home, ".claude", sub)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			if err := os.RemoveAll(dir); err != nil {
				log.Printf("[cleanup] failed to remove legacy %s: %v", dir, err)
			} else {
				log.Printf("[cleanup] removed legacy directory: %s", dir)
			}
		}
	}
}

// installClaudeRules copies embedded claude/* to the rules directory (~/.ai-hub/rules/)
// only if the target rule does not exist. System prompt is built on-the-fly via BuildSystemPrompt().
func installClaudeRules(dataDir string) {
	targetBase := filepath.Join(dataDir, "rules")
	log.Printf("[rules] template dir: %s", targetBase)

	// Clean up legacy split rule files (merged back into CLAUDE.md in #30)
	legacyFiles := []string{"rule-ai-behavior.md", "rule-memory-knowledge.md"}
	for _, name := range legacyFiles {
		p := filepath.Join(targetBase, name)
		if _, err := os.Stat(p); err == nil {
			os.Remove(p)
			log.Printf("[rules] removed legacy file: %s", p)
		}
	}

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
