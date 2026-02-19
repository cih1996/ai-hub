package api

import (
	"ai-hub/server/core"
	"ai-hub/server/model"
	"ai-hub/server/store"
	"context"
	"encoding/json"
	"log"
	"net/http"
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
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			case <-pingDone:
				return
			}
		}
	}()

	client := &wsClient{conn: conn}
	registerClient(client)
	defer unregisterClient(client)
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
			}
		}
	}
}

// SendChat handles POST /api/v1/chat/send
// Validates/creates session, saves user message, kicks off streaming in background, returns immediately.
func SendChat(c *gin.Context) {
	var req struct {
		SessionID int64  `json:"session_id"`
		Content   string `json:"content"`
		WorkDir   string `json:"work_dir"`
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
		session, err = store.CreateSessionWithMessage(provider.ID, req.Content, req.WorkDir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create session failed: " + err.Error()})
			return
		}
		// Broadcast new session to all connected clients
		sessionJSON, _ := json.Marshal(session)
		broadcast(WSMessage{Type: "session_created", SessionID: session.ID, Content: string(sessionJSON)})
	} else {
		var err error
		session, err = store.GetSession(req.SessionID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		// Check if session is already streaming
		if IsSessionStreaming(session.ID) {
			c.JSON(http.StatusConflict, gin.H{"error": "session is already processing"})
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

	// Render templates before starting chat (refresh dynamic variables like time)
	core.RenderAllTemplates()

	// Kick off streaming in background — results are pushed via WS broadcast
	go runStream(session, req.Content, isNewSession)

	c.JSON(http.StatusOK, gin.H{
		"session_id": session.ID,
		"status":     "started",
	})
}

// runStream executes the AI streaming in background, pushing events via WS to subscribed clients
func runStream(session *model.Session, query string, isNewSession bool) {
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
	}()

	provider, err := store.GetProvider(session.ProviderID)
	if err != nil {
		stream.Send(WSMessage{Type: "error", SessionID: session.ID, Content: "provider not found: " + err.Error()})
		return
	}

	var fullResponse string

	log.Printf("[chat] session=%d provider=%s mode=%s model=%s base_url=%s",
		session.ID, provider.Name, provider.Mode, provider.ModelID, provider.BaseURL)

	switch provider.Mode {
	case "claude-code":
		isResume := !isNewSession
		fullResponse, err = streamClaudeCode(ctx, provider, query, session.ClaudeSessionID, isResume, stream.Send, session.ID, session.WorkDir)
	default:
		err = streamOpenAI(ctx, provider, session.ID, func(chunk string) {
			fullResponse += chunk
			stream.Send(WSMessage{Type: "chunk", SessionID: session.ID, Content: chunk})
		})
	}

	if err != nil {
		log.Printf("[chat] error: %v", err)
		stream.Send(WSMessage{Type: "error", SessionID: session.ID, Content: err.Error()})
		return
	}

	if fullResponse != "" {
		assistantMsg := &model.Message{
			SessionID: session.ID,
			Role:      "assistant",
			Content:   fullResponse,
		}
		store.AddMessage(assistantMsg)
	}

	stream.Send(WSMessage{Type: "done", SessionID: session.ID})
}

func streamClaudeCode(ctx context.Context, p *model.Provider, query, sessionID string, resume bool, send func(WSMessage), sessID int64, workDir string) (string, error) {
	req := core.ClaudeCodeRequest{
		Query:        query,
		SessionID:    sessionID,
		Resume:       resume,
		BaseURL:      p.BaseURL,
		APIKey:       p.APIKey,
		ModelID:      p.ModelID,
		WorkDir:      workDir,
		HubSessionID: sessID,
	}

	// Inject session rules as system prompt if available
	if rules, err := ReadSessionRules(sessID); err == nil && rules != "" {
		req.SystemPrompt = rules
	}
	var fullResponse string

	// Track content block index -> tool ID for correlating deltas
	toolIDs := make(map[int]string)

	err := claudeClient.StreamPersistent(ctx, req, func(line string) {
		// First parse the top-level wrapper
		var wrapper struct {
			Type             string          `json:"type"`
			Subtype          string          `json:"subtype"`
			Result           string          `json:"result"`
			Event            json.RawMessage `json:"event"`
			ConversationName string          `json:"conversation_name"`
			Error            *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &wrapper); err != nil {
			log.Printf("[claude] json parse error: %v, line: %.200s", err, line)
			return
		}

		switch wrapper.Type {
		case "error":
			// API-level error
			errMsg := "unknown error"
			if wrapper.Error != nil && wrapper.Error.Message != "" {
				errMsg = wrapper.Error.Message
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
				} `json:"delta"`
			}
			if err := json.Unmarshal(wrapper.Event, &inner); err != nil {
				return
			}

			switch inner.Type {
			case "content_block_start":
				if inner.ContentBlock.Type == "tool_use" {
					toolIDs[inner.Index] = inner.ContentBlock.ID
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
						send(WSMessage{Type: "thinking", SessionID: sessID, Content: inner.Delta.Thinking})
					}
				case "input_json_delta":
					if inner.Delta.PartialJSON != "" {
						toolID := toolIDs[inner.Index]
						send(WSMessage{Type: "tool_input", SessionID: sessID, ToolID: toolID, Content: inner.Delta.PartialJSON})
					}
				}
			case "content_block_stop":
				if toolID, ok := toolIDs[inner.Index]; ok {
					send(WSMessage{Type: "tool_result", SessionID: sessID, ToolID: toolID})
					delete(toolIDs, inner.Index)
				}
			}

		case "result":
			if wrapper.ConversationName != "" {
				if err := store.UpdateSessionTitle(sessID, wrapper.ConversationName); err == nil {
					broadcast(WSMessage{Type: "session_title_update", SessionID: sessID, Content: wrapper.ConversationName})
				}
			}
			if wrapper.Subtype == "success" && wrapper.Result != "" && fullResponse == "" {
				fullResponse = wrapper.Result
				send(WSMessage{Type: "chunk", SessionID: sessID, Content: wrapper.Result})
			} else if wrapper.Subtype == "error" && wrapper.Result != "" {
				send(WSMessage{Type: "error", SessionID: sessID, Content: wrapper.Result})
			}

		// Ignore "assistant" and "system" types — they are duplicates of stream_event data
		default:
		}
	})
	return fullResponse, err
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
