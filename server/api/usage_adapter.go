package api

import (
	"ai-hub/server/model"
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// UsageAdapter estimates missing usage fields for specific providers.
// It is only used when provider.UsageMode == "middleware".
type UsageAdapter interface {
	Name() string
	Match(p *model.Provider) bool
	EstimateInputTokens(p *model.Provider, requestBody []byte) int64
	EstimateOutputTokens(p *model.Provider, outputText string, responseBody []byte) int64
}

func selectUsageAdapter(p *model.Provider) UsageAdapter {
	if p == nil || p.UsageMode != "middleware" {
		return nil
	}
	adapter := &ollamaUsageAdapter{}
	if adapter.Match(p) {
		return adapter
	}
	return nil
}

type ollamaUsageAdapter struct{}

func (a *ollamaUsageAdapter) Name() string { return "ollama" }

func (a *ollamaUsageAdapter) Match(p *model.Provider) bool {
	if p == nil {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(p.BaseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if strings.Contains(host, "ollama") {
		return true
	}
	return (host == "localhost" || host == "127.0.0.1") && port == "11434"
}

func (a *ollamaUsageAdapter) EstimateInputTokens(p *model.Provider, requestBody []byte) int64 {
	text := collectRequestText(requestBody)
	if strings.TrimSpace(text) == "" {
		return 0
	}
	if n := ollamaTokenize(p.BaseURL, p.ModelID, text); n > 0 {
		return n
	}
	return roughTokenEstimate(text)
}

func (a *ollamaUsageAdapter) EstimateOutputTokens(p *model.Provider, outputText string, _ []byte) int64 {
	if strings.TrimSpace(outputText) == "" {
		return 0
	}
	if n := ollamaTokenize(p.BaseURL, p.ModelID, outputText); n > 0 {
		return n
	}
	return roughTokenEstimate(outputText)
}

func collectRequestText(requestBody []byte) string {
	var raw map[string]any
	if err := json.Unmarshal(requestBody, &raw); err != nil {
		return ""
	}
	var sb strings.Builder
	var walk func(v any)
	walk = func(v any) {
		switch vv := v.(type) {
		case map[string]any:
			for k, val := range vv {
				// Prefer semantic text fields; skip noisy keys.
				if (k == "text" || k == "content" || k == "system" || k == "thinking") && reflectString(val) != "" {
					sb.WriteString(reflectString(val))
					sb.WriteString("\n")
					continue
				}
				walk(val)
			}
		case []any:
			for _, it := range vv {
				walk(it)
			}
		case string:
			s := strings.TrimSpace(vv)
			if s != "" {
				sb.WriteString(s)
				sb.WriteString("\n")
			}
		}
	}
	walk(raw)
	return sb.String()
}

func reflectString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func ollamaTokenize(baseURL, modelID, text string) int64 {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(modelID) == "" || strings.TrimSpace(text) == "" {
		return 0
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":  modelID,
		"prompt": text,
	})
	url := strings.TrimRight(baseURL, "/") + "/api/tokenize"
	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	b, _ := io.ReadAll(resp.Body)
	var data struct {
		Tokens    []int `json:"tokens"`
		TokenIDs  []int `json:"token_ids"`
		Count     int64 `json:"count"`
		NumTokens int64 `json:"num_tokens"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return 0
	}
	switch {
	case len(data.Tokens) > 0:
		return int64(len(data.Tokens))
	case len(data.TokenIDs) > 0:
		return int64(len(data.TokenIDs))
	case data.NumTokens > 0:
		return data.NumTokens
	case data.Count > 0:
		return data.Count
	default:
		return 0
	}
}

func roughTokenEstimate(text string) int64 {
	// Fallback heuristic for mixed Chinese/English text.
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return 0
	}
	return int64(math.Ceil(float64(len(runes)) / 3.2))
}
