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
	return fmt.Sprintf(`你是注意力模式 AI，负责为会话 #%d 提供预处理支持。

## 你的任务

分析用户请求，查询相关信息，生成预处理文本来增强本体会话的执行效果。

### 查询步骤（请按顺序执行）

1. 查近期聊天记录了解上下文：
   ai-hub sessions %d messages --limit 20

2. 查会话规则：
   ai-hub rules get %d

3. 如果会话属于团队「%s」，查团队规则：
   ai-hub rules list --level team
   （然后读取相关规则文件）

4. 根据聊天记录和用户请求，按需查询记忆：
   - 会话级记忆：ai-hub search "关键词" --level session
   - 团队级记忆：ai-hub search "关键词" --level team
   - 全局记忆（慎用）：ai-hub search "关键词" --level global

### 目标会话信息
- 会话 ID：%d
- 团队：%s

### 用户当前请求
%s

### 输出要求

完成查询后，直接输出预处理文本（不需要任何标签包裹），内容包括：
- 发现的需要注意的事项
- 相关规则约束
- 相关记忆信息
- 对本次请求的建议

如果没有特别需要注意的事项，输出：
无特别注意事项，可正常执行。

---

请开始执行查询，然后输出预处理结果。`,
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
	SaveMessageFn            func(sessionID int64, role, content string) error
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
	e.broadcastStatus("注意力模式：正在分析请求...")
	log.Printf("[attention-v3] session %d: Phase 1 - Creating attention AI", parentID)

	attentionSession, err := e.CreateAttentionSessionFn(parentID)
	if err != nil {
		state.Phase = "failed"
		state.Error = err.Error()
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

	// Run attention AI to do preprocessing
	e.broadcastStatus("注意力模式：正在查询相关信息...")
	attentionPrompt := BuildAttentionAIPrompt(parentID, e.ParentSession.GroupName, userMessage)

	preprocessResponse, err := e.RunAttentionStreamFn(attentionSession, attentionPrompt, parentID)
	if err != nil {
		state.Phase = "failed"
		state.Error = err.Error()
		log.Printf("[attention-v3] session %d: attention AI failed: %v", parentID, err)
		return fmt.Errorf("attention AI failed: %w", err)
	}
	state.PreprocessedText = preprocessResponse
	log.Printf("[attention-v3] session %d: preprocessing complete, len=%d", parentID, len(preprocessResponse))
	e.broadcastStatusWithDetail("注意力模式：预处理完成", preprocessResponse)

	// ========== Phase 2: Send to parent session with wrapped context ==========
	state.Phase = "executing"
	e.broadcastStatus("注意力模式：正在执行...")
	log.Printf("[attention-v3] session %d: Phase 2 - Executing with parent session", parentID)

	// Build enhanced message: wrapped preprocessing + user message
	wrappedContext := WrapWithAttentionTag(preprocessResponse)
	enhancedMessage := wrappedContext + userMessage

	// Save user message (original, without attention context)
	if e.SaveMessageFn != nil {
		if err := e.SaveMessageFn(parentID, "user", userMessage); err != nil {
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
	e.broadcastStatusWithDetail(message, "")
}

// broadcastStatusWithDetail sends a status message with optional detail
func (e *AttentionV3Executor) broadcastStatusWithDetail(message, detail string) {
	if e.BroadcastFn != nil {
		e.BroadcastFn(e.ParentSession.ID, "attention_status", message, detail)
	}
	if detail != "" {
		log.Printf("[attention-v3] session %d: %s (detail len=%d)", e.ParentSession.ID, message, len(detail))
	} else {
		log.Printf("[attention-v3] session %d: %s", e.ParentSession.ID, message)
	}
}
