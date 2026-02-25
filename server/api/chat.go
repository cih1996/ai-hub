package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSMessage struct {
	Type      string `json:"type"` // "chat" | "stop" | "subscribe" | "error" | "chunk" | "thinking" | "tool_start" | "tool_input" | "tool_result" | "done" | "session_created" | "streaming_status" | "session_update"
	SessionID int64  `json:"session_id"`
	Content   string `json:"content"`
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

// ActiveStream tracks an in-progress chat stream so new WS connections can reattach
type ActiveStream struct {
	mu       sync.Mutex
	sendFn   func(WSMessage)
	cancelFn context.CancelFunc
}

func (s *ActiveStream) Send(msg WSMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendFn != nil {
		s.sendFn(msg)
	}
}

func (s *ActiveStream) SwapSend(fn func(WSMessage)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendFn = fn
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
	openaiClient    = core.NewOpenAIClient()
	activeStreams   = make(map[int64]*ActiveStream)
	activeStreamsMu sync.RWMutex
)

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
				stream.SwapSend(sendJSON)
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
		provider, err := store.GetDefaultProvider()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No default provider configured. Go to Settings to add one."})
			return
		}
		session, err = store.CreateSessionWithMessage(provider.ID, req.Content, req.WorkDir, req.GroupName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create session failed: " + err.Error()})
			return
		}
		// Broadcast new session to all connected clients
		sessionJSON, _ := json.Marshal(session)
		broadcast(WSMessage{Type: "session_created", SessionID: session.ID, Content: string(sessionJSON)})

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
			broadcast(WSMessage{Type: "message_queued", SessionID: session.ID, Content: req.Content})
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

	// Reset proxy usage accumulator at stream start (Issue #72)
	ResetProxyUsage(session.ID)

	log.Printf("[chat] session=%d provider=%s mode=%s model=%s base_url=%s",
		session.ID, provider.Name, provider.Mode, provider.ModelID, provider.BaseURL)

	switch provider.Mode {
	case "claude-code":
		isResume := !isNewSession && core.Pool.HasProcess(session.ID)
		fullResponse, metadataJSON, usageInput, usageOutput, usageCacheCreation, usageCacheRead, err = streamClaudeCode(ctx, provider, query, session.ClaudeSessionID, isResume, stream.Send, session.ID, session.WorkDir)
	default:
		err = streamOpenAI(ctx, provider, session.ID, func(chunk string) {
			fullResponse += chunk
			stream.Send(WSMessage{Type: "chunk", SessionID: session.ID, Content: chunk})
		})
	}

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
			assistantMsg := &model.Message{
				SessionID: session.ID,
				Role:      "assistant",
				Content:   content,
				Metadata:  metadataJSON,
			}
			store.AddMessage(assistantMsg)
			// Save token usage even on error (partial response)
			if usageInput > 0 || usageOutput > 0 || usageCacheCreation > 0 || usageCacheRead > 0 {
				tu := &model.TokenUsage{SessionID: session.ID, MessageID: assistantMsg.ID, InputTokens: usageInput, OutputTokens: usageOutput, CacheCreationInputTokens: usageCacheCreation, CacheReadInputTokens: usageCacheRead}
				store.AddTokenUsage(tu)
				usageJSON, _ := json.Marshal(tu)
				broadcast(WSMessage{Type: "token_usage", SessionID: session.ID, Content: string(usageJSON)})
			}
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
		assistantMsg := &model.Message{
			SessionID: session.ID,
			Role:      "assistant",
			Content:   content,
			Metadata:  metadataJSON,
		}
		store.AddMessage(assistantMsg)
		// Save and broadcast token usage
		if usageInput > 0 || usageOutput > 0 || usageCacheCreation > 0 || usageCacheRead > 0 {
			tu := &model.TokenUsage{SessionID: session.ID, MessageID: assistantMsg.ID, InputTokens: usageInput, OutputTokens: usageOutput, CacheCreationInputTokens: usageCacheCreation, CacheReadInputTokens: usageCacheRead}
			store.AddTokenUsage(tu)
			usageJSON, _ := json.Marshal(tu)
			broadcast(WSMessage{Type: "token_usage", SessionID: session.ID, Content: string(usageJSON)})
		}
	}

	// Broadcast done so even reconnected/new WS clients receive it (stream.Send is single-client)
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

func streamClaudeCode(ctx context.Context, p *model.Provider, query, sessionID string, resume bool, send func(WSMessage), sessID int64, workDir string) (string, string, int64, int64, int64, int64, error) {
	req := core.ClaudeCodeRequest{
		Query:        query,
		SessionID:    sessionID,
		Resume:       resume,
		BaseURL:      p.BaseURL,
		APIKey:       p.APIKey,
		AuthMode:     p.AuthMode,
		ModelID:      p.ModelID,
		WorkDir:      workDir,
		HubSessionID: sessID,
	}

	// Build system prompt: global rules + session rules
	var promptParts []string
	if globalPrompt := core.BuildSystemPrompt(); globalPrompt != "" {
		promptParts = append(promptParts, globalPrompt)
	}
	if rules, err := ReadSessionRules(sessID); err == nil && rules != "" {
		promptParts = append(promptParts, rules)
	}
	if len(promptParts) > 0 {
		req.SystemPrompt = strings.Join(promptParts, "\n\n---\n\n")
	}
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
			Event            json.RawMessage `json:"event"`
			ConversationName string          `json:"conversation_name"`
			Error json.RawMessage `json:"error"`
			Usage struct {
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
			if wrapper.ConversationName != "" {
				if err := store.UpdateSessionTitle(sessID, wrapper.ConversationName); err == nil {
					broadcast(WSMessage{Type: "session_title_update", SessionID: sessID, Content: wrapper.ConversationName})
				}
			}
			if wrapper.Subtype == "success" && fullResponse == "" {
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
			} else if wrapper.Subtype == "error" && wrapper.Result != "" {
				send(WSMessage{Type: "error", SessionID: sessID, Content: wrapper.Result})
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

func streamOpenAI(ctx context.Context, p *model.Provider, sessionID int64, onChunk func(string)) error {
	msgs, err := store.GetMessages(sessionID)
	if err != nil {
		return err
	}
	var chatMsgs []core.ChatMessage
	for _, m := range msgs {
		chatMsgs = append(chatMsgs, core.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return openaiClient.Stream(ctx, core.OpenAIRequest{
		BaseURL:  p.BaseURL,
		APIKey:   p.APIKey,
		ModelID:  p.ModelID,
		Messages: chatMsgs,
	}, onChunk)
}
