package api

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- Proxy Usage Accumulator ----
// Accumulates token usage captured by the proxy per session.
// runStream reads and resets after streaming completes.

type ProxyUsage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
	Captured                 bool // true if proxy captured any usage
}

type meteringContext struct {
	Provider    *model.Provider
	Adapter     UsageAdapter
	EstInput    int64
	RequestBody []byte
}

var (
	proxyUsageMap = make(map[int64]*ProxyUsage)
	proxyUsageMu  sync.Mutex
)

// proxyAccumulate adds usage from one API call to the session accumulator.
func proxyAccumulate(sessionID int64, input, output, cacheCreation, cacheRead int64) {
	proxyUsageMu.Lock()
	defer proxyUsageMu.Unlock()
	u, ok := proxyUsageMap[sessionID]
	if !ok {
		u = &ProxyUsage{}
		proxyUsageMap[sessionID] = u
	}
	u.InputTokens += input
	u.OutputTokens += output
	u.CacheCreationInputTokens += cacheCreation
	u.CacheReadInputTokens += cacheRead
	u.Captured = true
}

// ConsumeProxyUsage returns accumulated proxy usage for a session and resets it.
// Called by runStream after streaming completes.
func ConsumeProxyUsage(sessionID int64) *ProxyUsage {
	proxyUsageMu.Lock()
	defer proxyUsageMu.Unlock()
	u, ok := proxyUsageMap[sessionID]
	if !ok || !u.Captured {
		return nil
	}
	delete(proxyUsageMap, sessionID)
	return u
}

// ResetProxyUsage clears proxy usage for a session (called when stream starts).
func ResetProxyUsage(sessionID int64) {
	proxyUsageMu.Lock()
	defer proxyUsageMu.Unlock()
	delete(proxyUsageMap, sessionID)
}

// ---- Anthropic API Reverse Proxy ----

