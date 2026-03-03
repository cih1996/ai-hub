package core

import (
	"ai-hub/server/model"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// BuildIntelligentRecoverySeed calls Claude one-shot to generate a high-quality
// context summary from the full message history. Returns ("", err) on failure —
// callers should fall back to buildRecoverySeed.
//
// It uses the one-shot Stream() client (not the persistent pool) so it does not
// pollute or interfere with the existing session process.
func BuildIntelligentRecoverySeed(msgs []model.Message, provider *model.Provider, hubSessionID int64) (string, error) {
	if len(msgs) == 0 {
		return "", fmt.Errorf("no messages to summarize")
	}

	// Build the conversation text
	var history strings.Builder
	history.WriteString(fmt.Sprintf("以下是完整会话历史（共 %d 条消息）：\n\n", len(msgs)))
	for _, m := range msgs {
		role := "用户"
		if m.Role == "assistant" {
			role = "助手"
		}
		history.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, m.Content))
	}

	systemPrompt := `你是一个会话上下文压缩助手。任务：阅读以下完整会话历史，提炼出一份简洁但信息密度高的「上下文恢复摘要」。

要求：
1. 保留用户的核心目标、已完成事项、当前进行中的任务、关键决策和约定
2. 压缩重复内容，去除闲聊、中间过程输出
3. 保留所有重要的文件路径、变量名、命令、代码片段（完整保留，不截断）
4. 输出格式为 Markdown，包含以下章节：【用户目标】【已完成】【进行中】【关键上下文】【下一步】
5. 字数控制在 1000 字以内，优先保证信息完整性
6. 直接输出摘要内容，不加前言或解释`

	query := history.String()

	client := NewClaudeCodeClient()

	// Use a 90-second timeout for the summarization call
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	req := ClaudeCodeRequest{
		Query:        query,
		SystemPrompt: systemPrompt,
		BaseURL:      provider.BaseURL,
		APIKey:       provider.APIKey,
		AuthMode:     provider.AuthMode,
		ProxyURL:     provider.ProxyURL,
		ModelID:      provider.ModelID,
		HubSessionID: hubSessionID,
	}

	var result strings.Builder
	err := client.Stream(ctx, req, func(line string) {
		// Parse stream-json lines to extract text chunks
		var ev map[string]json.RawMessage
		if jsonErr := json.Unmarshal([]byte(line), &ev); jsonErr != nil {
			return
		}
		evType, _ := ev["type"]
		var typeStr string
		if jsonErr := json.Unmarshal(evType, &typeStr); jsonErr != nil {
			return
		}
		// Extract text from assistant message content blocks
		if typeStr == "assistant" {
			var msg struct {
				Message struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			}
			if jsonErr := json.Unmarshal([]byte(line), &msg); jsonErr == nil {
				for _, block := range msg.Message.Content {
					if block.Type == "text" {
						result.WriteString(block.Text)
					}
				}
			}
		}
		// Also handle result type
		if typeStr == "result" {
			var res struct {
				Result string `json:"result"`
			}
			if jsonErr := json.Unmarshal([]byte(line), &res); jsonErr == nil && res.Result != "" {
				if result.Len() == 0 {
					result.WriteString(res.Result)
				}
			}
		}
	})

	if err != nil {
		log.Printf("[compress] intelligent compress failed for session %d: %v", hubSessionID, err)
		return "", err
	}

	summary := strings.TrimSpace(result.String())
	if summary == "" {
		return "", fmt.Errorf("intelligent compress returned empty summary")
	}

	log.Printf("[compress] intelligent compress succeeded for session %d: %d chars", hubSessionID, len(summary))

	seed := fmt.Sprintf(`【上下文恢复】本轮因「上下文自动压缩」进入新会话。以下是 AI 生成的智能压缩摘要，请基于此继续任务。

%s

---
如需完整历史，请调用：GET /api/v1/sessions/%d/messages
请继续处理上面最后一条用户消息的请求；若存在未完成任务，延续执行。`, summary, hubSessionID)

	return seed, nil
}
