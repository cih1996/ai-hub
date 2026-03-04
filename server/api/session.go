package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SessionResponse wraps Session with runtime streaming status
type SessionResponse struct {
	model.Session
	Streaming    bool   `json:"streaming"`
	HasTriggers  bool   `json:"has_triggers"`
	ProcessAlive bool   `json:"process_alive"`
	ProcessPid   int    `json:"process_pid,omitempty"`
	ProcessState string `json:"process_state,omitempty"`
	UptimeSec    int64  `json:"uptime_sec,omitempty"`
	IdleSec      int64  `json:"idle_sec,omitempty"`
}

func ListSessions(c *gin.Context) {
	list, err := store.ListSessions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	streamingIDs := GetStreamingSessionIDs()
	triggerSessions, _ := store.SessionsWithTriggers()
	poolStatus := core.Pool.Status()
	resp := make([]SessionResponse, 0, len(list))
	for _, s := range list {
		sr := SessionResponse{
			Session:     s,
			Streaming:   streamingIDs[s.ID],
			HasTriggers: triggerSessions[s.ID],
		}
		if info, ok := poolStatus[s.ID]; ok {
			sr.ProcessAlive = true
			sr.ProcessPid = info.Pid
			sr.ProcessState = info.State
			sr.UptimeSec = info.UptimeSec
			sr.IdleSec = info.IdleSec
		}
		resp = append(resp, sr)
	}
	c.JSON(http.StatusOK, resp)
}

func CreateSession(c *gin.Context) {
	var s model.Session
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := store.CreateSession(&s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, s)
}

func GetSession(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	s, err := store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, s)
}

