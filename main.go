package main

import (
	"ai-hub/cli"
	"ai-hub/server/api"
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed all:web/dist
var frontendFS embed.FS

//go:embed skills/*
var builtinSkillsFS embed.FS

//go:embed claude/*
var claudeRulesFS embed.FS

//go:embed demo.html
var demoHTML []byte

//go:embed avatars/*
var avatarsFS embed.FS

var (
	Version = "dev"
	BuildAt = ""
)

func main() {
	// Expand PATH early so exec.Command LookPath can find node, npm, etc.
	// Critical for launchd/systemd service mode where PATH is minimal.
	core.InitEnhancedPATH()

	// Set Windows console to UTF-8 to fix Chinese character display
	if runtime.GOOS == "windows" {
		exec.Command("cmd", "/c", "chcp", "65001").Run()
	}

	// Self-install mode: when run without arguments, check and install/upgrade
	if len(os.Args) == 1 {
		if runSelfInstall() {
			return
		}
		// If self-install returns false, continue to start server
	}

	// CLI mode detection: if first arg is not a server flag, route to CLI
	// Server flags: -port, -data, --data-dir
	// CLI triggers: --version, --help, or any non-flag argument
	if len(os.Args) > 1 {
		firstArg := os.Args[1]
		isServerFlag := firstArg == "-port" || firstArg == "-data" || firstArg == "--data-dir"
		if !isServerFlag {
			cli.Version = Version
			os.Exit(cli.Run(os.Args[1:]))
		}
	}

	port := flag.Int("port", 9527, "server port")
	dataDir := flag.String("data", "", "data directory (default: ~/.ai-hub or AI_HUB_DATA_DIR)")
	dataDirLong := flag.String("data-dir", "", "data directory (alias for -data)")
	flag.Parse()

	// Priority: --data-dir > -data > AI_HUB_DATA_DIR > ~/.ai-hub
	effectiveDataDir := *dataDir
	if *dataDirLong != "" {
		effectiveDataDir = *dataDirLong
	}

	// Initialize global data directory (handles env var fallback)
	core.InitDataDir(effectiveDataDir)
	effectiveDataDir = core.GetDataDir()

	// Setup log file: <data-dir>/logs/ai-hub.log (truncate on startup)
	logDir := filepath.Join(effectiveDataDir, "logs")
	os.MkdirAll(logDir, 0755)
	logFile, err := os.OpenFile(filepath.Join(logDir, "ai-hub.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags)
	log.Printf("[main] data directory: %s", effectiveDataDir)

	// Init database
	if err := store.Init(effectiveDataDir); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	defer store.Close()

	// Check dependencies and auto-install claude CLI
	core.Deps.CheckAll()
	core.Deps.AutoInstallClaude()

	// Init template system
	core.InitTemplates(effectiveDataDir)
	core.SetPort(*port)

	// Init persistent process pool
	core.InitPool(core.NewClaudeCodeClient())
	defer core.Pool.ShutdownPool()

	// Register process state change callback for WS broadcast
	core.Pool.OnStateChange = func(hubSessionID int64, alive bool, state string) {
		api.BroadcastProcessState(hubSessionID, alive, state)
	}
	// Inject active-stream checker for non-time-based zombie detection.
	core.Pool.IsStreaming = api.IsSessionStreaming

	// Pass version to API layer
	api.SetVersion(Version)

	// Init API data dir (for skills disable path)
	api.InitDataDir(effectiveDataDir)

	// Pass embedded default templates to API for "restore default" feature
	api.SetDefaultTemplatesFS(claudeRulesFS)

	// Install built-in skills to <data-dir>/skills/
	installBuiltinSkills(effectiveDataDir)

	// Migrate rules/rules/ → rules/ (flatten legacy nested structure)
	migrateNestedRules(effectiveDataDir)

	// Migrate team dirs: <data-dir>/<团队名>/ → <data-dir>/teams/<团队名>/
	migrateTeamDirs(effectiveDataDir)

	// Install default rules (skip if already exists)
	installClaudeRules(effectiveDataDir)

	// Create symlink: <data-dir>/bin/ai-hub → self binary (for CLI in Claude subprocess)
	installCLISymlink(effectiveDataDir)

	// No longer render templates to ~/.claude/ — system prompt is built on-the-fly

	// Clean up legacy ~/.claude/rules/ and ~/.claude/skills/ (migrated to ~/.ai-hub/ since v1.17.0)
	cleanLegacyClaudeDirs()

	// Init Go-native vector engine (no Python dependency)
	core.InitVectorEngine("")
	defer func() {
		if core.Vector != nil {
			core.Vector.Stop()
		}
	}()

	// Start vector file watcher
	vectorWatcher := core.StartVectorWatcher()
	core.Watcher = vectorWatcher // expose globally for SyncFileToVector
	defer vectorWatcher.Stop()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false
	r.Use(gin.Recovery())

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// API routes
	v1 := r.Group("/api/v1")
	{
		// Providers
		v1.GET("/providers", api.ListProviders)
		v1.POST("/providers", api.CreateProvider)
		v1.PUT("/providers/:id", api.UpdateProvider)
		v1.PUT("/providers/:id/default", api.SetProviderDefault)
		v1.DELETE("/providers/:id", api.DeleteProvider)
		v1.GET("/claude/auth-status", api.GetClaudeAuthStatus)

		// Sessions
		v1.GET("/sessions", api.ListSessions)
		v1.POST("/sessions", api.CreateSession)
		v1.GET("/sessions/:id", api.GetSession)
		v1.PUT("/sessions/:id", api.UpdateSession)
		v1.DELETE("/sessions/:id", api.DeleteSession)
		v1.GET("/sessions/:id/messages", api.GetMessages)
		v1.DELETE("/sessions/:id/messages", api.TruncateMessages)
		v1.POST("/sessions/:id/compress", api.CompressSession)
		v1.POST("/sessions/:id/reset", api.ResetSession)
		v1.GET("/sessions/:id/last-request", api.GetLastRawRequest)
		v1.GET("/sessions/:id/messages/:msg_id", api.GetMessageWithContext)
		v1.PUT("/sessions/:id/provider", api.SwitchProvider)
		v1.PUT("/sessions/:id/attention", api.ToggleAttention)
		v1.GET("/sessions/:id/attention-rules", api.GetAttentionRules)
		v1.PUT("/sessions/:id/attention-rules", api.UpdateAttentionRules)

		// AI error tracking
		v1.GET("/sessions/:id/errors", api.GetSessionErrors)
		v1.GET("/stats/errors", api.GetErrorStats)

		// Session health (Issue #213)
		v1.GET("/sessions/:id/health", api.GetSessionHealth)
		v1.PUT("/sessions/:id/health", api.UpdateSessionHealth)

		// Session group transfer
		v1.PUT("/sessions/:id/group", api.UpdateSessionGroup)

		// Groups
		v1.GET("/groups", api.ListGroups)
		v1.POST("/groups", api.CreateGroup)
		v1.GET("/groups/:name", api.GetGroup)
		v1.PUT("/groups/:name", api.UpdateGroup)
		v1.DELETE("/groups/:name", api.DeleteGroup)

		// Avatars
		v1.GET("/avatars", api.ListAvatars)
		v1.POST("/avatars/upload", api.UploadAvatar)

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

		// System init status (for first-run guide)
		v1.GET("/system/init-status", api.GetInitStatus)
		v1.POST("/system/install-dep", api.InstallDep)

		// Skills
		v1.GET("/skills", api.ListSkills)
		v1.GET("/skills/:name", api.GetSkillContent)
		v1.POST("/skills", api.CreateSkill)
		v1.PUT("/skills/:name", api.UpdateSkill)
		v1.DELETE("/skills/:name", api.DeleteSkill)
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

		// Services
		v1.GET("/services", api.ListServices)
		v1.POST("/services", api.CreateService)
		v1.GET("/services/:id", api.GetService)
		v1.PUT("/services/:id", api.UpdateService)
		v1.DELETE("/services/:id", api.DeleteService)
		v1.POST("/services/:id/start", api.StartService)
		v1.POST("/services/:id/stop", api.StopService)
		v1.POST("/services/:id/restart", api.RestartService)
		v1.GET("/services/:id/logs", api.GetServiceLogs)

		// Webhooks (IM platform callbacks)
		v1.POST("/webhook/feishu", api.HandleFeishuWebhook)
		v1.POST("/webhook/qq", api.HandleQQWebhook)

		// Vector engine (Skill tools)
		v1.POST("/vector/search", api.SearchVector)
		v1.POST("/vector/search_memory", api.SearchMemory)
		v1.POST("/vector/read_memory", api.ReadMemory)
		v1.POST("/vector/write_memory", api.WriteMemory)
		v1.POST("/vector/write", api.WriteVector)
		v1.POST("/vector/delete_memory", api.DeleteMemory)
		v1.POST("/vector/delete", api.DeleteVector)
		v1.GET("/vector/stats", api.StatsVector)
		v1.GET("/vector/status", api.VectorStatus)
		v1.GET("/vector/health", api.VectorHealth)
		v1.POST("/vector/restart", api.RestartVector)
		v1.GET("/vector/list", api.ListVectorFiles)
		v1.GET("/vector/list_files", api.ListVectorFilesRich) // Issue #109: rich list with preview+type+source_session_id
		v1.GET("/vector/list_memory", api.ListMemoryFiles)
		v1.POST("/vector/read", api.ReadVector)
		v1.POST("/vector/update_metadata", api.UpdateVectorMetadata)
		v1.POST("/vector/get_doc", api.GetVectorDoc)

		// Export / Import
		v1.GET("/export/session/:id", api.ExportSession)
		v1.GET("/export/team/:name", api.ExportTeam)
		v1.POST("/import", api.ImportArchive)

		// Token usage
		v1.GET("/token-usage/message/:id", api.GetMessageTokenUsage)
		v1.GET("/token-usage/session/:id", api.GetSessionTokenUsage)
		v1.GET("/token-usage/system", api.GetSystemTokenUsage)
		v1.GET("/token-usage/daily", api.GetDailyTokenUsage)
		v1.GET("/token-usage/ranking", api.GetTokenUsageRanking)
		v1.GET("/token-usage/hourly", api.GetHourlyTokenUsage)

		// Global settings
		v1.GET("/settings/compress", api.GetCompressSettings)
		v1.PUT("/settings/compress", api.UpdateCompressSettings)

		// System management (daemon, reload)
		v1.POST("/shutdown", api.Shutdown)
		v1.POST("/reload/vector", api.ReloadVector)
		v1.POST("/reload/config", api.ReloadConfig)
		v1.POST("/reload/skills", api.ReloadSkills)

		// Static mount management
		v1.GET("/mounts", api.ListMounts)
		v1.POST("/mounts", api.CreateMount)
		v1.DELETE("/mounts/:alias", api.DeleteMount)

		// Schemas (JSON Schema definitions for structured memory)
		v1.GET("/schemas", api.ListSchemas)
		v1.GET("/schemas/:name", api.GetSchema)
		v1.POST("/schemas", api.CreateSchema)
		v1.PUT("/schemas/:name", api.UpdateSchema)
		v1.DELETE("/schemas/:name", api.DeleteSchema)

		// File transfer
		v1.POST("/transfer/upload", api.TransferInit)
		v1.PUT("/transfer/upload/:id/chunk", api.TransferChunk)
		v1.POST("/transfer/upload/:id/complete", api.TransferComplete)
		v1.GET("/transfer/status/:id", api.TransferStatus)
		v1.GET("/transfer/list", api.TransferList)
		v1.GET("/transfer/download/:id", api.TransferDownload)
		v1.DELETE("/transfer/delete/:id", api.TransferDelete)

		// Injection router (Issue #210: structured memory injection)
		v1.GET("/injection-router", api.ListInjectionRoutes)
		v1.POST("/injection-router", api.CreateInjectionRoute)
		v1.PUT("/injection-router/:id", api.UpdateInjectionRoute)
		v1.DELETE("/injection-router/:id", api.DeleteInjectionRoute)

		// Structured memory (Issue #210)
		v1.GET("/structured-memory", api.ListStructuredMemory)
		v1.GET("/structured-memory/:category", api.GetStructuredMemory)
		v1.PUT("/structured-memory/:category", api.PutStructuredMemory)

		// Hooks (Issue #211: event hook system)
		v1.GET("/hooks", api.ListHooks)
		v1.GET("/hooks/:id", api.GetHook)
		v1.POST("/hooks", api.CreateHook)
		v1.PUT("/hooks/:id", api.UpdateHook)
		v1.DELETE("/hooks/:id", api.DeleteHook)
		v1.POST("/hooks/:id/enable", api.EnableHook)
		v1.POST("/hooks/:id/disable", api.DisableHook)

		// Changelog (Issue #212: memory change tracking)
		v1.GET("/changelog", api.GetChangelog)
		v1.POST("/changelog/rollback", api.RollbackChangelog)

		// Shadow AI (Issue #215)
		v1.GET("/shadow-ai/status", api.GetShadowAIStatus)
		v1.POST("/shadow-ai/enable", api.EnableShadowAI)
		v1.POST("/shadow-ai/disable", api.DisableShadowAI)
		v1.PUT("/shadow-ai/config", api.UpdateShadowAIConfig)
		v1.GET("/shadow-ai/logs", api.GetShadowAILogs)
		v1.GET("/shadow-ai/metrics", api.GetShadowAIMetrics)
		v1.GET("/shadow-ai/activities", api.GetShadowAIActivities)
		v1.POST("/shadow-ai/activity", api.CreateShadowAIActivity)

		// Anthropic API reverse proxy for precise token metering (Issue #72)
		v1.Any("/proxy/s/:session_id/anthropic/*path", api.HandleAnthropicProxy)
		v1.Any("/proxy/anthropic/*path", api.HandleAnthropicProxy) // legacy compat
	}

	// WebSocket
	r.GET("/ws/chat", api.HandleChat)

	// Static mount serving: /static/:alias/*filepath
	r.GET("/static/:alias/*filepath", api.ServeStaticMount)

	// Serve new version (demo.html) at /new
	r.GET("/new", func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", demoHTML)
	})

	// Serve avatars (embedded defaults)
	r.GET("/avatars/:name", func(c *gin.Context) {
		name := c.Param("name")
		data, err := avatarsFS.ReadFile("avatars/" + name)
		if err != nil {
			c.Status(404)
			return
		}
		c.Data(200, "image/svg+xml", data)
	})

	// Serve custom uploaded avatars from ~/.ai-hub/avatars/
	r.GET("/avatars/custom/:name", func(c *gin.Context) {
		name := c.Param("name")
		homeDir, _ := os.UserHomeDir()
		filePath := filepath.Join(homeDir, ".ai-hub", "avatars", name)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			c.Status(404)
			return
		}
		// Detect content type
		ext := strings.ToLower(filepath.Ext(name))
		contentType := "application/octet-stream"
		switch ext {
		case ".svg":
			contentType = "image/svg+xml"
		case ".png":
			contentType = "image/png"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".gif":
			contentType = "image/gif"
		case ".webp":
			contentType = "image/webp"
		}
		c.File(filePath)
		c.Header("Content-Type", contentType)
	})

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

	// Initialize service manager with WS callback
	core.InitServiceManager(func(svc *model.Service) {
		content := fmt.Sprintf(`{"id":%d,"status":"%s","pid":%d}`, svc.ID, svc.Status, svc.PID)
		api.BroadcastRaw("service_status", content)
	})
	// Auto-start services
	go func() {
		services, _ := store.ListServices()
		for i := range services {
			if services[i].AutoStart {
				log.Printf("[service] auto-starting %q", services[i].Name)
				core.ServiceMgr.Start(&services[i])
			}
		}
	}()

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
		core.StopServiceManager()
		if core.Vector != nil {
			core.Vector.Stop()
		}
		vectorWatcher.Stop()
		core.Pool.ShutdownPool()
		store.Close()
		os.Exit(0)
	}()

	addr := fmt.Sprintf(":%d", *port)

	// Windows environment bootstrap (checks and fixes)
	windowsBootstrap(*port)

	// Check if port is available before starting
	if err := checkPortAvailable(*port); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ 端口 %d 已被占用\n\n", *port)
		fmt.Fprintf(os.Stderr, "解决方案：\n")
		fmt.Fprintf(os.Stderr, "  1. 使用其他端口启动：ai-hub -port 9528\n")
		fmt.Fprintf(os.Stderr, "  2. 查找占用进程：lsof -i:%d (macOS/Linux) 或 netstat -ano | findstr %d (Windows)\n", *port, *port)
		fmt.Fprintf(os.Stderr, "  3. 终止占用进程后重试\n\n")
		os.Exit(1)
	}

	log.Printf("AI Hub running at http://localhost%s", addr)
	fmt.Printf("AI Hub running at http://localhost%s\n", addr)
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

