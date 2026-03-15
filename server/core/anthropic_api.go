package core

import (
	"ai-hub/server/model"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// CallAnthropicMessagesAPI calls the Anthropic Messages API directly via HTTP.
// This avoids the nested Claude Code session error when running inside Claude Code.
// Parameters:
//   - ctx: context for cancellation/timeout
//   - provider: provider configuration (base URL, API key, model)
//   - userMessage: the user message to send
//   - systemPrompt: optional system prompt (empty string for default)
//   - maxTokens: max tokens for response (0 for default 1024)
func CallAnthropicMessagesAPI(ctx context.Context, provider *model.Provider, userMessage, systemPrompt string, maxTokens int) (string, error) {
	// Determine base URL
	baseURL := provider.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Determine model
	modelID := provider.ModelID
	if modelID == "" {
		modelID = "claude-sonnet-4-20250514"
	}

	// Default max tokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	// Default system prompt
	if systemPrompt == "" {
		systemPrompt = "You are a helpful AI assistant."
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":      modelID,
		"max_tokens": maxTokens,
		"system": []map[string]string{
			{"type": "text", "text": systemPrompt},
		},
		"messages": []map[string]string{
			{"role": "user", "content": userMessage},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	// Set API key
	apiKey := provider.APIKey
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	// HTTP client with timeout
	client := &http.Client{Timeout: 120 * time.Second}
	if provider.ProxyURL != "" {
		log.Printf("[anthropic-api] proxy configured but not used for direct API call: %s", provider.ProxyURL)
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	// Check for error response
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return "", fmt.Errorf("API error (%s): %s", errResp.Error.Type, errResp.Error.Message)
		}
		return "", fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	// Extract text content
	var result strings.Builder
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			result.WriteString(block.Text)
		}
	}

	log.Printf("[anthropic-api] success: input=%d output=%d tokens", apiResp.Usage.InputTokens, apiResp.Usage.OutputTokens)
	return result.String(), nil
}
