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

const defaultCompressionSystemPrompt = `你是一个专门负责“对话上下文压缩”的 AI 助手。

你的任务是阅读给定的完整历史记录，输出一份高密度、可直接继续工作的压缩总结。

要求：
1. 保留用户目标、约束、偏好、已完成结果、未完成事项、关键决策。
2. 保留重要文件路径、命令、变量名、接口、数据结构、报错结论。
3. 删除寒暄、重复表述、无价值中间过程。
4. 用中文输出。
5. 输出 Markdown，使用以下结构：
   - 用户目标
   - 关键约束
   - 已完成
   - 当前进展
   - 关键资料
   - 下一步
6. 直接输出压缩结果，不要写前言或解释。`

// BuildIntelligentRecoverySeed calls Claude one-shot to generate a high-quality
// context summary from the full archived conversation logs. Returns ("", err)
// on failure — callers should fall back to buildRecoverySeed.
//
// It uses the one-shot Stream() client (not the persistent pool) so it does not
// pollute or interfere with the existing session process.
func BuildIntelligentRecoverySeed(logs []model.ConversationLog, provider *model.Provider, hubSessionID int64, systemPrompt string) (string, error) {
	if len(logs) == 0 {
		return "", fmt.Errorf("no messages to summarize")
	}

	// Build the conversation text
	var history strings.Builder
	history.WriteString(fmt.Sprintf("以下是完整会话历史归档（共 %d 条记录）：\n\n", len(logs)))
	for _, m := range logs {
		role := "用户"
		if m.Role == "assistant" {
			role = "助手"
		}
		history.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, m.Content))
	}

	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultCompressionSystemPrompt
	}

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
	if provider.AuthMode == "oauth" {
		req.ModelID = ""
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
如需完整历史，请调用：GET /api/v1/sessions/%d/logs
请继续处理上面最后一条用户消息的请求；若存在未完成任务，延续执行。`, summary, hubSessionID)

	return seed, nil
}