// migrateTeamDirs moves team directories from ~/.ai-hub/<团队名>/ to ~/.ai-hub/teams/<团队名>/.
// This consolidates all team data under a single "teams/" parent to avoid cluttering the root.
func migrateTeamDirs(dataDir string) {
	systemDirs := map[string]bool{
		"rules": true, "knowledge": true, "memory": true,
		"skills": true, "notes": true, "scripts": true,
		"logs": true, "session-rules": true, "models": true, "vector-data": true,
		"team": true, "teams": true,
	}
	teamsDir := filepath.Join(dataDir, "teams")
	os.MkdirAll(teamsDir, 0755)

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || systemDirs[e.Name()] {
			continue
		}
		oldPath := filepath.Join(dataDir, e.Name())
		// Check if it has team subdirectories (memory/ or rules/)
		hasTeamContent := false
		for _, sub := range []string{"memory", "rules"} {
			if info, err2 := os.Stat(filepath.Join(oldPath, sub)); err2 == nil && info.IsDir() {
				hasTeamContent = true
				break
			}
		}
		if !hasTeamContent {
			continue
		}
		newPath := filepath.Join(teamsDir, e.Name())
		if _, err2 := os.Stat(newPath); err2 == nil {
			log.Printf("[migrate] skip team dir (already exists in teams/): %s", e.Name())
			continue
		}
		if err2 := os.Rename(oldPath, newPath); err2 == nil {
			log.Printf("[migrate] moved team dir: %s → teams/%s", e.Name(), e.Name())
		} else {
			log.Printf("[migrate] failed to move team dir %s: %v", e.Name(), err2)
		}
	}
	// Remove empty legacy "team" placeholder directory
	teamDir := filepath.Join(dataDir, "team")
	if ents, err2 := os.ReadDir(teamDir); err2 == nil && len(ents) == 0 {
		os.Remove(teamDir)
	}
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
// on every startup (always overwrite, like installBuiltinSkills). This ensures system-level
// rule updates (e.g. new sections, self-correction mechanism) are applied to all
// existing installations. User customisations should live in separate rule-*.md files.
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
		log.Printf("[rules] installed/updated template %s (%d bytes)", target, len(data))
		count++
		return nil
	})
	log.Printf("[rules] done, installed/updated %d template(s)", count)
}

