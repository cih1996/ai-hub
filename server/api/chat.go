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
	SystemPrompt string    `json:"system_prompt"`
	Query        string    `json:"query"`
	CapturedAt   time.Time `json:"captured_at"`
}

// lastRawRequests stores the most recent raw request per session (sessID → RawRequestSnapshot).
var lastRawRequests sync.Map

type WSMessage struct {
	Type      string `json:"type"` // "chat" | "stop" | "subscribe" | "error" | "chunk" | "thinking" | "tool_start" | "tool_input" | "tool_result" | "done" | "session_created" | "streaming_status" | "session_update"
	SessionID int64  `json:"session_id"`
	Content   string `json:"content"`
	Detail    string `json:"detail,omitempty"` // Optional detail content for attention_status
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
	claudeClient    = core.NewClaudeCodeClient()
	activeStreams   = make(map[int64]*ActiveStream)
	activeStreamsMu sync.RWMutex
	forceFreshMu    sync.Mutex
	forceFreshRun   = make(map[int64]bool) // sessionID -> next run must start fresh (no --resume)
)

func markForceFreshRun(sessionID int64) {
	forceFreshMu.Lock()
	forceFreshRun[sessionID] = true
	forceFreshMu.Unlock()
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
		SessionID    int64  `json:"session_id"`
		Content      string `json:"content"`
		WorkDir      string `json:"work_dir"`
		GroupName    string `json:"group_name"`
		SessionRules string `json:"session_rules"`
		ProviderID   string `json:"provider_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	var session *model.Session
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
			providerID = p.ID
		} else {
			// Fall back to default provider
			provider, err := store.GetDefaultProvider()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No default provider configured. Go to Settings to add one."})
				return
			}
			providerID = provider.ID
		}
		var err error
		session, err = store.CreateSessionWithMessage(providerID, req.Content, req.WorkDir, req.GroupName)
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
			Content:         req.Content,
		})

		// Write session rules before starting stream (avoids race condition with putSessionRules)
		if req.SessionRules != "" {
			dir := sessionRulesDir()
			os.MkdirAll(dir, 0755)
			os.WriteFile(sessionRulesPath(session.ID), []byte(req.SessionRules), 0644)
		}
	} else {
		var err error
		session, err = store.GetSession(req.SessionID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}

		// Attention system V2: use shadow session flow when enabled
		originalContent := req.Content
		if session.AttentionEnabled {
			// Check if session is already streaming
			if IsSessionStreaming(session.ID) {
				c.JSON(http.StatusConflict, gin.H{"error": "session is busy (attention mode)"})
				return
			}
			// V3: Don't save user message here, let the flow save it at the end (clean sync)
			// Kick off attention mode v3 flow in background
			go runAttentionV3Flow(session, originalContent)
			c.JSON(http.StatusOK, gin.H{
				"session_id": session.ID,
				"status":     "started",
				"mode":       "attention_v3",
			})
			return
		}

		// Check if session is already streaming — queue message instead of rejecting
		if IsSessionStreaming(session.ID) {
			userMsg := &model.Message{
				SessionID: session.ID,
				Role:      "user",
				Content:   req.Content,
			}
			if err := store.AddMessage(userMsg); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "save message failed: " + err.Error()})
				return
			}
			log.Printf("[chat] session %d is streaming, message queued (msg_id=%d)", session.ID, userMsg.ID)
			// Broadcast queued message so frontend displays it
			broadcast(WSMessage{Type: "message_queued", SessionID: session.ID, Content: originalContent})
			c.JSON(http.StatusOK, gin.H{
				"session_id": session.ID,
				"status":     "queued",
			})
			return
		}
		userMsg := &model.Message{
			SessionID: session.ID,
			Role:      "user",
			Content:   req.Content,
		}
		if err := store.AddMessage(userMsg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "save message failed: " + err.Error()})
			return
		}
	}

	// Fire message.received hooks (for both new and existing sessions)
	go fireMessageReceivedHook(session.ID, req.Content)

	// Kick off streaming in background — results are pushed via WS broadcast
	triggerMsgID := store.GetLastUserMessageID(session.ID)
	go runStream(session, req.Content, isNewSession, triggerMsgID)

	c.JSON(http.StatusOK, gin.H{
		"session_id": session.ID,
		"status":     "started",
	})
}

// runStream executes the AI streaming in background, pushing events via WS to subscribed clients
func runStream(session *model.Session, query string, isNewSession bool, triggerMsgID int64) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start with a no-op send — a client will attach via "subscribe"
	stream := &ActiveStream{sendFn: func(WSMessage) {}, cancelFn: cancel}

	// Register active stream so clients can subscribe
	activeStreamsMu.Lock()
	activeStreams[session.ID] = stream
	activeStreamsMu.Unlock()
	broadcast(WSMessage{Type: "session_update", SessionID: session.ID, Content: "streaming"})
	defer func() {
		activeStreamsMu.Lock()
		delete(activeStreams, session.ID)
		activeStreamsMu.Unlock()
		broadcast(WSMessage{Type: "session_update", SessionID: session.ID, Content: "idle"})
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
		log.Printf("[chat] session=%d failed to pre-insert assistant message: %v", session.ID, err)
		broadcast(WSMessage{Type: "error", SessionID: session.ID, Content: "failed to save message"})
		return
	}
	progressMsgID := assistantMsg.ID

	// Reset proxy usage accumulator at stream start (Issue #72)
	ResetProxyUsage(session.ID)

	log.Printf("[chat] session=%d provider=%s mode=%s model=%s base_url=%s",
		session.ID, provider.Name, provider.Mode, provider.ModelID, provider.BaseURL)

	// One-shot recovery seed after session reset actions.
	if seed := takePendingRecoverySeed(session.ID); strings.TrimSpace(seed) != "" {
		query = seed + "\n\n---\n\n" + query
	}

	// isResume: true when the persistent process is alive OR when the session has
	// completed assistant messages in DB (i.e., we need to restore the conversation).
	// If previous turn detected "No conversation found", force one fresh run to avoid
	// getting stuck in a resume loop.
	isResume := !isNewSession && (core.Pool.HasProcess(session.ID) || store.HasAssistantMessages(session.ID))
	if consumeForceFreshRun(session.ID) {
		isResume = false
	}
	fullResponse, metadataJSON, usageInput, usageOutput, usageCacheCreation, usageCacheRead, err = streamClaudeCode(ctx, provider, query, session.ClaudeSessionID, isResume, stream.Send, session.ID, session.WorkDir, session.GroupName, progressMsgID)

	log.Printf("[chat-flow] session=%d streamClaudeCode returned: err=%v, fullResponse_len=%d, metadata_len=%d",
		session.ID, err, len(fullResponse), len(metadataJSON))

	if err != nil {
		log.Printf("[chat] session=%d provider=%s error: %v", session.ID, provider.Name, err)
		// Prefer proxy-captured usage on error path too (Issue #72)
		if pu := ConsumeProxyUsage(session.ID); pu != nil {
			usageInput = pu.InputTokens
			usageOutput = pu.OutputTokens
			usageCacheCreation = pu.CacheCreationInputTokens
			usageCacheRead = pu.CacheReadInputTokens
		}
		// Save partial response before reporting error — don't lose already-received content
		if fullResponse != "" || metadataJSON != "" {
			content := fullResponse
			if content == "" {
				content = "[任务已执行，详见执行步骤]"
			}
			store.UpdateMessageContent(progressMsgID, content, metadataJSON)
			extractAndSaveErrors(session.ID, progressMsgID, content)
			// Save token usage even on error (partial response)
			if usageInput > 0 || usageOutput > 0 || usageCacheCreation > 0 || usageCacheRead > 0 {
				tu := &model.TokenUsage{SessionID: session.ID, MessageID: progressMsgID, InputTokens: usageInput, OutputTokens: usageOutput, CacheCreationInputTokens: usageCacheCreation, CacheReadInputTokens: usageCacheRead}
				store.AddTokenUsage(tu)
				usageJSON, _ := json.Marshal(tu)
				broadcast(WSMessage{Type: "token_usage", SessionID: session.ID, Content: string(usageJSON)})
			}
		} else {
			// No content received — update the pre-inserted message with error instead of deleting
			errContent := "❌ " + err.Error()
			store.UpdateMessageContent(progressMsgID, errContent, "")
			broadcast(WSMessage{Type: "chunk", SessionID: session.ID, Content: errContent})
		}
		broadcast(WSMessage{Type: "error", SessionID: session.ID, Content: err.Error()})
		return
	}

	// Prefer proxy-captured usage (has accurate cache tokens) over stream-json fallback (Issue #72)
	if pu := ConsumeProxyUsage(session.ID); pu != nil {
		log.Printf("[chat] session=%d using proxy usage: input=%d output=%d cache_create=%d cache_read=%d",
			session.ID, pu.InputTokens, pu.OutputTokens, pu.CacheCreationInputTokens, pu.CacheReadInputTokens)
		usageInput = pu.InputTokens
		usageOutput = pu.OutputTokens
		usageCacheCreation = pu.CacheCreationInputTokens
		usageCacheRead = pu.CacheReadInputTokens
	}

	if fullResponse != "" || metadataJSON != "" {
		content := fullResponse
		if content == "" {
			content = "[任务已执行，详见执行步骤]"
		}
		// Final update of the pre-inserted assistant message
		store.UpdateMessageContent(progressMsgID, content, metadataJSON)
		extractAndSaveErrors(session.ID, progressMsgID, content)
		// Save and broadcast token usage
		if usageInput > 0 || usageOutput > 0 || usageCacheCreation > 0 || usageCacheRead > 0 {
			tu := &model.TokenUsage{SessionID: session.ID, MessageID: progressMsgID, InputTokens: usageInput, OutputTokens: usageOutput, CacheCreationInputTokens: usageCacheCreation, CacheReadInputTokens: usageCacheRead}
			store.AddTokenUsage(tu)
			usageJSON, _ := json.Marshal(tu)
			broadcast(WSMessage{Type: "token_usage", SessionID: session.ID, Content: string(usageJSON)})
		}
		// Auto-compress check: run async so it never blocks the response path
		if usageInput > 0 {
			go maybeAutoCompress(session, usageInput)
		}
		// Attention system: check for _attention_trigger and run review if found
		// This is a universal mechanism that works regardless of attention_enabled flag
		maybeRunAttentionReview(session, fullResponse, provider)
	} else {
		// No content received — remove the empty pre-inserted message
		log.Printf("[chat-flow] session=%d no content received, deleting empty message %d", session.ID, progressMsgID)
		store.DeleteMessage(progressMsgID)
	}

	// Broadcast done so even reconnected/new WS clients receive it (stream.Send is single-client)
	log.Printf("[chat-flow] session=%d broadcasting done event", session.ID)
	broadcast(WSMessage{Type: "done", SessionID: session.ID, Content: metadataJSON})
}

// processQueuedMessages checks for user messages that arrived after triggerMsgID,
// merges them, and kicks off a new runStream to process them.
func processQueuedMessages(sessionID int64, triggerMsgID int64) {
	pending, err := store.GetPendingUserMessages(sessionID, triggerMsgID)
	if err != nil || len(pending) == 0 {
		return
	}

	// Guard: if another stream already started (race), bail out
	if IsSessionStreaming(sessionID) {
		return
	}

	session, err := store.GetSession(sessionID)
	if err != nil {
		log.Printf("[queue] session %d not found: %v", sessionID, err)
		return
	}

	// Merge all pending messages into one query
	var contents []string
	for _, m := range pending {
		contents = append(contents, m.Content)
	}
	merged := strings.Join(contents, "\n\n---\n\n")

	// Use the last pending message ID as triggerMsgID for the next round
	// This prevents infinite retry if streaming fails without saving assistant message
	newTriggerMsgID := pending[len(pending)-1].ID

	log.Printf("[queue] session %d: processing %d queued message(s), triggerMsgID %d -> %d", sessionID, len(pending), triggerMsgID, newTriggerMsgID)
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

// buildStructuredMemoryInjection builds the structured memory block for system prompt.
// It loads injection routes from the database, matches the query against them,
// and assembles the fixed + conditionally-matched memory categories.
// Returns empty string if no structured memory content is available.
func buildStructuredMemoryInjection(query string) string {
	// Load injection routes from DB
	dbRoutes, err := store.ListInjectionRoutes()
	if err != nil {
		log.Printf("[injection] failed to load routes: %v", err)
		dbRoutes = nil
	}
	// Convert store routes to core routes for matching
	var coreRoutes []core.InjectionRoute
	for _, r := range dbRoutes {
		coreRoutes = append(coreRoutes, core.InjectionRoute{
			Keywords:         r.Keywords,
			InjectCategories: r.InjectCategories,
		})
	}
	// Match query against routes
	matchedConditional := core.MatchInjectionRoutes(query, coreRoutes)
	// Build the injection block
	return core.BuildStructuredMemoryBlock(matchedConditional)
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
	// Structured memory injection (Issue #210): append memory block after all rules
	if memBlock := buildStructuredMemoryInjection(query); memBlock != "" {
		promptParts = append(promptParts, memBlock)
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
	lastRawRequests.Store(sessID, RawRequestSnapshot{
		SystemPrompt: req.SystemPrompt,
		Query:        query,
		CapturedAt:   time.Now(),
	})
	var fullResponse string

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
				// Capture per-turn usage from message_delta (output_tokens in delta.usage)
				if inner.Delta.Usage.OutputTokens > 0 {
					usageOutput += inner.Delta.Usage.OutputTokens
				}
				// Some models put usage at top level of message_delta
				if inner.Usage.OutputTokens > 0 {
					usageOutput += inner.Usage.OutputTokens
				}
				usageCacheCreation += inner.Usage.CacheCreationInputTokens
				usageCacheRead += inner.Usage.CacheReadInputTokens
			case "message_stop":
				// Capture per-turn usage from message_stop
				usageInput += inner.Usage.InputTokens
				usageOutput += inner.Usage.OutputTokens
				usageCacheCreation += inner.Usage.CacheCreationInputTokens
				usageCacheRead += inner.Usage.CacheReadInputTokens
			case "message_start":
				// Capture input_tokens from message_start (reported once per turn)
				usageInput += inner.Message.Usage.InputTokens
				usageOutput += inner.Message.Usage.OutputTokens
				usageCacheCreation += inner.Message.Usage.CacheCreationInputTokens
				usageCacheRead += inner.Message.Usage.CacheReadInputTokens
			}

		case "result":
			log.Printf("[claude-flow] session %d: received result event, subtype=%s, is_error=%v, result_len=%d",
				sessID, wrapper.Subtype, wrapper.IsError, len(wrapper.Result))
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

				// Auto-recovery: "No conversation found" → reset claude_session_id
				for _, msg := range errMsgs {
					if strings.Contains(strings.ToLower(msg), "no conversation found") {
						log.Printf("[claude] session %d: detected 'No conversation found', resetting claude_session_id", sessID)
						newUUID := uuid.New().String()
						if err := store.UpdateClaudeSessionID(sessID, newUUID); err == nil {
							core.Pool.Kill(sessID)
							markForceFreshRun(sessID)
							errContent += " (会话已自动重置，请重新发送消息)"
							log.Printf("[claude] session %d: claude_session_id reset to %s", sessID, newUUID)
						}
						break
					}
				}
				send(WSMessage{Type: "error", SessionID: sessID, Content: errContent})
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
			// Capture token usage (accumulate, not overwrite)
			if wrapper.Usage.InputTokens > 0 || wrapper.Usage.OutputTokens > 0 || wrapper.Usage.CacheCreationInputTokens > 0 || wrapper.Usage.CacheReadInputTokens > 0 {
				usageInput += wrapper.Usage.InputTokens
				usageOutput += wrapper.Usage.OutputTokens
				usageCacheCreation += wrapper.Usage.CacheCreationInputTokens
				usageCacheRead += wrapper.Usage.CacheReadInputTokens
				log.Printf("[claude] session %d: result usage +input=%d +output=%d +cache_create=%d +cache_read=%d (total: input=%d output=%d cache_create=%d cache_read=%d)", sessID, wrapper.Usage.InputTokens, wrapper.Usage.OutputTokens, wrapper.Usage.CacheCreationInputTokens, wrapper.Usage.CacheReadInputTokens, usageInput, usageOutput, usageCacheCreation, usageCacheRead)
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
	val, ok := lastRawRequests.Load(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no request captured yet for this session"})
		return
	}
	snap := val.(RawRequestSnapshot)
	msgs, _ := store.GetMessages(id)

	resp := gin.H{
		"system_prompt": snap.SystemPrompt,
		"query":         snap.Query,
		"context_count": len(msgs),
		"captured_at":   snap.CapturedAt,
	}

	// Attach the actual Anthropic API request body (captured at the proxy layer).
	// This contains the complete messages array with full conversation history,
	// exactly as Claude Code CLI sent it to Anthropic.
	if proxyBody := GetLastProxyBody(id); proxyBody != nil {
		resp["anthropic_request"] = proxyBody
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

// buildAttentionPrompt wraps user content with attention system activation rules.
// Note: This is legacy v1 code, kept for backward compatibility.
// Attention mode v2 uses a different flow via runAttentionV2Flow.
func buildAttentionPrompt(userContent string, customActivationRules string) string {
	var parts []string
	if customActivationRules != "" {
		parts = append(parts, customActivationRules)
	}
	parts = append(parts, "用户请求：\n"+userContent)
	return strings.Join(parts, "\n\n")
}

// attentionReviewState tracks the review retry count per session
var (
	attentionRetryCount   = make(map[int64]int)
	attentionRetryCountMu sync.Mutex
)

// resetAttentionRetry resets the retry count for a session
func resetAttentionRetry(sessionID int64) {
	attentionRetryCountMu.Lock()
	delete(attentionRetryCount, sessionID)
	attentionRetryCountMu.Unlock()
}

// incrementAttentionRetry increments and returns the retry count
func incrementAttentionRetry(sessionID int64) int {
	attentionRetryCountMu.Lock()
	defer attentionRetryCountMu.Unlock()
	attentionRetryCount[sessionID]++
	return attentionRetryCount[sessionID]
}

// maybeRunAttentionReview checks if the AI response contains _attention_trigger
// and runs the review process if found. Returns true if review was triggered.
func maybeRunAttentionReview(session *model.Session, fullResponse string, provider *model.Provider) bool {
	// Detect attention trigger in response
	trigger := core.DetectAttentionTrigger(fullResponse)
	if trigger == nil {
		// No trigger found, reset retry count and continue normally
		resetAttentionRetry(session.ID)
		return false
	}

	log.Printf("[attention] session %d: detected trigger, action=%s", session.ID, trigger.Action)

	// Check retry count
	retryCount := incrementAttentionRetry(session.ID)
	if retryCount > core.MaxReviewRetries {
		log.Printf("[attention] session %d: max retries (%d) exceeded, blocking", session.ID, core.MaxReviewRetries)
		// Send block message to session
		blockMsg := "【注意力系统】审核重试次数超过上限（" + strconv.Itoa(core.MaxReviewRetries) + " 次），请人工介入检查。"
		sendAttentionFeedback(session.ID, blockMsg)
		resetAttentionRetry(session.ID)
		return true
	}

	// Run review in background
	go runAttentionReview(session, trigger, provider, retryCount)
	return true
}

// runAttentionReview executes the review process in an independent context
// Note: This is legacy v1 code, kept for backward compatibility.
func runAttentionReview(session *model.Session, trigger *core.AttentionTrigger, provider *model.Provider, retryCount int) {
	log.Printf("[attention] session %d: starting review (retry %d/%d)", session.ID, retryCount, core.MaxReviewRetries)

	// Build review context
	rulesData := core.ParseAttentionRules(session.AttentionRules)

	// Gather context for review: session rules, team rules, global rules, memory
	var contextParts []string

	// Add custom review rules if any
	if rulesData.ReviewCustom != "" {
		contextParts = append(contextParts, "【用户自定义审核规则】\n"+rulesData.ReviewCustom)
	}

	// Session rules
	if sessionRules, err := ReadSessionRules(session.ID); err == nil && sessionRules != "" {
		contextParts = append(contextParts, "【会话规则】\n"+sessionRules)
	}

	// Team rules
	if session.GroupName != "" {
		if teamRules := core.BuildTeamRulesWithVars(session.GroupName, nil); teamRules != "" {
			contextParts = append(contextParts, "【团队规则】\n"+teamRules)
		}
	}

	// Global rules
	if globalRules := core.BuildSystemPromptWithVars(nil); globalRules != "" {
		// Truncate if too long
		if len(globalRules) > 2000 {
			globalRules = globalRules[:2000] + "...(已截断)"
		}
		contextParts = append(contextParts, "【全局规则】\n"+globalRules)
	}

	// Build the review query
	var reviewQuery string
	if len(contextParts) > 0 {
		reviewQuery = "审核依据：\n\n" + strings.Join(contextParts, "\n\n---\n\n") + "\n\n---\n\n"
	}
	reviewQuery += "待审核的执行计划：\n" + trigger.Plan

	// Call Anthropic Messages API directly (not via CLI to avoid nested session error)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	systemPrompt := "你是注意力审核 AI。只输出审核结果，格式为 [PASS] 或 [REJECT:原因]。不要调用任何工具，不要执行任何操作。"
	reviewResponse, err := core.CallAnthropicMessagesAPI(ctx, provider, reviewQuery, systemPrompt, 1024)
	if err != nil {
		log.Printf("[attention] session %d: review failed: %v", session.ID, err)
		sendAttentionFeedback(session.ID, "【注意力系统】审核失败: "+err.Error())
		return
	}

	// Check for empty response
	if strings.TrimSpace(reviewResponse) == "" {
		log.Printf("[attention] session %d: review returned empty response", session.ID)
		sendAttentionFeedback(session.ID, "【注意力系统】审核失败: 审核 AI 未返回有效响应，请重试。")
		return
	}

	// Parse review result
	passed, reason := core.ParseReviewResult(reviewResponse)
	log.Printf("[attention] session %d: review result passed=%v reason=%s response=%s", session.ID, passed, reason, reviewResponse)

	if passed {
		// Review passed, send approval and let AI continue
		resetAttentionRetry(session.ID)
		sendAttentionFeedback(session.ID, "【注意力系统】审核通过，请继续执行计划。")
	} else {
		// Review rejected, send rejection reason
		sendAttentionFeedback(session.ID, "【注意力系统】审核未通过: "+reason+"\n\n请根据反馈修正计划后重新提交。")
	}
}

// sendAttentionFeedback sends a system message to the session as user input
// This triggers the AI to respond to the feedback
func sendAttentionFeedback(sessionID int64, message string) {
	session, err := store.GetSession(sessionID)
	if err != nil {
		log.Printf("[attention] session %d not found: %v", sessionID, err)
		return
	}

	// Save as user message
	userMsg := &model.Message{
		SessionID: sessionID,
		Role:      "user",
		Content:   message,
	}
	if err := store.AddMessage(userMsg); err != nil {
		log.Printf("[attention] failed to save feedback message: %v", err)
		return
	}

	// Broadcast the message
	broadcast(WSMessage{Type: "message_queued", SessionID: sessionID, Content: message})

	// Trigger AI response
	triggerMsgID := userMsg.ID
	go runStream(session, message, false, triggerMsgID)
}

// ============================================================================
// Attention Mode V2: Shadow Session Flow
// ============================================================================

// runAttentionV2Flow orchestrates the full attention mode v2 execution
// using shadow sessions for isolated execution with parent session broadcast
func runAttentionV2Flow(parentSession *model.Session, userMessage string) {
	parentID := parentSession.ID
	fmt.Printf("[attention-v2] session %d: starting flow\n", parentID)
	log.Printf("[attention-v2] session %d: starting flow", parentID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register as active stream so UI shows streaming status
	stream := &ActiveStream{sendFn: func(WSMessage) {}, cancelFn: cancel}
	activeStreamsMu.Lock()
	activeStreams[parentID] = stream
	activeStreamsMu.Unlock()
	broadcast(WSMessage{Type: "session_update", SessionID: parentID, Content: "streaming"})
	defer func() {
		activeStreamsMu.Lock()
		delete(activeStreams, parentID)
		activeStreamsMu.Unlock()
		broadcast(WSMessage{Type: "session_update", SessionID: parentID, Content: "idle"})
		fmt.Printf("[attention-v2] session %d: flow ended\n", parentID)
		log.Printf("[attention-v2] session %d: flow ended", parentID)
	}()

	// Get provider
	provider, err := store.GetProvider(parentSession.ProviderID)
	if err != nil {
		fmt.Printf("[attention-v2] session %d: provider %s not found, trying default\n", parentID, parentSession.ProviderID)
		log.Printf("[attention-v2] session %d: provider %s not found, trying default", parentID, parentSession.ProviderID)
		provider, err = store.GetDefaultProvider()
		if err != nil {
			errMsg := "provider not found"
			fmt.Printf("[attention-v2] session %d: %s\n", parentID, errMsg)
			log.Printf("[attention-v2] session %d: %s", parentID, errMsg)
			broadcast(WSMessage{Type: "error", SessionID: parentID, Content: errMsg})
			return
		}
	}
	fmt.Printf("[attention-v2] session %d: using provider %s\n", parentID, provider.Name)
	log.Printf("[attention-v2] session %d: using provider %s", parentID, provider.Name)

	// Broadcast status helper
	broadcastStatus := func(status string) {
		fmt.Printf("[attention-v2] session %d: status: %s\n", parentID, status)
		log.Printf("[attention-v2] session %d: status: %s", parentID, status)
		broadcast(WSMessage{Type: "attention_status", SessionID: parentID, Content: status})
	}

	// Create executor with callbacks
	executor := &core.AttentionV2Executor{
		ParentSession: parentSession,
		Provider:      provider,
		BroadcastFn: func(sessionID int64, msgType, content, detail string) {
			broadcast(WSMessage{Type: msgType, SessionID: sessionID, Content: content, Detail: detail})
		},
		RunShadowStreamFn: func(shadowSession *model.Session, query string, broadcastAsID int64) (string, string, error) {
			return runShadowStream(ctx, shadowSession, query, broadcastAsID, provider)
		},
	}

	// Create shadow session callback
	createShadowFn := func() (*model.Session, error) {
		return store.CreateShadowSession(parentID)
	}

	// Delete shadow session callback
	deleteShadowFn := func(shadowID int64) error {
		return store.DeleteShadowSession(shadowID)
	}

	// Copy messages callback
	copyMessagesFn := func(parentID, shadowID int64, limit int) error {
		return store.CopyRecentMessagesToShadow(parentID, shadowID, limit)
	}

	// Execute the full flow
	broadcastStatus("注意力模式：正在处理...")
	fmt.Printf("[attention-v2] session %d: calling ExecuteWithShadow\n", parentID)
	finalResult, err := executor.ExecuteWithShadow(ctx, userMessage, createShadowFn, deleteShadowFn, copyMessagesFn)
	fmt.Printf("[attention-v2] session %d: ExecuteWithShadow returned, err=%v, result_len=%d\n", parentID, err, len(finalResult))
	if err != nil {
		fmt.Printf("[attention-v2] session %d: flow failed: %v\n", parentID, err)
		log.Printf("[attention-v2] session %d: flow failed: %v", parentID, err)
		broadcast(WSMessage{Type: "error", SessionID: parentID, Content: "注意力模式执行失败: " + err.Error()})
		return
	}

	// Save final result to parent session
	if finalResult != "" {
		fmt.Printf("[attention-v2] session %d: saving final result (len=%d)\n", parentID, len(finalResult))
		assistantMsg := &model.Message{
			SessionID: parentID,
			Role:      "assistant",
			Content:   finalResult,
		}
		if err := store.AddMessage(assistantMsg); err != nil {
			fmt.Printf("[attention-v2] session %d: failed to save final result: %v\n", parentID, err)
			log.Printf("[attention-v2] session %d: failed to save final result: %v", parentID, err)
		} else {
			fmt.Printf("[attention-v2] session %d: final result saved\n", parentID)
		}
	} else {
		fmt.Printf("[attention-v2] session %d: no final result to save\n", parentID)
	}

	broadcast(WSMessage{Type: "done", SessionID: parentID, Content: ""})
	log.Printf("[attention-v2] session %d: flow completed", parentID)
}

// ============================================================================
// Attention Mode V3: Simplified Flow
// ============================================================================

// runAttentionV3Flow orchestrates the simplified attention mode execution
// Phase 1: Attention AI queries info and generates preprocessing text
// Phase 2: Send enhanced message (wrapped context + user message) to parent session
func runAttentionV3Flow(parentSession *model.Session, userMessage string) {
	parentID := parentSession.ID
	log.Printf("[attention-v3] session %d: starting flow", parentID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register as active stream
	stream := &ActiveStream{sendFn: func(WSMessage) {}, cancelFn: cancel}
	activeStreamsMu.Lock()
	activeStreams[parentID] = stream
	activeStreamsMu.Unlock()
	broadcast(WSMessage{Type: "session_update", SessionID: parentID, Content: "streaming"})
	defer func() {
		activeStreamsMu.Lock()
		delete(activeStreams, parentID)
		activeStreamsMu.Unlock()
		broadcast(WSMessage{Type: "session_update", SessionID: parentID, Content: "idle"})
		log.Printf("[attention-v3] session %d: flow ended", parentID)
	}()

	// Get provider
	provider, err := store.GetProvider(parentSession.ProviderID)
	if err != nil {
		log.Printf("[attention-v3] session %d: provider %s not found, trying default", parentID, parentSession.ProviderID)
		provider, err = store.GetDefaultProvider()
		if err != nil {
			broadcast(WSMessage{Type: "error", SessionID: parentID, Content: "provider not found"})
			return
		}
	}
	log.Printf("[attention-v3] session %d: using provider %s", parentID, provider.Name)

	// Create executor with callbacks
	executor := &core.AttentionV3Executor{
		ParentSession: parentSession,
		Provider:      provider,
		BroadcastFn: func(sessionID int64, msgType, content, detail string) {
			broadcast(WSMessage{Type: msgType, SessionID: sessionID, Content: content, Detail: detail})
		},
		CreateAttentionSessionFn: func(parentID int64) (*model.Session, error) {
			return store.CreateShadowSessionWithTitle(parentID, "注意力AI")
		},
		DeleteAttentionSessionFn: func(sessionID int64) error {
			return store.DeleteShadowSession(sessionID)
		},
		RunAttentionStreamFn: func(attentionSession *model.Session, query string, broadcastAsID int64) (string, error) {
			// Run attention AI silently (no broadcast to frontend)
			response, err := runSilentStream(ctx, attentionSession, query, provider)
			return response, err
		},
		RunParentStreamFn: func(parentSession *model.Session, query string) error {
			// Run parent session normally with enhanced message
			return runParentStream(ctx, parentSession, query, provider)
		},
		SaveMessageFn: func(sessionID int64, role, content, attentionContext string) error {
			msg := &model.Message{
				SessionID:        sessionID,
				Role:             role,
				Content:          content,
				AttentionContext: attentionContext,
			}
			return store.AddMessage(msg)
		},
	}

	// Execute the flow
	err = executor.Execute(ctx, userMessage)
	if err != nil {
		log.Printf("[attention-v3] session %d: flow failed: %v", parentID, err)
		broadcast(WSMessage{Type: "error", SessionID: parentID, Content: "注意力模式执行失败: " + err.Error()})
		return
	}

	log.Printf("[attention-v3] session %d: flow completed", parentID)
	broadcast(WSMessage{Type: "done", SessionID: parentID, Content: ""})
}

// runParentStream runs the parent session with enhanced message
// Reuses streamClaudeCode for consistent streaming behavior
func runParentStream(ctx context.Context, parentSession *model.Session, query string, provider *model.Provider) error {
	log.Printf("[parent-stream] session=%d: starting", parentSession.ID)

	// Pre-insert empty assistant message for incremental saves
	assistantMsg := &model.Message{
		SessionID: parentSession.ID,
		Role:      "assistant",
		Content:   "",
		Metadata:  "",
	}
	if err := store.AddMessage(assistantMsg); err != nil {
		log.Printf("[parent-stream] session=%d: failed to pre-insert message: %v", parentSession.ID, err)
		return err
	}
	progressMsgID := assistantMsg.ID

	// Reset proxy usage accumulator
	ResetProxyUsage(parentSession.ID)

	// Send function that broadcasts to parent session
	sendFn := func(msg WSMessage) {
		msg.SessionID = parentSession.ID
		broadcast(msg)
	}

	// isResume: parent session continues from existing conversation
	isResume := core.Pool.HasProcess(parentSession.ID) || store.HasAssistantMessages(parentSession.ID)

	// Use streamClaudeCode for consistent streaming behavior
	fullResponse, metadataJSON, usageInput, usageOutput, usageCacheCreation, usageCacheRead, err := streamClaudeCode(
		ctx, provider, query, parentSession.ClaudeSessionID, isResume, sendFn,
		parentSession.ID, parentSession.WorkDir, parentSession.GroupName, progressMsgID,
	)

	if err != nil {
		log.Printf("[parent-stream] session=%d: error: %v", parentSession.ID, err)
		// Save partial response if any
		if fullResponse != "" || metadataJSON != "" {
			content := fullResponse
			if content == "" {
				content = "[任务已执行，详见执行步骤]"
			}
			store.UpdateMessageContent(progressMsgID, content, metadataJSON)
		} else {
			store.UpdateMessageContent(progressMsgID, "❌ "+err.Error(), "")
		}
		return err
	}

	// Prefer proxy-captured usage
	if pu := ConsumeProxyUsage(parentSession.ID); pu != nil {
		usageInput = pu.InputTokens
		usageOutput = pu.OutputTokens
		usageCacheCreation = pu.CacheCreationInputTokens
		usageCacheRead = pu.CacheReadInputTokens
	}

	// Save response
	if fullResponse != "" || metadataJSON != "" {
		content := fullResponse
		if content == "" {
			content = "[任务已执行，详见执行步骤]"
		}
		store.UpdateMessageContent(progressMsgID, content, metadataJSON)
		extractAndSaveErrors(parentSession.ID, progressMsgID, content)

		// Save token usage
		if usageInput > 0 || usageOutput > 0 || usageCacheCreation > 0 || usageCacheRead > 0 {
			tu := &model.TokenUsage{
				SessionID: parentSession.ID, MessageID: progressMsgID,
				InputTokens: usageInput, OutputTokens: usageOutput,
				CacheCreationInputTokens: usageCacheCreation, CacheReadInputTokens: usageCacheRead,
			}
			store.AddTokenUsage(tu)
			usageJSON, _ := json.Marshal(tu)
			broadcast(WSMessage{Type: "token_usage", SessionID: parentSession.ID, Content: string(usageJSON)})
		}
	} else {
		// No content - delete empty message
		store.DeleteMessage(progressMsgID)
	}

	log.Printf("[parent-stream] session=%d: complete", parentSession.ID)
	return nil
}

// runShadowStream executes streaming in shadow session but broadcasts to parent session ID
// This allows the frontend to receive updates as if they came from the parent session
func runShadowStream(ctx context.Context, shadowSession *model.Session, query string, broadcastAsID int64, provider *model.Provider) (string, string, error) {
	log.Printf("[shadow-stream] shadow=%d broadcast_as=%d: starting", shadowSession.ID, broadcastAsID)

	// Build system prompt for shadow session (inherit from parent)
	tplVars := runtimeTemplateVars(broadcastAsID, shadowSession.GroupName)
	var promptParts []string
	if globalPrompt := core.BuildSystemPromptWithVars(tplVars); globalPrompt != "" {
		promptParts = append(promptParts, globalPrompt)
	}
	if teamRules := core.BuildTeamRulesWithVars(shadowSession.GroupName, tplVars); teamRules != "" {
		promptParts = append(promptParts, teamRules)
	}
	// Auto-inject team members list
	if shadowSession.GroupName != "" {
		if membersList := buildTeamMembersList(shadowSession.GroupName, broadcastAsID); membersList != "" {
			promptParts = append(promptParts, membersList)
		}
	}
	if rules, err := ReadSessionRules(broadcastAsID); err == nil && rules != "" {
		promptParts = append(promptParts, core.RenderTemplateWithVars(rules, tplVars))
	}
	// Structured memory injection (Issue #210)
	if memBlock := buildStructuredMemoryInjection(query); memBlock != "" {
		promptParts = append(promptParts, memBlock)
	}

	req := core.ClaudeCodeRequest{
		Query:        query,
		SessionID:    shadowSession.ClaudeSessionID,
		Resume:       false, // Shadow sessions always start fresh
		BaseURL:      provider.BaseURL,
		APIKey:       provider.APIKey,
		AuthMode:     provider.AuthMode,
		ProxyURL:     provider.ProxyURL,
		ModelID:      strings.TrimSpace(provider.ModelID),
		WorkDir:      shadowSession.WorkDir,
		HubSessionID: shadowSession.ID,
		GroupName:    shadowSession.GroupName,
	}
	if provider.AuthMode == "oauth" {
		req.ModelID = ""
	}
	if len(promptParts) > 0 {
		req.SystemPrompt = strings.Join(promptParts, "\n\n---\n\n")
	}
	// For non-Anthropic API providers, append web_search disable hint
	if provider.BaseURL != "" && !strings.Contains(provider.BaseURL, "api.anthropic.com") {
		webSearchHint := "\n\n重要：当前 API 不支持 web_search 工具，请勿使用。如需搜索信息，请使用其他方式（如 MCP 浏览器工具）。"
		req.SystemPrompt += webSearchHint
	}

	var fullResponse string
	var metadataJSON string

	// Send function that broadcasts to parent session ID
	sendFn := func(msg WSMessage) {
		// Override session ID to broadcast as parent
		msg.SessionID = broadcastAsID
		broadcast(msg)
	}

	// Steps accumulator for metadata
	var steps []StepInfo
	var thinkingSummary string
	toolIDs := make(map[int]string)
	toolNames := make(map[int]string)
	toolInputs := make(map[int]string)
	var assistantFullText string

	err := claudeClient.StreamPersistent(ctx, req, func(line string) {
		// Parse the streaming event
		var wrapper struct {
			Type    string          `json:"type"`
			Subtype string          `json:"subtype"`
			Result  string          `json:"result"`
			Event   json.RawMessage `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &wrapper); err != nil {
			return
		}

		switch wrapper.Type {
		case "error":
			var errObj struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal([]byte(line), &errObj) == nil && errObj.Error.Message != "" {
				sendFn(WSMessage{Type: "error", Content: errObj.Error.Message})
			}

		case "stream_event":
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
				} `json:"delta"`
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
					sendFn(WSMessage{
						Type:     "tool_start",
						ToolID:   inner.ContentBlock.ID,
						ToolName: inner.ContentBlock.Name,
					})
				}

			case "content_block_delta":
				switch inner.Delta.Type {
				case "text_delta":
					if inner.Delta.Text != "" {
						fullResponse += inner.Delta.Text
						assistantFullText += inner.Delta.Text
						sendFn(WSMessage{Type: "chunk", Content: inner.Delta.Text})
					}
				case "thinking_delta":
					if inner.Delta.Thinking != "" {
						thinkingSummary += inner.Delta.Thinking
						sendFn(WSMessage{Type: "thinking", Content: inner.Delta.Thinking})
					}
				case "input_json_delta":
					if inner.Delta.PartialJSON != "" {
						if _, ok := toolIDs[inner.Index]; ok {
							toolInputs[inner.Index] += inner.Delta.PartialJSON
							sendFn(WSMessage{
								Type:    "tool_input",
								ToolID:  toolIDs[inner.Index],
								Content: inner.Delta.PartialJSON,
							})
						}
					}
				}

			case "content_block_stop":
				if toolID, ok := toolIDs[inner.Index]; ok {
					steps = append(steps, StepInfo{
						Type:   "tool",
						Name:   toolNames[inner.Index],
						Input:  truncateString(toolInputs[inner.Index], 200),
						Status: "done",
					})
					sendFn(WSMessage{Type: "tool_result", ToolID: toolID, Content: "done"})
				}
			}

		case "assistant":
			// Final assistant message
			var msg struct {
				Message struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(line), &msg) == nil {
				for _, block := range msg.Message.Content {
					if block.Type == "text" && block.Text != "" {
						if fullResponse == "" {
							fullResponse = block.Text
						}
					}
				}
			}

		case "result":
			// Stream completed
			if wrapper.Subtype == "success" && fullResponse == "" && assistantFullText != "" {
				fullResponse = assistantFullText
			}
		}
	})

	// Build metadata JSON
	if len(steps) > 0 || thinkingSummary != "" {
		meta := StepsMetadata{Steps: steps}
		if len(thinkingSummary) > 500 {
			meta.Thinking = thinkingSummary[:500] + "..."
		} else {
			meta.Thinking = thinkingSummary
		}
		if b, err := json.Marshal(meta); err == nil {
			metadataJSON = string(b)
		}
	}

	if err != nil {
		return fullResponse, metadataJSON, err
	}

	log.Printf("[shadow-stream] shadow=%d broadcast_as=%d: completed, response_len=%d", shadowSession.ID, broadcastAsID, len(fullResponse))
	return fullResponse, metadataJSON, nil
}

