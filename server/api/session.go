package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"fmt"
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

// TruncateMessages handles DELETE /api/v1/sessions/:id/messages?after=<msgId>
// Deletes all messages with id > afterMsgId in the session.
// Used by the retry-message feature to cut off history before re-sending.
func TruncateMessages(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	afterStr := c.Query("after")
	afterID, err := strconv.ParseInt(afterStr, 10, 64)
	if err != nil || afterID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "after parameter required and must be a positive integer"})
		return
	}
	if err := store.DeleteMessagesAfter(id, afterID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CompressSession handles POST /api/v1/sessions/:id/compress
// Generates a new claude_session_id, builds a condensed summary from recent messages,
// and starts a new CLI stream with the summary as the first message.
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

	newUUID := uuid.New().String()
	core.Pool.Kill(id)
	if err := store.UpdateClaudeSessionID(id, newUUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update session id"})
		return
	}
	session.ClaudeSessionID = newUUID

	// Save a system message indicating compression
	sysMsg := &model.Message{
		SessionID: session.ID,
		Role:      "user",
		Content:   "【系统】上下文已压缩，会话已重置。",
	}
	store.AddMessage(sysMsg)

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "context compressed, session reset"})
}

// buildCondensedQuery takes recent messages and builds a condensed prompt for context recovery.
func buildCondensedQuery(msgs []model.Message) string {
	const maxMsgs = 10
	const maxContentLen = 500

	start := 0
	if len(msgs) > maxMsgs {
		start = len(msgs) - maxMsgs
	}
	recent := msgs[start:]

	var sb strings.Builder
	sb.WriteString("【上下文恢复】之前的对话因上下文过长被手动压缩。以下是最近的对话记录，请基于这些信息继续工作：\n\n")

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

	sb.WriteString("---\n请继续处理上面最后一条用户消息的请求。如果之前有未完成的任务，请继续完成。")
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

	// Save system message
	sysMsg := &model.Message{
		SessionID: id,
		Role:      "user",
		Content:   fmt.Sprintf("【系统】模型已切换为 %s（%s），会话已重置。", provider.Name, provider.ModelID),
	}
	store.AddMessage(sysMsg)

	c.JSON(http.StatusOK, gin.H{"ok": true, "provider_id": body.ProviderID, "provider_name": provider.Name})
}