// installCLIBinary installs CLI binary to ~/.ai-hub/bin/ for Claude subprocess access.
// On Unix: creates symlink ai-hub → current binary
// On Windows: copies binary as ai-hub.exe (symlinks require admin privileges)
func installCLISymlink(dataDir string) {
	binDir := filepath.Join(dataDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		log.Printf("[cli-install] mkdir error: %v", err)
		return
	}
	selfPath, err := os.Executable()
	if err != nil {
		log.Printf("[cli-install] cannot resolve self path: %v", err)
		return
	}
	selfPath, err = filepath.EvalSymlinks(selfPath)
	if err != nil {
		log.Printf("[cli-install] cannot eval symlinks: %v", err)
		return
	}

	// Determine target filename based on OS
	targetName := "ai-hub"
	if runtime.GOOS == "windows" {
		targetName = "ai-hub.exe"
	}
	target := filepath.Join(binDir, targetName)

	// Remove existing file/symlink
	os.Remove(target)

	if runtime.GOOS == "windows" {
		// Windows: copy binary (symlinks require admin privileges)
		if err := copyFile(selfPath, target); err != nil {
			log.Printf("[cli-install] copy error: %v", err)
			return
		}
		log.Printf("[cli-install] copied %s → %s", selfPath, target)
	} else {
		// Unix: create symlink
		if err := os.Symlink(selfPath, target); err != nil {
			log.Printf("[cli-install] symlink error: %v", err)
			return
		}
		log.Printf("[cli-install] symlink %s → %s", target, selfPath)
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

// runSelfInstall handles the self-install/upgrade logic when run without arguments.
// Returns true if the program should exit after this function.
func runSelfInstall() bool {
	selfPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
		return true
	}
	selfPath, _ = filepath.EvalSymlinks(selfPath)

	installPath := getInstallPath()
	if installPath == "" {
		fmt.Fprintf(os.Stderr, "Unsupported platform: %s\n", runtime.GOOS)
		return true
	}

	// Check if service is already installed
	serviceInstalled := isServiceInstalled()

	// Check if already running
	runningVersion := getRunningVersion()

	// Case 1: Service installed and running
	if serviceInstalled && runningVersion != "" {
		// Compare versions
		cmp := compareVersions(Version, runningVersion)
		if cmp > 0 {
			// Current binary is newer, upgrade
			fmt.Printf("Upgrading AI Hub from %s to %s...\n", runningVersion, Version)
			if upgradeService(selfPath, installPath) {
				fmt.Printf("AI Hub upgraded to %s\n", Version)
				openBrowser("http://localhost:9527")
			}
			return true
		} else if cmp < 0 {
			// Running version is newer
			fmt.Printf("Warning: Running version (%s) is newer than this binary (%s)\n", runningVersion, Version)
			fmt.Println("Skipping installation. Use 'ai-hub daemon stop' to stop the running service.")
			return true
		} else {
			// Same version, just open browser
			fmt.Printf("AI Hub %s is already running\n", Version)
			openBrowser("http://localhost:9527")
			return true
		}
	}

	// Case 2: Service installed but not running
	if serviceInstalled && runningVersion == "" {
		fmt.Println("AI Hub service is installed but not running. Starting...")
		startService()
		waitForService(5 * time.Second)
		openBrowser("http://localhost:9527")
		return true
	}

	// Case 3: Not installed, do fresh install
	fmt.Printf("Installing AI Hub %s...\n", Version)
	if installService(selfPath, installPath) {
		fmt.Printf("AI Hub %s installed and started\n", Version)
		waitForService(5 * time.Second)
		openBrowser("http://localhost:9527")
	}
	return true
}

// getInstallPath returns the installation path for the current platform
func getInstallPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "bin", "ai-hub")
	case "linux":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "bin", "ai-hub")
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			home, _ := os.UserHomeDir()
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "ai-hub", "ai-hub.exe")
	default:
		return ""
	}
}

