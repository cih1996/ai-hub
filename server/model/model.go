package model

import (
	"net/url"
	"strings"
	"time"
)

// Provider 供应商配置
// Mode 由后端自动判断: "claude-code" 走 CLI, "direct" 走 OpenAI 兼容 API
type Provider struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Mode      string    `json:"mode"`      // "claude-code" | "direct" (auto-detected, not set by user)
	AuthMode  string    `json:"auth_mode"` // "api_key" (default) | "oauth" (Claude subscription)
	BaseURL   string    `json:"base_url"`
	APIKey    string    `json:"api_key"`
	ModelID   string    `json:"model_id"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DetectMode 根据 model_id 自动判断走 Claude Code CLI 还是直连 API
func (p *Provider) DetectMode() string {
	// OAuth subscription mode always uses Claude Code CLI.
	if strings.EqualFold(strings.TrimSpace(p.AuthMode), "oauth") {
		return "claude-code"
	}
	// 包含 claude 关键字的走 Claude Code CLI
	lower := strings.ToLower(p.ModelID)
	if strings.Contains(lower, "claude") {
		return "claude-code"
	}
	// Ollama 的 Anthropic 兼容端点也走 Claude Code CLI
	if isOllamaBaseURL(p.BaseURL) {
		return "claude-code"
	}
	return "direct"
}

func isOllamaBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()

	if strings.Contains(host, "ollama") {
		return true
	}
	// Local Ollama default endpoint
	return (host == "localhost" || host == "127.0.0.1") && port == "11434"
}

// Session 会话
type Session struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	ProviderID      string    `json:"provider_id"`
	ClaudeSessionID string    `json:"claude_session_id"` // UUID for Claude Code CLI --session-id
	WorkDir         string    `json:"work_dir"`          // 工作目录，空 = 系统默认(home)
	GroupName       string    `json:"group_name"`        // 会话分组名称
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Message 消息
type Message struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"session_id"`
	Role      string    `json:"role"` // "user" | "assistant"
	Content   string    `json:"content"`
	Metadata  string    `json:"metadata,omitempty"` // JSON: 执行步骤持久化数据
	CreatedAt time.Time `json:"created_at"`
}

// Trigger 定时触发器
type Trigger struct {
	ID          int64  `json:"id"`
	SessionID   int64  `json:"session_id"`
	Content     string `json:"content"`      // 自然语言指令
	TriggerTime string `json:"trigger_time"` // "2006-01-02 15:04:05" | "15:04:05" | "1h30m"
	MaxFires    int    `json:"max_fires"`    // -1=无限
	Enabled     bool   `json:"enabled"`
	FiredCount  int    `json:"fired_count"`
	Status      string `json:"status"`       // active/fired/failed/completed/disabled
	NextFireAt  string `json:"next_fire_at"` // 下次触发时间
	LastFiredAt string `json:"last_fired_at"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// TokenUsage Token 用量记录
type TokenUsage struct {
	ID                       int64     `json:"id"`
	SessionID                int64     `json:"session_id"`
	MessageID                int64     `json:"message_id"`
	InputTokens              int64     `json:"input_tokens"`
	OutputTokens             int64     `json:"output_tokens"`
	CacheCreationInputTokens int64     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64     `json:"cache_read_input_tokens"`
	CreatedAt                time.Time `json:"created_at"`
}

// TokenUsageStats 用量统计汇总
type TokenUsageStats struct {
	TotalInput         int64 `json:"total_input_tokens"`
	TotalOutput        int64 `json:"total_output_tokens"`
	TotalCacheCreation int64 `json:"total_cache_creation_tokens"`
	TotalCacheRead     int64 `json:"total_cache_read_tokens"`
	Count              int64 `json:"count"`
}

// Channel 通讯频道（IM 网关）
type Channel struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform"`   // "feishu" | "telegram" | "qq"
	SessionID int64     `json:"session_id"` // 绑定的会话 ID
	Config    string    `json:"config"`     // JSON: 平台配置
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
