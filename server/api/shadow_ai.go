package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ShadowAIConfig holds the shadow AI configuration
type ShadowAIConfig struct {
	PatrolInterval          string `json:"patrol_interval"`           // e.g. "10m"
	ExtractInterval         string `json:"extract_interval"`          // e.g. "1h"
	DeepScanInterval        string `json:"deep_scan_interval"`        // e.g. "6h"
	SelfCleanInterval       string `json:"self_clean_interval"`       // e.g. "24h"
	ErrorCorrectionInterval string `json:"error_correction_interval"` // e.g. "30m"
	ContextResetThreshold   int    `json:"context_reset_threshold"`   // e.g. 50
}

// ShadowAIStatus holds the shadow AI status response
type ShadowAIStatus struct {
	Enabled       bool            `json:"enabled"`
	SessionID     int64           `json:"session_id"`
	Status        string          `json:"status"` // "running" | "paused" | "uninitialized"
	Config        ShadowAIConfig  `json:"config"`
	Triggers      []model.Trigger `json:"triggers,omitempty"`
	CreatedAt     string          `json:"created_at,omitempty"`     // 影子AI创建时间
	LastActivity  string          `json:"last_activity,omitempty"`  // 最后活动时间
	UptimeSeconds int64           `json:"uptime_seconds,omitempty"` // 运行时长（秒）
}

var defaultShadowConfig = ShadowAIConfig{
	PatrolInterval:          "10m",
	ExtractInterval:         "1h",
	DeepScanInterval:        "6h",
	SelfCleanInterval:       "24h",
	ErrorCorrectionInterval: "30m",
	ContextResetThreshold:   50,
}

// Trigger content prefixes for matching triggers to config intervals
var triggerPrefixes = []struct {
	prefix    string
	configKey string
}{
	{"【定时巡检】", "patrol"},
	{"【定时提炼】", "extract"},
	{"【深度巡检】", "deep_scan"},
	{"【自我清理】", "self_clean"},
	{"【错误纠正】", "error_correction"},
}

// GetShadowAIStatus handles GET /api/v1/shadow-ai/status
func GetShadowAIStatus(c *gin.Context) {
	status := loadShadowStatus()
	c.JSON(http.StatusOK, status)
}

