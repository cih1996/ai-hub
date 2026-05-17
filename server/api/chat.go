package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// RawRequestSnapshot holds the last raw request sent to Claude Code CLI for a session.
type RawRequestSnapshot struct {
	SystemPrompt            string    `json:"system_prompt"`
	Query                   string    `json:"query"`
	CapturedAt              time.Time `json:"captured_at"`
	EstimatedTokens         int       `json:"estimated_tokens"`
	ProviderMaxTokens       int       `json:"provider_max_tokens"`
	ThresholdPercent        int       `json:"threshold_percent"`
	ThresholdTokens         int       `json:"threshold_tokens"`
	UsagePercent            float64   `json:"usage_percent"`
	CompressionEnabled      bool      `json:"compression_enabled"`
	WouldTriggerCompression bool      `json:"would_trigger_compression"`
	CompressionTriggered    bool      `json:"compression_triggered"`
}

// lastRawRequests stores the most recent raw request per session (sessID → RawRequestSnapshot).
var lastRawRequests sync.Map

type WSMessage struct {
	Type      string `json:"type"` // "chat" | "stop" | "subscribe" | "error" | "chunk" | "thinking" | "tool_start" | "tool_input" | "tool_result" | "done" | "session_created" | "streaming_status" | "session_update"
	SessionID int64  `json:"session_id"`
	Content   string `json:"content"`
	Detail    string `json:"detail,omitempty"`
	ToolID    string `json:"tool_id,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
}

// ---- WS Client Hub: tracks all connected clients for broadcasting ----

type wsClient struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (c *wsClient) Send(msg WSMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	c.conn.WriteJSON(msg)
}

func (c *wsClient) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteMessage(websocket.PingMessage, nil)
}

var (
	wsClients   = make(map[*wsClient]struct{})
	wsClientsMu sync.RWMutex
	// pendingRecoverySeed stores one-shot context recovery text per session.
	// It is consumed on the next runStream turn after session reset actions
	// (e.g. compress/switch provider), to avoid context loss.
	pendingRecoverySeed   = make(map[int64]string)
	pendingRecoverySeedMu sync.Mutex
)

func registerClient(c *wsClient) {
	wsClientsMu.Lock()
	wsClients[c] = struct{}{}
	wsClientsMu.Unlock()
}

func unregisterClient(c *wsClient) {
	wsClientsMu.Lock()
	delete(wsClients, c)
	wsClientsMu.Unlock()
}

func setPendingRecoverySeed(sessionID int64, seed string) {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		return
	}
	pendingRecoverySeedMu.Lock()
	pendingRecoverySeed[sessionID] = seed
	pendingRecoverySeedMu.Unlock()
}

func takePendingRecoverySeed(sessionID int64) string {
	pendingRecoverySeedMu.Lock()
	defer pendingRecoverySeedMu.Unlock()
	seed := pendingRecoverySeed[sessionID]
	delete(pendingRecoverySeed, sessionID)
	return seed
}

// Broadcast sends a message to ALL connected WS clients
func broadcast(msg WSMessage) {
	wsClientsMu.RLock()
	defer wsClientsMu.RUnlock()
	for c := range wsClients {
		go c.Send(msg)
	}
}

// BroadcastProcessState sends process state change to all WS clients
func BroadcastProcessState(hubSessionID int64, alive bool, state string) {
	content := "process_exit"
	if alive {
		content = "process_alive:" + state
	}
	broadcast(WSMessage{Type: "process_update", SessionID: hubSessionID, Content: content})
}

// BroadcastRaw sends a raw WS message with given type and content.
func BroadcastRaw(msgType string, content string) {
	broadcast(WSMessage{Type: msgType, Content: content})
}

// IsSessionStreaming checks if a session is currently active
func IsSessionStreaming(sessionID int64) bool {
	activeStreamsMu.RLock()
	defer activeStreamsMu.RUnlock()
	_, ok := activeStreams[sessionID]
	return ok
}

// GetStreamingSessionIDs returns all currently streaming session IDs
func GetStreamingSessionIDs() map[int64]bool {
	activeStreamsMu.RLock()
	defer activeStreamsMu.RUnlock()
	result := make(map[int64]bool, len(activeStreams))
	for id := range activeStreams {
		result[id] = true
	}
	return result
}

// ActiveStream tracks an in-progress chat stream so new WS connections can reattach.
// It maintains a buffer of replayable events (chunk, thinking, tool_*) so that
// clients subscribing mid-stream can catch up on content produced before they attached.
type ActiveStream struct {
	mu       sync.Mutex
	sendFn   func(WSMessage)
	cancelFn context.CancelFunc
	buffer   []WSMessage // buffered events for replay on subscribe
}

func (s *ActiveStream) Send(msg WSMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Buffer replayable event types so subscribe can catch up
	switch msg.Type {
	case "chunk", "thinking", "tool_start", "tool_input", "tool_result":
		s.buffer = append(s.buffer, msg)
	}
	if s.sendFn != nil {
		s.sendFn(msg)
	}
}

// SwapSendAndReplay atomically replaces the send function and replays all
// buffered events to the new function. This ensures no events are lost
// between the swap and the replay.
func (s *ActiveStream) SwapSendAndReplay(fn func(WSMessage)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendFn = fn
	for _, msg := range s.buffer {
		fn(msg)
	}
}

func (s *ActiveStream) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFn != nil {
		s.cancelFn()
	}
}

var (
	claudeClient       = core.NewClaudeCodeClient()
	activeStreams      = make(map[int64]*ActiveStream)
	activeStreamsMu    sync.RWMutex
	forceFreshMu       sync.Mutex
	forceFreshRun      = make(map[int64]bool) // sessionID -> next run must start fresh (no --resume)
	queueRetryMu       sync.Mutex
	queueRetryCursor   = make(map[int64]int64) // sessionID -> trigger cursor to reuse after a queued batch failed
	runningQueueCursor = make(map[int64]int64) // sessionID -> original trigger cursor for current queued batch
)

func markForceFreshRun(sessionID int64) {
	forceFreshMu.Lock()
	forceFreshRun[sessionID] = true
	forceFreshMu.Unlock()
}

func markQueueBatchRunning(sessionID int64, originalCursor int64) {
	if originalCursor <= 0 {
		return
	}
	queueRetryMu.Lock()
	runningQueueCursor[sessionID] = originalCursor
	queueRetryMu.Unlock()
}

func queueBatchSucceeded(sessionID int64) {
	queueRetryMu.Lock()
	delete(runningQueueCursor, sessionID)
	delete(queueRetryCursor, sessionID)
	queueRetryMu.Unlock()
}

func queueBatchFailed(sessionID int64) {
	queueRetryMu.Lock()
	if cursor := runningQueueCursor[sessionID]; cursor > 0 {
		queueRetryCursor[sessionID] = cursor
	}
	delete(runningQueueCursor, sessionID)
	queueRetryMu.Unlock()
}

func takeQueueRetryCursor(sessionID int64) int64 {
	queueRetryMu.Lock()
	defer queueRetryMu.Unlock()
	return queueRetryCursor[sessionID]
}

func consumeForceFreshRun(sessionID int64) bool {
	forceFreshMu.Lock()
	defer forceFreshMu.Unlock()
	if !forceFreshRun[sessionID] {
		return false
	}
	delete(forceFreshRun, sessionID)
	return true
}

type autoCompressionResult struct {
	Triggered       bool
	DeletedCount    int64
	EstimatedTokens int
	ThresholdTokens int
}

const (
	roughBasePromptBytes   = 2000  // Base prompt overhead in bytes (JSON structure, role fields, etc.)
	roughPerMessageBytes   = 100   // Per-message JSON overhead (role, type, structure)
	roughPerImageBytes     = 50000 // Per-image attachment estimate (~50KB base64 encoded)
	roughToolDefBytes      = 30000 // Claude Code built-in tool definitions (Read, Edit, Bash, etc.)
	minCompressionLogCount = 2
)

// estimateTextBytes returns the UTF-8 byte length of the content.
func estimateTextBytes(content string) int {
	return len(content)
}

// estimateSystemPromptBytes reads the actual system prompt files and returns total byte size.
func estimateSystemPromptBytes(sessionID int64, groupName string) int {
	total := 0
	// ① Global rules (~/.ai-hub/rules/*.md)
	if rulesDir := core.TemplateDir(); rulesDir != "" {
		if entries, err := os.ReadDir(rulesDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					if data, err := os.ReadFile(filepath.Join(rulesDir, e.Name())); err == nil {
						total += len(data)
					}
				}
			}
		}
	}
	// ② Team rules (~/.ai-hub/teams/<group>/rules/*.md)
	if groupName != "" {
		teamsDir := filepath.Join(core.GetDataDir(), "teams", groupName, "rules")
		if entries, err := os.ReadDir(teamsDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					if data, err := os.ReadFile(filepath.Join(teamsDir, e.Name())); err == nil {
						total += len(data)
					}
				}
			}
		}
	}
	// ③ Session rules (session-rules/<id>.md)
	if sessionID > 0 {
		if rules, err := ReadSessionRules(sessionID); err == nil && rules != "" {
			total += len(rules)
		}
	}
	// Conservative floor
	if total < 5000 {
		total = 5000
	}
	return total
}

// estimateMessagesBytes returns the estimated byte size of the messages array.
func estimateMessagesBytes(msgs []model.Message) int {
	total := roughBasePromptBytes
	for _, msg := range msgs {
		total += len(msg.Content) + roughPerMessageBytes
	}
	return total
}

// estimateOutgoingBytes returns the estimated byte size of the outgoing message.
func estimateOutgoingBytes(content string, attachments []model.ChatAttachment) int {
	total := len(content) + roughPerMessageBytes
	for _, att := range attachments {
		if att.Type == "image" {
			total += roughPerImageBytes
		}
	}
	return total
}

type requestTokenEstimate struct {
	EstimatedTokens         int // Now represents estimated request size in bytes
	ProviderMaxTokens       int // Now represents max request size in bytes (provider.MaxTokens repurposed)
	ThresholdPercent        int
	ThresholdTokens         int // Threshold in bytes
	UsagePercent            float64
	CompressionEnabled      bool
	WouldTriggerCompression bool
	CompressionTriggered    bool
}

func buildRequestTokenEstimate(provider *model.Provider, activeMsgs []model.Message, outgoingContent string, attachments []model.ChatAttachment, sessionID int64, groupName string) (*requestTokenEstimate, *model.CompressionSettings) {
	estimate := &requestTokenEstimate{
		EstimatedTokens: estimateMessagesBytes(activeMsgs) + estimateOutgoingBytes(outgoingContent, attachments) +
			estimateSystemPromptBytes(sessionID, groupName) + roughToolDefBytes,
	}
	if provider != nil {
		estimate.ProviderMaxTokens = provider.MaxTokens // MaxTokens now represents max request size in bytes
	}

	settings, err := store.GetCompressionSettings()
	if err != nil {
		log.Printf("[compress] failed to load compression settings for estimate: %v", err)
		return estimate, &model.CompressionSettings{}
	}

	estimate.CompressionEnabled = settings.Enabled
	estimate.ThresholdPercent = settings.ThresholdPercent
	if estimate.ProviderMaxTokens > 0 {
		estimate.UsagePercent = float64(estimate.EstimatedTokens) * 100 / float64(estimate.ProviderMaxTokens)
		if settings.Enabled && settings.ThresholdPercent > 0 {
			estimate.ThresholdTokens = estimate.ProviderMaxTokens * settings.ThresholdPercent / 100
			estimate.WouldTriggerCompression = estimate.EstimatedTokens >= estimate.ThresholdTokens
		}
	}
	return estimate, settings
}

func storeInitialRawRequestSnapshot(sessionID int64, query string, estimate *requestTokenEstimate) {
	snap := RawRequestSnapshot{
		Query:      query,
		CapturedAt: time.Now(),
	}
	if estimate != nil {
		snap.EstimatedTokens = estimate.EstimatedTokens
		snap.ProviderMaxTokens = estimate.ProviderMaxTokens
		snap.ThresholdPercent = estimate.ThresholdPercent
		snap.ThresholdTokens = estimate.ThresholdTokens
		snap.UsagePercent = estimate.UsagePercent
		snap.CompressionEnabled = estimate.CompressionEnabled
		snap.WouldTriggerCompression = estimate.WouldTriggerCompression
		snap.CompressionTriggered = estimate.CompressionTriggered
	}
	lastRawRequests.Store(sessionID, snap)
	persistRawRequestSnapshot(sessionID, snap)
}

func updateRawRequestSnapshot(sessionID int64, systemPrompt string, query string) {
	snap := RawRequestSnapshot{
		SystemPrompt: systemPrompt,
		Query:        query,
		CapturedAt:   time.Now(),
	}
	if existing, ok := loadRawRequestSnapshot(sessionID); ok {
		snap.EstimatedTokens = existing.EstimatedTokens
		snap.ProviderMaxTokens = existing.ProviderMaxTokens
		snap.ThresholdPercent = existing.ThresholdPercent
		snap.ThresholdTokens = existing.ThresholdTokens
		snap.UsagePercent = existing.UsagePercent
		snap.CompressionEnabled = existing.CompressionEnabled
		snap.WouldTriggerCompression = existing.WouldTriggerCompression
		snap.CompressionTriggered = existing.CompressionTriggered
	}
	lastRawRequests.Store(sessionID, snap)
	persistRawRequestSnapshot(sessionID, snap)
}

func loadRawRequestSnapshot(sessionID int64) (RawRequestSnapshot, bool) {
	if val, ok := lastRawRequests.Load(sessionID); ok {
		return val.(RawRequestSnapshot), true
	}
	var payload string
	err := store.DB.QueryRow(`SELECT payload FROM session_raw_requests WHERE session_id = ?`, sessionID).Scan(&payload)
	if err == nil {
		var snap RawRequestSnapshot
		if err := json.Unmarshal([]byte(payload), &snap); err == nil {
			lastRawRequests.Store(sessionID, snap)
			return snap, true
		}
	}
	return RawRequestSnapshot{}, false
}

func persistRawRequestSnapshot(sessionID int64, snap RawRequestSnapshot) {
	payload, err := json.Marshal(snap)
	if err == nil {
		store.DB.Exec(`INSERT INTO session_raw_requests (session_id, payload, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP) ON CONFLICT(session_id) DO UPDATE SET payload=excluded.payload, updated_at=CURRENT_TIMESTAMP`, sessionID, string(payload))
	}
}

// maybeAutoCompressBeforeRun is a reusable compression check called before any runStream invocation.
// It ensures webhook, hook, and queued message paths also trigger compression when needed.
// Returns true if compression was triggered (caller can broadcast context_reset if desired).
func maybeAutoCompressBeforeRun(session *model.Session, content string) bool {
	if session == nil {
		return false
	}
	provider := resolveSessionProviderForSend(session)
	if provider == nil {
		return false
	}
	activeMsgs, err := store.GetMessages(session.ID)
	if err != nil {
		log.Printf("[compress] session %d: failed to load messages: %v", session.ID, err)
		return false
	}
	estimate, compressionSettings := buildRequestTokenEstimate(provider, activeMsgs, content, nil, session.ID, session.GroupName)
	compressResult, err := maybeAutoCompressSession(session, provider, activeMsgs, compressionSettings, estimate)
	if err != nil {
		log.Printf("[compress] session %d: auto compression error: %v", session.ID, err)
		return false
	}
	if compressResult != nil && compressResult.Triggered {
		broadcast(WSMessage{
			Type:      "context_reset",
			SessionID: session.ID,
			Content:   fmt.Sprintf("deleted:%d", compressResult.DeletedCount),
			Detail:    fmt.Sprintf("estimated_tokens=%d threshold_tokens=%d", compressResult.EstimatedTokens, compressResult.ThresholdTokens),
		})
		log.Printf("[compress] session %d: auto-compressed before run (deleted=%d, estimated=%d, threshold=%d)",
			session.ID, compressResult.DeletedCount, compressResult.EstimatedTokens, compressResult.ThresholdTokens)
		go core.FireHooks(core.HookEvent{
			Type:            "session.compressed",
			SourceSessionID: session.ID,
			Content:         fmt.Sprintf("压缩完成，删除 %d 条消息，请求体 %d / 阈值 %d", compressResult.DeletedCount, compressResult.EstimatedTokens, compressResult.ThresholdTokens),
		})
		return true
	}
	return false
}

func resolveSessionProviderForSend(session *model.Session) *model.Provider {
	if session == nil {
		return nil
	}
	provider, err := store.GetProvider(session.ProviderID)
	if err == nil {
		return provider
	}
	log.Printf("[chat] session %d: provider %s not found before send, trying default", session.ID, session.ProviderID)
	provider, err = store.GetDefaultProvider()
	if err != nil {
		log.Printf("[chat] session %d: unable to resolve provider before send: %v", session.ID, err)
		return nil
	}
	session.ProviderID = provider.ID
	if err := store.UpdateSessionProvider(session.ID, provider.ID); err != nil {
		log.Printf("[chat] session %d: failed to persist fallback provider %s: %v", session.ID, provider.ID, err)
	}
	return provider
}

func resetSessionRuntimeContext(session *model.Session) (int64, error) {
	if session == nil {
		return 0, fmt.Errorf("session is nil")
	}
	deleted, err := store.ResetSessionMessages(session.ID, 0)
	if err != nil {
		return 0, err
	}
	core.Pool.Kill(session.ID)
	newUUID := uuid.New().String()
	if err := store.UpdateClaudeSessionID(session.ID, newUUID); err != nil {
		return 0, err
	}
	session.ClaudeSessionID = newUUID
	markForceFreshRun(session.ID)
	return deleted, nil
}

func maybeAutoCompressSession(session *model.Session, provider *model.Provider, activeMsgs []model.Message, compressionSettings *model.CompressionSettings, estimate *requestTokenEstimate) (*autoCompressionResult, error) {
	result := &autoCompressionResult{}
	if session == nil || provider == nil {
		return result, nil
	}
	if provider.MaxTokens <= 0 {
		return result, nil
	}
	if compressionSettings == nil {
		compressionSettings = &model.CompressionSettings{}
	}
	if !compressionSettings.Enabled || compressionSettings.ThresholdPercent <= 0 {
		return result, nil
	}
	if len(activeMsgs) == 0 {
		return result, nil
	}
	if estimate == nil || estimate.ThresholdTokens <= 0 {
		return result, nil
	}
	result.EstimatedTokens = estimate.EstimatedTokens
	result.ThresholdTokens = estimate.ThresholdTokens
	if estimate.EstimatedTokens < estimate.ThresholdTokens {
		return result, nil
	}

	logs, err := store.GetConversationLogs(session.ID)
	if err != nil {
		return nil, err
	}
	if len(logs) < minCompressionLogCount {
		return result, nil
	}

	// Notify frontend that compression is starting
	broadcast(WSMessage{
		Type:      "compressing",
		SessionID: session.ID,
		Content:   fmt.Sprintf("estimated=%d threshold=%d", estimate.EstimatedTokens, estimate.ThresholdTokens),
	})

	seed, err := core.BuildIntelligentRecoverySeed(logs, provider, session.ID, compressionSettings.SystemPrompt)
	if err != nil {
		log.Printf("[compress] session %d: intelligent compression failed, using full log recovery seed: %v", session.ID, err)
		seed = buildRecoverySeedFromLogs(logs, "上下文自动压缩后恢复")
	}
	seed = strings.TrimSpace(seed)
	if seed == "" {
		return result, nil
	}

	deleted, err := resetSessionRuntimeContext(session)
	if err != nil {
		return nil, err
	}
	setPendingRecoverySeed(session.ID, seed)

	result.Triggered = true
	result.DeletedCount = deleted
	estimate.CompressionTriggered = true
	log.Printf("[compress] session %d: auto compression triggered, estimated=%d threshold=%d deleted=%d", session.ID, result.EstimatedTokens, result.ThresholdTokens, deleted)
	return result, nil
}

func HandleChat(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	client := &wsClient{conn: conn}
	registerClient(client)
	defer unregisterClient(client)

	// Heartbeat: ping every 30s, expect pong within 60s
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := client.Ping(); err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()
	defer close(pingDone)

	sendJSON := func(msg WSMessage) {
		client.Send(msg)
	}

	var subscribedSessionID int64

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg WSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "stop":
			if subscribedSessionID > 0 {
				activeStreamsMu.RLock()
				stream, ok := activeStreams[subscribedSessionID]
				activeStreamsMu.RUnlock()
				if ok {
					stream.Cancel()
				}
			}
		case "subscribe":
			subscribedSessionID = msg.SessionID
			activeStreamsMu.RLock()
			stream, ok := activeStreams[msg.SessionID]
			activeStreamsMu.RUnlock()
			if ok {
				// Atomically swap send function and replay all buffered events
				// so the client catches up on content produced before subscribing
				stream.SwapSendAndReplay(sendJSON)
				sendJSON(WSMessage{Type: "streaming_status", SessionID: msg.SessionID, Content: "streaming"})
			} else {
				// Session is not streaming — tell client to correct its state
				sendJSON(WSMessage{Type: "streaming_status", SessionID: msg.SessionID, Content: "idle"})
			}
		}
	}
}

// SendChat handles POST /api/v1/chat/send
// Validates/creates session, saves user message, kicks off streaming in background, returns immediately.
func SendChat(c *gin.Context) {
	var req struct {
		SessionID    int64                  `json:"session_id"`
		Content      string                 `json:"content"`
		WorkDir      string                 `json:"work_dir"`
		GroupName    string                 `json:"group_name"`
		SessionRules string                 `json:"session_rules"`
		ProviderID   string                 `json:"provider_id"`
		Attachments  []model.ChatAttachment `json:"attachments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Content) == "" && len(req.Attachments) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content or attachments is required"})
		return
	}
	for _, att := range req.Attachments {
		if att.Type != "image" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported attachment type: " + att.Type})
			return
		}
		if !strings.HasPrefix(att.MimeType, "image/") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid attachment mime_type: " + att.MimeType})
			return
		}
		if strings.TrimSpace(att.Data) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "attachment data is required"})
			return
		}
	}

	storedContent := buildStoredUserContent(req.Content, req.Attachments)
	queryContent := buildClaudeQuery(req.Content, req.Attachments)

	var session *model.Session
	var providerForEstimate *model.Provider
	var requestEstimate *requestTokenEstimate
	isNewSession := req.SessionID == 0

	if isNewSession {
		var providerID string
		if req.ProviderID != "" {
			// Use explicitly specified provider
			p, err := store.GetProvider(req.ProviderID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "specified provider not found"})
				return
			}
			providerForEstimate = p
			providerID = p.ID
		} else {
			// Fall back to default provider
			provider, err := store.GetDefaultProvider()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No default provider configured. Go to Settings to add one."})
				return
			}
			providerForEstimate = provider
			providerID = provider.ID
		}
		var err error
		session, err = store.CreateSessionWithMessage(providerID, storedContent, req.WorkDir, req.GroupName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create session failed: " + err.Error()})
			return
		}
		// Broadcast new session to all connected clients
		sessionJSON, _ := json.Marshal(session)
		broadcast(WSMessage{Type: "session_created", SessionID: session.ID, Content: string(sessionJSON)})

		// Fire session.created hooks
		go core.FireHooks(core.HookEvent{
			Type:            "session.created",
			SourceSessionID: session.ID,
			Content:         storedContent,
		})

		// Write session rules before starting stream (avoids race condition with putSessionRules)
		if req.SessionRules != "" {
			dir := sessionRulesDir()
			os.MkdirAll(dir, 0755)
			os.WriteFile(sessionRulesPath(session.ID), []byte(req.SessionRules), 0644)
		}
		requestEstimate, _ = buildRequestTokenEstimate(providerForEstimate, nil, storedContent, req.Attachments, session.ID, req.GroupName)
	} else {
		var err error
		session, err = store.GetSession(req.SessionID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}

		// Check if session is already streaming — queue message instead of rejecting
		if IsSessionStreaming(session.ID) {
			userMsg := &model.Message{
				SessionID: session.ID,
				Role:      "user",
				Content:   storedContent,
			}
			if err := store.AddMessage(userMsg); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "save message failed: " + err.Error()})
				return
			}
			if err := store.AddConversationLog(&model.ConversationLog{
				SessionID: session.ID,
				MessageID: userMsg.ID,
				Role:      "user",
				Content:   storedContent,
			}); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "save conversation log failed: " + err.Error()})
				return
			}
			log.Printf("[chat] session %d is streaming, message queued (msg_id=%d)", session.ID, userMsg.ID)
			// Broadcast queued message so frontend displays the original text only.
			broadcast(WSMessage{Type: "message_queued", SessionID: session.ID, Content: storedContent})
			c.JSON(http.StatusOK, gin.H{
				"session_id": session.ID,
				"status":     "queued",
			})
			return
		}

		provider := resolveSessionProviderForSend(session)
		providerForEstimate = provider
		activeMsgs, err := store.GetMessages(session.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load messages failed: " + err.Error()})
			return
		}

		// Compression check: use last proxy body size (real data) as primary signal.
		// Only fall back to rough estimate when no proxy data exists (new sessions).
		compressionSettings, _ := store.GetCompressionSettings()
		var requestEstimate *requestTokenEstimate
		if compressionSettings != nil && compressionSettings.Enabled && provider != nil && provider.MaxTokens > 0 {
			thresholdBytes := provider.MaxTokens * compressionSettings.ThresholdPercent / 100
			if lastBodySize, err := store.GetLatestRequestBodySize(session.ID); err == nil && lastBodySize > 0 && thresholdBytes > 0 && lastBodySize >= int64(thresholdBytes) {
				// Last proxy body already at/over threshold — compress NOW using real data
				log.Printf("[compress] session %d: last proxy body %d >= threshold %d, triggering compression before send", session.ID, lastBodySize, thresholdBytes)
				requestEstimate = &requestTokenEstimate{
					EstimatedTokens:    int(lastBodySize),
					ProviderMaxTokens:  provider.MaxTokens,
					ThresholdPercent:   compressionSettings.ThresholdPercent,
					ThresholdTokens:    int(thresholdBytes),
					CompressionEnabled: true,
				}
				compressResult, err := maybeAutoCompressSession(session, provider, activeMsgs, compressionSettings, requestEstimate)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "auto compression failed: " + err.Error()})
					return
				}
				if compressResult != nil && compressResult.Triggered {
					// Reload messages after compression
					activeMsgs, _ = store.GetMessages(session.ID)
					broadcast(WSMessage{
						Type:      "context_reset",
						SessionID: session.ID,
						Content:   fmt.Sprintf("deleted:%d", compressResult.DeletedCount),
						Detail:    fmt.Sprintf("estimated_tokens=%d threshold_tokens=%d", compressResult.EstimatedTokens, compressResult.ThresholdTokens),
					})
					go core.FireHooks(core.HookEvent{
						Type:            "session.compressed",
						SourceSessionID: session.ID,
						Content:         fmt.Sprintf("压缩完成，删除 %d 条消息，请求体 %d / 阈值 %d", compressResult.DeletedCount, compressResult.EstimatedTokens, compressResult.ThresholdTokens),
					})
				}
			}
		}
		// Fallback: build rough estimate (for progress bar display and new sessions without proxy data)
		if requestEstimate == nil {
			requestEstimate, _ = buildRequestTokenEstimate(provider, activeMsgs, storedContent, req.Attachments, session.ID, session.GroupName)
		}
		userMsg := &model.Message{
			SessionID: session.ID,
			Role:      "user",
			Content:   storedContent,
		}
		if err := store.AddMessage(userMsg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "save message failed: " + err.Error()})
			return
		}
		if err := store.AddConversationLog(&model.ConversationLog{
			SessionID: session.ID,
			MessageID: userMsg.ID,
			Role:      "user",
			Content:   storedContent,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "save conversation log failed: " + err.Error()})
			return
		}
	}

	storeInitialRawRequestSnapshot(session.ID, queryContent, requestEstimate)

	// Broadcast context usage to frontend for energy progress bar.
	// Use the latest proxy body size as baseline when available (accurate),
	// falling back to the rough estimate for new sessions.
	if requestEstimate != nil && requestEstimate.ProviderMaxTokens > 0 {
		displayPct := 0.0
		if requestEstimate.ThresholdTokens > 0 {
			// Try to anchor to the last actual proxy body size for better accuracy
			estimatedBytes := requestEstimate.EstimatedTokens
			if lastBodySize, err := store.GetLatestRequestBodySize(session.ID); err == nil && lastBodySize > 0 {
				// Baseline from real proxy data + estimated new message overhead
				outgoingEstimate := estimateOutgoingBytes(queryContent, nil)
				estimatedBytes = int(lastBodySize) + outgoingEstimate
			}
			displayPct = float64(estimatedBytes) * 100 / float64(requestEstimate.ThresholdTokens)
			if displayPct > 100 {
				displayPct = 100
			}
		}
		ctxInfo, _ := json.Marshal(gin.H{
			"estimated_tokens":   requestEstimate.EstimatedTokens,
			"threshold_percent":  requestEstimate.ThresholdPercent,
			"threshold_tokens":   requestEstimate.ThresholdTokens,
			"display_percent":    displayPct,
			"compression_enabled": requestEstimate.CompressionEnabled,
		})
		broadcast(WSMessage{
			Type:      "context_usage",
			SessionID: session.ID,
			Content:   string(ctxInfo),
		})
	}

	// Fire message.received hooks (for both new and existing sessions)
	go fireMessageReceivedHook(session.ID, storedContent)

	// Kick off streaming in background — results are pushed via WS broadcast
	triggerMsgID := store.GetLastUserMessageID(session.ID)
	go runStream(session, queryContent, isNewSession, triggerMsgID)

	c.JSON(http.StatusOK, gin.H{
		"session_id": session.ID,
		"status":     "started",
	})
}