// isServiceInstalled checks if the service is installed
func isServiceInstalled() bool {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.ai-hub.server.plist")
		_, err := os.Stat(plistPath)
		return err == nil
	case "linux":
		home, _ := os.UserHomeDir()
		servicePath := filepath.Join(home, ".config", "systemd", "user", "ai-hub.service")
		_, err := os.Stat(servicePath)
		return err == nil
	case "windows":
		// Check for startup shortcut
		home, _ := os.UserHomeDir()
		shortcutPath := filepath.Join(home, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "AI Hub.lnk")
		_, err := os.Stat(shortcutPath)
		return err == nil
	default:
		return false
	}
}

// getRunningVersion returns the version of the running service, or empty if not running
func getRunningVersion() string {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:9527/api/v1/version")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}
	if v, ok := result["version"].(string); ok {
		return v
	}
	return ""
}

// compareVersions compares two version strings (e.g., "v1.2.3")
// Returns: 1 if a > b, -1 if a < b, 0 if equal
func compareVersions(a, b string) int {
	// Strip 'v' prefix
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	// Simple string comparison for semver
	// For more robust comparison, use a semver library
	if a > b {
		return 1
	} else if a < b {
		return -1
	}
	return 0
}

// installService installs the service for the current platform
func installService(srcPath, dstPath string) bool {
	// Ensure directory exists
	os.MkdirAll(filepath.Dir(dstPath), 0755)

	// Copy binary
	if srcPath != dstPath {
		if err := copyFile(srcPath, dstPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to copy binary: %v\n", err)
			return false
		}
		os.Chmod(dstPath, 0755)
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchdService(dstPath)
	case "linux":
		return installSystemdService(dstPath)
	case "windows":
		return installWindowsService(dstPath)
	default:
		return false
	}
}