// EnableShadowAI handles POST /api/v1/shadow-ai/enable
func EnableShadowAI(c *gin.Context) {
	// Check if already enabled
	existing := loadShadowStatus()
	if existing.Enabled {
		c.JSON(http.StatusOK, gin.H{
			"ok":         true,
			"message":    "shadow AI already enabled",
			"session_id": existing.SessionID,
		})
		return
	}

	// Accept optional config override
	var reqConfig ShadowAIConfig
	if err := c.ShouldBindJSON(&reqConfig); err != nil {
		reqConfig = defaultShadowConfig
	}
	// Fill defaults for zero values
	if reqConfig.PatrolInterval == "" {
		reqConfig.PatrolInterval = defaultShadowConfig.PatrolInterval
	}
	if reqConfig.ExtractInterval == "" {
		reqConfig.ExtractInterval = defaultShadowConfig.ExtractInterval
	}
	if reqConfig.DeepScanInterval == "" {
		reqConfig.DeepScanInterval = defaultShadowConfig.DeepScanInterval
	}
	if reqConfig.SelfCleanInterval == "" {
		reqConfig.SelfCleanInterval = defaultShadowConfig.SelfCleanInterval
	}
	if reqConfig.ErrorCorrectionInterval == "" {
		reqConfig.ErrorCorrectionInterval = defaultShadowConfig.ErrorCorrectionInterval
	}
	if reqConfig.ContextResetThreshold <= 0 {
		reqConfig.ContextResetThreshold = defaultShadowConfig.ContextResetThreshold
	}

	// === Re-enable path: reuse existing session if available ===
	if existing.SessionID > 0 {
		oldSession, err := store.GetSession(existing.SessionID)
		if err == nil && oldSession != nil {
			log.Printf("[shadow-ai] re-enabling with existing session #%d", existing.SessionID)

			// Clean up old triggers (delete all, will recreate below)
			oldTriggers, _ := store.ListTriggersBySession(existing.SessionID)
			for _, t := range oldTriggers {
				store.DeleteTrigger(t.ID)
			}
			log.Printf("[shadow-ai] cleaned up %d old triggers", len(oldTriggers))

			// Create new triggers with current config
			now := time.Now().In(time.FixedZone("CST", 8*3600))
			triggerDefs := []struct {
				interval string
				content  string
			}{
				{reqConfig.PatrolInterval, "【定时巡检】快速巡检：扫描所有活跃会话的错误统计、用户纠正，记录异常。"},
				{reqConfig.ExtractInterval, "【定时提炼】记忆提炼：从最近对话中提取有价值的用户偏好、习惯、纠正内容，写入结构化记忆。"},
				{reqConfig.DeepScanInterval, "【深度巡检】全面检查所有会话健康度、Schema演进、记忆一致性。"},
				{reqConfig.SelfCleanInterval, "【自我清理】归档工作日志，清理过期临时数据，更新 shadow/status.md。"},
			{reqConfig.ErrorCorrectionInterval, "【错误纠正】错误纠正：识别反复出现的错误模式，自动修改会话规则以预防同类错误。"},
			}
			var triggerIDs []int64
			for _, td := range triggerDefs {
				t := &model.Trigger{
					SessionID:   existing.SessionID,
					Content:     td.content,
					TriggerTime: td.interval,
					MaxFires:    -1,
					Enabled:     true,
					Status:      "active",
					CreatedAt:   now.Format("2006-01-02 15:04:05"),
					UpdatedAt:   now.Format("2006-01-02 15:04:05"),
				}
				if err := store.CreateTrigger(t); err != nil {
					log.Printf("[shadow-ai] failed to create trigger: %v", err)
					continue
				}
				triggerIDs = append(triggerIDs, t.ID)
			}

		// Update rules file with latest template
		rulesContent := generateShadowRules(existing.SessionID)
		rulesDir := filepath.Join(core.GetDataDir(), "session-rules")
		os.MkdirAll(rulesDir, 0755)
		rulesPath := filepath.Join(rulesDir, fmt.Sprintf("%d.md", existing.SessionID))
		if err := os.WriteFile(rulesPath, []byte(rulesContent), 0644); err != nil {
			log.Printf("[shadow-ai] failed to update rules: %v", err)
		}

		// Ensure all shadow memory files exist (create missing ones)
		ensureShadowMemoryFiles(existing.SessionID, now, reqConfig)

		// Update config and status
			saveShadowSettings(true, existing.SessionID, reqConfig)

			log.Printf("[shadow-ai] re-enabled: session=%d, triggers=%v", existing.SessionID, triggerIDs)
			c.JSON(http.StatusOK, gin.H{
				"ok":         true,
				"session_id": existing.SessionID,
				"triggers":   triggerIDs,
				"config":     reqConfig,
				"reused":     true,
			})
			return
		}
		// Old session not found, fall through to full creation
		log.Printf("[shadow-ai] old session #%d not found, creating new", existing.SessionID)
	}

	// === Full creation flow (first-time enable) ===

	// Step 1: Create shadow AI session (with IsShadow flag)
	session := &model.Session{
		Title:    "影子AI",
		IsShadow: true,
	}
	if err := store.CreateSession(session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session: " + err.Error()})
		return
	}
	log.Printf("[shadow-ai] created session #%d", session.ID)

	// Step 2: Auto-assign default Provider
	if p, err := store.GetDefaultProvider(); err == nil && p != nil {
		store.UpdateSessionProvider(session.ID, p.ID)
		log.Printf("[shadow-ai] assigned default provider %s to session #%d", p.ID, session.ID)
	} else {
		// Fallback: use first available provider
		providers, _ := store.ListProviders()
		if len(providers) > 0 {
			store.UpdateSessionProvider(session.ID, providers[0].ID)
			log.Printf("[shadow-ai] assigned fallback provider %s to session #%d", providers[0].ID, session.ID)
		} else {
			log.Printf("[shadow-ai] WARNING: no providers available, shadow AI may not function")
		}
	}

	// Step 3: Write preset rules
	rulesContent := generateShadowRules(session.ID)
	rulesDir := filepath.Join(core.GetDataDir(), "session-rules")
	os.MkdirAll(rulesDir, 0755)
	rulesPath := filepath.Join(rulesDir, fmt.Sprintf("%d.md", session.ID))
	if err := os.WriteFile(rulesPath, []byte(rulesContent), 0644); err != nil {
		log.Printf("[shadow-ai] failed to write rules: %v", err)
	}

	// Step 4: Set auto_reset_threshold
	store.UpdateAutoResetThreshold(session.ID, reqConfig.ContextResetThreshold)

	// Step 5: Register multi-frequency triggers
	now := time.Now().In(time.FixedZone("CST", 8*3600))
	triggerDefs := []struct {
		interval string
		content  string
	}{
		{reqConfig.PatrolInterval, "【定时巡检】快速巡检：扫描所有活跃会话的错误统计、用户纠正，记录异常。"},
		{reqConfig.ExtractInterval, "【定时提炼】记忆提炼：从最近对话中提取有价值的用户偏好、习惯、纠正内容，写入结构化记忆。"},
		{reqConfig.DeepScanInterval, "【深度巡检】全面检查所有会话健康度、Schema演进、记忆一致性。"},
		{reqConfig.SelfCleanInterval, "【自我清理】归档工作日志，清理过期临时数据，更新 shadow/status.md。"},
		{reqConfig.ErrorCorrectionInterval, "【错误纠正】错误纠正：识别反复出现的错误模式，自动修改会话规则以预防同类错误。"},
	}

	var triggerIDs []int64
	for _, td := range triggerDefs {
		t := &model.Trigger{
			SessionID:   session.ID,
			Content:     td.content,
			TriggerTime: td.interval,
			MaxFires:    -1, // infinite
			Enabled:     true,
			Status:      "active",
			CreatedAt:   now.Format("2006-01-02 15:04:05"),
			UpdatedAt:   now.Format("2006-01-02 15:04:05"),
		}
		if err := store.CreateTrigger(t); err != nil {
			log.Printf("[shadow-ai] failed to create trigger %s: %v", td.interval, err)
			continue
		}
		triggerIDs = append(triggerIDs, t.ID)
	}

	// Step 6: Initialize shadow AI self-management memory files
	initShadowMemoryFiles(session.ID, now, reqConfig)

	// Step 7: Create default injection routes (only if none exist)
	existingRoutes, _ := store.ListInjectionRoutes()
	if len(existingRoutes) == 0 {
		defaultRoutes := []struct {
			keywords   string
			categories string
		}{
			{"开发|编程|代码|bug|fix|功能|feature", "domain,lessons,active"},
			{"部署|上线|发布|运维|服务器", "domain,lessons,decisions"},
			{"设计|架构|方案|评审|选型", "domain,decisions"},
			{"错误|失败|问题|异常|报错", "lessons,error-genome"},
		}
		for _, dr := range defaultRoutes {
			store.CreateInjectionRoute(dr.keywords, dr.categories)
		}
		log.Printf("[shadow-ai] created %d default injection routes", len(defaultRoutes))
	} else {
		log.Printf("[shadow-ai] skipped injection routes creation, %d already exist", len(existingRoutes))
	}

	// Step 8: Save config to settings
	saveShadowSettings(true, session.ID, reqConfig)

	log.Printf("[shadow-ai] enabled: session=%d, triggers=%v", session.ID, triggerIDs)

	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"session_id": session.ID,
		"triggers":   triggerIDs,
		"config":     reqConfig,
	})
}

