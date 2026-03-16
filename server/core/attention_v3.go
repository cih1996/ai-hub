package core

import (
	"ai-hub/server/model"
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// AttentionContextTag is the tag used to wrap attention context in messages
const AttentionContextTag = "attention-context"

// AttentionV3State tracks the state of an attention mode execution
type AttentionV3State struct {
	ParentSessionID    int64
	AttentionSessionID int64  // 注意力 AI 的会话
	Phase              string // "preprocessing" | "executing" | "done" | "failed"
	UserMessage        string
	PreprocessedText   string // 注意力 AI 生成的预处理文本
	FinalResult        string
	Error              string
	CreatedAt          time.Time
}

// AttentionV3Manager manages attention mode v3 executions
type AttentionV3Manager struct {
	mu     sync.RWMutex
	states map[int64]*AttentionV3State
}

var AttentionV3Mgr = &AttentionV3Manager{
	states: make(map[int64]*AttentionV3State),
}

func (m *AttentionV3Manager) GetState(parentSessionID int64) *AttentionV3State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[parentSessionID]
}

func (m *AttentionV3Manager) SetState(parentSessionID int64, state *AttentionV3State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[parentSessionID] = state
}

func (m *AttentionV3Manager) ClearState(parentSessionID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, parentSessionID)
}

// BuildAttentionAIPrompt builds the prompt for the attention AI
func BuildAttentionAIPrompt(targetSessionID int64, targetGroupName string, userMessage string) string {
	return fmt.Sprintf(`你是注意力模式预处理 AI。你的任务是为会话 #%d 生成简洁的预处理提示。

## 执行步骤

1. 查近期聊天记录：ai-hub sessions %d messages --limit 10
2. 查会话规则：ai-hub rules get %d
3. 如有团队「%s」，查团队规则：ai-hub rules list --level team

## 目标会话
- ID：%d
- 团队：%s

## 用户请求
%s

## 输出要求

根据查询结果，输出简洁的预处理提示（3-5 行），包括：
- 需要遵守的关键规则
- 需要注意的事项
- 相关上下文提醒

如果没有特别注意事项，只输出：无特别注意事项。

注意：只输出最终的预处理提示，不要输出查询过程。`,
		targetSessionID, targetSessionID, targetSessionID, targetGroupName, targetSessionID, targetGroupName, userMessage)
}

// WrapWithAttentionTag wraps content with attention context tag
func WrapWithAttentionTag(content string) string {
	return fmt.Sprintf("<%s>\n%s\n</%s>\n\n", AttentionContextTag, content, AttentionContextTag)
}

// AttentionV3Executor orchestrates the simplified attention mode flow
type AttentionV3Executor struct {
	ParentSession *model.Session
	Provider      *model.Provider

	// Callbacks
	BroadcastFn              func(sessionID int64, msgType, content, detail string)
	CreateAttentionSessionFn func(parentID int64) (*model.Session, error)
	DeleteAttentionSessionFn func(sessionID int64) error
	RunAttentionStreamFn     func(attentionSession *model.Session, query string, broadcastAsID int64) (string, error)
	RunParentStreamFn        func(parentSession *model.Session, query string) error
	SaveMessageFn            func(sessionID int64, role, content, attentionContext string) error
}

// Execute runs the simplified attention mode flow
// 1. Create attention AI session, run preprocessing
// 2. Wrap preprocessing result with tag
// 3. Send to parent session with wrapped context
func (e *AttentionV3Executor) Execute(ctx context.Context, userMessage string) error {
	parentID := e.ParentSession.ID
	log.Printf("[attention-v3] session %d: starting execution", parentID)

	// Initialize state
	state := &AttentionV3State{
		ParentSessionID: parentID,
		Phase:           "preprocessing",
		UserMessage:     userMessage,
		CreatedAt:       time.Now(),
	}
	AttentionV3Mgr.SetState(parentID, state)
	defer AttentionV3Mgr.ClearState(parentID)

	// ========== Phase 1: Create Attention AI and do preprocessing ==========
	e.broadcastStatus("注意力模式：正在预处理...")
	log.Printf("[attention-v3] session %d: Phase 1 - Creating attention AI", parentID)

	attentionSession, err := e.CreateAttentionSessionFn(parentID)
	if err != nil {
		state.Phase = "failed"
		state.Error = err.Error()
		e.broadcastClear() // Clear status on failure
		log.Printf("[attention-v3] session %d: create attention session failed: %v", parentID, err)
		return fmt.Errorf("create attention session failed: %w", err)
	}
	state.AttentionSessionID = attentionSession.ID
	log.Printf("[attention-v3] session %d: attention session created: %d", parentID, attentionSession.ID)

	// Cleanup attention session on exit
	defer func() {
		log.Printf("[attention-v3] session %d: cleaning up attention session %d", parentID, attentionSession.ID)
		e.DeleteAttentionSessionFn(attentionSession.ID)
	}()

	// Run attention AI to do preprocessing (silently)
	attentionPrompt := BuildAttentionAIPrompt(parentID, e.ParentSession.GroupName, userMessage)

	preprocessResponse, err := e.RunAttentionStreamFn(attentionSession, attentionPrompt, parentID)
	if err != nil {
		state.Phase = "failed"
		state.Error = err.Error()
		e.broadcastClear() // Clear status on failure
		log.Printf("[attention-v3] session %d: attention AI failed: %v", parentID, err)
		return fmt.Errorf("attention AI failed: %w", err)
	}
	state.PreprocessedText = preprocessResponse
	log.Printf("[attention-v3] session %d: preprocessing complete, len=%d", parentID, len(preprocessResponse))

	// ========== Phase 2: Send to parent session with wrapped context ==========
	state.Phase = "executing"
	e.broadcastClear() // Clear attention status before parent execution
	log.Printf("[attention-v3] session %d: Phase 2 - Executing with parent session", parentID)

	// Build enhanced message: wrapped preprocessing + user message
	wrappedContext := WrapWithAttentionTag(preprocessResponse)
	enhancedMessage := wrappedContext + userMessage

	// Save user message (original, without attention context in content, but with attention_context field)
	if e.SaveMessageFn != nil {
		if err := e.SaveMessageFn(parentID, "user", userMessage, preprocessResponse); err != nil {
			log.Printf("[attention-v3] session %d: failed to save user message: %v", parentID, err)
		}
	}

	// Run parent session with enhanced message
	err = e.RunParentStreamFn(e.ParentSession, enhancedMessage)
	if err != nil {
		state.Phase = "failed"
		state.Error = err.Error()
		log.Printf("[attention-v3] session %d: parent execution failed: %v", parentID, err)
		return fmt.Errorf("parent execution failed: %w", err)
	}

	state.Phase = "done"
	log.Printf("[attention-v3] session %d: execution complete", parentID)
	return nil
}

// broadcastStatus sends a status message
func (e *AttentionV3Executor) broadcastStatus(message string) {
	if e.BroadcastFn != nil {
		e.BroadcastFn(e.ParentSession.ID, "attention_status", message, "")
	}
	log.Printf("[attention-v3] session %d: %s", e.ParentSession.ID, message)
}

// broadcastClear clears the attention status
func (e *AttentionV3Executor) broadcastClear() {
	if e.BroadcastFn != nil {
		e.BroadcastFn(e.ParentSession.ID, "attention_clear", "", "")
	}
	log.Printf("[attention-v3] session %d: status cleared", e.ParentSession.ID)
}
