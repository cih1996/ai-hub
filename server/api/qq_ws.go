package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// QQWSManager manages WebSocket client connections to NapCat WS servers.
// Each enabled QQ channel with a napcat_ws_url gets a persistent connection.
type QQWSManager struct {
	mu    sync.Mutex
	conns map[int64]*qqWSConn // channel_id → connection
}

type qqWSConn struct {
	channelID int64
	wsURL     string
	token     string
	sessionID int64
	httpURL   string // for credentials in forwarded messages
	conn      *websocket.Conn
	done      chan struct{}
	stopped   bool
}

var QQWSMgr = &QQWSManager{
	conns: make(map[int64]*qqWSConn),
}

// StartAll scans all enabled QQ channels and connects those with napcat_ws_url.
func (m *QQWSManager) StartAll() {
	channels, err := store.ListChannels()
	if err != nil {
		log.Printf("[qq-ws] failed to list channels: %v", err)
		return
	}
	for _, ch := range channels {
		if ch.Platform == "qq" && ch.Enabled && ch.SessionID > 0 {
			m.tryConnect(&ch)
		}
	}
}

// Shutdown closes all WS connections.
func (m *QQWSManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.conns {
		c.stop()
		delete(m.conns, id)
	}
	log.Println("[qq-ws] all connections closed")
}

// OnChannelCreated handles new channel creation.
func (m *QQWSManager) OnChannelCreated(ch *model.Channel) {
	if ch.Platform == "qq" && ch.Enabled && ch.SessionID > 0 {
		m.tryConnect(ch)
	}
}

// OnChannelUpdated handles channel config/state changes.
func (m *QQWSManager) OnChannelUpdated(ch *model.Channel) {
	m.mu.Lock()
	old, exists := m.conns[ch.ID]
	m.mu.Unlock()

	if exists {
		old.stop()
		m.mu.Lock()
		delete(m.conns, ch.ID)
		m.mu.Unlock()
	}

	if ch.Platform == "qq" && ch.Enabled && ch.SessionID > 0 {
		m.tryConnect(ch)
	}
}

// OnChannelDeleted handles channel deletion.
func (m *QQWSManager) OnChannelDeleted(channelID int64) {
	m.mu.Lock()
	if c, ok := m.conns[channelID]; ok {
		c.stop()
		delete(m.conns, channelID)
	}
	m.mu.Unlock()
}

// tryConnect parses channel config and starts a WS connection if napcat_ws_url is set.
func (m *QQWSManager) tryConnect(ch *model.Channel) {
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return
	}
	wsURL, _ := cfg["napcat_ws_url"].(string)
	if wsURL == "" {
		return // no WS URL configured, skip (HTTP webhook mode)
	}
	token, _ := cfg["token"].(string)
	httpURL, _ := cfg["napcat_http_url"].(string)

	c := &qqWSConn{
		channelID: ch.ID,
		wsURL:     wsURL,
		token:     token,
		sessionID: ch.SessionID,
		httpURL:   httpURL,
		done:      make(chan struct{}),
	}

	m.mu.Lock()
	m.conns[ch.ID] = c
	m.mu.Unlock()

	go c.connectLoop()
	log.Printf("[qq-ws] channel %d: starting connection to %s", ch.ID, wsURL)
}

// stop signals the connection goroutine to exit and closes the WS.
func (c *qqWSConn) stop() {
	if c.stopped {
		return
	}
	c.stopped = true
	close(c.done)
	if c.conn != nil {
		c.conn.Close()
	}
	log.Printf("[qq-ws] channel %d: stopped", c.channelID)
}

// connectLoop maintains the WS connection with exponential backoff reconnection.
func (c *qqWSConn) connectLoop() {
	backoff := time.Second
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-c.done:
			return
		default:
		}

		err := c.dial()
		if err != nil {
			log.Printf("[qq-ws] channel %d: connect failed: %v, retry in %v", c.channelID, err, backoff)
			select {
			case <-c.done:
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Connected, reset backoff
		backoff = time.Second
		log.Printf("[qq-ws] channel %d: connected to %s", c.channelID, c.wsURL)

		// Read messages until error
		c.readLoop()

		// readLoop exited, check if we should reconnect
		select {
		case <-c.done:
			return
		default:
			log.Printf("[qq-ws] channel %d: disconnected, reconnecting in %v", c.channelID, backoff)
		}
	}
}

// dial establishes the WebSocket connection with optional token auth.
func (c *qqWSConn) dial() error {
	wsURL := c.wsURL
	if c.token != "" {
		u, err := url.Parse(wsURL)
		if err != nil {
			return err
		}
		q := u.Query()
		q.Set("access_token", c.token)
		u.RawQuery = q.Encode()
		wsURL = u.String()
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

// readLoop reads messages from the WS connection and processes OneBot 11 events.
func (c *qqWSConn) readLoop() {
	defer func() {
		if c.conn != nil {
			c.conn.Close()
		}
	}()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if !c.stopped {
				log.Printf("[qq-ws] channel %d: read error: %v", c.channelID, err)
			}
			return
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(msg, &raw); err != nil {
			continue
		}

		postType, _ := raw["post_type"].(string)
		if postType != "message" {
			continue
		}

		c.handleMessage(raw)
	}
}

// handleMessage processes a single OneBot 11 message event and forwards to the bound session.
func (c *qqWSConn) handleMessage(raw map[string]interface{}) {
	msgType, _ := raw["message_type"].(string)
	userID := jsonNumber(raw["user_id"])
	groupID := jsonNumber(raw["group_id"])
	messageID := jsonNumber(raw["message_id"])
	message, _ := raw["message"].(string)
	if message == "" {
		message, _ = raw["raw_message"].(string)
	}
	if message == "" {
		return
	}

	typeLabel := "私聊"
	if msgType == "group" {
		typeLabel = "群聊"
	}

	// Build credentials string
	credLines := []string{}
	if c.httpURL != "" {
		credLines = append(credLines, fmt.Sprintf("NapCat地址: %s", c.httpURL))
	}
	if c.token != "" {
		credLines = append(credLines, fmt.Sprintf("Token: %s", c.token))
	}
	creds := strings.Join(credLines, "\n")
	if creds == "" {
		creds = "（频道未配置凭证）"
	}

	forwarded := fmt.Sprintf("【QQ消息】\n类型: %s\n发送者: %s", typeLabel, userID)
	if msgType == "group" && groupID != "" {
		forwarded += fmt.Sprintf("\n群号: %s", groupID)
	}
	forwarded += fmt.Sprintf("\n消息ID: %s\n内容: %s\n---\n频道凭证（用于回复）:\n%s",
		messageID, message, creds)

	log.Printf("[qq-ws] channel %d: forwarding to session %d: %s", c.channelID, c.sessionID, message)
	forwardToSession(c.sessionID, forwarded)
}