// DisableShadowAI handles POST /api/v1/shadow-ai/disable
func DisableShadowAI(c *gin.Context) {
	status := loadShadowStatus()
	if !status.Enabled {
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "shadow AI already disabled"})
		return
	}

	// Delete all triggers for the shadow session (not just disable)
	triggers, _ := store.ListTriggersBySession(status.SessionID)
	for _, t := range triggers {
		store.DeleteTrigger(t.ID)
	}

	// Kill any running process
	core.Pool.Kill(status.SessionID)

	// Mark as disabled (keep session and data)
	saveShadowSettings(false, status.SessionID, status.Config)

	log.Printf("[shadow-ai] disabled: session=%d, deleted %d triggers", status.SessionID, len(triggers))

	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"session_id": status.SessionID,
		"message":    "shadow AI disabled, session preserved",
	})
}

// UpdateShadowAIConfig handles PUT /api/v1/shadow-ai/config
func UpdateShadowAIConfig(c *gin.Context) {
	status := loadShadowStatus()
	if !status.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "shadow AI not enabled"})
		return
	}

	var req ShadowAIConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Merge: only update non-zero fields
	if req.PatrolInterval != "" {
		status.Config.PatrolInterval = req.PatrolInterval
	}
	if req.ExtractInterval != "" {
		status.Config.ExtractInterval = req.ExtractInterval
	}
	if req.DeepScanInterval != "" {
		status.Config.DeepScanInterval = req.DeepScanInterval
	}
	if req.SelfCleanInterval != "" {
		status.Config.SelfCleanInterval = req.SelfCleanInterval
	}
	if req.ContextResetThreshold > 0 {
		status.Config.ContextResetThreshold = req.ContextResetThreshold
		store.UpdateAutoResetThreshold(status.SessionID, req.ContextResetThreshold)
	}

	// Save updated config
	saveShadowSettings(status.Enabled, status.SessionID, status.Config)

	// Update trigger intervals to match new config
	triggers, _ := store.ListTriggersBySession(status.SessionID)
	intervalMap := map[string]string{
		"patrol":    status.Config.PatrolInterval,
		"extract":   status.Config.ExtractInterval,
		"deep_scan": status.Config.DeepScanInterval,
		"self_clean": status.Config.SelfCleanInterval,
	}

	for i := range triggers {
		for _, tp := range triggerPrefixes {
			if strings.HasPrefix(triggers[i].Content, tp.prefix) {
				newInterval := intervalMap[tp.configKey]
				if newInterval != "" && newInterval != triggers[i].TriggerTime {
					triggers[i].TriggerTime = newInterval
					triggers[i].UpdatedAt = time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
					store.UpdateTrigger(&triggers[i])
					log.Printf("[shadow-ai] updated trigger #%d interval to %s", triggers[i].ID, newInterval)
				}
				break
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "config": status.Config})
}

// GetShadowAILogs handles GET /api/v1/shadow-ai/logs
func GetShadowAILogs(c *gin.Context) {
	logPath := filepath.Join(core.GetDataDir(), "memory", "shadow", "work-log.md")
	data, err := os.ReadFile(logPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"content": "", "exists": false})
		return
	}

	content := string(data)
	linesParam := c.DefaultQuery("lines", "0")
	maxLines, _ := strconv.Atoi(linesParam)
	if maxLines > 0 {
		content = lastNLines(content, maxLines)
	}

	c.JSON(http.StatusOK, gin.H{"content": content, "exists": true})
}

// GetShadowAIMetrics handles GET /api/v1/shadow-ai/metrics
func GetShadowAIMetrics(c *gin.Context) {
	// Count structured memory categories with data
	memoryCount := 0
	categories := []string{"identity", "preferences", "error-genome", "domain", "lessons", "active", "decisions"}
	for _, cat := range categories {
		content := core.ReadStructuredMemory(cat)
		if content != "" {
			memoryCount++
		}
	}

	// Count injection router rules
	routes, _ := store.ListInjectionRoutes()
	routerCount := len(routes)

	// Count session health
	sessions, _ := store.ListSessions()
	healthy, warning, errorCount := 0, 0, 0
	for _, s := range sessions {
		if s.HealthScore == "green" {
			healthy++
		} else if s.HealthScore == "yellow" {
			warning++
		} else if s.HealthScore == "red" {
			errorCount++
		}
	}

	// Get last patrol time from shadow_activities
	var lastPatrol string
	row := store.DB.QueryRow("SELECT timestamp FROM shadow_activities WHERE type = 'patrol' ORDER BY timestamp DESC LIMIT 1")
	row.Scan(&lastPatrol)

	c.JSON(http.StatusOK, gin.H{
		"memory_count": memoryCount,
		"router_count": routerCount,
		"session_health": gin.H{
			"healthy": healthy,
			"warning": warning,
			"error":   errorCount,
		},
		"last_patrol": lastPatrol,
	})
}