func buildStoredUserContent(content string, attachments []model.ChatAttachment) string {
	text := strings.TrimSpace(content)
	if len(attachments) == 0 {
		return text
	}

	parts := make([]string, 0, len(attachments)+1)
	if text != "" {
		parts = append(parts, text)
	}
	for _, att := range attachments {
		if att.Type != "image" {
			continue
		}
		name := strings.TrimSpace(att.Name)
		if name == "" {
			name = "图片"
		}
		// Store as markdown image with base64 data URL so frontend can render inline
		if att.Data != "" && att.MimeType != "" {
			parts = append(parts, fmt.Sprintf("![%s](data:%s;base64,%s)", name, att.MimeType, att.Data))
		} else {
			parts = append(parts, fmt.Sprintf("[图片附件: %s]", name))
		}
	}
	return strings.Join(parts, "\n\n")
}

func buildAnthropicQueryPayload(query string) []map[string]any {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return []map[string]any{{"type": "text", "text": ""}}
	}
	if !strings.HasPrefix(trimmed, "{") {
		return []map[string]any{{"type": "text", "text": query}}
	}
	var payload struct {
		Type    string           `json:"type"`
		Content []map[string]any `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil || payload.Type != "multimodal" || len(payload.Content) == 0 {
		return []map[string]any{{"type": "text", "text": query}}
	}
	return payload.Content
}

func buildClaudeQuery(content string, attachments []model.ChatAttachment) string {
	if len(attachments) == 0 {
		return strings.TrimSpace(content)
	}
	blocks := buildAnthropicQueryPayload(strings.TrimSpace(content))
	if len(blocks) == 1 {
		if t, ok := blocks[0]["type"].(string); ok && t == "text" {
			if txt, ok := blocks[0]["text"].(string); ok && strings.TrimSpace(txt) == "" {
				blocks = []map[string]any{}
			}
		}
	}
	for _, att := range attachments {
		if att.Type != "image" {
			continue
		}
		blocks = append(blocks, map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": att.MimeType,
				"data":       att.Data,
			},
		})
	}
	if len(blocks) == 0 {
		return strings.TrimSpace(content)
	}
	payload, err := json.Marshal(map[string]any{
		"type":    "multimodal",
		"content": blocks,
	})
	if err != nil {
		return strings.TrimSpace(content)
	}
	return string(payload)
}

// runStream executes the AI streaming in background, pushing events via WS to subscribed clients
func runStream(session *model.Session, query string, isNewSession bool, triggerMsgID int64) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamErr := error(nil)

	// Start with a no-op send — a client will attach via "subscribe"
	stream := &ActiveStream{sendFn: func(WSMessage) {}, cancelFn: cancel}

	// Register active stream so clients can subscribe
	activeStreamsMu.Lock()
	activeStreams[session.ID] = stream
	activeStreamsMu.Unlock()
	log.Printf("[chat-run] start session=%d triggerMsgID=%d isNew=%v", session.ID, triggerMsgID, isNewSession)
	broadcast(WSMessage{Type: "session_update", SessionID: session.ID, Content: "streaming"})
	defer func() {
		activeStreamsMu.Lock()
		delete(activeStreams, session.ID)
		activeStreamsMu.Unlock()
		broadcast(WSMessage{Type: "session_update", SessionID: session.ID, Content: "idle"})
		if streamErr != nil {
			queueBatchFailed(session.ID)
		} else {
			queueBatchSucceeded(session.ID)
		}
		log.Printf("[chat-run] end session=%d triggerMsgID=%d err=%v", session.ID, triggerMsgID, streamErr)
		// Process any messages that were queued while streaming
		processQueuedMessages(session.ID, triggerMsgID)
	}()

	provider, err := store.GetProvider(session.ProviderID)
	if err != nil {
		// Provider deleted — fallback to default provider
		log.Printf("[chat] session %d: provider %s not found, trying default", session.ID, session.ProviderID)
		provider, err = store.GetDefaultProvider()
		if err != nil {
			errMsg := "provider not found and no default provider configured"
			streamErr = fmt.Errorf("%s", errMsg)
			broadcast(WSMessage{Type: "error", SessionID: session.ID, Content: errMsg})
			return
		}
		// Update session to use the default provider
		log.Printf("[chat] session %d: falling back to default provider %s (%s)", session.ID, provider.Name, provider.ID)
		_ = store.UpdateSessionProvider(session.ID, provider.ID)
	}

	var fullResponse string
	var metadataJSON string
	var usageInput, usageOutput, usageCacheCreation, usageCacheRead int64

	// Pre-insert empty assistant message for incremental saves (Issue #163)
	// This ensures partial content survives process crashes during streaming.
	assistantMsg := &model.Message{
		SessionID: session.ID,
		Role:      "assistant",
		Content:   "",
		Metadata:  "",
	}
	if err := store.AddMessage(assistantMsg); err != nil {
		streamErr = fmt.Errorf("failed to save assistant message: %w", err)
		log.Printf("[chat] session=%d failed to pre-insert assistant message: %v", session.ID, err)
		broadcast(WSMessage{Type: "error", SessionID: session.ID, Content: "failed to save message"})
		return
	}
	progressMsgID := assistantMsg.ID

	// Reset proxy usage accumulator at stream start (Issue #72)
	ResetProxyUsage(session.ID)

	log.Printf("[chat] session=%d provider=%s mode=%s model=%s base_url=%s",
		session.ID, provider.Name, provider.Mode, provider.ModelID, provider.BaseURL)

	// Save original query for retry (before seed is prepended)
	originalQuery := query

	// One-shot recovery seed after session reset actions.
	if seed := takePendingRecoverySeed(session.ID); strings.TrimSpace(seed) != "" {
		query = seed + "\n\n---\n\n" + query
	}

	// --- Retry loop for "No conversation found" (silent auto-recovery) ---
	// When the Claude CLI reports the session is gone, we silently reset and retry
	// instead of exposing the raw error to the user. Maximum 1 retry.
	const maxNoConvRetries = 1
	for noConvRetry := 0; noConvRetry <= maxNoConvRetries; noConvRetry++ {
		// isResume: true when the persistent process is alive OR when the session has
		// completed assistant messages in DB (i.e., we need to restore the conversation).
		// If previous turn detected "No conversation found", force one fresh run to avoid
		// getting stuck in a resume loop.
		isResume := !isNewSession && (core.Pool.HasProcess(session.ID) || store.HasAssistantMessages(session.ID))
		if consumeForceFreshRun(session.ID) {
			isResume = false
		}
		fullResponse, metadataJSON, usageInput, usageOutput, usageCacheCreation, usageCacheRead, err = streamClaudeCode(ctx, provider, query, session.ClaudeSessionID, isResume, stream.Send, session.ID, session.WorkDir, session.GroupName, progressMsgID)
		streamErr = err

		log.Printf("[chat-flow] session=%d triggerMsgID=%d progressMsgID=%d streamClaudeCode returned: err=%v, fullResponse_len=%d, metadata_len=%d",
			session.ID, triggerMsgID, progressMsgID, err, len(fullResponse), len(metadataJSON))

		// Auto-retry on "No conversation found" — silent recovery, user sees nothing
		if err != nil && noConvRetry < maxNoConvRetries && strings.Contains(strings.ToLower(err.Error()), "no conversation found") {
			log.Printf("[chat] session=%d: 'No conversation found' detected, silently retrying (attempt %d/%d)", session.ID, noConvRetry+1, maxNoConvRetries)
			// Clean up the failed assistant message (no conversation log exists yet for empty msg)
			store.DeleteMessage(progressMsgID)
			// Reload session (claude_session_id was reset by streamClaudeCode)
			if fresh, serr := store.GetSession(session.ID); serr == nil {
				session = fresh
			}
			// Consume recovery seed (set by streamClaudeCode's error handler) + original query
			if seed := takePendingRecoverySeed(session.ID); strings.TrimSpace(seed) != "" {
				query = seed + "\n\n---\n\n" + originalQuery
			}
			// Create a fresh assistant message for the retry
			retryMsg := &model.Message{SessionID: session.ID, Role: "assistant", Content: "", Metadata: ""}
			if err := store.AddMessage(retryMsg); err != nil {
				log.Printf("[chat] session=%d: failed to create retry message: %v", session.ID, err)
				break
			}
			progressMsgID = retryMsg.ID
			ResetProxyUsage(session.ID)
			continue // retry
		}

		break // success or non-retryable error
	}

	var proxyRequestBodySize int64

	if err != nil {
		log.Printf("[chat] session=%d provider=%s error: %v", session.ID, provider.Name, err)
		// Prefer proxy-captured usage on error path too (Issue #72)
		if pu := ConsumeProxyUsage(session.ID); pu != nil {
			usageInput = pu.InputTokens
			usageOutput = pu.OutputTokens
			usageCacheCreation = pu.CacheCreationInputTokens
			usageCacheRead = pu.CacheReadInputTokens
			proxyRequestBodySize = pu.RequestBodySize
		}
		// Save partial response before reporting error — don't lose already-received content
		if fullResponse != "" || metadataJSON != "" {
			content := fullResponse
			store.UpdateMessageContent(progressMsgID, content, metadataJSON)
			if strings.TrimSpace(content) != "" {
				if err := store.AddConversationLog(&model.ConversationLog{
					SessionID: session.ID,
					MessageID: progressMsgID,
					Role:      "assistant",
					Content:   content,
				}); err != nil {
					log.Printf("[chat] session=%d failed to archive assistant log: %v", session.ID, err)
				}
			}
			extractAndSaveErrors(session.ID, progressMsgID, content)
			// Save token usage even on error (partial response)
			if usageInput > 0 || usageOutput > 0 || usageCacheCreation > 0 || usageCacheRead > 0 {
				tu := &model.TokenUsage{SessionID: session.ID, MessageID: progressMsgID, InputTokens: usageInput, OutputTokens: usageOutput, CacheCreationInputTokens: usageCacheCreation, CacheReadInputTokens: usageCacheRead}
				store.AddTokenUsage(tu)
				usageJSON, _ := json.Marshal(tu)
				broadcast(WSMessage{Type: "token_usage", SessionID: session.ID, Content: string(usageJSON)})
			}
		} else {
			// No content received — save error to DB for debugging, but don't broadcast
			// as chunk (the error event below handles frontend display)
			errContent := "❌ " + err.Error()
			store.UpdateMessageContent(progressMsgID, errContent, "")
			if strings.TrimSpace(errContent) != "" {
				if logErr := store.AddConversationLog(&model.ConversationLog{
					SessionID: session.ID,
					MessageID: progressMsgID,
					Role:      "assistant",
					Content:   errContent,
				}); logErr != nil {
					log.Printf("[chat] session=%d failed to archive assistant error log: %v", session.ID, logErr)
				}
			}
		}
		broadcast(WSMessage{Type: "error", SessionID: session.ID, Content: err.Error()})
		return
	}

	// Prefer proxy-captured usage (has accurate cache tokens) over stream-json fallback (Issue #72)
	if pu := ConsumeProxyUsage(session.ID); pu != nil {
		log.Printf("[chat] session=%d using proxy usage: input=%d output=%d cache_create=%d cache_read=%d body_size=%d",
			session.ID, pu.InputTokens, pu.OutputTokens, pu.CacheCreationInputTokens, pu.CacheReadInputTokens, pu.RequestBodySize)
		usageInput = pu.InputTokens
		usageOutput = pu.OutputTokens
		usageCacheCreation = pu.CacheCreationInputTokens
		usageCacheRead = pu.CacheReadInputTokens
		proxyRequestBodySize = pu.RequestBodySize
	}

	if fullResponse != "" || metadataJSON != "" {
		content := fullResponse
		// Final update of the pre-inserted assistant message
		store.UpdateMessageContent(progressMsgID, content, metadataJSON)
		if strings.TrimSpace(content) != "" {
			if err := store.AddConversationLog(&model.ConversationLog{
				SessionID: session.ID,
				MessageID: progressMsgID,
				Role:      "assistant",
				Content:   content,
			}); err != nil {
				log.Printf("[chat] session=%d failed to archive assistant log: %v", session.ID, err)
			}
		}
		extractAndSaveErrors(session.ID, progressMsgID, content)
		// Save and broadcast token usage
		if usageInput > 0 || usageOutput > 0 || usageCacheCreation > 0 || usageCacheRead > 0 || proxyRequestBodySize > 0 {
			tu := &model.TokenUsage{SessionID: session.ID, MessageID: progressMsgID, InputTokens: usageInput, OutputTokens: usageOutput, CacheCreationInputTokens: usageCacheCreation, CacheReadInputTokens: usageCacheRead, RequestBodySize: proxyRequestBodySize}
			store.AddTokenUsage(tu)
			usageJSON, _ := json.Marshal(tu)
			broadcast(WSMessage{Type: "token_usage", SessionID: session.ID, Content: string(usageJSON)})
		}
		// Fire message.sent hook
		go core.FireHooks(core.HookEvent{
			Type:            "message.sent",
			SourceSessionID: session.ID,
			Content:         content,
		})
	} else {
		// No content received — remove the empty pre-inserted message
		log.Printf("[chat-flow] session=%d no content received, deleting empty message %d", session.ID, progressMsgID)
		store.DeleteMessage(progressMsgID)
	}

	streamErr = nil

	// Re-broadcast context usage after streaming completes using ACTUAL request body size from proxy.
	// Falls back gracefully if proxy body size not available (e.g. claude-code provider).
	if proxyRequestBodySize > 0 {
		if provider := resolveSessionProviderForSend(session); provider != nil && provider.MaxTokens > 0 {
			if settings, err := store.GetCompressionSettings(); err == nil && settings.Enabled && settings.ThresholdPercent > 0 {
				thresholdBytes := int64(provider.MaxTokens) * int64(settings.ThresholdPercent) / 100
				displayPct := float64(proxyRequestBodySize) * 100 / float64(thresholdBytes)
				if displayPct > 100 {
					displayPct = 100
				}
				ctxInfo, _ := json.Marshal(gin.H{
					"estimated_tokens":    proxyRequestBodySize, // bytes
					"threshold_percent":   settings.ThresholdPercent,
					"threshold_tokens":    thresholdBytes, // bytes
					"display_percent":     displayPct,
					"compression_enabled": true,
				})
				broadcast(WSMessage{Type: "context_usage", SessionID: session.ID, Content: string(ctxInfo)})
			}
		}
	}

	// Broadcast done so even reconnected/new WS clients receive it (stream.Send is single-client)
	log.Printf("[chat-flow] session=%d broadcasting done event", session.ID)
	broadcast(WSMessage{Type: "done", SessionID: session.ID, Content: metadataJSON})
}

func buildMergedQueuedInput(pending []model.Message) string {
	if len(pending) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("以下是你处理上一条消息期间收到的 %d 条新消息，请按顺序综合处理：", len(pending)))
	for i, m := range pending {
		sb.WriteString("\n\n")
		if i > 0 {
			sb.WriteString("---\n\n")
		}
		sb.WriteString(fmt.Sprintf("[消息 %d | message_id=%d | 来源=%s]\n", i+1, m.ID, detectMessageSource(m.Content)))
		sb.WriteString(m.Content)
	}
	return sb.String()
}

func detectMessageSource(content string) string {
	firstLine := strings.TrimSpace(strings.SplitN(content, "\n", 2)[0])
	if firstLine == "" {
		return "用户"
	}
	if strings.HasPrefix(firstLine, "【") && strings.Contains(firstLine, "】") {
		return firstLine
	}
	return "用户"
}

// processQueuedMessages checks for user messages that arrived after triggerMsgID,
// merges them, and kicks off a new runStream to process them.
func processQueuedMessages(sessionID int64, triggerMsgID int64) {
	originalTriggerMsgID := triggerMsgID
	if retryCursor := takeQueueRetryCursor(sessionID); retryCursor > 0 && retryCursor < triggerMsgID {
		log.Printf("[queue] session %d: retrying queued batch from cursor %d instead of %d", sessionID, retryCursor, triggerMsgID)
		triggerMsgID = retryCursor
	}
	pending, err := store.GetPendingUserMessages(sessionID, triggerMsgID)
	if err != nil {
		log.Printf("[queue] session %d: failed to load pending messages after %d: %v", sessionID, triggerMsgID, err)
		return
	}
	if len(pending) == 0 {
		return
	}

	// Guard: if another stream already started (race), bail out
	if IsSessionStreaming(sessionID) {
		log.Printf("[queue] session %d: skip processing %d queued message(s), session already streaming", sessionID, len(pending))
		return
	}

	session, err := store.GetSession(sessionID)
	if err != nil {
		log.Printf("[queue] session %d not found: %v", sessionID, err)
		return
	}

	// Merge all pending messages into one query. Do not persist this wrapper;
	// messages and conversation_logs keep the original user inputs.
	merged := buildMergedQueuedInput(pending)

	// Use the last pending message ID as the cursor for the next round only after success.
	// On failure, the original cursor is stored so the same batch can be retried.
	newTriggerMsgID := pending[len(pending)-1].ID
	markQueueBatchRunning(sessionID, triggerMsgID)

	log.Printf("[queue] session %d: processing %d queued message(s), triggerMsgID %d(original=%d) -> %d", sessionID, len(pending), triggerMsgID, originalTriggerMsgID, newTriggerMsgID)
	maybeAutoCompressBeforeRun(session, merged)
	go runStream(session, merged, false, newTriggerMsgID)
}

// StepInfo represents a single execution step for metadata persistence
type StepInfo struct {
	Type   string `json:"type"`             // "thinking" | "tool"
	Name   string `json:"name,omitempty"`   // tool name
	Input  string `json:"input,omitempty"`  // tool input summary
	Status string `json:"status,omitempty"` // "done"
}

// StepsMetadata is the JSON structure stored in message.metadata
type StepsMetadata struct {
	Steps    []StepInfo `json:"steps"`
	Thinking string     `json:"thinking,omitempty"` // truncated thinking summary
}

func runtimeTemplateVars(sessID int64, groupName string) map[string]string {
	vars := map[string]string{
		"AI_HUB_SESSION_ID": strconv.FormatInt(sessID, 10),
	}
	if port := core.GetPort(); port != "" {
		vars["AI_HUB_PORT"] = port
		vars["AI_HUB_SESSION_MESSAGES_API"] = "http://127.0.0.1:" + port + "/api/v1/sessions/" + strconv.FormatInt(sessID, 10) + "/messages"
	}
	if strings.TrimSpace(groupName) != "" {
		vars["AI_HUB_GROUP_NAME"] = groupName
	}
	return vars
}


// buildTeamMembersList generates a markdown table of team members for injection into system prompt
func buildTeamMembersList(groupName string, currentSessionID int64) string {
	if groupName == "" {
		return ""
	}
	sessions, err := store.ListSessionsByGroup(groupName)
	if err != nil || len(sessions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# 团队成员\n\n")
	sb.WriteString("| 会话ID | 角色名称 |\n")
	sb.WriteString("|--------|----------|\n")

	for _, s := range sessions {
		marker := ""
		if s.ID == currentSessionID {
			marker = " (当前)"
		}
		sb.WriteString(fmt.Sprintf("| %d | %s%s |\n", s.ID, s.Title, marker))
	}

	return sb.String()
}

func streamClaudeCode(ctx context.Context, p *model.Provider, query, sessionID string, resume bool, send func(WSMessage), sessID int64, workDir string, groupName string, progressMsgID int64) (string, string, int64, int64, int64, int64, error) {
	req := core.ClaudeCodeRequest{
		Query:        query,
		SessionID:    sessionID,
		Resume:       resume,
		BaseURL:      p.BaseURL,
		APIKey:       p.APIKey,
		AuthMode:     p.AuthMode,
		ProxyURL:     p.ProxyURL,
		ModelID:      strings.TrimSpace(p.ModelID),
		WorkDir:      workDir,
		HubSessionID: sessID,
		GroupName:    groupName,
	}
	// OAuth/subscription mode uses Claude default model selection.
	if p.AuthMode == "oauth" {
		req.ModelID = ""
	}

	// Build system prompt: 三层合并（优先级从低到高）
	// ① 全局规则 (~/.ai-hub/rules/*.md)
	// ② 团队规则 (~/.ai-hub/teams/<group_name>/rules/*.md)，仅团队会话生效
	// ③ 会话角色规则 (session-rules/<sessID>.md)
	// 变量替换：支持会话级动态变量（如 {{AI_HUB_SESSION_ID}}）
	// 注：Claude CLI 的 --setting-sources 可控制是否加载项目级 .claude/CLAUDE.md，
	//     当前暂不禁用，仍允许工作目录项目规则生效（待后续评估）
	tplVars := runtimeTemplateVars(sessID, groupName)
	var promptParts []string
	if globalPrompt := core.BuildSystemPromptWithVars(tplVars); globalPrompt != "" {
		promptParts = append(promptParts, globalPrompt)
	}
	if teamRules := core.BuildTeamRulesWithVars(groupName, tplVars); teamRules != "" {
		promptParts = append(promptParts, teamRules)
	}
	// Auto-inject team members list (between team rules and session rules)
	if groupName != "" {
		if membersList := buildTeamMembersList(groupName, sessID); membersList != "" {
			promptParts = append(promptParts, membersList)
		}
	}
	if rules, err := ReadSessionRules(sessID); err == nil && rules != "" {
		promptParts = append(promptParts, core.RenderTemplateWithVars(rules, tplVars))
	}
	if len(promptParts) > 0 {
		req.SystemPrompt = strings.Join(promptParts, "\n\n---\n\n")
	}
	// For non-Anthropic API providers, append web_search disable hint
	// (--disallowed-tools only works for client tools, not server_tool_use like web_search)
	if p.BaseURL != "" && !strings.Contains(p.BaseURL, "api.anthropic.com") {
		webSearchHint := "\n\n重要：当前 API 不支持 web_search 工具，请勿使用。如需搜索信息，请使用其他方式（如 MCP 浏览器工具）。"
		req.SystemPrompt += webSearchHint
	}
	// Capture raw request snapshot for diagnostic purposes (GET /sessions/:id/last-request)
	updateRawRequestSnapshot(sessID, req.SystemPrompt, query)
	if anthropicPayload := buildAnthropicQueryPayload(query); anthropicPayload != nil {
		if rawPayload, err := json.Marshal(map[string]any{
			"messages": []map[string]any{{
				"role":    "user",
				"content": anthropicPayload,
			}},
		}); err == nil {
			updateRawRequestSnapshot(sessID, req.SystemPrompt, string(rawPayload))
		}
	}
	var fullResponse string
	var streamErr error

	// Steps accumulator for metadata persistence
	var steps []StepInfo
	var thinkingSummary string
	// Track content block index -> tool ID for correlating deltas
	toolIDs := make(map[int]string)
	toolNames := make(map[int]string)
	toolInputs := make(map[int]string)
	// Full text from assistant message (preserves newlines, used as fallback)
	var assistantFullText string
	var usageInput, usageOutput, usageCacheCreation, usageCacheRead int64

	// Incremental save throttle state (Issue #163)
	var lastSaveTime time.Time
	var lastSaveLen int

	// Track whether we've synced the real CLI session UUID back to DB.
	// The Claude CLI may use a different UUID than what we passed via --session-id/--resume.
	// We capture it from stream events and persist it so --resume works after server restarts.
	cliSessionSynced := false

	err := claudeClient.StreamPersistent(ctx, req, func(line string) {
		// Debug: log raw line type for troubleshooting (especially Windows)
		if len(line) > 0 {
			// Parse type first to decide log level
			var peek struct {
				Type   string `json:"type"`
				Result string `json:"result"`
			}
			if json.Unmarshal([]byte(line), &peek) == nil && (peek.Type == "result" || peek.Result == "error_during_execution" || peek.Result == "error") {
				log.Printf("[claude-debug] session %d: RESULT line (len=%d): %s", sessID, len(line), line)
			} else if len(line) > 200 {
				log.Printf("[claude-debug] session %d: raw line (len=%d): %.200s...", sessID, len(line), line)
			} else {
				log.Printf("[claude-debug] session %d: raw line (len=%d): %s", sessID, len(line), line)
			}
		}
		// First parse the top-level wrapper
		var wrapper struct {
			Type             string          `json:"type"`
			Subtype          string          `json:"subtype"`
			Result           string          `json:"result"`
			IsError          bool            `json:"is_error"`
			Event            json.RawMessage `json:"event"`
			ConversationName string          `json:"conversation_name"`
			Error            json.RawMessage `json:"error"`
			Errors           json.RawMessage `json:"errors"` // Can be []string or []struct{message,type}
			SessionID        string          `json:"session_id"` // Actual CLI session UUID (may differ from DB's claude_session_id)
			Usage            struct {
				InputTokens              int64 `json:"input_tokens"`
				OutputTokens             int64 `json:"output_tokens"`
				CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(line), &wrapper); err != nil {
			log.Printf("[claude] json parse error: %v, line: %.200s", err, line)
			return
		}

		// Sync real CLI session UUID back to DB (first valid occurrence).
		// This ensures --resume works after server restarts, since the CLI may use
		// a different UUID than what we passed via --session-id.
		if !cliSessionSynced && wrapper.SessionID != "" && wrapper.SessionID != sessionID {
			log.Printf("[claude] session %d: syncing real CLI session UUID: DB has %s, CLI uses %s", sessID, sessionID, wrapper.SessionID)
			if err := store.UpdateClaudeSessionID(sessID, wrapper.SessionID); err == nil {
				cliSessionSynced = true
				sessionID = wrapper.SessionID // update local ref for subsequent resume retries
			}
		}

		switch wrapper.Type {
		case "error":
			// API-level error: Error can be string or object
			errMsg := "unknown error"
			if len(wrapper.Error) > 0 {
				// Try string first
				var errStr string
				if err := json.Unmarshal(wrapper.Error, &errStr); err == nil {
					errMsg = errStr
				} else {
					// Try object
					var errObj struct {
						Message string `json:"message"`
						Type    string `json:"type"`
					}
					if err := json.Unmarshal(wrapper.Error, &errObj); err == nil && errObj.Message != "" {
						errMsg = errObj.Message
					}
				}
			}
			streamErr = fmt.Errorf("claude api error: %s", errMsg)
			log.Printf("[claude] API error: %s", errMsg)
			send(WSMessage{Type: "error", SessionID: sessID, Content: errMsg})

		case "stream_event":
			// Real-time streaming events from --include-partial-messages
			var inner struct {
				Type         string `json:"type"`
				Index        int    `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					Name string `json:"name"`
					ID   string `json:"id"`
				} `json:"content_block"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					Thinking    string `json:"thinking"`
					PartialJSON string `json:"partial_json"`
					Usage       struct {
						OutputTokens int64 `json:"output_tokens"`
					} `json:"usage"`
				} `json:"delta"`
				Usage struct {
					InputTokens              int64 `json:"input_tokens"`
					OutputTokens             int64 `json:"output_tokens"`
					CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
					CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
				} `json:"usage"`
				Message struct {
					Usage struct {
						InputTokens              int64 `json:"input_tokens"`
						OutputTokens             int64 `json:"output_tokens"`
						CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
						CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal(wrapper.Event, &inner); err != nil {
				return
			}

			switch inner.Type {
			case "content_block_start":
				if inner.ContentBlock.Type == "tool_use" {
					toolIDs[inner.Index] = inner.ContentBlock.ID
					toolNames[inner.Index] = inner.ContentBlock.Name
					toolInputs[inner.Index] = ""
					send(WSMessage{
						Type:      "tool_start",
						SessionID: sessID,
						ToolID:    inner.ContentBlock.ID,
						ToolName:  inner.ContentBlock.Name,
						Content:   inner.ContentBlock.Name,
					})
				}
			case "content_block_delta":
				switch inner.Delta.Type {
				case "text_delta":
					if inner.Delta.Text != "" {
						fullResponse += inner.Delta.Text
						send(WSMessage{Type: "chunk", SessionID: sessID, Content: inner.Delta.Text})
						// Incremental save: every 5s or 2000 chars since last save (Issue #163)
						if progressMsgID > 0 {
							charsSinceSave := len(fullResponse) - lastSaveLen
							timeSinceSave := time.Since(lastSaveTime)
							if charsSinceSave >= 2000 || timeSinceSave >= 5*time.Second {
								store.UpdateMessageContent(progressMsgID, fullResponse, "")
								lastSaveTime = time.Now()
								lastSaveLen = len(fullResponse)
							}
						}
					}
				case "thinking_delta":
					if inner.Delta.Thinking != "" {
						// Accumulate thinking summary (truncate to 200 chars)
						if len([]rune(thinkingSummary)) < 200 {
							thinkingSummary += inner.Delta.Thinking
							if len([]rune(thinkingSummary)) > 200 {
								thinkingSummary = string([]rune(thinkingSummary)[:200])
							}
						}
						send(WSMessage{Type: "thinking", SessionID: sessID, Content: inner.Delta.Thinking})
					}
				case "input_json_delta":
					if inner.Delta.PartialJSON != "" {
						toolID := toolIDs[inner.Index]
						toolInputs[inner.Index] += inner.Delta.PartialJSON
						send(WSMessage{Type: "tool_input", SessionID: sessID, ToolID: toolID, Content: inner.Delta.PartialJSON})
					}
				}
			case "content_block_stop":
				if toolID, ok := toolIDs[inner.Index]; ok {
					// Record tool step for metadata
					inputSummary := toolInputs[inner.Index]
					if len([]rune(inputSummary)) > 300 {
						inputSummary = string([]rune(inputSummary)[:300])
					}
					steps = append(steps, StepInfo{
						Type:   "tool",
						Name:   toolNames[inner.Index],
						Input:  inputSummary,
						Status: "done",
					})
					send(WSMessage{Type: "tool_result", SessionID: sessID, ToolID: toolID})
					delete(toolIDs, inner.Index)
					delete(toolNames, inner.Index)
					delete(toolInputs, inner.Index)
				}
			case "message_delta":
				// Capture per-turn usage from message_delta
				if inner.Delta.Usage.OutputTokens > 0 {
					usageOutput += inner.Delta.Usage.OutputTokens
				}
				// Top-level usage in message_delta contains the real input_tokens and cache tokens.
				// input_tokens is the TOTAL context window size (not per-turn), so overwrite, don't accumulate.
				if inner.Usage.InputTokens > 0 {
					usageInput = inner.Usage.InputTokens
				}
				if inner.Usage.OutputTokens > 0 {
					usageOutput += inner.Usage.OutputTokens
				}
				if inner.Usage.CacheCreationInputTokens > 0 {
					usageCacheCreation = inner.Usage.CacheCreationInputTokens
				}
				if inner.Usage.CacheReadInputTokens > 0 {
					usageCacheRead = inner.Usage.CacheReadInputTokens
				}
			case "message_stop":
				// input_tokens is total context size — overwrite, don't accumulate
				if inner.Usage.InputTokens > 0 {
					usageInput = inner.Usage.InputTokens
				}
				usageOutput += inner.Usage.OutputTokens
				if inner.Usage.CacheCreationInputTokens > 0 {
					usageCacheCreation = inner.Usage.CacheCreationInputTokens
				}
				if inner.Usage.CacheReadInputTokens > 0 {
					usageCacheRead = inner.Usage.CacheReadInputTokens
				}
			case "message_start":
				// input_tokens from message_start (reported once per turn) — overwrite, don't accumulate
				if inner.Message.Usage.InputTokens > 0 {
					usageInput = inner.Message.Usage.InputTokens
				}
				usageOutput += inner.Message.Usage.OutputTokens
				if inner.Message.Usage.CacheCreationInputTokens > 0 {
					usageCacheCreation = inner.Message.Usage.CacheCreationInputTokens
				}
				if inner.Message.Usage.CacheReadInputTokens > 0 {
					usageCacheRead = inner.Message.Usage.CacheReadInputTokens
				}
			}

		case "result":
			log.Printf("[claude-flow] session %d: received result event, subtype=%s, is_error=%v, result_len=%d",
				sessID, wrapper.Subtype, wrapper.IsError, len(wrapper.Result))
			// Capture usage from result event wrapper level (most reliable source)
			if wrapper.Usage.InputTokens > 0 {
				usageInput = wrapper.Usage.InputTokens // overwrite, don't accumulate
			}
			if wrapper.Usage.OutputTokens > 0 {
				usageOutput += wrapper.Usage.OutputTokens
			}
			usageCacheCreation += wrapper.Usage.CacheCreationInputTokens
			usageCacheRead += wrapper.Usage.CacheReadInputTokens
			if wrapper.ConversationName != "" {
				if err := store.UpdateSessionTitle(sessID, wrapper.ConversationName); err == nil {
					broadcast(WSMessage{Type: "session_title_update", SessionID: sessID, Content: wrapper.ConversationName})
				}
			}

			// Collect error messages from the errors array (if any)
			// CLI may send errors as []string or []struct{message,type} — handle both
			var errMsgs []string
			if len(wrapper.Errors) > 0 {
				// Try []string first (e.g. ["No conversation found with session ID: ..."])
				var strErrs []string
				if err := json.Unmarshal(wrapper.Errors, &strErrs); err == nil {
					for _, s := range strErrs {
						if s != "" {
							errMsgs = append(errMsgs, s)
						}
					}
				} else {
					// Try []struct{message,type}
					var objErrs []struct {
						Message string `json:"message"`
						Type    string `json:"type"`
					}
					if err := json.Unmarshal(wrapper.Errors, &objErrs); err == nil {
						for _, e := range objErrs {
							if e.Message != "" {
								errMsgs = append(errMsgs, e.Message)
							}
						}
					} else {
						// Last resort: log raw errors for debugging
						log.Printf("[claude] session %d: unparseable errors field: %s", sessID, string(wrapper.Errors))
					}
				}
			}

			// Determine if this result is an error condition
			// Only treat as error if is_error=true OR subtype="error"
			// error_during_execution with is_error=false is non-fatal (e.g., telemetry failures)
			isResultError := wrapper.IsError || wrapper.Subtype == "error"

			if isResultError {
				// Build composite error message
				errContent := wrapper.Result
				if len(errMsgs) > 0 {
					errContent = strings.Join(errMsgs, "; ")
				}
				if errContent == "" {
					errContent = "unknown CLI error (subtype=" + wrapper.Subtype + ")"
				}
				log.Printf("[claude] session %d: result error: subtype=%s is_error=%v errors=%v result=%s",
					sessID, wrapper.Subtype, wrapper.IsError, errMsgs, wrapper.Result)
				streamErr = fmt.Errorf("claude result error: %s", errContent)

				// Auto-recovery: "No conversation found" → reuse same UUID with --session-id
				// to rebuild the JSONL cache. Only compression is allowed to change the UUID.
				// runStream will silently retry, so skip error broadcast for this case
				isNoConvFound := false
				for _, msg := range errMsgs {
					if strings.Contains(strings.ToLower(msg), "no conversation found") {
						isNoConvFound = true
						log.Printf("[claude] session %d: detected 'No conversation found', reusing UUID for fresh --session-id", sessID)
						// Kill the failed process and force fresh run (--session-id instead of --resume)
						core.Pool.Kill(sessID)
						markForceFreshRun(sessID)
						// Inject recovery seed from conversation_logs to preserve context
						seed := ""
						if logs, logErr := store.GetConversationLogs(sessID); logErr == nil && len(logs) > 0 {
							if compressed, compErr := core.BuildIntelligentRecoverySeed(logs, p, sessID, ""); compErr == nil {
								seed = strings.TrimSpace(compressed)
								log.Printf("[claude] session %d: intelligent recovery seed (%d logs, %d bytes)", sessID, len(logs), len(seed))
							} else {
								seed = buildRecoverySeedFromLogs(logs, "Claude CLI 会话缓存丢失")
								log.Printf("[claude] session %d: AI compression failed (%v), using full log recovery seed (%d logs, %d bytes)", sessID, compErr, len(logs), len(seed))
							}
						}
						if seed == "" {
							if histMsgs, msgErr := store.GetMessages(sessID); msgErr == nil && len(histMsgs) > 0 {
								seed = buildRecoverySeed(histMsgs, "Claude CLI 会话缓存丢失")
								log.Printf("[claude] session %d: last-resort recovery seed from messages (%d msgs, %d bytes)", sessID, len(histMsgs), len(seed))
							}
						}
						if seed != "" {
							setPendingRecoverySeed(sessID, seed)
						}
						log.Printf("[claude] session %d: reusing existing claude_session_id, will retry with --session-id (fresh JSONL)", sessID)
						break
					}
				}
				if !isNoConvFound {
					send(WSMessage{Type: "error", SessionID: sessID, Content: errContent})
				}
			} else if wrapper.Subtype == "success" && fullResponse == "" {
				// Prefer assistant message content (preserves newlines) over result summary
				fallback := assistantFullText
				if fallback == "" {
					fallback = wrapper.Result
				}
				if fallback != "" {
					log.Printf("[claude] session %d: using result fallback (assistant_text=%d bytes, result=%d bytes)", sessID, len(assistantFullText), len(wrapper.Result))
					fullResponse = fallback
					send(WSMessage{Type: "chunk", SessionID: sessID, Content: fallback})
				}
			}
			// Capture token usage — input_tokens is total context size (overwrite), output accumulates
			if wrapper.Usage.InputTokens > 0 || wrapper.Usage.OutputTokens > 0 || wrapper.Usage.CacheCreationInputTokens > 0 || wrapper.Usage.CacheReadInputTokens > 0 {
				if wrapper.Usage.InputTokens > 0 {
					usageInput = wrapper.Usage.InputTokens // overwrite, not accumulate
				}
				usageOutput += wrapper.Usage.OutputTokens
				if wrapper.Usage.CacheCreationInputTokens > 0 {
					usageCacheCreation = wrapper.Usage.CacheCreationInputTokens
				}
				if wrapper.Usage.CacheReadInputTokens > 0 {
					usageCacheRead = wrapper.Usage.CacheReadInputTokens
				}
				log.Printf("[claude] session %d: result usage input=%d output=%d cache_create=%d cache_read=%d (total: input=%d output=%d cache_create=%d cache_read=%d)", sessID, wrapper.Usage.InputTokens, wrapper.Usage.OutputTokens, wrapper.Usage.CacheCreationInputTokens, wrapper.Usage.CacheReadInputTokens, usageInput, usageOutput, usageCacheCreation, usageCacheRead)
			}

		case "assistant":
			// Parse assistant message to capture full text with formatting (newlines preserved)
			var aMsg struct {
				Message struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(line), &aMsg); err == nil {
				var texts []string
				for _, b := range aMsg.Message.Content {
					if b.Type == "text" && b.Text != "" {
						texts = append(texts, b.Text)
					}
				}
				if len(texts) > 0 {
					assistantFullText = strings.Join(texts, "\n")
				}
			}

		default:
		}
	})

	// Build metadata JSON from accumulated steps
	if err == nil && streamErr != nil {
		err = streamErr
	}
	if err != nil && strings.Contains(err.Error(), "context canceled") && streamErr != nil {
		err = streamErr
	}
	var metadataJSON string
	if thinkingSummary != "" {
		steps = append([]StepInfo{{Type: "thinking", Name: "Thinking", Status: "done"}}, steps...)
	}
	if len(steps) > 0 {
		meta := StepsMetadata{Steps: steps, Thinking: thinkingSummary}
		if b, err := json.Marshal(meta); err == nil {
			metadataJSON = string(b)
		}
	}

	return fullResponse, metadataJSON, usageInput, usageOutput, usageCacheCreation, usageCacheRead, err
}

// GetLastRawRequest returns the last raw request sent to Claude Code CLI for a session.
// GET /api/v1/sessions/:id/last-request
func GetLastRawRequest(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	snap, ok := loadRawRequestSnapshot(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no request captured yet for this session"})
		return
	}
	msgs, _ := store.GetMessages(id)

	resp := gin.H{
		"system_prompt":             snap.SystemPrompt,
		"query":                     snap.Query,
		"context_count":             len(msgs),
		"captured_at":               snap.CapturedAt,
		"estimated_tokens":          snap.EstimatedTokens,
		"provider_max_tokens":       snap.ProviderMaxTokens,
		"threshold_percent":         snap.ThresholdPercent,
		"threshold_tokens":          snap.ThresholdTokens,
		"usage_percent":             snap.UsagePercent,
		"compression_enabled":       snap.CompressionEnabled,
		"would_trigger_compression": snap.WouldTriggerCompression,
		"compression_triggered":     snap.CompressionTriggered,
	}

	// Attach the actual Anthropic API request body (captured at the proxy layer).
	// This contains the complete messages array with full conversation history,
	// exactly as Claude Code CLI sent it to Anthropic.
	if proxyBody := GetLastProxyBody(id); proxyBody != nil {
		resp["anthropic_request"] = proxyBody
	} else {
		// Fallback: load from DB (survives restarts)
		var pb string
		if err := store.DB.QueryRow(`SELECT proxy_body FROM session_raw_requests WHERE session_id = ? AND proxy_body != ''`, id).Scan(&pb); err == nil && pb != "" {
			resp["anthropic_request"] = json.RawMessage(pb)
		}
	}

	c.JSON(http.StatusOK, resp)
}

// errorTagRe matches <!--error:xxx--> and <!--warning:xxx--> tags in AI responses.
var errorTagRe = regexp.MustCompile(`<!--(error|warning):\s*(.+?)-->`)

// extractAndSaveErrors scans content for error/warning tags and persists them.
func extractAndSaveErrors(sessionID, messageID int64, content string) {
	matches := errorTagRe.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		e := &model.AIError{
			SessionID: sessionID,
			MessageID: messageID,
			Level:     m[1],
			Summary:   strings.TrimSpace(m[2]),
		}
		if err := store.AddAIError(e); err != nil {
			log.Printf("[ai-error] save failed: %v", err)
		}

		// Fire session.error hooks for error-level events
		if m[1] == "error" {
			go core.FireHooks(core.HookEvent{
				Type:            "session.error",
				SourceSessionID: sessionID,
				Content:         strings.TrimSpace(m[2]),
			})
		}
	}
}

func fireMessageReceivedHook(sessionID int64, content string) {
	msgCount, _ := store.GetMessagesCount(sessionID)

	core.FireHooks(core.HookEvent{
		Type:            "message.received",
		SourceSessionID: sessionID,
		Content:         content,
		MessageCount:    msgCount,
	})

	core.FireHooks(core.HookEvent{
		Type:            "message.count",
		SourceSessionID: sessionID,
		Content:         content,
		MessageCount:    msgCount,
	})
}

func initHookStreamCallback() {
	core.SetHookStreamCallback(func(session *model.Session, content string, triggerMsgID int64) {
		maybeAutoCompressBeforeRun(session, content)
		go runStream(session, content, false, triggerMsgID)
	})
}