// upgradeService upgrades the running service
func upgradeService(srcPath, dstPath string) bool {
	// Stop the service first
	stopService()
	time.Sleep(1 * time.Second)

	// Copy new binary
	if srcPath != dstPath {
		if err := copyFile(srcPath, dstPath); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to copy binary: %v\n", err)
			return false
		}
		os.Chmod(dstPath, 0755)
	}

	// Start the service
	startService()
	return true
}

// startService starts the service
func startService() {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("launchctl", "start", "com.ai-hub.server").Run()
	case "linux":
		exec.Command("systemctl", "--user", "start", "ai-hub").Run()
	case "windows":
		// Use VBS launcher script (consistent with installWindowsService)
		home, _ := os.UserHomeDir()
		vbsPath := filepath.Join(home, ".ai-hub", "ai-hub-launcher.vbs")
		exec.Command("wscript", vbsPath).Start()
	}
}

// stopService stops the service gracefully
func stopService() {
	// Try graceful shutdown via API first
	client := &http.Client{Timeout: 2 * time.Second}
	client.Post("http://localhost:9527/api/v1/shutdown", "application/json", nil)
	time.Sleep(500 * time.Millisecond)

	// Fallback to platform-specific stop
	switch runtime.GOOS {
	case "darwin":
		exec.Command("launchctl", "stop", "com.ai-hub.server").Run()
	case "linux":
		exec.Command("systemctl", "--user", "stop", "ai-hub").Run()
	case "windows":
		exec.Command("schtasks", "/End", "/TN", "AIHub").Run()
	}
}