// runSilentStream executes Claude CLI without broadcasting to frontend
// Used for attention AI preprocessing - only returns the final response
func runSilentStream(ctx context.Context, session *model.Session, query string, provider *model.Provider) (string, error) {
	log.Printf("[silent-stream] session=%d: starting", session.ID)

	// Build system prompt
	tplVars := runtimeTemplateVars(session.ID, session.GroupName)
	var promptParts []string
	if globalPrompt := core.BuildSystemPromptWithVars(tplVars); globalPrompt != "" {
		promptParts = append(promptParts, globalPrompt)
	}
	if teamRules := core.BuildTeamRulesWithVars(session.GroupName, tplVars); teamRules != "" {
		promptParts = append(promptParts, teamRules)
	}
	// Auto-inject team members list
	if session.GroupName != "" {
		if membersList := buildTeamMembersList(session.GroupName, session.ID); membersList != "" {
			promptParts = append(promptParts, membersList)
		}
	}
	// Structured memory injection (Issue #210)
	if memBlock := buildStructuredMemoryInjection(query); memBlock != "" {
		promptParts = append(promptParts, memBlock)
	}

	req := core.ClaudeCodeRequest{
		Query:        query,
		SessionID:    session.ClaudeSessionID,
		Resume:       false,
		BaseURL:      provider.BaseURL,
		APIKey:       provider.APIKey,
		AuthMode:     provider.AuthMode,
		ProxyURL:     provider.ProxyURL,
		ModelID:      strings.TrimSpace(provider.ModelID),
		WorkDir:      session.WorkDir,
		HubSessionID: session.ID,
		GroupName:    session.GroupName,
	}
	if provider.AuthMode == "oauth" {
		req.ModelID = ""
	}
	if len(promptParts) > 0 {
		req.SystemPrompt = strings.Join(promptParts, "\n\n---\n\n")
	}
	// For non-Anthropic API providers, append web_search disable hint
	if provider.BaseURL != "" && !strings.Contains(provider.BaseURL, "api.anthropic.com") {
		webSearchHint := "\n\n重要：当前 API 不支持 web_search 工具，请勿使用。如需搜索信息，请使用其他方式（如 MCP 浏览器工具）。"
		req.SystemPrompt += webSearchHint
	}

	var fullResponse string
	var assistantFullText string

	err := claudeClient.StreamPersistent(ctx, req, func(line string) {
		var wrapper struct {
			Type    string          `json:"type"`
			Subtype string          `json:"subtype"`
			Event   json.RawMessage `json:"event"`
		}
		if err := json.Unmarshal([]byte(line), &wrapper); err != nil {
			return
		}

		switch wrapper.Type {
		case "stream_event":
			var inner struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if json.Unmarshal(wrapper.Event, &inner) == nil {
				if inner.Type == "content_block_delta" && inner.Delta.Type == "text_delta" {
					assistantFullText += inner.Delta.Text
				}
			}

		case "assistant":
			var msg struct {
				Message struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(line), &msg) == nil {
				for _, block := range msg.Message.Content {
					if block.Type == "text" && block.Text != "" {
						if fullResponse == "" {
							fullResponse = block.Text
						}
					}
				}
			}

		case "result":
			if wrapper.Subtype == "success" && fullResponse == "" && assistantFullText != "" {
				fullResponse = assistantFullText
			}
		}
	})

	if err != nil {
		return fullResponse, err
	}

	log.Printf("[silent-stream] session=%d: completed, response_len=%d", session.ID, len(fullResponse))
	return fullResponse, nil
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// fireMessageReceivedHook fires message.received and message.count hooks.
func fireMessageReceivedHook(sessionID int64, content string) {
	// Get message count for message.count hooks
	msgCount, _ := store.GetMessagesCount(sessionID)

	// Fire message.received
	core.FireHooks(core.HookEvent{
		Type:            "message.received",
		SourceSessionID: sessionID,
		Content:         content,
		MessageCount:    msgCount,
	})

	// Fire message.count
	core.FireHooks(core.HookEvent{
		Type:            "message.count",
		SourceSessionID: sessionID,
		Content:         content,
		MessageCount:    msgCount,
	})
}

// initHookStreamCallback registers the stream callback for hook-triggered messages.
// Must be called during api initialization.
func initHookStreamCallback() {
	core.SetHookStreamCallback(func(session *model.Session, content string, triggerMsgID int64) {
		go runStream(session, content, false, triggerMsgID)
	})
}