// HandleAnthropicProxy handles /api/v1/proxy/anthropic/*path
// It forwards requests to the real Anthropic API and captures usage from SSE responses.
func HandleAnthropicProxy(c *gin.Context) {
	// Extract session_id from path parameter (new) or query parameter (legacy)
	sidStr := c.Param("session_id")
	if sidStr == "" {
		sidStr = c.Query("session_id")
	}
	sessionID, _ := strconv.ParseInt(sidStr, 10, 64)

	// Get the sub-path after /api/v1/proxy/anthropic
	subPath := c.Param("path")
	if subPath == "" {
		subPath = "/"
	}

	// Look up session → provider → real base_url
	var (
		realBaseURL string
		provider    *model.Provider
	)
	if sessionID > 0 {
		session, err := store.GetSession(sessionID)
		if err == nil {
			p, err := store.GetProvider(session.ProviderID)
			if err == nil {
				provider = p
			}
			if provider != nil && provider.BaseURL != "" {
				realBaseURL = provider.BaseURL
			}
		}
	}
	if realBaseURL == "" {
		realBaseURL = "https://api.anthropic.com"
	}

	// Build target URL
	targetURL := strings.TrimRight(realBaseURL, "/") + subPath
	log.Printf("[proxy] session=%d forwarding %s %s → %s", sessionID, c.Request.Method, subPath, targetURL)

	// Read request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body: " + err.Error()})
		return
	}

	meter := &meteringContext{Provider: provider, RequestBody: body}
	if provider != nil {
		meter.Adapter = selectUsageAdapter(provider)
		if meter.Adapter != nil {
			meter.EstInput = meter.Adapter.EstimateInputTokens(provider, body)
			log.Printf("[proxy] session=%d metering adapter=%s mode=%s estimated_input=%d",
				sessionID, meter.Adapter.Name(), provider.UsageMode, meter.EstInput)
		}
	}

	// Create forwarding request
	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, strings.NewReader(string(body)))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create request: " + err.Error()})
		return
	}

	// Copy headers (except Host)
	for key, vals := range c.Request.Header {
		if strings.EqualFold(key, "Host") {
			continue
		}
		for _, v := range vals {
			proxyReq.Header.Add(key, v)
		}
	}

	// Send request to real API
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("[proxy] session=%d upstream error: %v", sessionID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, vals := range resp.Header {
		for _, v := range vals {
			c.Writer.Header().Add(key, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)

	// Check if SSE streaming response
	contentType := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(contentType, "text/event-stream")

	if !isSSE {
		// Non-streaming: just copy body through and try to parse usage
		respBody, _ := io.ReadAll(resp.Body)
		parseNonStreamUsage(sessionID, respBody, meter)
		c.Writer.Write(respBody)
		c.Writer.Flush()
		return
	}

	// SSE streaming: parse usage while forwarding
	streamProxySSE(c, resp.Body, sessionID, meter)
}

// streamProxySSE reads SSE lines from upstream, parses usage, and forwards to client.
func streamProxySSE(c *gin.Context, upstream io.Reader, sessionID int64, meter *meteringContext) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		log.Printf("[proxy] session=%d ResponseWriter does not support Flush", sessionID)
	}

	var turnInput, turnOutput, turnCacheCreation, turnCacheRead int64
	var outputText strings.Builder

	scanner := bufio.NewScanner(upstream)
	scanner.Buffer(make([]byte, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Forward line as-is to client
		fmt.Fprintf(c.Writer, "%s\n", line)
		if ok {
			flusher.Flush()
		}

		// Parse SSE data lines for usage
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			continue
		}

		var evt struct {
			Type    string `json:"type"`
			Message struct {
				Usage struct {
					InputTokens              int64 `json:"input_tokens"`
					OutputTokens             int64 `json:"output_tokens"`
					CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
					CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}

		switch evt.Type {
		case "message_start":
			turnInput += evt.Message.Usage.InputTokens
			turnCacheCreation += evt.Message.Usage.CacheCreationInputTokens
			turnCacheRead += evt.Message.Usage.CacheReadInputTokens
		case "message_delta":
			turnOutput += evt.Usage.OutputTokens
		case "content_block_delta":
			var txtEvt struct {
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &txtEvt); err == nil && txtEvt.Delta.Type == "text_delta" && txtEvt.Delta.Text != "" {
				outputText.WriteString(txtEvt.Delta.Text)
			}
		}
	}

	if meter != nil && meter.Adapter != nil {
		if turnInput == 0 && meter.EstInput > 0 {
			turnInput = meter.EstInput
		}
		if turnOutput == 0 {
			if n := meter.Adapter.EstimateOutputTokens(meter.Provider, outputText.String(), nil); n > 0 {
				turnOutput = n
			}
		}
	}

	// Accumulate this API call's usage into session total
	if sessionID > 0 && (turnInput > 0 || turnOutput > 0 || turnCacheCreation > 0 || turnCacheRead > 0) {
		proxyAccumulate(sessionID, turnInput, turnOutput, turnCacheCreation, turnCacheRead)
		log.Printf("[proxy] session=%d usage: input=%d output=%d cache_create=%d cache_read=%d",
			sessionID, turnInput, turnOutput, turnCacheCreation, turnCacheRead)
	}
}

// parseNonStreamUsage parses usage from a non-streaming Anthropic API response.
func parseNonStreamUsage(sessionID int64, body []byte, meter *meteringContext) {
	if sessionID <= 0 {
		return
	}
	var resp struct {
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return
	}
	u := resp.Usage
	if meter != nil && meter.Adapter != nil {
		if u.InputTokens == 0 && meter.EstInput > 0 {
			u.InputTokens = meter.EstInput
		}
		if u.OutputTokens == 0 {
			if n := meter.Adapter.EstimateOutputTokens(meter.Provider, "", body); n > 0 {
				u.OutputTokens = n
			}
		}
	}
	if u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheCreationInputTokens > 0 || u.CacheReadInputTokens > 0 {
		proxyAccumulate(sessionID, u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens)
		log.Printf("[proxy] session=%d non-stream usage: input=%d output=%d cache_create=%d cache_read=%d",
			sessionID, u.InputTokens, u.OutputTokens, u.CacheCreationInputTokens, u.CacheReadInputTokens)
	}
}