// waitForService waits for the service to become available
func waitForService(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://localhost:9527/api/v1/version")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// openBrowser opens the default browser to the given URL
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	cmd.Start()
}

// installLaunchdService installs launchd service on macOS
func installLaunchdService(binaryPath string) bool {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".ai-hub")
	logPath := filepath.Join(dataDir, "logs", "ai-hub.log")
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.ai-hub.server.plist")

	os.MkdirAll(filepath.Dir(logPath), 0755)
	os.MkdirAll(filepath.Dir(plistPath), 0755)

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ai-hub.server</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>-port</string>
        <string>9527</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>%s/.local/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    </dict>
</dict>
</plist>`, binaryPath, logPath, logPath, dataDir, home)

	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write plist: %v\n", err)
		return false
	}

	exec.Command("launchctl", "unload", plistPath).Run()
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load service: %v\n", err)
		return false
	}

	return true
}

// installSystemdService installs systemd user service on Linux
func installSystemdService(binaryPath string) bool {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".ai-hub")
	servicePath := filepath.Join(home, ".config", "systemd", "user", "ai-hub.service")

	os.MkdirAll(filepath.Dir(servicePath), 0755)

	service := fmt.Sprintf(`[Unit]
Description=AI Hub Server
After=network.target

[Service]
Type=simple
ExecStart=%s -port 9527
WorkingDirectory=%s
Restart=always
RestartSec=5
Environment=PATH=/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=default.target
`, binaryPath, dataDir)

	if err := os.WriteFile(servicePath, []byte(service), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write service file: %v\n", err)
		return false
	}

	exec.Command("systemctl", "--user", "daemon-reload").Run()
	exec.Command("systemctl", "--user", "enable", "ai-hub").Run()
	exec.Command("systemctl", "--user", "start", "ai-hub").Run()

	// Enable lingering
	user := os.Getenv("USER")
	if user != "" {
		exec.Command("loginctl", "enable-linger", user).Run()
	}

	// Add ~/.local/bin to PATH in shell rc files
	addLinuxPathConfig(home)

	return true
}

// addLinuxPathConfig adds ~/.local/bin to PATH in .bashrc and .zshrc
func addLinuxPathConfig(home string) {
	localBin := filepath.Join(home, ".local", "bin")
	pathLine := fmt.Sprintf(`export PATH="$HOME/.local/bin:$PATH"`)
	marker := ".local/bin"

	// Check and update .bashrc
	bashrc := filepath.Join(home, ".bashrc")
	if _, err := os.Stat(bashrc); err == nil {
		if !fileContains(bashrc, marker) {
			appendToFile(bashrc, "\n# Added by AI Hub\n"+pathLine+"\n")
			fmt.Println("Added ~/.local/bin to PATH in ~/.bashrc")
		}
	}

	// Check and update .zshrc
	zshrc := filepath.Join(home, ".zshrc")
	if _, err := os.Stat(zshrc); err == nil {
		if !fileContains(zshrc, marker) {
			appendToFile(zshrc, "\n# Added by AI Hub\n"+pathLine+"\n")
			fmt.Println("Added ~/.local/bin to PATH in ~/.zshrc")
		}
	}

	// Also create .profile if neither exists (for login shells)
	if _, err := os.Stat(bashrc); os.IsNotExist(err) {
		if _, err := os.Stat(zshrc); os.IsNotExist(err) {
			profile := filepath.Join(home, ".profile")
			if !fileContains(profile, marker) {
				appendToFile(profile, "\n# Added by AI Hub\n"+pathLine+"\n")
				fmt.Println("Added ~/.local/bin to PATH in ~/.profile")
			}
		}
	}

	// Ensure the directory exists
	os.MkdirAll(localBin, 0755)
}

// fileContains checks if a file contains a substring
func fileContains(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
}

// appendToFile appends content to a file
func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// installWindowsService installs AI Hub to start on Windows login
// Uses Startup folder shortcut (no admin required) instead of Task Scheduler
func installWindowsService(binaryPath string) bool {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".ai-hub")

	os.MkdirAll(dataDir, 0755)

	// Get Startup folder path
	startupDir := filepath.Join(home, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	os.MkdirAll(startupDir, 0755)

	// Create a VBS script to launch AI Hub hidden (no console window)
	vbsPath := filepath.Join(dataDir, "ai-hub-launcher.vbs")
	vbsContent := fmt.Sprintf(`Set WshShell = CreateObject("WScript.Shell")
