package core

import (
	"ai-hub/server/model"
	"ai-hub/server/store"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
)

// HookEvent represents an event that can trigger hooks.
type HookEvent struct {
	Type            string // session.created | message.received | message.count | session.error
	SourceSessionID int64
	Content         string // message content or error summary
	MessageCount    int64  // for message.count events
}

// FireHooks checks all enabled hooks for the given event and fires matching ones.
// This should be called asynchronously (go FireHooks(...)) to avoid blocking.
func FireHooks(event HookEvent) {
	hooks, err := store.ListHooksByEvent(event.Type)
	if err != nil {
		log.Printf("[hooks] failed to list hooks for event %s: %v", event.Type, err)
		return
	}

	for _, hook := range hooks {
		if !hook.Enabled {
			continue
		}
		if !matchCondition(hook.Condition, event) {
			continue
		}

		// Build the payload with variable substitution
		payload := expandPayload(hook.Payload, event)

		// Send message to target session via the internal chat/send API
		log.Printf("[hooks] hook #%d fired: event=%s target_session=%d", hook.ID, event.Type, hook.TargetSession)

		go sendHookMessage(hook.TargetSession, payload)

		// Increment fired count
		store.IncrementHookFiredCount(hook.ID)
	}
}

// matchCondition checks if the event matches the hook's condition.
// Condition formats:
//   - "" (empty) → always match
//   - "content_match:pattern1|pattern2" → match if content contains any pattern
//   - "count_gt:N" → match if message count > N
func matchCondition(condition string, event HookEvent) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true
	}

	parts := strings.SplitN(condition, ":", 2)
	if len(parts) != 2 {
		return false
	}

	condType := parts[0]
	condValue := parts[1]

	switch condType {
	case "content_match":
		// Match if content contains any of the pipe-separated patterns
		patterns := strings.Split(condValue, "|")
		for _, p := range patterns {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			// Try as regex first, fall back to simple contains
			re, err := regexp.Compile(p)
			if err != nil {
				// Simple substring match
				if strings.Contains(event.Content, p) {
					return true
				}
			} else {
				if re.MatchString(event.Content) {
					return true
				}
			}
		}
		return false

	case "count_gt":
		threshold, err := strconv.ParseInt(condValue, 10, 64)
		if err != nil {
			return false
		}
		return event.MessageCount > threshold

	default:
		log.Printf("[hooks] unknown condition type: %s", condType)
		return false
	}
}

// expandPayload replaces template variables in the payload string.
// Supported variables:
//   - {source_session_id} → the session that triggered the event
//   - {event_type} → the event type
//   - {content} → the message content or error summary (truncated)
//   - {message_count} → the message count (for message.count events)
func expandPayload(payload string, event HookEvent) string {
	r := strings.NewReplacer(
		"{source_session_id}", fmt.Sprintf("%d", event.SourceSessionID),
		"{event_type}", event.Type,
		"{content}", truncateForPayload(event.Content, 500),
		"{message_count}", fmt.Sprintf("%d", event.MessageCount),
	)
	return r.Replace(payload)
}

func truncateForPayload(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}

// sendHookMessage sends a message to the target session via the internal API.
// This uses store directly instead of HTTP to avoid circular dependencies.
func sendHookMessage(targetSessionID int64, content string) {
	session, err := store.GetSession(targetSessionID)
	if err != nil {
		log.Printf("[hooks] target session %d not found: %v", targetSessionID, err)
		return
	}

	// Save the user message
	msg := &model.Message{
		SessionID: targetSessionID,
		Role:      "user",
		Content:   content,
	}
	if err := store.AddMessage(msg); err != nil {
		log.Printf("[hooks] failed to add message to session %d: %v", targetSessionID, err)
		return
	}

	// If the session has a provider, we could trigger a stream.
	// For now, hooks just deliver the message; the AI will pick it up if the session is active.
	// If the session is not streaming, we need to trigger it.
	if session.ProviderID != "" {
		// Use the HookStreamCallback if set (avoids import cycle with api package)
		if hookStreamCb != nil {
			hookStreamCb(session, content, msg.ID)
		} else {
			log.Printf("[hooks] message delivered to session %d (no stream callback)", targetSessionID)
		}
	}
}

// HookStreamFunc is the callback type for triggering a stream from hooks.
type HookStreamFunc func(session *model.Session, content string, triggerMsgID int64)

var hookStreamCb HookStreamFunc

// SetHookStreamCallback registers the callback for triggering streams from hooks.
// Called from the api package during initialization to break the import cycle.
func SetHookStreamCallback(cb HookStreamFunc) {
	hookStreamCb = cb
}
