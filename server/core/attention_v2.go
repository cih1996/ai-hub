package core

import (
	"ai-hub/server/model"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AttentionV2State tracks the state of an attention mode execution
type AttentionV2State struct {
	ParentSessionID  int64
	ShadowSessionID  int64
	Phase            string // "preprocessing" | "planning" | "reviewing" | "executing" | "done" | "failed"
	ReviewAttempts   int
	UserMessage      string
	ExtractedContext string
	Plan             string
	FinalResult      string
	Error            string
	CreatedAt        time.Time
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

// PreprocessingSystemPrompt is the system prompt for the preprocessing AI
const PreprocessingSystemPrompt = `你是注意力预处理 AI，负责从规则和记忆中提取与用户请求相关的重要信息。

你的任务：
1. 分析用户的请求意图
2. 从提供的规则和记忆中找出相关内容
3. 提取需要注意的约束、限制或上下文

输出格式：
- 只输出与本次请求相关的内容
- 使用简洁的要点形式
- 如果没有相关内容，输出"无特别注意事项"

注意：不要执行任何操作，只做信息提取。`

// ExtractRelevantContext uses AI to extract relevant memory and rules for the user's request
func (p *AttentionPreprocessor) ExtractRelevantContext(ctx context.Context, userMessage string) (string, error) {
	log.Printf("[attention-v2] session %d: extracting relevant context", p.Session.ID)

	// Gather all available context
	var contextParts []string

	// 1. Session rules
	if sessionRules := ReadSessionRulesFromFile(p.Session.ID); sessionRules != "" {
		contextParts = append(contextParts, "【会话规则】\n"+sessionRules)
	}

	// 2. Team rules
	if p.Session.GroupName != "" {
		if teamRules := BuildTeamRulesWithVars(p.Session.GroupName, nil); teamRules != "" {
			// Truncate if too long
			if len(teamRules) > 3000 {
				teamRules = teamRules[:3000] + "...(已截断)"
			}
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

	// 4. Search memory for relevant entries (using vector search)
	if memoryContext := p.searchRelevantMemory(userMessage); memoryContext != "" {
		contextParts = append(contextParts, "【相关记忆】\n"+memoryContext)
	}

	if len(contextParts) == 0 {
		log.Printf("[attention-v2] session %d: no context available", p.Session.ID)
		return "无特别注意事项", nil
	}

	// Build extraction prompt
	extractPrompt := fmt.Sprintf(`请分析以下用户请求，从规则和记忆中提取相关的注意事项。

用户请求：
%s

可用上下文：
%s

请提取与本次请求相关的重要信息。`, userMessage, strings.Join(contextParts, "\n\n---\n\n"))

	// Call AI to extract
	result, err := CallAnthropicMessagesAPI(ctx, p.Provider, extractPrompt, PreprocessingSystemPrompt, 2048)
	if err != nil {
		return "", fmt.Errorf("context extraction failed: %w", err)
	}

	extracted := strings.TrimSpace(result)
	log.Printf("[attention-v2] session %d: extracted context length=%d", p.Session.ID, len(extracted))
	return extracted, nil
}

// searchRelevantMemory searches for relevant memory entries using vector search
func (p *AttentionPreprocessor) searchRelevantMemory(query string) string {
	// TODO: Implement vector search for memory
	// For now, return empty string
	// This would call the vector engine to find relevant memory entries
	return ""
}

// ReadSessionRulesFromFile reads session rules from the rules file
func ReadSessionRulesFromFile(sessionID int64) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	rulesPath := filepath.Join(homeDir, ".ai-hub", "rules", "sessions", fmt.Sprintf("%d.md", sessionID))
	content, err := os.ReadFile(rulesPath)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(content))
}

// AttentionReviewer reviews execution plans
type AttentionReviewer struct {
	Session  *model.Session
	Provider *model.Provider
}

// ReviewerSystemPrompt is the system prompt for the review AI
const ReviewerSystemPrompt = `你是注意力审核 AI，负责审核目标会话 AI 的执行计划。

审核标准：
1. 计划是否遵循会话规则
2. 计划是否遵循团队规则
3. 计划是否遵循全局规则
4. 是否遗漏相关记忆库信息
5. 计划是否合理、安全

审核结果格式：
- 通过：输出 [PASS]
- 拒绝：输出 [REJECT:具体原因]

注意：
- 只输出审核结果，不要执行任何操作
- 拒绝时必须说明具体违反了哪条规则或遗漏了什么信息`

// ReviewPlan reviews an execution plan and returns (passed, reason, error)
func (r *AttentionReviewer) ReviewPlan(ctx context.Context, plan string, extractedContext string) (bool, string, error) {
	log.Printf("[attention-v2] session %d: reviewing plan", r.Session.ID)

	// Build review context
	rulesData := ParseAttentionRules(r.Session.AttentionRules)

	var reviewParts []string

	// Add custom review rules if any
	if rulesData.ReviewCustom != "" {
		reviewParts = append(reviewParts, "【用户自定义审核规则】\n"+rulesData.ReviewCustom)
	}

	// Add extracted context if available
	if extractedContext != "" && extractedContext != "无特别注意事项" {
		reviewParts = append(reviewParts, "【预处理提取的相关上下文】\n"+extractedContext)
	}

	// Build review query
	reviewQuery := "请审核以下执行计划：\n\n" + plan
	if len(reviewParts) > 0 {
		reviewQuery = strings.Join(reviewParts, "\n\n---\n\n") + "\n\n---\n\n" + reviewQuery
	}

	// Call AI to review
	result, err := CallAnthropicMessagesAPI(ctx, r.Provider, reviewQuery, ReviewerSystemPrompt, 1024)
	if err != nil {
		return false, "", fmt.Errorf("review failed: %w", err)
	}

	passed, reason := ParseReviewResult(result)
	log.Printf("[attention-v2] session %d: review result passed=%v reason=%s", r.Session.ID, passed, reason)
	return passed, reason, nil
}

// AttentionV2Executor orchestrates the full attention mode v2 flow
type AttentionV2Executor struct {
	ParentSession *model.Session
	Provider      *model.Provider
	// Callbacks for integration with chat.go
	BroadcastFn       func(sessionID int64, msgType, content string)
	RunShadowStreamFn func(shadowSession *model.Session, query string, broadcastAsID int64) (string, string, error)
}

// MaxReviewAttempts is the maximum number of review attempts before failing
const MaxReviewAttempts = 3

// Execute runs the full attention mode v2 flow
// This is the main entry point for attention mode v2
func (e *AttentionV2Executor) Execute(ctx context.Context, userMessage string) error {
	parentID := e.ParentSession.ID
	log.Printf("[attention-v2] session %d: starting execution", parentID)

	// Initialize state
	state := &AttentionV2State{
		ParentSessionID: parentID,
		Phase:           "preprocessing",
		UserMessage:     userMessage,
		CreatedAt:       time.Now(),
	}
	AttentionMgr.SetState(parentID, state)

	// Phase 1: Preprocessing - Extract relevant context
	e.broadcastStatus("注意力模式：正在分析请求...")
	preprocessor := &AttentionPreprocessor{
		Session:  e.ParentSession,
		Provider: e.Provider,
	}
	extractedContext, err := preprocessor.ExtractRelevantContext(ctx, userMessage)
	if err != nil {
		state.Phase = "failed"
		state.Error = err.Error()
		e.broadcastStatus("注意力模式：预处理失败 - " + err.Error())
		AttentionMgr.ClearState(parentID)
		return err
	}
	state.ExtractedContext = extractedContext
	log.Printf("[attention-v2] session %d: preprocessing complete", parentID)

	// Phase 2: Create shadow session
	state.Phase = "planning"
	e.broadcastStatus("注意力模式：正在创建执行环境...")

	// Note: Shadow session creation and execution will be handled by chat.go
	// through the RunShadowStreamFn callback, which has access to store functions

	// For now, store the extracted context in state for later use
	state.Phase = "ready"
	log.Printf("[attention-v2] session %d: ready for shadow execution", parentID)

	return nil
}

// ExecuteWithShadow runs the full flow including shadow session execution
// This should be called from chat.go after setting up the callbacks
// Returns (finalResult, error)
func (e *AttentionV2Executor) ExecuteWithShadow(ctx context.Context, userMessage string, createShadowFn func() (*model.Session, error), deleteShadowFn func(int64) error, copyMessagesFn func(parentID, shadowID int64, limit int) error) (string, error) {
	parentID := e.ParentSession.ID
	fmt.Printf("[attention-v2] session %d: ExecuteWithShadow started\n", parentID)

	// Initialize state
	state := &AttentionV2State{
		ParentSessionID: parentID,
		Phase:           "preprocessing",
		UserMessage:     userMessage,
		CreatedAt:       time.Now(),
	}
	AttentionMgr.SetState(parentID, state)
	defer AttentionMgr.ClearState(parentID)

	// ========== Phase 1: Preprocessing - Extract relevant context ==========
	e.broadcastStatus("注意力模式：正在提取相关规则和记忆...")
	fmt.Printf("[attention-v2] session %d: Phase 1 - Preprocessing\n", parentID)

	preprocessor := &AttentionPreprocessor{
		Session:  e.ParentSession,
		Provider: e.Provider,
	}
	extractedContext, err := preprocessor.ExtractRelevantContext(ctx, userMessage)
	if err != nil {
		// Preprocessing failed, but we can continue with empty context
		fmt.Printf("[attention-v2] session %d: preprocessing failed: %v, continuing with empty context\n", parentID, err)
		extractedContext = ""
	}
	state.ExtractedContext = extractedContext
	fmt.Printf("[attention-v2] session %d: extracted context length=%d\n", parentID, len(extractedContext))

	// ========== Phase 2: Create shadow session ==========
	state.Phase = "planning"
	e.broadcastStatus("注意力模式：正在创建影子会话...")
	fmt.Printf("[attention-v2] session %d: Phase 2 - Creating shadow session\n", parentID)

	shadowSession, err := createShadowFn()
	if err != nil {
		state.Phase = "failed"
		state.Error = err.Error()
		fmt.Printf("[attention-v2] session %d: create shadow failed: %v\n", parentID, err)
		return "", fmt.Errorf("create shadow session failed: %w", err)
	}
	fmt.Printf("[attention-v2] session %d: shadow session created: %d\n", parentID, shadowSession.ID)
	state.ShadowSessionID = shadowSession.ID

	// Ensure cleanup on exit
	defer func() {
		fmt.Printf("[attention-v2] session %d: cleaning up shadow session %d\n", parentID, shadowSession.ID)
		if err := deleteShadowFn(shadowSession.ID); err != nil {
			fmt.Printf("[attention-v2] session %d: failed to delete shadow session: %v\n", parentID, err)
		}
	}()

	// Copy recent messages to shadow session for context
	if err := copyMessagesFn(parentID, shadowSession.ID, 20); err != nil {
		fmt.Printf("[attention-v2] session %d: failed to copy messages: %v\n", parentID, err)
	}

	// ========== Phase 3: Planning - Send to shadow session with context ==========
	e.broadcastStatus("注意力模式：正在生成执行计划...")
	fmt.Printf("[attention-v2] session %d: Phase 3 - Planning\n", parentID)

	// Build planning prompt: extracted context + user message
	planningPrompt := e.buildPlanningPromptV2(userMessage, extractedContext)

	if e.RunShadowStreamFn == nil {
		return "", fmt.Errorf("RunShadowStreamFn not configured")
	}

	// Run shadow session to get the plan
	planResponse, _, err := e.RunShadowStreamFn(shadowSession, planningPrompt, parentID)
	if err != nil {
		state.Phase = "failed"
		state.Error = err.Error()
		fmt.Printf("[attention-v2] session %d: planning failed: %v\n", parentID, err)
		return "", fmt.Errorf("planning failed: %w", err)
	}
	state.Plan = planResponse
	fmt.Printf("[attention-v2] session %d: plan generated, length=%d\n", parentID, len(planResponse))

	// ========== Phase 4: Review loop ==========
	state.Phase = "reviewing"
	fmt.Printf("[attention-v2] session %d: Phase 4 - Reviewing\n", parentID)

	reviewer := &AttentionReviewer{
		Session:  e.ParentSession,
		Provider: e.Provider,
	}

	for attempt := 1; attempt <= MaxReviewAttempts; attempt++ {
		state.ReviewAttempts = attempt
		e.broadcastStatus(fmt.Sprintf("注意力模式：正在审核计划（第 %d/%d 次）...", attempt, MaxReviewAttempts))
		fmt.Printf("[attention-v2] session %d: review attempt %d/%d\n", parentID, attempt, MaxReviewAttempts)

		passed, reason, err := reviewer.ReviewPlan(ctx, state.Plan, extractedContext)
		if err != nil {
			fmt.Printf("[attention-v2] session %d: review error: %v\n", parentID, err)
			// On error, continue to next attempt
			continue
		}

		if passed {
			fmt.Printf("[attention-v2] session %d: review PASSED\n", parentID)
			break
		}

		fmt.Printf("[attention-v2] session %d: review REJECTED: %s\n", parentID, reason)

		if attempt >= MaxReviewAttempts {
			state.Phase = "failed"
			state.Error = "审核多次未通过: " + reason
			e.broadcastStatus("注意力模式：审核未通过，请人工介入")
			return "", fmt.Errorf("review failed after %d attempts: %s", MaxReviewAttempts, reason)
		}

		// Send rejection feedback to shadow session for revision
		e.broadcastStatus("注意力模式：审核未通过，正在修正...")
		revisionPrompt := fmt.Sprintf("你的计划审核未通过，原因：%s\n\n请根据反馈修正你的计划并重新输出。", reason)

		revisedResponse, _, err := e.RunShadowStreamFn(shadowSession, revisionPrompt, parentID)
		if err != nil {
			fmt.Printf("[attention-v2] session %d: revision failed: %v\n", parentID, err)
			continue
		}
		state.Plan = revisedResponse
		fmt.Printf("[attention-v2] session %d: plan revised, length=%d\n", parentID, len(revisedResponse))
	}

	// ========== Phase 5: Execute the approved plan ==========
	state.Phase = "executing"
	e.broadcastStatus("注意力模式：审核通过，正在执行...")
	fmt.Printf("[attention-v2] session %d: Phase 5 - Executing\n", parentID)

	executePrompt := "审核已通过，请执行你的计划。"
	finalResponse, _, err := e.RunShadowStreamFn(shadowSession, executePrompt, parentID)
	if err != nil {
		state.Phase = "failed"
		state.Error = err.Error()
		fmt.Printf("[attention-v2] session %d: execution failed: %v\n", parentID, err)
		return "", fmt.Errorf("execution failed: %w", err)
	}

	state.FinalResult = finalResponse
	state.Phase = "done"
	fmt.Printf("[attention-v2] session %d: execution complete, result_len=%d\n", parentID, len(finalResponse))

	return finalResponse, nil
}

// buildPlanningPromptV2 builds the prompt for planning phase without fixed format requirements
func (e *AttentionV2Executor) buildPlanningPromptV2(userMessage, extractedContext string) string {
	var parts []string

	// Add extracted context if available
	if extractedContext != "" {
		parts = append(parts, "【相关规则和记忆】\n"+extractedContext)
	}

	// Add user's custom activation rules if any
	rulesData := ParseAttentionRules(e.ParentSession.AttentionRules)
	if rulesData.ActivationCustom != "" {
		parts = append(parts, "【用户自定义规则】\n"+rulesData.ActivationCustom)
	}

	// Add user message
	parts = append(parts, "【用户请求】\n"+userMessage)

	// Add instruction to output plan
	parts = append(parts, "请先分析用户请求，输出你的执行计划，然后等待审核通过后再执行。")

	return strings.Join(parts, "\n\n")
}

// broadcastStatus sends a status message to the parent session
func (e *AttentionV2Executor) broadcastStatus(message string) {
	if e.BroadcastFn != nil {
		e.BroadcastFn(e.ParentSession.ID, "attention_status", message)
	}
	log.Printf("[attention-v2] session %d: %s", e.ParentSession.ID, message)
}