WshShell.Run """%s"" -port 9527", 0, False
`, strings.ReplaceAll(binaryPath, `\`, `\\`))

	if err := os.WriteFile(vbsPath, []byte(vbsContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write launcher script: %v\n", err)
		return false
	}

	// Create shortcut in Startup folder pointing to the VBS script
	shortcutPath := filepath.Join(startupDir, "AI Hub.lnk")

	// Use PowerShell to create shortcut
	psScript := fmt.Sprintf(`
$WshShell = New-Object -ComObject WScript.Shell
$Shortcut = $WshShell.CreateShortcut('%s')
$Shortcut.TargetPath = '%s'
$Shortcut.WorkingDirectory = '%s'
$Shortcut.Description = 'AI Hub Server'
$Shortcut.Save()
`, strings.ReplaceAll(shortcutPath, `'`, `''`),
		strings.ReplaceAll(vbsPath, `'`, `''`),
		strings.ReplaceAll(dataDir, `'`, `''`))

	cmd := exec.Command("powershell", "-Command", psScript)
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create startup shortcut: %v\n%s\n", err, output)
		return false
	}

	fmt.Printf("Created startup shortcut: %s\n", shortcutPath)

	// Start AI Hub now (hidden)
	cmd = exec.Command("wscript", vbsPath)
	cmd.Start()

	// Add install directory to user PATH
	addWindowsPathConfig(filepath.Dir(binaryPath))

	return true
}

// addWindowsPathConfig adds the install directory to user PATH on Windows
func addWindowsPathConfig(installDir string) {
	// Get current user PATH
	cmd := exec.Command("powershell", "-Command",
		"[Environment]::GetEnvironmentVariable('PATH', 'User')")
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to get user PATH: %v\n", err)
		return
	}

	currentPath := strings.TrimSpace(string(output))

	// Check if already in PATH (case-insensitive on Windows)
	pathLower := strings.ToLower(currentPath)
	installDirLower := strings.ToLower(installDir)
	if strings.Contains(pathLower, installDirLower) {
		return // Already in PATH
	}

	// Add to PATH using setx
	var newPath string
	if currentPath == "" {
		newPath = installDir
	} else {
		newPath = installDir + ";" + currentPath
	}

	// setx has a 1024 character limit, use PowerShell for longer paths
	if len(newPath) > 1024 {
		cmd = exec.Command("powershell", "-Command",
			fmt.Sprintf("[Environment]::SetEnvironmentVariable('PATH', '%s', 'User')",
				strings.ReplaceAll(newPath, "'", "''")))
	} else {
		cmd = exec.Command("setx", "PATH", newPath)
	}

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to add to PATH: %v\n", err)
		return
	}

	fmt.Printf("Added %s to user PATH\n", installDir)
	fmt.Println("Note: Restart your terminal for PATH changes to take effect")
}


// checkPortAvailable checks if a port is available for binding
func checkPortAvailable(port int) error {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	ln.Close()
	return nil
}

// windowsBootstrap performs Windows-specific environment checks and fixes
// Called at startup on Windows to ensure proper environment
func windowsBootstrap(port int) {
	if runtime.GOOS != "windows" {
		return
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  AI Hub 环境检测")
	fmt.Println("═══════════════════════════════════════════════════════════")

	// 1. Check admin privileges (informational only)
	checkWindowsAdmin()

	// 2. Check and fix PowerShell execution policy
	checkWindowsPowerShellPolicy()

	// 3. Check for Chinese characters in path
	checkWindowsChinesePath()

	// 4. Check Node.js
	checkWindowsNodeJS()

	fmt.Println()
	fmt.Printf("  启动服务中... http://localhost:%d\n", port)
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println()
}

// checkWindowsAdmin checks if running with admin privileges
func checkWindowsAdmin() {
	cmd := exec.Command("powershell", "-Command",
		"([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)")
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("  [?] 管理员权限：无法检测")
		return
	}

	isAdmin := strings.TrimSpace(string(output)) == "True"
	if isAdmin {
		fmt.Println("  [✓] 管理员权限：已获取")
	} else {
		fmt.Println("  [i] 管理员权限：普通用户（大部分功能正常）")
	}
}

// checkWindowsPowerShellPolicy checks and fixes PowerShell execution policy
func checkWindowsPowerShellPolicy() {
	cmd := exec.Command("powershell", "-Command", "Get-ExecutionPolicy -Scope CurrentUser")
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("  [?] PowerShell 执行策略：无法检测")
		return
	}

	policy := strings.TrimSpace(string(output))
	if policy == "Restricted" || policy == "AllSigned" {
		// Try to fix
		fixCmd := exec.Command("powershell", "-Command",
			"Set-ExecutionPolicy RemoteSigned -Scope CurrentUser -Force")
		if err := fixCmd.Run(); err != nil {
			fmt.Printf("  [✗] PowerShell 执行策略：%s（自动修复失败，npm 可能无法运行）\n", policy)
		} else {
			fmt.Println("  [✓] PowerShell 执行策略：已自动修复为 RemoteSigned")
		}
	} else {
		fmt.Printf("  [✓] PowerShell 执行策略：%s\n", policy)
	}
}

// checkWindowsChinesePath checks for Chinese characters in user path
func checkWindowsChinesePath() {
	home, _ := os.UserHomeDir()
	exe, _ := os.Executable()

	// Check for non-ASCII characters
	hasNonASCII := false
	for _, r := range home + exe {
		if r > 127 {
			hasNonASCII = true
			break
		}
	}

	if hasNonASCII {
		fmt.Println("  [!] 路径检测：包含中文字符，可能影响部分功能")
		fmt.Println("      建议将 AI Hub 安装到 C:\\ai-hub\\ 目录")
	} else {
		fmt.Println("  [✓] 路径检测：正常")
	}
}

// checkWindowsNodeJS checks if Node.js is installed
func checkWindowsNodeJS() {
	cmd := exec.Command("node", "--version")
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("  [✗] Node.js：未安装 → 请在 Web 界面完成安装")
	} else {
		version := strings.TrimSpace(string(output))
		fmt.Printf("  [✓] Node.js：%s\n", version)
	}
}
