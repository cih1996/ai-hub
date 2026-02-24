package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ListChannels GET /api/v1/channels
func ListChannels(c *gin.Context) {
	list, err := store.ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []model.Channel{}
	}
	c.JSON(http.StatusOK, list)
}

// CreateChannel POST /api/v1/channels
func CreateChannel(c *gin.Context) {
	var ch model.Channel
	if err := c.ShouldBindJSON(&ch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if ch.Name == "" || ch.Platform == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and platform are required"})
		return
	}
	ch.Enabled = true
	if err := store.CreateChannel(&ch); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	QQWSMgr.OnChannelCreated(&ch)
	c.JSON(http.StatusOK, ch)
}

// UpdateChannel PUT /api/v1/channels/:id
func UpdateChannel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	existing, err := store.GetChannel(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}
	var req struct {
		Name      *string `json:"name"`
		Platform  *string `json:"platform"`
		SessionID *int64  `json:"session_id"`
		Config    *string `json:"config"`
		Enabled   *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Platform != nil {
		existing.Platform = *req.Platform
	}
	if req.SessionID != nil {
		existing.SessionID = *req.SessionID
	}
	if req.Config != nil {
		existing.Config = *req.Config
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if err := store.UpdateChannel(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	QQWSMgr.OnChannelUpdated(existing)
	c.JSON(http.StatusOK, existing)
}

// DeleteChannel DELETE /api/v1/channels/:id
func DeleteChannel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := store.DeleteChannel(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	QQWSMgr.OnChannelDeleted(id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- Webhook Endpoints ----

// HandleFeishuWebhook POST /api/v1/webhook/feishu
// Receives feishu event callbacks, forwards messages to bound sessions.
func HandleFeishuWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body failed"})
		return
	}
	log.Printf("[webhook/feishu] received: %s", string(body))

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	// Handle URL verification challenge
	if challenge, ok := raw["challenge"].(string); ok {
		log.Printf("[webhook/feishu] URL verification challenge")
		c.JSON(http.StatusOK, gin.H{"challenge": challenge})
		return
	}

	// Handle event callback v2
	header, _ := raw["header"].(map[string]interface{})
	if header == nil {
		// Try v1 format
		handleFeishuEventV1(c, raw)
		return
	}

	eventType, _ := header["event_type"].(string)
	appID, _ := header["app_id"].(string)
	log.Printf("[webhook/feishu] event_type=%s app_id=%s", eventType, appID)

	if eventType != "im.message.receive_v1" {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	// Find channel by app_id
	ch, err := store.GetChannelByPlatformConfig("feishu", "app_id", appID)
	if err != nil || ch == nil {
		log.Printf("[webhook/feishu] no channel found for app_id=%s", appID)
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	if ch.SessionID == 0 {
		log.Printf("[webhook/feishu] channel %d has no bound session", ch.ID)
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	// Extract message content
	event, _ := raw["event"].(map[string]interface{})
	if event == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	message, _ := event["message"].(map[string]interface{})
	if message == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	msgType, _ := message["message_type"].(string)
	contentStr, _ := message["content"].(string)
	chatID, _ := message["chat_id"].(string)
	messageID, _ := message["message_id"].(string)

	// Extract sender info
	sender, _ := event["sender"].(map[string]interface{})
	senderID := ""
	if senderIDObj, ok := sender["sender_id"].(map[string]interface{}); ok {
		senderID, _ = senderIDObj["open_id"].(string)
	}

	// Parse text content
	text := extractFeishuText(msgType, contentStr)
	if text == "" {
		log.Printf("[webhook/feishu] empty or unsupported message type: %s", msgType)
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	// Build forwarded message with context and channel credentials
	forwarded := fmt.Sprintf("【飞书消息】\n发送者: %s\n会话: %s\n消息ID: %s\n内容: %s\n---\n频道凭证（用于回复）:\n%s",
		senderID, chatID, messageID, text, extractChannelCredentials(ch))

	log.Printf("[webhook/feishu] forwarding to session %d: %s", ch.SessionID, text)

	// Forward to bound session via internal SendChat logic
	forwardToSession(ch.SessionID, forwarded)

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleFeishuEventV1 handles legacy v1 format events
func handleFeishuEventV1(c *gin.Context, raw map[string]interface{}) {
	// v1 format: {"event": {"type": "message", "text": "...", "app_id": "..."}}
	event, _ := raw["event"].(map[string]interface{})
	if event == nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	appID, _ := event["app_id"].(string)
	text, _ := event["text"].(string)
	if appID == "" || text == "" {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	ch, err := store.GetChannelByPlatformConfig("feishu", "app_id", appID)
	if err != nil || ch == nil || ch.SessionID == 0 {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	// Remove @bot mention prefix if present
	text = strings.TrimSpace(text)
	forwarded := fmt.Sprintf("【飞书消息】\n内容: %s\n---\n频道凭证（用于回复）:\n%s", text, extractChannelCredentials(ch))
	forwardToSession(ch.SessionID, forwarded)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// extractFeishuText extracts plain text from feishu message content
func extractFeishuText(msgType, content string) string {
	if msgType != "text" {
		return fmt.Sprintf("[%s 类型消息，暂不支持解析]", msgType)
	}
	var textContent struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &textContent); err != nil {
		return content
	}
	return textContent.Text
}

// extractChannelCredentials extracts credentials from channel config JSON based on platform
func extractChannelCredentials(ch *model.Channel) string {
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return "（无法解析频道配置）"
	}
	lines := []string{}
	switch ch.Platform {
	case "feishu":
		if v, _ := cfg["app_id"].(string); v != "" {
			lines = append(lines, fmt.Sprintf("App ID: %s", v))
		}
		if v, _ := cfg["app_secret"].(string); v != "" {
			lines = append(lines, fmt.Sprintf("App Secret: %s", v))
		}
	case "qq":
		if v, _ := cfg["napcat_http_url"].(string); v != "" {
			lines = append(lines, fmt.Sprintf("NapCat地址: %s", v))
		}
		if v, _ := cfg["token"].(string); v != "" {
			lines = append(lines, fmt.Sprintf("Token: %s", v))
		}
	default:
		for k, v := range cfg {
			if s, ok := v.(string); ok && s != "" {
				lines = append(lines, fmt.Sprintf("%s: %s", k, s))
			}
		}
	}
	if len(lines) == 0 {
		return "（频道未配置凭证）"
	}
	return strings.Join(lines, "\n")
}

// HandleQQWebhook POST /api/v1/webhook/qq
// Receives OneBot 11 HTTP POST events from NapCat, forwards messages to bound sessions.
func HandleQQWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body failed"})
		return
	}
	log.Printf("[webhook/qq] received: %s", string(body))

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	// Only handle message events
	postType, _ := raw["post_type"].(string)
	if postType != "message" {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	// Find channel: by channel_id param or first enabled qq channel
	var ch *model.Channel
	if cidStr := c.Query("channel_id"); cidStr != "" {
		cid, _ := strconv.ParseInt(cidStr, 10, 64)
		if cid > 0 {
			ch, _ = store.GetChannel(cid)
			if ch != nil && (ch.Platform != "qq" || !ch.Enabled) {
				ch = nil
			}
		}
	}
	if ch == nil {
		ch, _ = store.GetEnabledChannelByPlatform("qq")
	}
	if ch == nil || (ch.SessionID == 0 && !qqChannelHasRoutes(ch.Config)) {
		log.Printf("[webhook/qq] no enabled qq channel with bound session or routing rules")
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	// Extract message fields
	msgType, _ := raw["message_type"].(string)
	userID := jsonNumber(raw["user_id"])
	groupID := jsonNumber(raw["group_id"])
	messageID := jsonNumber(raw["message_id"])
	message, _ := raw["message"].(string)
	// Some NapCat versions send message as array, try raw_message fallback
	if message == "" {
		message, _ = raw["raw_message"].(string)
	}
	if message == "" {
		log.Printf("[webhook/qq] empty message")
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	// Dedup: skip if this message_id was already processed (shared with WS path)
	if qqGlobalDedup.isDuplicate(messageID) {
		log.Printf("[webhook/qq] duplicate message_id %s (source=HTTP), skipped", messageID)
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	// Build forwarded message
	typeLabel := "私聊"
	if msgType == "group" {
		typeLabel = "群聊"
	}
	forwarded := fmt.Sprintf("【QQ消息】\n类型: %s\n发送者: %s", typeLabel, userID)
	if msgType == "group" && groupID != "" {
		forwarded += fmt.Sprintf("\n群号: %s", groupID)
	}
	forwarded += fmt.Sprintf("\n消息ID: %s\n内容: %s\n---\n频道凭证（用于回复）:\n%s",
		messageID, message, extractChannelCredentials(ch))

	// Route message: use routing rules if available, fallback to channel default session
	var targetSession int64
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err == nil {
		// Token auth check
		if token, _ := cfg["token"].(string); token != "" {
			authHeader := c.GetHeader("Authorization")
			if authHeader != "Bearer "+token && authHeader != token {
				log.Printf("[webhook/qq] token mismatch for channel %d", ch.ID)
				c.JSON(http.StatusForbidden, gin.H{"error": "token mismatch"})
				return
			}
		}
		// Apply routing rules
		routes := parseRoutingRules(cfg)
		if len(routes) > 0 {
			for _, r := range routes {
				switch {
				case r.Type == "group" && msgType == "group":
					if _, ok := r.idSet[groupID]; ok {
						targetSession = r.SessionID
					}
				case r.Type == "private" && msgType == "private":
					if _, ok := r.idSet[userID]; ok {
						targetSession = r.SessionID
					}
				}
				if targetSession > 0 {
					break
				}
			}
		}
	}
	if targetSession <= 0 {
		targetSession = ch.SessionID
	}
	if targetSession <= 0 {
		log.Printf("[webhook/qq] channel %d: no matching route and no default session, dropping", ch.ID)
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	log.Printf("[webhook/qq] forwarding to session %d: %s", targetSession, message)
	// Content-based dedup: same content to same session within 30s → skip
	if qqContentDedup.isDuplicate(contentDedupKey(targetSession, message)) {
		log.Printf("[webhook/qq] duplicate content to session %d (source=HTTP), skipped", targetSession)
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	forwardToSession(targetSession, forwarded)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// jsonNumber extracts a number field as string from JSON (handles both float64 and string)
func jsonNumber(v interface{}) string {
	switch n := v.(type) {
	case float64:
		return fmt.Sprintf("%.0f", n)
	case string:
		return n
	default:
		return ""
	}
}

// forwardToSession sends a message to a session, triggering AI processing
func forwardToSession(sessionID int64, content string) {
	session, err := store.GetSession(sessionID)
	if err != nil {
		log.Printf("[webhook] session %d not found: %v", sessionID, err)
		return
	}
	// Save user message (even if busy — queue processing will pick it up)
	msg := &model.Message{SessionID: session.ID, Role: "user", Content: content}
	if err := store.AddMessage(msg); err != nil {
		log.Printf("[webhook] save message failed: %v", err)
		return
	}
	if IsSessionStreaming(session.ID) {
		log.Printf("[webhook] session %d is streaming, message queued (msg_id=%d)", sessionID, msg.ID)
		broadcast(WSMessage{Type: "message_queued", SessionID: session.ID, Content: content})
		return
	}
	// Kick off streaming
	go runStream(session, content, false, msg.ID)
}
