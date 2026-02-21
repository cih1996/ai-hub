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

	// Build forwarded message with context
	forwarded := fmt.Sprintf("【飞书消息】\n发送者: %s\n会话: %s\n消息ID: %s\n内容: %s",
		senderID, chatID, messageID, text)

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
	forwarded := fmt.Sprintf("【飞书消息】\n内容: %s", text)
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

// forwardToSession sends a message to a session, triggering AI processing
func forwardToSession(sessionID int64, content string) {
	session, err := store.GetSession(sessionID)
	if err != nil {
		log.Printf("[webhook] session %d not found: %v", sessionID, err)
		return
	}
	if IsSessionStreaming(session.ID) {
		log.Printf("[webhook] session %d is busy, queuing message", sessionID)
		// Save as user message even if busy
		msg := &model.Message{SessionID: session.ID, Role: "user", Content: content}
		store.AddMessage(msg)
		return
	}
	// Save user message
	msg := &model.Message{SessionID: session.ID, Role: "user", Content: content}
	if err := store.AddMessage(msg); err != nil {
		log.Printf("[webhook] save message failed: %v", err)
		return
	}
	// Kick off streaming
	go runStream(session, content, false)
}
