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
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSMessage struct {
	Type      string `json:"type"` // "chat" | "stop" | "error" | "chunk" | "done" | "session_created" | "title_update"
	SessionID int64  `json:"session_id"`
	Content   string `json:"content"`
}

var (
	claudeClient = core.NewClaudeCodeClient()
	openaiClient = core.NewOpenAIClient()
)

func HandleChat(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	var mu sync.Mutex
	sendJSON := func(msg WSMessage) {
		mu.Lock()
		defer mu.Unlock()
		conn.WriteJSON(msg)
	}

	var cancelFn context.CancelFunc

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
			if cancelFn != nil {
				cancelFn()
			}
		case "chat":
			go func(msg WSMessage) {
				handleChatMessage(msg, sendJSON, &cancelFn)
			}(msg)
		}
	}
}

func handleChatMessage(msg WSMessage, send func(WSMessage), cancelFn *context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	*cancelFn = cancel
	defer cancel()

	var session *model.Session

	if msg.SessionID == 0 {
		// Auto-create session on first message
		provider, err := store.GetDefaultProvider()
		if err != nil {
			send(WSMessage{Type: "error", Content: "No default provider configured. Go to Settings to add one."})
			return
		}
		session, err = store.CreateSessionWithMessage(provider.ID, msg.Content)
		if err != nil {
			send(WSMessage{Type: "error", Content: "create session failed: " + err.Error()})
			return
		}
		// Notify frontend of the new session
		sessionJSON, _ := json.Marshal(session)
		send(WSMessage{Type: "session_created", SessionID: session.ID, Content: string(sessionJSON)})
	} else {
		var err error
		session, err = store.GetSession(msg.SessionID)
		if err != nil {
			send(WSMessage{Type: "error", Content: "session not found"})
			return
		}
		// Save user message
		userMsg := &model.Message{
			SessionID: session.ID,
			Role:      "user",
			Content:   msg.Content,
		}
		if err := store.AddMessage(userMsg); err != nil {
			send(WSMessage{Type: "error", Content: "save message failed: " + err.Error()})
			return
		}
	}

	// Get provider
	provider, err := store.GetProvider(session.ProviderID)
	if err != nil {
		send(WSMessage{Type: "error", Content: "provider not found: " + err.Error()})
		return
	}

	var fullResponse string
	sid := fmt.Sprintf("%d", session.ID)

	switch provider.Mode {
	case "claude-code":
		err = streamClaudeCode(ctx, provider, msg.Content, sid, func(chunk string) {
			fullResponse += chunk
			send(WSMessage{Type: "chunk", SessionID: session.ID, Content: chunk})
		})
	default:
		err = streamOpenAI(ctx, provider, session.ID, func(chunk string) {
			fullResponse += chunk
			send(WSMessage{Type: "chunk", SessionID: session.ID, Content: chunk})
		})
	}

	if err != nil {
		send(WSMessage{Type: "error", SessionID: session.ID, Content: err.Error()})
		return
	}

	// Save assistant message
	if fullResponse != "" {
		assistantMsg := &model.Message{
			SessionID: session.ID,
			Role:      "assistant",
			Content:   fullResponse,
		}
		store.AddMessage(assistantMsg)
	}

	send(WSMessage{Type: "done", SessionID: session.ID})
}

func streamClaudeCode(ctx context.Context, p *model.Provider, query, sessionID string, onChunk func(string)) error {
	req := core.ClaudeCodeRequest{
		Query:     query,
		SessionID: sessionID,
		BaseURL:   p.BaseURL,
		APIKey:    p.APIKey,
		ModelID:   p.ModelID,
	}
	return claudeClient.Stream(ctx, req, func(line string) {
		var evt struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			return
		}
		switch evt.Type {
		case "content_block_delta":
			var delta struct {
				Delta struct {
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(line), &delta); err == nil && delta.Delta.Text != "" {
				onChunk(delta.Delta.Text)
			}
		case "assistant":
			for _, c := range evt.Content {
				if c.Type == "text" && c.Text != "" {
					onChunk(c.Text)
				}
			}
		case "result":
			var result struct {
				Result string `json:"result"`
			}
			if err := json.Unmarshal([]byte(line), &result); err == nil && result.Result != "" {
				onChunk(result.Result)
			}
		}
	})
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
