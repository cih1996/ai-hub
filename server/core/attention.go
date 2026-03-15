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
