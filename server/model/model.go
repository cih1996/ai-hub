package model

import (
	"net/url"
	"strings"
	"time"
)

// Provider 供应商配置
// Mode 固定为 "claude-code"，统一走 Claude Code CLI。
type Provider struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Mode      string    `json:"mode"`       // "claude-code" (fixed)
	AuthMode  string    `json:"auth_mode"`  // "api_key" (default) | "oauth" (Claude subscription)
	UsageMode string    `json:"usage_mode"` // "upstream" (default) | "middleware"
	ProxyURL  string    `json:"proxy_url"`  // optional: force HTTP(S) proxy for Claude CLI subprocess
	BaseURL   string    `json:"base_url"`
	APIKey    string    `json:"api_key"`
	ModelID   string    `json:"model_id"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DetectMode 固定返回 Claude Code 模式。
func (p *Provider) DetectMode() string {
	return "claude-code"
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
	ID                int64     `json:"id"`
	Title             string    `json:"title"`
	Icon              string    `json:"icon"`                 // 图标文件名，如 avatar1.svg
	ProviderID        string    `json:"provider_id"`
	ClaudeSessionID   string    `json:"claude_session_id"`    // UUID for Claude Code CLI --session-id
	WorkDir           string    `json:"work_dir"`             // 工作目录，空 = 系统默认(home)
	GroupName         string    `json:"group_name"`           // 会话分组名称
	LastCompressMsgID int64     `json:"last_compress_msg_id"` // 上次压缩时最新消息 ID，用于增量统计
	AttentionEnabled  bool      `json:"attention_enabled"`    // 注意力系统开关
	AttentionRules    string    `json:"attention_rules"`      // 注意力规则（会话级别）
	// Shadow session fields (for attention mode)
	IsShadow       bool  `json:"is_shadow"`        // 是否为影子会话
	ParentID       int64 `json:"parent_id"`        // 本体会话 ID（影子会话专用）
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Message 消息
type Message struct {
	ID               int64     `json:"id"`
	SessionID        int64     `json:"session_id"`
	Role             string    `json:"role"` // "user" | "assistant"
	Content          string    `json:"content"`
	Metadata         string    `json:"metadata,omitempty"`          // JSON: 执行步骤持久化数据
	AttentionContext string    `json:"attention_context,omitempty"` // 注意力模式预处理内容
	CreatedAt        time.Time `json:"created_at"`
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

// CompressSettings 会话自动压缩配置
type CompressSettings struct {
	AutoEnabled bool   `json:"auto_enabled"` // 是否启用自动压缩
	Threshold   int    `json:"threshold"`    // 触发阈值（累计 input tokens 绝对值，建议 80000）
	Mode        string `json:"mode"`         // "auto"（智能优先，降级简单）| "intelligent"（仅智能）| "simple"（仅简单截取）
	MinTurns    int    `json:"min_turns"`    // 最小对话轮数阈值（user 消息数），默认 10；token 与轮数同时满足才触发压缩
}

// AIError AI 错误追踪记录
type AIError struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"session_id"`
	MessageID int64     `json:"message_id"`
	Level     string    `json:"level"`   // "error" | "warning"
	Summary   string    `json:"summary"` // 错误摘要
	CreatedAt time.Time `json:"created_at"`
}

// ErrorStat 按会话聚合的错误统计
type ErrorStat struct {
	SessionID    int64  `json:"session_id"`
	SessionTitle string `json:"session_title"`
	ErrorCount   int    `json:"error_count"`
	WarningCount int    `json:"warning_count"`
}

// ErrorCount 单会话错误/警告计数
type ErrorCount struct {
	ErrorCount   int `json:"error_count"`
	WarningCount int `json:"warning_count"`
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

// Service 托管服务
type Service struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Command   string    `json:"command"`
	WorkDir   string    `json:"work_dir"`
	Port      int       `json:"port"`
	LogPath   string    `json:"log_path"`
	PID       int       `json:"pid"`
	Status    string    `json:"status"`     // stopped / running / dead
	AutoStart bool      `json:"auto_start"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Group 团队分组
type Group struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"` // 图标文件名，如 avatar1.svg
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}
