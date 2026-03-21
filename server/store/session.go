package store

import (
	"ai-hub/server/model"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func CreateSession(s *model.Session) error {
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	s.ClaudeSessionID = uuid.New().String()
	if s.Title == "" {
		s.Title = "New Chat"
	}
	result, err := DB.Exec(
		`INSERT INTO sessions (title, provider_id, claude_session_id, work_dir, group_name, is_shadow, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Title, s.ProviderID, s.ClaudeSessionID, s.WorkDir, s.GroupName, s.IsShadow, s.ParentID, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id
	return nil
}

func ListSessions() ([]model.Session, error) {
	// Exclude shadow sessions from normal listing
	rows, err := DB.Query(`SELECT id, title, icon, provider_id, claude_session_id, work_dir, group_name, last_compress_msg_id, attention_enabled, attention_rules, is_shadow, parent_id, health_score, health_updated_at, correction_count, drift_count, auto_reset_threshold, created_at, updated_at FROM sessions WHERE is_shadow = 0 ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Session
	for rows.Next() {
		var s model.Session
		if err := rows.Scan(&s.ID, &s.Title, &s.Icon, &s.ProviderID, &s.ClaudeSessionID, &s.WorkDir, &s.GroupName, &s.LastCompressMsgID, &s.AttentionEnabled, &s.AttentionRules, &s.IsShadow, &s.ParentID, &s.HealthScore, &s.HealthUpdatedAt, &s.CorrectionCount, &s.DriftCount, &s.AutoResetThreshold, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

func GetSession(id int64) (*model.Session, error) {
	var s model.Session
	err := DB.QueryRow(
		`SELECT id, title, icon, provider_id, claude_session_id, work_dir, group_name, last_compress_msg_id, attention_enabled, attention_rules, is_shadow, parent_id, health_score, health_updated_at, correction_count, drift_count, auto_reset_threshold, created_at, updated_at FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.Title, &s.Icon, &s.ProviderID, &s.ClaudeSessionID, &s.WorkDir, &s.GroupName, &s.LastCompressMsgID, &s.AttentionEnabled, &s.AttentionRules, &s.IsShadow, &s.ParentID, &s.HealthScore, &s.HealthUpdatedAt, &s.CorrectionCount, &s.DriftCount, &s.AutoResetThreshold, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func UpdateSession(s *model.Session) error {
	s.UpdatedAt = time.Now()
	_, err := DB.Exec(
		`UPDATE sessions SET title=?, icon=?, provider_id=?, group_name=?, attention_enabled=?, auto_reset_threshold=?, updated_at=? WHERE id=?`,
		s.Title, s.Icon, s.ProviderID, s.GroupName, s.AttentionEnabled, s.AutoResetThreshold, s.UpdatedAt, s.ID,
	)
	return err
}

func DeleteSession(id int64) error {
	_, err := DB.Exec(`DELETE FROM messages WHERE session_id = ?`, id)
	if err != nil {
		return err
	}
	DB.Exec(`DELETE FROM token_usage WHERE session_id = ?`, id)
	_, err = DB.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func AddMessage(m *model.Message) error {
	m.CreatedAt = time.Now()
	result, err := DB.Exec(
		`INSERT INTO messages (session_id, role, content, metadata, attention_context, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		m.SessionID, m.Role, m.Content, m.Metadata, m.AttentionContext, m.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	m.ID = id
	_, err = DB.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`, m.CreatedAt, m.SessionID)
	return err
}

// UpdateMessageContent updates the content and metadata of an existing message.
// Used for incremental saves during streaming to prevent data loss on crash.
func UpdateMessageContent(id int64, content string, metadata string) error {
	_, err := DB.Exec(
		`UPDATE messages SET content = ?, metadata = ? WHERE id = ?`,
		content, metadata, id,
	)
	return err
}

// DeleteMessage removes a single message by ID.
// Used to clean up empty pre-inserted messages when streaming produces no content.
func DeleteMessage(id int64) error {
	_, err := DB.Exec(`DELETE FROM messages WHERE id = ?`, id)
	return err
}

func GetMessages(sessionID int64) ([]model.Message, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, role, content, metadata, attention_context, created_at FROM messages WHERE session_id = ? ORDER BY created_at`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Metadata, &m.AttentionContext, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

// GetMessagesPaginated returns messages for a session with cursor-based pagination.
// beforeID > 0: return messages with id < beforeID (older messages).
// limit <= 0: defaults to 50.
// Results are ordered by id ASC (oldest first) so the frontend can prepend them.
func GetMessagesPaginated(sessionID int64, beforeID int64, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows interface{ Next() bool; Scan(...interface{}) error; Close() error; Err() error }
	var err error
	if beforeID > 0 {
		// Subquery: get the last `limit` rows before beforeID, then re-order ASC
		rows2, err2 := DB.Query(
			`SELECT id, session_id, role, content, metadata, attention_context, created_at FROM (
				SELECT id, session_id, role, content, metadata, attention_context, created_at FROM messages
				WHERE session_id = ? AND id < ? ORDER BY id DESC LIMIT ?
			) sub ORDER BY id ASC`,
			sessionID, beforeID, limit,
		)
		rows = rows2
		err = err2
	} else {
		// No cursor: get the latest `limit` messages
		rows2, err2 := DB.Query(
			`SELECT id, session_id, role, content, metadata, attention_context, created_at FROM (
				SELECT id, session_id, role, content, metadata, attention_context, created_at FROM messages
				WHERE session_id = ? ORDER BY id DESC LIMIT ?
			) sub ORDER BY id ASC`,
			sessionID, limit,
		)
		rows = rows2
		err = err2
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Metadata, &m.AttentionContext, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

// GetMessagesCount returns the total number of messages in a session.
func GetMessagesCount(sessionID int64) (int64, error) {
	var count int64
	err := DB.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, sessionID).Scan(&count)
	return count, err
}

// GetMessagesCountBefore returns the number of messages with id < beforeID.
func GetMessagesCountBefore(sessionID int64, beforeID int64) (int64, error) {
	var count int64
	err := DB.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ? AND id < ?`, sessionID, beforeID).Scan(&count)
	return count, err
}

// GetMessagesByOffset returns messages with OFFSET/LIMIT pagination, ordered by id ASC.
func GetMessagesByOffset(sessionID int64, offset, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := DB.Query(
		`SELECT id, session_id, role, content, metadata, created_at FROM messages
		 WHERE session_id = ? ORDER BY id ASC LIMIT ? OFFSET ?`,
		sessionID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// GetMessagesByPage returns messages for a given page (1-based), ordered by id ASC.
func GetMessagesByPage(sessionID int64, page, pageSize int) ([]model.Message, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize
	return GetMessagesByOffset(sessionID, offset, pageSize)
}

// GetMessageWithContext returns a single message plus surrounding context messages.
func GetMessageWithContext(sessionID, msgID int64, ctx int) ([]model.Message, error) {
	if ctx < 0 {
		ctx = 0
	}
	// Get `ctx` messages before the target
	beforeRows, err := DB.Query(
		`SELECT id, session_id, role, content, metadata, created_at FROM (
			SELECT id, session_id, role, content, metadata, created_at FROM messages
			WHERE session_id = ? AND id < ? ORDER BY id DESC LIMIT ?
		) sub ORDER BY id ASC`,
		sessionID, msgID, ctx,
	)
	if err != nil {
		return nil, err
	}
	defer beforeRows.Close()
	before, err := scanMessages(beforeRows)
	if err != nil {
		return nil, err
	}

	// Get the target message + `ctx` messages after
	afterRows, err := DB.Query(
		`SELECT id, session_id, role, content, metadata, created_at FROM messages
		 WHERE session_id = ? AND id >= ? ORDER BY id ASC LIMIT ?`,
		sessionID, msgID, ctx+1,
	)
	if err != nil {
		return nil, err
	}
	defer afterRows.Close()
	after, err := scanMessages(afterRows)
	if err != nil {
		return nil, err
	}

	return append(before, after...), nil
}

// SearchMessages searches messages by keyword (SQL LIKE), ordered by id DESC.
func SearchMessages(sessionID int64, keyword string, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := DB.Query(
		`SELECT id, session_id, role, content, metadata, created_at FROM messages
		 WHERE session_id = ? AND content LIKE ? ORDER BY id DESC LIMIT ?`,
		sessionID, "%"+keyword+"%", limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// scanMessages is a helper to scan message rows into a slice.
func scanMessages(rows interface {
	Next() bool
	Scan(...interface{}) error
}) ([]model.Message, error) {
	var list []model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Metadata, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

func CreateSessionWithMessage(providerID string, content string, workDir string, groupName string) (*model.Session, error) {
	s := &model.Session{
		Title:      truncateTitle(content),
		ProviderID: providerID,
		WorkDir:    workDir,
		GroupName:  groupName,
	}
	if err := CreateSession(s); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	msg := &model.Message{
		SessionID: s.ID,
		Role:      "user",
		Content:   content,
	}
	if err := AddMessage(msg); err != nil {
		return nil, fmt.Errorf("add message: %w", err)
	}
	return s, nil
}

// GetPendingUserMessages returns user messages that arrived after the trigger message.
// triggerMsgID is the user message that started the current streaming round.
func GetPendingUserMessages(sessionID int64, triggerMsgID int64) ([]model.Message, error) {
	rows, err := DB.Query(`
		SELECT id, session_id, role, content, metadata, created_at FROM messages
		WHERE session_id = ? AND role = 'user' AND id > ?
		ORDER BY created_at`,
		sessionID, triggerMsgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Metadata, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

// GetLastUserMessageID returns the ID of the last user message in a session.
func GetLastUserMessageID(sessionID int64) int64 {
	var id int64
	DB.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM messages WHERE session_id = ? AND role = 'user'`, sessionID).Scan(&id)
	return id
}

// DeleteMessagesFrom removes the message with the given id AND all messages after it
// for the given session (id >= fromMsgID). Used by the retry-message feature so the
// original user message + any subsequent AI reply are both cleared before re-sending.
func DeleteMessagesFrom(sessionID int64, fromMsgID int64) error {
	_, err := DB.Exec(`DELETE FROM messages WHERE session_id = ? AND id >= ?`, sessionID, fromMsgID)
	return err
}

// HasAssistantMessages returns true if the session has at least one assistant message.
// Used to determine whether a dead process's conversation can be resumed.
func HasAssistantMessages(sessionID int64) bool {
	var count int
	DB.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ? AND role = 'assistant' LIMIT 1`, sessionID).Scan(&count)
	return count > 0
}

func UpdateSessionTitle(id int64, title string) error {
	_, err := DB.Exec(`UPDATE sessions SET title=?, updated_at=? WHERE id=?`, title, time.Now(), id)
	return err
}

// UpdateSessionProvider updates the provider_id for a session (used for fallback when original provider is deleted).
func UpdateSessionProvider(id int64, providerID string) error {
	_, err := DB.Exec(`UPDATE sessions SET provider_id=?, updated_at=? WHERE id=?`, providerID, time.Now(), id)
	return err
}

// UpdateClaudeSessionID replaces the Claude CLI session UUID (used for context overflow recovery).
func UpdateClaudeSessionID(id int64, newUUID string) error {
	_, err := DB.Exec(`UPDATE sessions SET claude_session_id=?, updated_at=? WHERE id=?`, newUUID, time.Now(), id)
	return err
}

// UpdateLastCompressMsgID records the latest message ID at compress time,
// so subsequent auto-compress checks only count messages/tokens after this point.
func UpdateLastCompressMsgID(sessionID int64, msgID int64) error {
	_, err := DB.Exec(`UPDATE sessions SET last_compress_msg_id=?, updated_at=? WHERE id=?`, msgID, time.Now(), sessionID)
	return err
}

// UpdateAttentionEnabled toggles the attention system for a session.
func UpdateAttentionEnabled(sessionID int64, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	_, err := DB.Exec(`UPDATE sessions SET attention_enabled=?, updated_at=? WHERE id=?`, val, time.Now(), sessionID)
	return err
}

// UpdateAttentionRules updates the attention rules for a session.
func UpdateAttentionRules(sessionID int64, rules string) error {
	_, err := DB.Exec(`UPDATE sessions SET attention_rules=?, updated_at=? WHERE id=?`, rules, time.Now(), sessionID)
	return err
}

func truncateTitle(s string) string {
	for i, c := range s {
		if c == '\n' || c == '\r' {
			s = s[:i]
			break
		}
	}
	if len([]rune(s)) > 50 {
		runes := []rune(s)
		return string(runes[:50]) + "..."
	}
	return s
}

// ========== Shadow Session Functions (Attention Mode v2) ==========

// CreateShadowSession creates a shadow session for attention mode execution.
// It copies essential fields from the parent session and marks it as shadow.
func CreateShadowSession(parentID int64) (*model.Session, error) {
	parent, err := GetSession(parentID)
	if err != nil {
		return nil, fmt.Errorf("parent session not found: %w", err)
	}

	shadow := &model.Session{
		Title:            "[Shadow] " + parent.Title,
		ProviderID:       parent.ProviderID,
		WorkDir:          parent.WorkDir,
		GroupName:        parent.GroupName,
		AttentionEnabled: false, // Shadow sessions don't have attention mode
		IsShadow:         true,
		ParentID:         parentID,
	}

	if err := CreateSession(shadow); err != nil {
		return nil, err
	}

	return shadow, nil
}

// CreateShadowSessionWithTitle creates a shadow session with a custom title suffix
func CreateShadowSessionWithTitle(parentID int64, titleSuffix string) (*model.Session, error) {
	parent, err := GetSession(parentID)
	if err != nil {
		return nil, fmt.Errorf("parent session not found: %w", err)
	}

	shadow := &model.Session{
		Title:            fmt.Sprintf("[%s] %s", titleSuffix, parent.Title),
		ProviderID:       parent.ProviderID,
		WorkDir:          parent.WorkDir,
		GroupName:        parent.GroupName,
		AttentionEnabled: false,
		IsShadow:         true,
		ParentID:         parentID,
	}

	if err := CreateSession(shadow); err != nil {
		return nil, err
	}

	return shadow, nil
}

// CopyRecentMessagesToShadow copies the last N messages from parent to shadow session.
// This provides context for the shadow session to work with.
func CopyRecentMessagesToShadow(parentID, shadowID int64, limit int) error {
	if limit <= 0 {
		limit = 20 // Default to last 20 messages
	}

	// Get recent messages from parent (oldest first for correct order)
	rows, err := DB.Query(`
		SELECT role, content, metadata FROM messages
		WHERE session_id = ?
		ORDER BY id DESC LIMIT ?
	`, parentID, limit)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Collect messages in reverse order (to insert oldest first)
	type msg struct {
		role, content, metadata string
	}
	var msgs []msg
	for rows.Next() {
		var m msg
		if err := rows.Scan(&m.role, &m.content, &m.metadata); err != nil {
			return err
		}
		msgs = append(msgs, m)
	}

	// Insert in reverse order (oldest first)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		_, err := DB.Exec(`
			INSERT INTO messages (session_id, role, content, metadata, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, shadowID, m.role, m.content, m.metadata, time.Now())
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteShadowSession deletes a shadow session and all its messages.
func DeleteShadowSession(shadowID int64) error {
	// Verify it's actually a shadow session
	session, err := GetSession(shadowID)
	if err != nil {
		return err
	}
	if !session.IsShadow {
		return fmt.Errorf("session %d is not a shadow session", shadowID)
	}

	// Delete messages first (foreign key constraint)
	if _, err := DB.Exec(`DELETE FROM messages WHERE session_id = ?`, shadowID); err != nil {
		return err
	}

	// Delete the session
	if _, err := DB.Exec(`DELETE FROM sessions WHERE id = ?`, shadowID); err != nil {
		return err
	}

	return nil
}

// GetShadowSessionByParent finds an active shadow session for a parent session.
// Returns nil if no shadow session exists.
func GetShadowSessionByParent(parentID int64) (*model.Session, error) {
	var s model.Session
	err := DB.QueryRow(`
		SELECT id, title, icon, provider_id, claude_session_id, work_dir, group_name,
		       last_compress_msg_id, attention_enabled, attention_rules, is_shadow, parent_id,
		       health_score, health_updated_at, correction_count, drift_count, auto_reset_threshold,
		       created_at, updated_at
		FROM sessions
		WHERE parent_id = ? AND is_shadow = 1
		ORDER BY created_at DESC LIMIT 1
	`, parentID).Scan(&s.ID, &s.Title, &s.Icon, &s.ProviderID, &s.ClaudeSessionID, &s.WorkDir, &s.GroupName,
		&s.LastCompressMsgID, &s.AttentionEnabled, &s.AttentionRules, &s.IsShadow, &s.ParentID,
		&s.HealthScore, &s.HealthUpdatedAt, &s.CorrectionCount, &s.DriftCount, &s.AutoResetThreshold,
		&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// CleanupOldShadowSessions deletes shadow sessions older than the specified duration.
func CleanupOldShadowSessions(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)

	// First delete messages
	DB.Exec(`DELETE FROM messages WHERE session_id IN (SELECT id FROM sessions WHERE is_shadow = 1 AND created_at < ?)`, cutoff)

	// Then delete sessions
	result, err := DB.Exec(`DELETE FROM sessions WHERE is_shadow = 1 AND created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// ========== Health Score Functions (Issue #213) ==========

// GetSessionHealth returns the health info for a session.
func GetSessionHealth(sessionID int64) (score string, updatedAt string, correctionCount, driftCount int, err error) {
	err = DB.QueryRow(
		`SELECT health_score, health_updated_at, correction_count, drift_count FROM sessions WHERE id = ?`, sessionID,
	).Scan(&score, &updatedAt, &correctionCount, &driftCount)
	return
}

// UpdateSessionHealth sets the health score for a session.
func UpdateSessionHealth(sessionID int64, score string) error {
	now := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
	_, err := DB.Exec(
		`UPDATE sessions SET health_score=?, health_updated_at=?, updated_at=? WHERE id=?`,
		score, now, time.Now(), sessionID,
	)
	return err
}

// IncrementCorrectionCount increments the correction_count for a session.
func IncrementCorrectionCount(sessionID int64) error {
	now := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
	_, err := DB.Exec(
		`UPDATE sessions SET correction_count = correction_count + 1, health_updated_at=?, updated_at=? WHERE id=?`,
		now, time.Now(), sessionID,
	)
	return err
}

// IncrementDriftCount increments the drift_count for a session.
func IncrementDriftCount(sessionID int64) error {
	now := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
	_, err := DB.Exec(
		`UPDATE sessions SET drift_count = drift_count + 1, health_updated_at=?, updated_at=? WHERE id=?`,
		now, time.Now(), sessionID,
	)
	return err
}

// ========== Context Reset Functions (Issue #214) ==========

// ResetSessionMessages deletes messages from a session, optionally keeping the last N.
// Returns the number of messages deleted.
func ResetSessionMessages(sessionID int64, keepLast int) (int64, error) {
	if keepLast <= 0 {
		// Delete all messages
		result, err := DB.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID)
		if err != nil {
			return 0, err
		}
		return result.RowsAffected()
	}

	// Keep last N messages: delete all except the most recent N
	result, err := DB.Exec(`
		DELETE FROM messages WHERE session_id = ? AND id NOT IN (
			SELECT id FROM messages WHERE session_id = ? ORDER BY id DESC LIMIT ?
		)`, sessionID, sessionID, keepLast)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// UpdateAutoResetThreshold sets the auto_reset_threshold for a session.
func UpdateAutoResetThreshold(sessionID int64, threshold int) error {
	_, err := DB.Exec(`UPDATE sessions SET auto_reset_threshold=?, updated_at=? WHERE id=?`,
		threshold, time.Now(), sessionID)
	return err
}