// GetShadowAIActivities handles GET /api/v1/shadow-ai/activities
func GetShadowAIActivities(c *gin.Context) {
	limit := 10
	offset := 0

	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Query activities
	rows, err := store.DB.Query("SELECT timestamp, type, summary, details FROM shadow_activities ORDER BY timestamp DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	activities := []map[string]interface{}{}
	for rows.Next() {
		var timestamp, actType, summary string
		var details *string
		rows.Scan(&timestamp, &actType, &summary, &details)

		activity := map[string]interface{}{
			"timestamp": timestamp,
			"type":      actType,
			"summary":   summary,
		}
		if details != nil {
			activity["details"] = *details
		}
		activities = append(activities, activity)
	}

	// Count total
	var total int
	store.DB.QueryRow("SELECT COUNT(*) FROM shadow_activities").Scan(&total)

	c.JSON(http.StatusOK, gin.H{
		"activities": activities,
		"total":      total,
	})
}

// CreateShadowAIActivity handles POST /api/v1/shadow-ai/activity
func CreateShadowAIActivity(c *gin.Context) {
	var req struct {
		Type    string `json:"type" binding:"required"`
		Summary string `json:"summary" binding:"required"`
		Details string `json:"details"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}

	// Validate type
	validTypes := []string{"patrol", "extract", "deep_scan", "self_clean"}
	valid := false
	for _, t := range validTypes {
		if req.Type == t {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type"})
		return
	}

	// Insert activity
	timestamp := time.Now().Format(time.RFC3339)
	_, err := store.DB.Exec("INSERT INTO shadow_activities (timestamp, type, summary, details, created_at) VALUES (?, ?, ?, ?, ?)",
		timestamp, req.Type, req.Summary, req.Details, timestamp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// loadShadowStatus reads the shadow AI state from settings
func loadShadowStatus() ShadowAIStatus {
	status := ShadowAIStatus{
		Config: defaultShadowConfig,
		Status: "uninitialized",
	}

	enabledStr, _ := store.GetSetting("shadow_ai.enabled")
	status.Enabled = enabledStr == "true"

	sidStr, _ := store.GetSetting("shadow_ai.session_id")
	if sidStr != "" {
		if id, err := strconv.ParseInt(sidStr, 10, 64); err == nil {
			status.SessionID = id
		}
	}

	configStr, _ := store.GetSetting("shadow_ai.config")
	if configStr != "" {
		json.Unmarshal([]byte(configStr), &status.Config)
	}

	if status.Enabled && status.SessionID > 0 {
		status.Status = "running"

		// Get session info to fill created_at and calculate uptime
		if session, err := store.GetSession(status.SessionID); err == nil && session != nil {
			// Fill created_at (convert time.Time to string)
			status.CreatedAt = session.CreatedAt.Format("2006-01-02 15:04:05")

			// Calculate uptime_seconds
			status.UptimeSeconds = int64(time.Since(session.CreatedAt).Seconds())
		}

		// Check if triggers are active and get last activity time
		triggers, _ := store.ListTriggersBySession(status.SessionID)
		status.Triggers = triggers

		// Find the latest last_fired_at as last_activity
		var latestActivity time.Time
		for _, t := range triggers {
			if t.LastFiredAt != "" {
				if firedTime, err := time.Parse("2006-01-02 15:04:05", t.LastFiredAt); err == nil {
					if firedTime.After(latestActivity) {
						latestActivity = firedTime
					}
				}
			}
		}

		if !latestActivity.IsZero() {
			status.LastActivity = latestActivity.Format("2006-01-02 15:04:05")
		}

		// Check if all triggers are disabled
		allDisabled := true
		for _, t := range triggers {
			if t.Enabled {
				allDisabled = false
				break
			}
		}
		if allDisabled && len(triggers) > 0 {
			status.Status = "paused"
		}
	} else if !status.Enabled && status.SessionID > 0 {
		status.Status = "paused"
	}

	return status
}

// saveShadowSettings persists shadow AI state to settings table
func saveShadowSettings(enabled bool, sessionID int64, config ShadowAIConfig) {
	enabledStr := "false"
	if enabled {
		enabledStr = "true"
	}
	store.SetSetting("shadow_ai.enabled", enabledStr)
	store.SetSetting("shadow_ai.session_id", strconv.FormatInt(sessionID, 10))
	configJSON, _ := json.Marshal(config)
	store.SetSetting("shadow_ai.config", string(configJSON))
}

// initShadowMemoryFiles creates initial self-management files for shadow AI
func initShadowMemoryFiles(sessionID int64, now time.Time, config ShadowAIConfig) {
	memDir := filepath.Join(core.GetDataDir(), "memory", "shadow")
	os.MkdirAll(memDir, 0755)

	files := map[string]string{
		"status.md": fmt.Sprintf(`# 影子AI状态

- 启动时间: %s
- 会话ID: %d
- 状态: 已启动
- 最后巡检: 无
- 最后提炼: 无
- 最后深扫: 无
- 最后清理: 无
`, now.Format("2006-01-02 15:04:05"), sessionID),

		"work-log.md": fmt.Sprintf(`# 影子AI工作日志

## %s
- [%s] 系统初始化完成，会话 #%d
`, now.Format("2006-01-02"), now.Format("15:04:05"), sessionID),

		"patrol-result.md": "# 最近巡检结果\n\n暂无巡检记录。\n",

		"rule-changes.md": `# 影子AI规则修改历史

暂无规则修改记录。

## 记录格式示例

` + "```markdown" + `
## 2026-03-23 02:30:00
- 会话ID: 123
- 原因: 反复出现路径错误（错误ID: #456, #457, #458）
- 修改: 添加"禁止使用相对路径"规则
- 预期效果: 减少路径相关错误
` + "```" + `
`,

		"config.md": fmt.Sprintf(`# 影子AI运行配置

- 巡检间隔: %s
- 提炼间隔: %s
- 深扫间隔: %s
- 清理间隔: %s
- 重置阈值: %d
`, config.PatrolInterval, config.ExtractInterval, config.DeepScanInterval, config.SelfCleanInterval, config.ContextResetThreshold),
	}

	for name, content := range files {
		path := filepath.Join(memDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			log.Printf("[shadow-ai] failed to write memory file %s: %v", name, err)
		}
	}
	log.Printf("[shadow-ai] initialized %d memory files", len(files))
}

// ensureShadowMemoryFiles checks and creates missing shadow memory files (for re-enable path)
func ensureShadowMemoryFiles(sessionID int64, now time.Time, config ShadowAIConfig) {
	memDir := filepath.Join(core.GetDataDir(), "memory", "shadow")
	os.MkdirAll(memDir, 0755)

	// Define all required files with their default content
	files := map[string]string{
		"status.md": fmt.Sprintf(`# 影子AI状态

- 启动时间: %s
- 会话ID: %d
- 状态: 已启动
- 最后巡检: 无
- 最后提炼: 无
- 最后深扫: 无
- 最后清理: 无
`, now.Format("2006-01-02 15:04:05"), sessionID),

		"work-log.md": fmt.Sprintf(`# 影子AI工作日志

## %s
- [%s] 系统初始化完成，会话 #%d
`, now.Format("2006-01-02"), now.Format("15:04:05"), sessionID),

		"patrol-result.md": "# 最近巡检结果\n\n暂无巡检记录。\n",

		"rule-changes.md": `# 影子AI规则修改历史

暂无规则修改记录。

## 记录格式示例

` + "```markdown" + `
## 2026-03-23 02:30:00
- 会话ID: 123
- 原因: 反复出现路径错误（错误ID: #456, #457, #458）
- 修改: 添加"禁止使用相对路径"规则
- 预期效果: 减少路径相关错误
` + "```" + `
`,

		"config.md": fmt.Sprintf(`# 影子AI运行配置

- 巡检间隔: %s
- 提炼间隔: %s
- 深扫间隔: %s
- 清理间隔: %s
- 重置阈值: %d
`, config.PatrolInterval, config.ExtractInterval, config.DeepScanInterval, config.SelfCleanInterval, config.ContextResetThreshold),
	}

	// Only create files that don't exist
	createdCount := 0
	for name, content := range files {
		path := filepath.Join(memDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				log.Printf("[shadow-ai] failed to create missing file %s: %v", name, err)
			} else {
				createdCount++
				log.Printf("[shadow-ai] created missing file: %s", name)
			}
		}
	}
	if createdCount > 0 {
		log.Printf("[shadow-ai] ensured %d missing memory files", createdCount)
	}
}

// lastNLines returns the last n lines of a string
func lastNLines(s string, n int) string {
	scanner := bufio.NewScanner(strings.NewReader(s))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// getDefaultShadowRules returns the default shadow AI rules template
func getDefaultShadowRules() string {
	return `# 影子AI — 全局记忆管理者 & 会话健康守护者

## 身份定义
你是影子AI（Shadow AI），系统级智能代理。你的职责是：
- 守护所有会话的健康状态
- 提炼和管理结构化记忆
- 巡检错误和偏离
- 维护系统级知识的一致性

## 工作流程
每次被唤醒时：
1. 读取 shadow/status.md 了解上次工作状态
2. 根据触发类型执行对应任务
3. 更新状态文件，记录工作日志
4. 写完状态文件才算任务完成

## 巡检清单
- 错误统计：ai-hub errors（检查新增错误/警告）
- 纠正次数：各会话的 correction_count 变化
- 规则偏离：检查 drift_count 异常
- 健康度：调用 /sessions/:id/health 评估

## 记忆提炼标准
**值得提炼的：**
- 用户偏好（语言、格式、工具偏好）
- 工作习惯和流程
- 被纠正的内容（教训）
- 反复出现的知识点

**不记录的：**
- 临时闲聊
- 一次性查询
- 已过时的信息

## 结构化记忆管理（核心职责）

你负责维护 7 大结构化记忆分类，这些记忆会自动注入到所有会话的 system prompt 中。

### 7 个分类说明

**固定分类（始终注入每个会话）：**
- **identity** — 用户身份画像：姓名、角色、公司、时区、语言、工作方式
- **preferences** — 用户偏好习惯：编码风格、交互偏好、工具偏好、厌恶的做法
- **error-genome** — AI 常犯错误模式库：被纠正的错误、典型失败案例、禁止事项

**条件分类（按关键词匹配注入）：**
- **domain** — 用户领域知识：技术栈、架构模式、专业术语、行业背景
- **lessons** — 踩过的坑和教训：历史失败案例、避坑指南
- **active** — 当前进行中的事项：正在做的项目、待办任务、进度状态
- **decisions** — 重要决策记录：架构决策、技术选型、方案评审结果

### 写入方法（重要：必须严格按此格式）

**步骤1：读取现有内容**
` + "`" + `bash
curl -s http://localhost:{{AI_HUB_PORT}}/api/v1/structured-memory/<category>
` + "`" + `
返回 JSON：` + "`" + `{"category": "identity", "label": "用户身份画像", "content": "现有内容"}` + "`" + `

**步骤2：合并新内容**
- 如果现有内容为空，直接写入新内容
- 如果已有内容，追加新发现的信息（不要重复已有的）
- 保持 Markdown 格式，用标题和列表组织

**步骤3：写回**
` + "`" + `bash
curl -X PUT http://localhost:{{AI_HUB_PORT}}/api/v1/structured-memory/<category> \
  -H 'Content-Type: application/json' \
  -d '{"content": "合并后的完整内容"}'
` + "`" + `

**内容格式示例（identity）：**
` + "`" + `markdown
# 用户身份

- 姓名：张三
- 角色：全栈开发工程师
- 公司：某科技公司
- 时区：UTC+8（中国）
- 语言：中文为主，英文技术文档
- 工作方式：远程办公，晚上效率高
` + "`" + `

**内容格式示例（preferences）：**
` + "`" + `markdown
# 编码偏好

- 语言：Go、TypeScript、Python
- 框架：Gin、Vue3、FastAPI
- 代码风格：简洁优先，避免过度抽象
- 注释：关键逻辑必须注释，简单代码不注释

# 交互偏好

- 先结论后证据
- 代码示例必须完整可运行
- 避免冗长解释，直接给方案
` + "`" + `

**内容格式示例（error-genome）：**
` + "`" + `markdown
# 常犯错误模式

## 类型1：路径处理
- 错误：使用相对路径导致找不到文件
- 正确：始终用绝对路径或 filepath.Join
- 案例：Issue #123

## 类型2：并发安全
- 错误：多个 goroutine 同时写 map 导致 panic
- 正确：用 sync.RWMutex 保护共享数据
- 案例：会话 #456 崩溃
` + "`" + `

### 提炼规则（每次【定时提炼】触发时执行）

1. 列出最近活跃的会话（最近1小时有消息的）：
   ` + "`" + `bash
   ai-hub sessions | grep -v "idle"
   ` + "`" + `

2. 读取这些会话的最近消息（最后20条）：
   ` + "`" + `bash
   ai-hub sessions <id> messages --limit 20
   ` + "`" + `

3. 从对话中提取以下信息：
   - 用户被纠正的内容 → **error-genome**
   - 用户明确表达的偏好 → **preferences**
   - 用户透露的身份信息 → **identity**
   - 讨论的专业知识点 → **domain**
   - 遇到的问题和解决方案 → **lessons**
   - 提到的进行中项目 → **active**
   - 做出的技术决策 → **decisions**

4. 对每个分类：
   - 先读取现有内容（步骤1）
   - 合并新提取的信息（步骤2）
   - 写回完整内容（步骤3）

5. 验证写入成功：
   ` + "`" + `bash
   curl -s http://localhost:{{AI_HUB_PORT}}/api/v1/structured-memory/<category> | grep "新增的关键词"
   ` + "`" + `

### 注入路由管理（可选，首次启动已自动创建）

查看现有路由：
` + "`" + `bash
curl -s http://localhost:{{AI_HUB_PORT}}/api/v1/injection-router
` + "`" + `

如需新增关键词映射：
` + "`" + `bash
curl -X POST http://localhost:{{AI_HUB_PORT}}/api/v1/injection-router \
  -H 'Content-Type: application/json' \
  -d '{"keywords": "新关键词|同义词", "inject_categories": "domain,lessons"}'
` + "`" + `

## 自管理文件更新（每次任务后必做）

你有 4 个自管理文件，存储在全局记忆的 shadow 目录下。每次任务完成后必须更新。

### 文件1：memory/shadow/status.md（工作状态）

**更新时机**：每次任务完成后

**内容格式**：
` + "`" + `markdown
# 影子AI状态

- 启动时间: 2026-03-22 03:00:00
- 会话ID: {{SESSION_ID}}
- 状态: 运行中
- 最后巡检: 2026-03-22 04:32:27（第8次）
- 最后提炼: 2026-03-22 04:12:27（第2次）
- 最后深扫: 无
- 最后清理: 无

## 当前基线数据

- 总会话数: 50
- 错误会话数: 20
- 总错误数: 9
- 总警告数: 15
` + "`" + `

**更新方法**：
` + "`" + `bash
# 先读取现有内容
curl -s http://localhost:{{AI_HUB_PORT}}/api/v1/files/content?path=memory/shadow/status.md

# 更新对应字段后写回
curl -X PUT http://localhost:{{AI_HUB_PORT}}/api/v1/files/content \
  -H 'Content-Type: application/json' \
  -d '{"path": "memory/shadow/status.md", "content": "更新后的完整内容"}'
` + "`" + `

### 文件2：memory/shadow/work-log.md（工作日志）

**更新时机**：每次任务完成后追加一条

**内容格式**：
` + "`" + `markdown
# 影子AI工作日志

## 2026-03-22
- [03:00:59] 系统初始化完成，会话 #{{SESSION_ID}}
- [03:22:27] 巡检 #1：发现 20 个会话有错误，3 个严重
- [04:01:32] 提炼 #1：从 5 个会话提取记忆，更新 domain/lessons
- [04:22:27] 巡检 #2：无新增异常，系统静默

## 2026-03-23
- [10:00:00] 深扫 #1：全面检查 50 个会话健康度
` + "`" + `

**更新方法（追加模式）**：
` + "`" + `bash
# 读取现有日志
curl -s http://localhost:{{AI_HUB_PORT}}/api/v1/files/content?path=memory/shadow/work-log.md

# 在末尾追加新条目后写回
curl -X PUT http://localhost:{{AI_HUB_PORT}}/api/v1/files/content \
  -H 'Content-Type: application/json' \
  -d '{"path": "memory/shadow/work-log.md", "content": "原内容 + 新条目"}'
` + "`" + `

### 文件3：memory/shadow/patrol-result.md（最近巡检结果）

**更新时机**：每次巡检后覆盖

**内容格式**：
` + "`" + `markdown
# 最近巡检结果

**时间**：2026-03-22 04:32:27
**类型**：快速巡检 #8

## 发现

- 总会话：50
- 错误会话：20（无变化）
- 新增错误：0
- 新增警告：0

## 结论

✅ 系统静默，无需干预
` + "`" + `

**更新方法**：
` + "`" + `bash
curl -X PUT http://localhost:{{AI_HUB_PORT}}/api/v1/files/content \
  -H 'Content-Type: application/json' \
  -d '{"path": "memory/shadow/patrol-result.md", "content": "最新巡检结果"}'
` + "`" + `

### 文件4：shadow-config.md（运行配置）

**更新时机**：配置变更时（很少）

**内容格式**：已在初始化时写入，一般不需要更新。

### 重要提醒

- 所有自管理文件都存储在 ` + "`" + `memory/shadow/` + "`" + ` 目录下
- 使用 HTTP 文件 API 而非 CLI，因为 API 支持子目录结构
- 文件路径格式：` + "`" + `memory/shadow/status.md` + "`" + `、` + "`" + `memory/shadow/work-log.md` + "`" + ` 等
- 每次任务完成后至少更新 status 和 work-log 两个文件
- 更新前先读取，避免覆盖丢失数据

## 干预分级
- 🟢 记录：静默写入记忆库，不打扰任何会话
- 🟡 提醒：发消息到异常会话提醒注意
- 🔴 告警：通知用户（发送到用户主会话）

## 权限约束
- 可以读取其他会话的消息（GET /sessions/:id/messages）
- 可以读写记忆库（ai-hub search/write/read）
- 可以设置健康度（PUT /sessions/:id/health）
- 可以写入结构化记忆（PUT /structured-memory/:category）
- 可以管理注入路由（POST/PUT/DELETE /injection-router）
- **可以**修改其他会话的规则（用于错误纠正和优化）
- **禁止**删除其他会话的消息

### 会话规则修改指南

**使用场景：**
- 发现会话反复犯同类错误，需要在规则中添加禁止事项
- 会话健康度持续低于阈值，需要调整规则优化交互
- 用户偏好发生变化，需要更新会话规则

**操作步骤：**

1. 读取现有规则：
` + "`" + `bash
ai-hub rules get <session_id>
` + "`" + `

2. 分析问题并设计修改方案（必须基于具体错误证据）

3. 修改规则（追加模式，不要删除现有规则）：
` + "`" + `bash
ai-hub rules set <session_id> --content "原规则内容 + 新增规则"
` + "`" + `

4. 记录修改历史到 memory/shadow/rule-changes.md：
` + "`" + `markdown
## 2026-03-23 02:30:00
- 会话ID: 123
- 原因: 反复出现路径错误（错误ID: #456, #457, #458）
- 修改: 添加"禁止使用相对路径"规则
- 预期效果: 减少路径相关错误
` + "`" + `

**修改原则：**
- 必须基于具体错误证据（至少3次同类错误）
- 只追加规则，不删除现有规则
- 修改后必须记录到 rule-changes.md
- 每次修改后观察至少1小时，评估效果

## 自我管理
- 不依赖上下文历史，全靠记忆库
- 每次唤醒先从记忆库恢复状态
- 上下文阈值已设置为自动重置
- 你的会话ID是 {{SESSION_ID}}

## 活动记录（重要：每次任务完成后必须记录）

每次执行巡检、提炼、深度扫描、自清理任务后，必须调用活动记录接口：

` + "`" + `bash
curl -X POST http://localhost:{{AI_HUB_PORT}}/api/v1/shadow-ai/activity \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "patrol",
    "summary": "巡检完成：发现 3 个新增错误",
    "details": "详细信息（可选）"
  }'
` + "`" + `

**活动类型（type）：**
- ` + "`" + `patrol` + "`" + ` — 定时巡检
- ` + "`" + `extract` + "`" + ` — 定时提炼
- ` + "`" + `deep_scan` + "`" + ` — 深度巡检
- ` + "`" + `self_clean` + "`" + ` — 自我清理
- ` + "`" + `error_correction` + "`" + ` — 错误纠正

**summary 格式建议：**
- 巡检：` + "`" + `巡检 #8：发现 3 个新增错误，2 个警告` + "`" + `
- 提炼：` + "`" + `提炼 #2：从 5 个会话提取记忆，更新 domain/lessons` + "`" + `
- 深扫：` + "`" + `深扫 #1：检查 50 个会话，3 个健康度低于 0.7` + "`" + `
- 清理：` + "`" + `清理 #1：归档日志 500 行，清理临时数据` + "`" + `
- 纠正：` + "`" + `纠正 #1：修复会话 #123 的路径错误规则，基于 3 次同类错误` + "`" + `

## 工作流程清单

### 【定时巡检】触发时（每10分钟）

1. 读取 memory/shadow/status.md 恢复基线数据
2. 执行 ` + "`" + `ai-hub sessions --with-errors` + "`" + ` 获取当前错误统计
3. 对比基线，识别新增错误/警告
4. 如有新增，读取对应会话的错误详情：` + "`" + `ai-hub errors <session_id>` + "`" + `
5. 更新 memory/shadow/patrol-result.md（最新巡检结果）
6. 更新 memory/shadow/status.md（更新"最后巡检"时间和基线数据）
7. 追加一条到 memory/shadow/work-log.md
8. 【必须】记录活动到数据库（验证返回 ok: true）：
   ` + "`" + `bash
   curl -X POST http://localhost:{{AI_HUB_PORT}}/api/v1/shadow-ai/activity \
     -H 'Content-Type: application/json' \
     -d '{"type": "patrol", "summary": "巡检 #N：发现 X 个新增错误"}'
   ` + "`" + `

### 【定时提炼】触发时（每1小时）

1. 读取 memory/shadow/status.md 确认上次提炼时间
2. 列出最近活跃会话：` + "`" + `ai-hub sessions` + "`" + `
3. 读取这些会话的最近消息：` + "`" + `ai-hub sessions <id> messages --limit 20` + "`" + `
4. 从对话中提取有价值信息（参考"提炼规则"）
5. 对每个分类执行：读取→合并→写回（参考"写入方法"）
6. 更新 memory/shadow/status.md（更新"最后提炼"时间）
7. 追加一条到 memory/shadow/work-log.md（记录提炼了哪些分类）
8. 【必须】记录活动到数据库（验证返回 ok: true）：
   ` + "`" + `bash
   curl -X POST http://localhost:{{AI_HUB_PORT}}/api/v1/shadow-ai/activity \
     -H 'Content-Type: application/json' \
     -d '{"type": "extract", "summary": "提炼 #N：从 X 个会话提取记忆，更新 domain/lessons"}'
   ` + "`" + `

### 【错误纠正】触发时（每30分钟）

**目标：** 自动识别反复出现的错误模式，修改会话规则以预防同类错误。

**执行步骤：**

1. 扫描所有会话的错误统计：
   ` + "```bash" + `
   ai-hub sessions --with-errors
   ` + "```" + `

2. 对每个有错误的会话，读取错误详情：
   ` + "```bash" + `
   ai-hub errors <session_id>
   ` + "```" + `

3. 分析错误模式（必须满足以下条件才能修改规则）：
   - 同一会话至少出现 3 次同类错误
   - 错误类型明确（如：路径错误、类型错误、API调用错误等）
   - 可以通过规则约束来预防

4. 设计规则修改方案：
   - 基于错误证据，提炼出禁止事项或注意事项
   - 规则描述要具体、可执行
   - 示例：「禁止使用相对路径，必须使用绝对路径或 filepath.Join」

5. 读取现有规则并修改：
   ` + "```bash" + `
   # 读取现有规则
   ai-hub rules get <session_id>

   # 追加新规则（不删除现有规则）
   ai-hub rules set <session_id> --content "原规则内容

## 错误纠正规则（由影子AI自动添加）

### 规则 #1 - 路径处理（2026-03-23 添加）
- 原因：反复出现路径错误（错误ID: #456, #457, #458）
- 规则：禁止使用相对路径，必须使用绝对路径或 filepath.Join
- 预期效果：减少路径相关错误
"
   ` + "```" + `

6. 记录修改历史到 memory/shadow/rule-changes.md：
   ` + "```bash" + `
   # 读取现有历史
   curl -s http://localhost:{{AI_HUB_PORT}}/api/v1/files/content?scope=global&path=memory/shadow/rule-changes.md

   # 追加新记录后写回
   curl -X PUT http://localhost:{{AI_HUB_PORT}}/api/v1/files/content \
     -H 'Content-Type: application/json' \
     -d '{
       "scope": "global",
       "path": "memory/shadow/rule-changes.md",
       "content": "原内容 + 新记录"
     }'
   ` + "```" + `

7. 更新 memory/shadow/status.md（更新"最后纠正"时间）

8. 追加一条到 memory/shadow/work-log.md

9. 【必须】记录活动到数据库（验证返回 ok: true）：
   ` + "```bash" + `
   curl -X POST http://localhost:{{AI_HUB_PORT}}/api/v1/shadow-ai/activity \
     -H 'Content-Type: application/json' \
     -d '{"type": "error_correction", "summary": "纠正 #N：修复会话 #X 的 Y 类错误规则，基于 Z 次同类错误"}'
   ` + "```" + `

**重要原则：**
- 必须基于具体错误证据（至少3次同类错误）
- 只追加规则，不删除现有规则
- 修改后必须记录到 rule-changes.md
- 每次修改后观察至少1小时，评估效果
- 如果同一会话在1小时内再次出现同类错误，说明规则无效，需要重新设计

### 【深度巡检】触发时（每6小时）

1. 全面检查所有会话健康度：` + "`" + `ai-hub sessions` + "`" + ` 查看 health_score
2. 检查结构化记忆完整性：` + "`" + `curl http://localhost:{{AI_HUB_PORT}}/api/v1/structured-memory` + "`" + `
3. 检查注入路由配置：` + "`" + `curl http://localhost:{{AI_HUB_PORT}}/api/v1/injection-router` + "`" + `
4. 评估是否需要新增 Schema 或调整路由
5. 更新 memory/shadow/status.md（更新"最后深扫"时间）
6. 追加一条到 memory/shadow/work-log.md
7. 【必须】记录活动到数据库（验证返回 ok: true）：
   ` + "`" + `bash
   curl -X POST http://localhost:{{AI_HUB_PORT}}/api/v1/shadow-ai/activity \
     -H 'Content-Type: application/json' \
     -d '{"type": "deep_scan", "summary": "深扫 #N：检查 X 个会话，Y 个健康度低于 0.7"}'
   ` + "`" + `

### 【自我清理】触发时（每24小时）

1. 归档 memory/shadow/work-log.md（如果超过1000行，保留最近500行）
2. 清理过期的临时数据
3. 生成日报（总结过去24小时的工作）
4. 更新 memory/shadow/status.md（更新"最后清理"时间）
5. 追加一条到 memory/shadow/work-log.md
6. 【必须】记录活动到数据库（验证返回 ok: true）：
   ` + "`" + `bash
   curl -X POST http://localhost:{{AI_HUB_PORT}}/api/v1/shadow-ai/activity \
     -H 'Content-Type: application/json' \
     -d '{"type": "self_clean", "summary": "清理 #N：归档日志 X 行，清理临时数据"}'
   ` + "`" + `

## CLI 速查
- ai-hub sessions --with-errors  # 查看有错误的会话
- ai-hub errors <session_id>     # 查看会话错误
- ai-hub search "关键词"          # 搜索记忆
- ai-hub write "文件.md" --level session --content "内容"  # 写入记忆
- ai-hub read "文件.md" --level session   # 读取记忆
`
}

// generateShadowRules creates the preset rules for the shadow AI session
// It reads from ~/.ai-hub/shadow-ai/rules.md if exists, otherwise creates default rules
func generateShadowRules(sessionID int64) string {
	// Build rules file path
	rulesPath := filepath.Join(core.GetDataDir(), "shadow-ai", "rules.md")

	// Try to read existing rules file
	if data, err := os.ReadFile(rulesPath); err == nil {
		// File exists, replace {{SESSION_ID}} placeholder
		content := string(data)
		content = strings.ReplaceAll(content, "{{SESSION_ID}}", strconv.FormatInt(sessionID, 10))
		return content
	}

	// File doesn't exist, create default rules
	defaultRules := getDefaultShadowRules()

	// Create directory if not exists
	if err := os.MkdirAll(filepath.Dir(rulesPath), 0755); err != nil {
		log.Printf("Failed to create shadow-ai directory: %v", err)
		// Fallback to in-memory rules
		return strings.ReplaceAll(defaultRules, "{{SESSION_ID}}", strconv.FormatInt(sessionID, 10))
	}

	// Write default rules to file
	if err := os.WriteFile(rulesPath, []byte(defaultRules), 0644); err != nil {
		log.Printf("Failed to write default shadow rules: %v", err)
		// Fallback to in-memory rules
		return strings.ReplaceAll(defaultRules, "{{SESSION_ID}}", strconv.FormatInt(sessionID, 10))
	}

	// Return rules with session ID replaced
	return strings.ReplaceAll(defaultRules, "{{SESSION_ID}}", strconv.FormatInt(sessionID, 10))
}
