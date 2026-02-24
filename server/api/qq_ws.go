package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// msgDedup is a bounded, TTL-based message ID deduplication cache.
type msgDedup struct {
	mu      sync.Mutex
	cache   map[string]time.Time
	maxSize int
	ttl     time.Duration
}

func newMsgDedup(maxSize int, ttl time.Duration) *msgDedup {
	return &msgDedup{
		cache:   make(map[string]time.Time),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// isDuplicate returns true if msgID was seen within TTL, otherwise records it.
func (d *msgDedup) isDuplicate(msgID string) bool {
	if msgID == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	if t, ok := d.cache[msgID]; ok && now.Sub(t) < d.ttl {
		return true
	}
	// Evict expired entries when at capacity
	if len(d.cache) >= d.maxSize {
		for k, t := range d.cache {
			if now.Sub(t) >= d.ttl {
				delete(d.cache, k)
			}
		}
	}
	// Still full → drop oldest
	if len(d.cache) >= d.maxSize {
		var oldK string
		var oldT time.Time
		for k, t := range d.cache {
			if oldK == "" || t.Before(oldT) {
				oldK, oldT = k, t
			}
		}
		delete(d.cache, oldK)
	}
	d.cache[msgID] = now
	return false
}

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
	routes    []routingRule
}

// routingRule defines a message routing rule for channel message dispatch.
// Messages matching type + ids are forwarded to the specified session_id.
type routingRule struct {
	Type      string   `json:"type"`       // "group" or "private"
	IDs       []string `json:"ids"`        // group_ids or user_ids
	SessionID int64    `json:"session_id"` // target session
	idSet     map[string]struct{}           // pre-built lookup set
}

// parseRoutingRules extracts routing_rules from channel config JSON.
func parseRoutingRules(cfg map[string]interface{}) []routingRule {
	rulesRaw, ok := cfg["routing_rules"]
	if !ok {
		return nil
	}
	data, err := json.Marshal(rulesRaw)
	if err != nil {
		return nil
	}
	var rules []routingRule
	if err := json.Unmarshal(data, &rules); err != nil {
		log.Printf("[qq-ws] failed to parse routing_rules: %v", err)
		return nil
	}
	// Build lookup sets for fast matching
	for i := range rules {
		rules[i].idSet = make(map[string]struct{}, len(rules[i].IDs))
		for _, id := range rules[i].IDs {
			rules[i].idSet[id] = struct{}{}
		}
	}
	return rules
}

// matchRoute finds the target session_id for a message based on routing rules.
// Returns the matched session_id, or the default fallback if no rule matches.
func (c *qqWSConn) matchRoute(msgType, groupID, userID string) int64 {
	for _, r := range c.routes {
		switch {
		case r.Type == "group" && msgType == "group":
			if _, ok := r.idSet[groupID]; ok {
				return r.SessionID
			}
		case r.Type == "private" && msgType == "private":
			if _, ok := r.idSet[userID]; ok {
				return r.SessionID
			}
		}
	}
	return c.sessionID // fallback to channel default
}

// qqChannelHasRoutes checks if a QQ channel config contains non-empty routing_rules.
func qqChannelHasRoutes(config string) bool {
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		return false
	}
	return len(parseRoutingRules(cfg)) > 0
}

// qqGlobalDedup is a shared dedup cache across all WS connections and HTTP webhooks.
var qqGlobalDedup = newMsgDedup(5000, 5*time.Minute)

// qqContentDedup is a content-based dedup cache to catch messages with different IDs but same content.
// Key = sha256(sessionID + content), TTL = 30s.
var qqContentDedup = newMsgDedup(2000, 30*time.Second)

// contentDedupKey builds a dedup key from session ID and message content.
func contentDedupKey(sessionID int64, content string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", sessionID, content)))
	return hex.EncodeToString(h[:16]) // 128-bit, collision-safe
}

// LogQQDedupConfig logs the dedup configuration at startup.
func LogQQDedupConfig() {
	log.Printf("[qq-dedup] msgID dedup: maxSize=%d ttl=%v | content dedup: maxSize=%d ttl=%v",
		qqGlobalDedup.maxSize, qqGlobalDedup.ttl, qqContentDedup.maxSize, qqContentDedup.ttl)
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
		if ch.Platform == "qq" && ch.Enabled && (ch.SessionID > 0 || qqChannelHasRoutes(ch.Config)) {
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
	if ch.Platform == "qq" && ch.Enabled && (ch.SessionID > 0 || qqChannelHasRoutes(ch.Config)) {
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

	if ch.Platform == "qq" && ch.Enabled && (ch.SessionID > 0 || qqChannelHasRoutes(ch.Config)) {
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
	routes := parseRoutingRules(cfg)

	c := &qqWSConn{
		channelID: ch.ID,
		wsURL:     wsURL,
		token:     token,
		sessionID: ch.SessionID,
		httpURL:   httpURL,
		done:      make(chan struct{}),
		routes:    routes,
	}

	m.mu.Lock()
	m.conns[ch.ID] = c
	m.mu.Unlock()

	go c.connectLoop()
	if len(routes) > 0 {
		log.Printf("[qq-ws] channel %d: starting connection to %s (%d routing rules)", ch.ID, wsURL, len(routes))
	} else {
		log.Printf("[qq-ws] channel %d: starting connection to %s", ch.ID, wsURL)
	}
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

// isChannelActive checks if the channel is still enabled in the database.
// A channel is active if enabled AND (has session binding OR has routing rules).
func (c *qqWSConn) isChannelActive() bool {
	ch, err := store.GetChannel(c.channelID)
	if err != nil || ch == nil {
		return false
	}
	if !ch.Enabled {
		return false
	}
	if ch.SessionID > 0 {
		return true
	}
	// SessionID=0 but has routing rules → still active
	return len(c.routes) > 0
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

		// Check channel status before reconnecting
		if !c.isChannelActive() {
			log.Printf("[qq-ws] channel %d: disabled or deleted, stopping reconnect", c.channelID)
			return
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
	// Check channel is still active before processing
	if !c.isChannelActive() {
		log.Printf("[qq-ws] channel %d: disabled, dropping message", c.channelID)
		return
	}

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

	// Dedup: skip if this message_id was already processed (global shared cache)
	if qqGlobalDedup.isDuplicate(messageID) {
		log.Printf("[qq-ws] channel %d: duplicate message_id %s (source=WS), skipped", c.channelID, messageID)
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

	// Route message to matched session or fallback to channel default
	targetSession := c.matchRoute(msgType, groupID, userID)
	if targetSession <= 0 {
		log.Printf("[qq-ws] channel %d: no matching route and no default session, dropping message from %s", c.channelID, userID)
		return
	}
	// Content-based dedup: same content to same session within 30s → skip
	if qqContentDedup.isDuplicate(contentDedupKey(targetSession, message)) {
		log.Printf("[qq-ws] channel %d: duplicate content to session %d (source=WS), skipped", c.channelID, targetSession)
		return
	}
	log.Printf("[qq-ws] channel %d: forwarding to session %d: %s", c.channelID, targetSession, message)
	forwardToSession(targetSession, forwarded)
}
