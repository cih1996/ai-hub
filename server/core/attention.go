package core

import (
	"encoding/json"
	"strings"
)

// AttentionRulesData stores the JSON structure for attention rules
type AttentionRulesData struct {
	ActivationCustom string `json:"activation_custom"` // 用户自定义激活规则
	ReviewCustom     string `json:"review_custom"`     // 用户自定义审核规则
}

// SystemActivationRule is the built-in activation rule (read-only)
const SystemActivationRule = `当你收到用户请求后，必须先输出一个 JSON 格式的执行计划，格式如下：

{"_attention_trigger": {"action": "review", "plan": "你的执行计划"}}

计划内容要求：
1. 简要复述用户的核心诉求（1-2句话）
2. 列出可能涉及的文件、模块或系统
3. 用编号列表说明执行步骤
4. 指出潜在风险或需要确认的点

重要：在输出计划 JSON 后，等待系统审核通过再执行实际操作。`

// SystemReviewRule is the built-in review rule (read-only)
const SystemReviewRule = `你是注意力审核 AI，负责审核目标会话 AI 的执行计划。

审核依据：
1. 检查计划是否遵循会话规则
2. 检查计划是否遵循团队规则
3. 检查计划是否遵循全局规则
4. 检查是否遗漏相关记忆库信息

审核结果格式：
- 通过：输出 [PASS]
- 拒绝：输出 [REJECT:具体原因]

注意：
- 只输出审核结果，不要执行任何操作
- 拒绝时必须说明具体违反了哪条规则或遗漏了什么信息`

// MaxReviewRetries is the maximum number of review retries before blocking
const MaxReviewRetries = 3

// AttentionTrigger represents the JSON structure AI outputs for review
type AttentionTrigger struct {
	Action string `json:"action"` // "review"
	Plan   string `json:"plan"`   // execution plan
}

// AttentionTriggerWrapper wraps the trigger for JSON detection
type AttentionTriggerWrapper struct {
	AttentionTrigger AttentionTrigger `json:"_attention_trigger"`
}

// ParseAttentionRules parses the JSON attention rules from database
func ParseAttentionRules(rulesJSON string) *AttentionRulesData {
	if strings.TrimSpace(rulesJSON) == "" {
		return &AttentionRulesData{}
	}
	var data AttentionRulesData
	if err := json.Unmarshal([]byte(rulesJSON), &data); err != nil {
		// Legacy format: treat as activation_custom for backward compatibility
		return &AttentionRulesData{ActivationCustom: rulesJSON}
	}
	return &data
}

// SerializeAttentionRules converts AttentionRulesData to JSON string
func SerializeAttentionRules(data *AttentionRulesData) string {
	if data == nil {
		return ""
	}
	b, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(b)
}

// BuildActivationPrompt builds the full activation prompt (system + custom)
func BuildActivationPrompt(customRules string) string {
	parts := []string{SystemActivationRule}
	if strings.TrimSpace(customRules) != "" {
		parts = append(parts, "用户补充规则：\n"+customRules)
	}
	return strings.Join(parts, "\n\n")
}

// BuildReviewPrompt builds the full review prompt (system + custom)
func BuildReviewPrompt(customRules string) string {
	parts := []string{SystemReviewRule}
	if strings.TrimSpace(customRules) != "" {
		parts = append(parts, "用户补充审核规则：\n"+customRules)
	}
	return strings.Join(parts, "\n\n")
}

// DetectAttentionTrigger checks if content contains _attention_trigger JSON
// Returns the trigger if found, nil otherwise
func DetectAttentionTrigger(content string) *AttentionTrigger {
	// Find JSON object containing _attention_trigger
	start := strings.Index(content, `{"_attention_trigger"`)
	if start == -1 {
		return nil
	}

	// Find matching closing brace
	depth := 0
	end := -1
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
		if end != -1 {
			break
		}
	}

	if end == -1 {
		return nil
	}

	jsonStr := content[start:end]
	var wrapper AttentionTriggerWrapper
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		return nil
	}

	if wrapper.AttentionTrigger.Action == "" {
		return nil
	}

	return &wrapper.AttentionTrigger
}

// ParseReviewResult parses the review AI's response
// Returns (passed bool, reason string)
func ParseReviewResult(content string) (bool, string) {
	content = strings.TrimSpace(content)

	// Check for [PASS]
	if strings.Contains(content, "[PASS]") {
		return true, ""
	}

	// Check for [REJECT:reason]
	if idx := strings.Index(content, "[REJECT:"); idx != -1 {
		start := idx + len("[REJECT:")
		end := strings.Index(content[start:], "]")
		if end != -1 {
			reason := strings.TrimSpace(content[start : start+end])
			return false, reason
		}
		// No closing bracket, take rest of line
		endLine := strings.Index(content[start:], "\n")
		if endLine != -1 {
			return false, strings.TrimSpace(content[start : start+endLine])
		}
		return false, strings.TrimSpace(content[start:])
	}

	// Default: treat as rejection with full content as reason
	return false, "审核结果格式不正确: " + content
}
