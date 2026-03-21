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
	PatrolInterval        string `json:"patrol_interval"`         // e.g. "10m"
	ExtractInterval       string `json:"extract_interval"`        // e.g. "1h"
	DeepScanInterval      string `json:"deep_scan_interval"`      // e.g. "6h"
	SelfCleanInterval     string `json:"self_clean_interval"`     // e.g. "24h"
	ContextResetThreshold int    `json:"context_reset_threshold"` // e.g. 50
}

// ShadowAIStatus holds the shadow AI status response
type ShadowAIStatus struct {
	Enabled   bool            `json:"enabled"`
	SessionID int64           `json:"session_id"`
	Status    string          `json:"status"` // "running" | "paused" | "uninitialized"
	Config    ShadowAIConfig  `json:"config"`
	Triggers  []model.Trigger `json:"triggers,omitempty"`
}

var defaultShadowConfig = ShadowAIConfig{
	PatrolInterval:        "10m",
	ExtractInterval:       "1h",
	DeepScanInterval:      "6h",
	SelfCleanInterval:     "24h",
	ContextResetThreshold: 50,
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
		// Check if triggers are active
		triggers, _ := store.ListTriggersBySession(status.SessionID)
		status.Triggers = triggers
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

// generateShadowRules creates the preset rules for the shadow AI session
func generateShadowRules(sessionID int64) string {
	return fmt.Sprintf(`# 影子AI — 全局记忆管理者 & 会话健康守护者

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

## 结构化记忆管理
你的核心职责之一是维护7大结构化记忆分类：

### 固定分类（始终注入每个会话的 system prompt）
- **identity**（用户身份画像）：用户名、角色、公司、时区、语言等
- **preferences**（用户偏好习惯）：编码风格、交互偏好、工具偏好等
- **error-genome**（AI常犯错误模式库）：被纠正的错误、典型失败模式

### 条件分类（按关键词匹配注入）
- **domain**（用户领域知识）：专业领域的术语、概念、架构
- **lessons**（踩过的坑和教训）：历史踩坑记录
- **active**（当前进行中的事项）：正在做的项目和任务
- **decisions**（重要决策记录）：架构决策、技术选型等

### 写入方法
通过 HTTP API 写入（比 CLI 更可靠）：
` + "`" + `curl -X PUT http://localhost:{{AI_HUB_PORT}}/api/v1/structured-memory/<category> \
  -H 'Content-Type: application/json' \
  -d '{"content": "内容"}'` + "`" + `

### 提炼规则
每次【定时提炼】被触发时：
1. 用 ` + "`" + `ai-hub sessions` + "`" + ` 列出活跃会话
2. 用 ` + "`" + `ai-hub sessions <id> messages` + "`" + ` 读取最近消息
3. 提取以下信息写入对应分类：
   - 用户被纠正的内容 → error-genome
   - 用户表达的偏好 → preferences
   - 用户身份信息 → identity
   - 专业知识点 → domain
   - 踩坑教训 → lessons
   - 正在进行的项目 → active
   - 做出的重要决策 → decisions
4. 写入时保持**追加合并**而非覆盖：先读取现有内容，合并后写回
5. 写入后用 ` + "`" + `ai-hub search` + "`" + ` 验证写入成功

### 注入路由管理
你也负责维护注入路由。通过 API 创建关键词→分类映射：
` + "`" + `curl -X POST http://localhost:{{AI_HUB_PORT}}/api/v1/injection-router \
  -H 'Content-Type: application/json' \
  -d '{"keywords": "开发|编程|代码", "inject_categories": "domain,lessons"}'` + "`" + `

首次启动时，应创建默认注入路由。

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
- **禁止**修改其他会话的规则
- **禁止**删除其他会话的消息

## 自我管理
- 不依赖上下文历史，全靠记忆库
- 每次唤醒先从记忆库恢复状态
- 上下文阈值已设置为自动重置
- 你的会话ID是 %d

## CLI 速查
- ai-hub sessions --with-errors  # 查看有错误的会话
- ai-hub errors <session_id>     # 查看会话错误
- ai-hub search "关键词"          # 搜索记忆
- ai-hub write "文件.md" --level session --content "内容"  # 写入记忆
- ai-hub read "文件.md" --level session   # 读取记忆
`, sessionID)
}