func UpdateSession(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	// Read existing session first so missing fields keep their original values
	existing, err := store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if err := c.ShouldBindJSON(existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	existing.ID = id // ensure ID is not changed
	if err := store.UpdateSession(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, existing)
}

func DeleteSession(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	core.Pool.Kill(id)
	if err := store.DeleteSession(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func GetMessages(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	// 先检查会话是否存在
	_, err = store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	msgs, err := store.GetMessages(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []model.Message{}
	}
	c.JSON(http.StatusOK, msgs)
}

// TruncateMessages handles DELETE /api/v1/sessions/:id/messages?from=<msgId>
// Deletes the message with id == fromMsgId AND all messages after it (id >= fromMsgId).
// Used by the retry-message feature: the original user message is deleted together with
// any subsequent AI reply, then sendMessage re-adds the user message fresh.
func TruncateMessages(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	// Accept both "from" (new) and "after" (legacy) query params for backwards compat
	fromStr := c.Query("from")
	if fromStr == "" {
		fromStr = c.Query("after")
	}
	fromID, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil || fromID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from parameter required and must be a positive integer"})
		return
	}
	if err := store.DeleteMessagesFrom(id, fromID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CompressSession handles POST /api/v1/sessions/:id/compress
// Supports query param ?mode=intelligent|simple|auto (overrides global setting).
func CompressSession(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	if IsSessionStreaming(id) {
		c.JSON(http.StatusConflict, gin.H{"error": "session is currently streaming"})
		return
	}

	session, err := store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Allow caller to override mode via query param
	mode := c.Query("mode")
	if mode == "" {
		if cfg, e := store.GetCompressSettings(); e == nil {
			mode = cfg.Mode
		}
	}
	if mode == "" {
		mode = "auto"
	}

	compressMode, err := doCompress(session, mode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "context compressed, session reset", "compress_mode": compressMode})
}

// doCompress performs the core compression logic: builds recovery seed, resets session UUID,
// kills pool process, marks force-fresh, and saves the system message.
// Returns the actual compress mode used ("intelligent" or "simple").
func doCompress(session *model.Session, mode string) (string, error) {
	msgs, _ := store.GetMessages(session.ID)

	seed, actualMode := buildCompressedSeed(msgs, session, mode)
	if seed != "" {
		setPendingRecoverySeed(session.ID, seed)
	}

	newUUID := uuid.New().String()
	core.Pool.Kill(session.ID)
	if err := store.UpdateClaudeSessionID(session.ID, newUUID); err != nil {
		return "", fmt.Errorf("failed to update session id: %w", err)
	}
	markForceFreshRun(session.ID)
	session.ClaudeSessionID = newUUID

	sysMsg := &model.Message{
		SessionID: session.ID,
		Role:      "user",
		Content:   fmt.Sprintf("【系统】上下文已压缩（%s 模式），会话已重置。", actualMode),
	}
	store.AddMessage(sysMsg)

	// Record the compress point so subsequent auto-compress only counts incremental data
	if sysMsg.ID > 0 {
		store.UpdateLastCompressMsgID(session.ID, sysMsg.ID)
		session.LastCompressMsgID = sysMsg.ID
	}

	// Notify all WS clients
	broadcast(WSMessage{Type: "auto_compressed", SessionID: session.ID, Content: actualMode})
	return actualMode, nil
}

// buildCompressedSeed picks intelligent or simple mode, with auto-fallback.
// Returns the seed string and the mode actually used.
func buildCompressedSeed(msgs []model.Message, session *model.Session, mode string) (string, string) {
	if len(msgs) == 0 {
		return "", "simple"
	}

	tryIntelligent := mode == "intelligent" || mode == "auto"

	if tryIntelligent {
		provider, err := store.GetProvider(session.ProviderID)
		if err != nil || provider == nil {
			log.Printf("[compress] session %d: provider not found, falling back to simple", session.ID)
			return buildRecoverySeed(msgs, "上下文压缩后恢复"), "simple"
		}
		seed, err := core.BuildIntelligentRecoverySeed(msgs, provider, session.ID)
		if err == nil && seed != "" {
			return seed, "intelligent"
		}
		log.Printf("[compress] session %d: intelligent failed (%v), falling back to simple", session.ID, err)
		if mode == "intelligent" {
			// strict mode: don't fallback
			return "", "intelligent"
		}
		// auto: fallback to simple
	}

	return buildRecoverySeed(msgs, "上下文压缩后恢复"), "simple"
}

// buildRecoverySeed takes recent messages and builds a condensed prompt for context recovery.
func buildRecoverySeed(msgs []model.Message, reason string) string {
	const maxMsgs = 10
	const maxContentLen = 500

	start := 0
	if len(msgs) > maxMsgs {
		start = len(msgs) - maxMsgs
	}
	recent := msgs[start:]

	var sb strings.Builder
	if strings.TrimSpace(reason) == "" {
		reason = "会话重置后恢复"
	}
	sb.WriteString(fmt.Sprintf("【上下文恢复】本轮因\"%s\"进入新会话。请先基于以下历史记录恢复上下文，再继续处理当前用户请求。\n\n", reason))

	for _, m := range recent {
		role := "用户"
		if m.Role == "assistant" {
			role = "助手"
		}
		content := m.Content
		runes := []rune(content)
		if len(runes) > maxContentLen {
			content = string(runes[:maxContentLen]) + "...(已截断)"
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}

	sb.WriteString(fmt.Sprintf("\n---\n如需完整历史，请调用：GET /api/v1/sessions/%d/messages（不要使用不存在的接口）。\n", msgs[len(msgs)-1].SessionID))
	sb.WriteString("请继续处理上面最后一条用户消息的请求；若存在未完成任务，延续执行。")
	return sb.String()
}

// maybeAutoCompress checks auto-compress settings and triggers compression if both
// the token threshold AND the minimum turn count are exceeded (dual-condition).
// Called asynchronously after each successful runStream; must not block.
func maybeAutoCompress(session *model.Session, newInputTokens int64) {
	cfg, err := store.GetCompressSettings()
	if err != nil || !cfg.AutoEnabled {
		return
	}

	// Use last_compress_msg_id for incremental counting (only data since last compress)
	afterMsgID := session.LastCompressMsgID

	// Condition 1: incremental input tokens since last compress must exceed threshold
	stats, err := store.GetSessionTokenStats(session.ID, afterMsgID)
	if err != nil {
		return
	}
	totalInput := stats.TotalInput
	if totalInput < int64(cfg.Threshold) {
		return
	}

	// Condition 2: conversation turns since last compress must reach MinTurns
	if cfg.MinTurns > 0 {
		turns := store.CountUserMessages(session.ID, afterMsgID)
		if turns < cfg.MinTurns {
			log.Printf("[compress] auto-compress skipped for session %d: turns=%d < min_turns=%d (tokens=%d, afterMsgID=%d)",
				session.ID, turns, cfg.MinTurns, totalInput, afterMsgID)
			return
		}
	}

	// Don't compress if streaming is in progress
	if IsSessionStreaming(session.ID) {
		return
	}

	log.Printf("[compress] auto-compress triggered for session %d: total_input=%d threshold=%d turns>=%d afterMsgID=%d",
		session.ID, totalInput, cfg.Threshold, cfg.MinTurns, afterMsgID)

	if _, err := doCompress(session, cfg.Mode); err != nil {
		log.Printf("[compress] auto-compress failed for session %d: %v", session.ID, err)
	}
}

// SwitchProvider handles PUT /api/v1/sessions/:id/provider
// Switches the provider for a session: updates provider_id, kills pool process,
// generates new claude_session_id, and saves a system message.
func SwitchProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	if IsSessionStreaming(id) {
		c.JSON(http.StatusConflict, gin.H{"error": "session is currently streaming"})
		return
	}

	var body struct {
		ProviderID string `json:"provider_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	session, err := store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	// Capture recent messages before provider switch reset.
	if msgs, err := store.GetMessages(id); err == nil && len(msgs) > 0 {
		setPendingRecoverySeed(id, buildRecoverySeed(msgs, "切换模型/供应商后恢复"))
	}

	// Verify provider exists
	provider, err := store.GetProvider(body.ProviderID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider not found"})
		return
	}

	// Update provider_id
	session.ProviderID = body.ProviderID
	if err := store.UpdateSession(session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Kill existing process and generate new claude_session_id
	core.Pool.Kill(id)
	newUUID := uuid.New().String()
	if err := store.UpdateClaudeSessionID(id, newUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Provider switched and CLI session reset: force one fresh run to avoid
	// accidental --resume against the new UUID.
	markForceFreshRun(id)

	// Save system message
	sysMsg := &model.Message{
		SessionID: id,
		Role:      "user",
		Content:   fmt.Sprintf("【系统】模型已切换为 %s（%s），会话已重置。", provider.Name, provider.ModelID),
	}
	store.AddMessage(sysMsg)

	c.JSON(http.StatusOK, gin.H{"ok": true, "provider_id": body.ProviderID, "provider_name": provider.Name})
}
