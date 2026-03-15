package core

import (
	"ai-hub/server/model"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// AttentionV2State tracks the state of an attention mode execution
type AttentionV2State struct {
	ParentSessionID int64
	ShadowSessionID int64
	Phase           string // "preprocessing" | "planning" | "reviewing" | "executing" | "done" | "failed"
	ReviewAttempts  int
	UserMessage     string
	ExtractedContext string
	Plan            string
	FinalResult     string
	Error           string
	CreatedAt       time.Time
}

// AttentionV2Manager manages attention mode v2 executions
type AttentionV2Manager struct {
	mu     sync.RWMutex
	states map[int64]*AttentionV2State // keyed by parent session ID
}

var AttentionMgr = &AttentionV2Manager{
	states: make(map[int64]*AttentionV2State),
}

// GetState returns the current attention state for a session
func (m *AttentionV2Manager) GetState(parentSessionID int64) *AttentionV2State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[parentSessionID]
}

// SetState sets the attention state for a session
func (m *AttentionV2Manager) SetState(parentSessionID int64, state *AttentionV2State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[parentSessionID] = state
}

// ClearState removes the attention state for a session
func (m *AttentionV2Manager) ClearState(parentSessionID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, parentSessionID)
}

// AttentionPreprocessor extracts relevant context from memory and rules
type AttentionPreprocessor struct {
	Session  *model.Session
	Provider *model.Provider
}

// ExtractRelevantContext uses AI to extract relevant memory and rules for the user's request
func (p *AttentionPreprocessor) ExtractRelevantContext(ctx context.Context, userMessage string) (string, error) {
	log.Printf("[attention-v2] session %d: extracting relevant context for message", p.Session.ID)

	// Gather all available context
	var contextParts []string

	// 1. Session rules
	if sessionRules := ReadSessionRulesFile(p.Session.ID); sessionRules != "" {
		contextParts = append(contextParts, "【会话规则】\n"+sessionRules)
	}

	// 2. Team rules
	if p.Session.GroupName != "" {
		if teamRules := BuildTeamRulesWithVars(p.Session.GroupName, nil); teamRules != "" {
			contextParts = append(contextParts, "【团队规则】\n"+teamRules)
		}
	}

	// 3. Global rules (truncated)
	if globalRules := BuildSystemPromptWithVars(nil); globalRules != "" {
		if len(globalRules) > 2000 {
			globalRules = globalRules[:2000] + "...(已截断)"
		}
		contextParts = append(contextParts, "【全局规则】\n"+globalRules)
	}

	// 4. TODO: Search memory for relevant entries
	// This would use vector search to find relevant memory entries

	if len(contextParts) == 0 {
		return "", nil // No context to extract
	}

	// Build extraction prompt
	extractPrompt := fmt.Sprintf(`你是注意力预处理 AI。请分析用户的请求，从以下规则和记忆中提取与本次请求相关的重要信息。

用户请求：
%s

可用上下文：
%s

请输出：
1. 与本次请求相关的规则要点（如有）
2. 需要注意的约束或限制
3. 相关的历史记忆或上下文（如有）

只输出相关内容，不要输出无关信息。如果没有相关内容，输出"无特别注意事项"。`,
		userMessage, strings.Join(contextParts, "\n\n---\n\n"))

	// Call AI to extract (using direct API to avoid nested CLI issue)
	result, err := CallAnthropicAPI(ctx, p.Provider, extractPrompt, "你是注意力预处理 AI，负责提取与用户请求相关的规则和记忆。只输出相关内容，不要执行任何操作。")
	if err != nil {
		return "", fmt.Errorf("context extraction failed: %w", err)
	}

	return strings.TrimSpace(result), nil
}

// ReadSessionRulesFile reads session rules from file (helper function)
func ReadSessionRulesFile(sessionID int64) string {
	// This should be implemented to read from the rules file
	// For now, return empty string - will be implemented when integrating
	return ""
}

// AttentionReviewer reviews execution plans
type AttentionReviewer struct {
	Session  *model.Session
	Provider *model.Provider
}

// ReviewPlan reviews an execution plan and returns (passed, reason)
func (r *AttentionReviewer) ReviewPlan(ctx context.Context, plan string, extractedContext string) (bool, string, error) {
	log.Printf("[attention-v2] session %d: reviewing plan", r.Session.ID)

	// Build review context
	rulesData := ParseAttentionRules(r.Session.AttentionRules)
	reviewPrompt := BuildReviewPrompt(rulesData.ReviewCustom)

	// Add extracted context if available
	if extractedContext != "" {
		reviewPrompt += "\n\n注意力预处理提取的相关上下文：\n" + extractedContext
	}

	reviewQuery := reviewPrompt + "\n\n---\n\n待审核的执行计划：\n" + plan

	// Call AI to review
	result, err := CallAnthropicAPI(ctx, r.Provider, reviewQuery, "你是注意力审核 AI。只输出审核结果，格式为 [PASS] 或 [REJECT:原因]。不要调用任何工具，不要执行任何操作。")
	if err != nil {
		return false, "", fmt.Errorf("review failed: %w", err)
	}

	passed, reason := ParseReviewResult(result)
	return passed, reason, nil
}

// CallAnthropicAPI is a placeholder - will be implemented to call the actual API
// This should use the same logic as callAnthropicMessagesAPI in chat.go
func CallAnthropicAPI(ctx context.Context, provider *model.Provider, userMessage, systemPrompt string) (string, error) {
	// This will be implemented to call the Anthropic API directly
	// For now, return an error indicating it needs implementation
	return "", fmt.Errorf("CallAnthropicAPI not yet implemented - use callAnthropicMessagesAPI from chat.go")
}
