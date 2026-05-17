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
	ErrorCount   int    `json:"error_count"`
	WarningCount int    `json:"warning_count"`
}

func ListSessions(c *gin.Context) {
	// Support filtering by group name
	groupName := c.Query("group")
	var list []model.Session
	var err error
	if groupName != "" {
		list, err = store.ListSessionsByGroup(groupName)
	} else {
		list, err = store.ListSessions()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	streamingIDs := GetStreamingSessionIDs()
	triggerSessions, _ := store.SessionsWithTriggers()
	poolStatus := core.Pool.Status()
	// Batch query error/warning counts
	sids := make([]int64, len(list))
	for i, s := range list {
		sids[i] = s.ID
	}
	errorCounts, _ := store.GetSessionErrorCounts(sids)
	resp := make([]SessionResponse, 0, len(list))
	for _, s := range list {
		sr := SessionResponse{
			Session:     s,
			Streaming:   streamingIDs[s.ID],
			HasTriggers: triggerSessions[s.ID],
		}
		if ec, ok := errorCounts[s.ID]; ok {
			sr.ErrorCount = ec.ErrorCount
			sr.WarningCount = ec.WarningCount
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

	// Build response with runtime status
	resp := SessionResponse{
		Session:   *s,
		Streaming: IsSessionStreaming(id),
	}

	// Check if session has triggers
	triggerSessions, _ := store.SessionsWithTriggers()
	resp.HasTriggers = triggerSessions[id]

	// Get process pool status
	poolStatus := core.Pool.Status()
	if info, ok := poolStatus[id]; ok {
		resp.ProcessAlive = true
		resp.ProcessPid = info.Pid
		resp.ProcessState = info.State
		resp.UptimeSec = info.UptimeSec
		resp.IdleSec = info.IdleSec
	}

	// Get error counts
	errorCounts, _ := store.GetSessionErrorCounts([]int64{id})
	if ec, ok := errorCounts[id]; ok {
		resp.ErrorCount = ec.ErrorCount
		resp.WarningCount = ec.WarningCount
	}

	c.JSON(http.StatusOK, resp)
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

	// Get total count (always returned in paginated responses)
	total, _ := store.GetMessagesCount(id)

	// Parse all query parameters
	limitStr := c.Query("limit")
	beforeIDStr := c.Query("before_id")
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")
	offsetStr := c.Query("offset")
	search := c.Query("search")

	// Priority: search > before_id > page > offset > default
	if search != "" {
		limit := 20
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}
		msgs, err := store.SearchMessages(id, search, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if msgs == nil {
			msgs = []model.Message{}
		}
		c.JSON(http.StatusOK, gin.H{"messages": msgs, "total": total, "has_more": false})
		return
	}

	if beforeIDStr != "" || limitStr != "" {
		// Cursor-based pagination (existing behavior + total)
		limit := 50
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}
		var beforeID int64
		if beforeIDStr != "" {
			if bid, err := strconv.ParseInt(beforeIDStr, 10, 64); err == nil && bid > 0 {
				beforeID = bid
			}
		}

		msgs, err := store.GetMessagesPaginated(id, beforeID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if msgs == nil {
			msgs = []model.Message{}
		}

		hasMore := false
		if len(msgs) >= limit && len(msgs) > 0 {
			oldestID := msgs[0].ID
			countBefore, _ := store.GetMessagesCountBefore(id, oldestID)
			hasMore = countBefore > 0
		}

		c.JSON(http.StatusOK, gin.H{"messages": msgs, "has_more": hasMore, "total": total})
		return
	}

	if pageStr != "" {
		// Page-based pagination
		page := 1
		pageSize := 50
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
		if ps := pageSizeStr; ps != "" {
			if s, err := strconv.Atoi(ps); err == nil && s > 0 {
				pageSize = s
			}
		}
		msgs, err := store.GetMessagesByPage(id, page, pageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if msgs == nil {
			msgs = []model.Message{}
		}
		hasMore := int64(page*pageSize) < total
		c.JSON(http.StatusOK, gin.H{"messages": msgs, "has_more": hasMore, "total": total, "page": page, "page_size": pageSize})
		return
	}

	if offsetStr != "" {
		// Offset-based pagination
		offset := 0
		limit := 50
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}
		msgs, err := store.GetMessagesByOffset(id, offset, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if msgs == nil {
			msgs = []model.Message{}
		}
		hasMore := int64(offset+limit) < total
		c.JSON(http.StatusOK, gin.H{"messages": msgs, "has_more": hasMore, "total": total})
		return
	}

	// Legacy mode: return all messages (backward compatible)
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

// GetConversationLogs handles GET /api/v1/sessions/:id/logs
func GetConversationLogs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	if _, err := store.GetSession(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	total, _ := store.GetConversationLogsCount(id)
	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	beforeID := int64(0)
	if beforeIDStr := c.Query("before_id"); beforeIDStr != "" {
		if bid, err := strconv.ParseInt(beforeIDStr, 10, 64); err == nil && bid > 0 {
			beforeID = bid
		}
	}

	var logs []model.ConversationLog
	if search := c.Query("search"); strings.TrimSpace(search) != "" {
		logs, err = store.SearchConversationLogs(id, search, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if logs == nil {
			logs = []model.ConversationLog{}
		}
		c.JSON(http.StatusOK, gin.H{"logs": logs, "has_more": false, "total": total})
		return
	}

	logs, err = store.GetConversationLogsPaginated(id, beforeID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if logs == nil {
		logs = []model.ConversationLog{}
	}
	hasMore := false
	if len(logs) >= limit && len(logs) > 0 {
		oldestID := logs[0].ID
		countBefore, _ := store.GetConversationLogsCountBefore(id, oldestID)
		hasMore = countBefore > 0
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs, "has_more": hasMore, "total": total})
}

// GetMessageWithContext returns a single message with surrounding context.
// GET /api/v1/sessions/:id/messages/:msg_id?context=2
func GetMessageWithContext(c *gin.Context) {
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	msgID, err := strconv.ParseInt(c.Param("msg_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}
	ctx := 2
	if ctxStr := c.Query("context"); ctxStr != "" {
		if v, err := strconv.Atoi(ctxStr); err == nil && v >= 0 {
			ctx = v
		}
	}
	msgs, err := store.GetMessageWithContext(sessionID, msgID, ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []model.Message{}
	}
	c.JSON(http.StatusOK, gin.H{"messages": msgs, "target_id": msgID})
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

// buildRecoverySeed takes recent messages and builds a condensed prompt for context recovery.
func buildRecoverySeed(msgs []model.Message, reason string) string {
	const maxMsgs = 50
	const maxContentLen = 4000

	start := 0
	if len(msgs) > maxMsgs {
		start = len(msgs) - maxMsgs
	}
	recent := msgs[start:]

	var sb strings.Builder
	if strings.TrimSpace(reason) == "" {
		reason = "会话重置后恢复"
	}
	sb.WriteString(fmt.Sprintf("【上下文恢复】本轮因「%s」进入新会话。以下是 AI 生成的智能压缩摘要，请基于此继续任务。\n\n", reason))

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

	sb.WriteString(fmt.Sprintf("\n---\n如需完整历史，请调用：GET /api/v1/sessions/%d/logs\n", msgs[len(msgs)-1].SessionID))
	sb.WriteString("请继续处理上面最后一条用户消息的请求；若存在未完成任务，延续执行。")
	return sb.String()
}

// buildRecoverySeedFromLogs builds a rich recovery seed directly from conversation_logs.
// Used when intelligent compression fails — provides the full recent history instead
// of a lossy summary, so the new session can pick up where it left off.
func buildRecoverySeedFromLogs(logs []model.ConversationLog, reason string) string {
	const maxLogs = 80
	const maxContentLen = 6000

	start := 0
	if len(logs) > maxLogs {
		start = len(logs) - maxLogs
	}
	recent := logs[start:]

	var sb strings.Builder
	if strings.TrimSpace(reason) == "" {
		reason = "会话重置后恢复"
	}
	sb.WriteString(fmt.Sprintf("【上下文恢复】本轮因「%s」进入新会话。以下是 AI 生成的智能压缩摘要，请基于此继续任务。\n\n", reason))

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

	sessionID := recent[len(recent)-1].SessionID
	sb.WriteString(fmt.Sprintf("\n---\n如需完整历史，请调用：GET /api/v1/sessions/%d/logs\n", sessionID))
	sb.WriteString("请继续处理上面最后一条用户消息的请求；若存在未完成任务，延续执行。")
	return sb.String()
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

// ResetSession handles POST /api/v1/sessions/:id/reset
// Deletes all messages from a session but preserves the session itself (rules, team, triggers, health).
// Optionally keeps the last N messages with keep_last parameter.
// Requires confirm:true in body for safety.
func ResetSession(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	var body struct {
		Confirm  bool `json:"confirm"`
		KeepLast int  `json:"keep_last"` // 保留最近N条消息，默认0=全清
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !body.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reset requires confirm:true (this operation is irreversible)"})
		return
	}

	// Check session exists
	session, err := store.GetSession(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Check not streaming
	if IsSessionStreaming(id) {
		c.JSON(http.StatusConflict, gin.H{"error": "session is currently streaming, cannot reset"})
		return
	}

	// Get message count before reset for logging
	totalBefore, _ := store.GetMessagesCount(id)

	// Execute reset
	deleted, err := store.ResetSessionMessages(id, body.KeepLast)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to reset: %v", err)})
		return
	}

	log.Printf("[reset] session %d: deleted %d messages (keep_last=%d, total_before=%d)", id, deleted, body.KeepLast, totalBefore)

	// Kill the Claude CLI process and reset the session UUID
	core.Pool.Kill(id)
	newUUID := uuid.New().String()
	store.UpdateClaudeSessionID(id, newUUID)
	markForceFreshRun(id)
	// Broadcast context reset to WS clients
	broadcast(WSMessage{Type: "context_reset", SessionID: id, Content: fmt.Sprintf("deleted:%d", deleted)})

	c.JSON(http.StatusOK, gin.H{
		"ok":            true,
		"session_id":    session.ID,
		"deleted_count": deleted,
		"kept_count":    body.KeepLast,
	})
}
