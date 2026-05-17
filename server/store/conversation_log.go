package store

import (
	"ai-hub/server/model"
	"time"
)

// AddConversationLog archives a user input or final assistant output.
func AddConversationLog(logItem *model.ConversationLog) error {
	logItem.CreatedAt = time.Now()
	result, err := DB.Exec(
		`INSERT INTO conversation_logs (session_id, message_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		logItem.SessionID, logItem.MessageID, logItem.Role, logItem.Content, logItem.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	logItem.ID = id
	return nil
}

func GetConversationLogs(sessionID int64) ([]model.ConversationLog, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, message_id, role, content, created_at
		 FROM conversation_logs WHERE session_id = ? ORDER BY id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConversationLogs(rows)
}

// GetConversationLogsPaginated returns archived logs with cursor-based pagination.
// beforeID > 0 returns older logs (id < beforeID). Results are ordered ASC.
func GetConversationLogsPaginated(sessionID int64, beforeID int64, limit int) ([]model.ConversationLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows interface {
		Next() bool
		Scan(...interface{}) error
		Close() error
	}
	var err error
	if beforeID > 0 {
		rows, err = DB.Query(
			`SELECT id, session_id, message_id, role, content, created_at FROM (
				SELECT id, session_id, message_id, role, content, created_at FROM conversation_logs
				WHERE session_id = ? AND id < ? ORDER BY id DESC LIMIT ?
			) sub ORDER BY id ASC`,
			sessionID, beforeID, limit,
		)
	} else {
		rows, err = DB.Query(
			`SELECT id, session_id, message_id, role, content, created_at FROM (
				SELECT id, session_id, message_id, role, content, created_at FROM conversation_logs
				WHERE session_id = ? ORDER BY id DESC LIMIT ?
			) sub ORDER BY id ASC`,
			sessionID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConversationLogs(rows)
}

func GetConversationLogsCount(sessionID int64) (int64, error) {
	var count int64
	err := DB.QueryRow(`SELECT COUNT(*) FROM conversation_logs WHERE session_id = ?`, sessionID).Scan(&count)
	return count, err
}

func GetConversationLogsCountBefore(sessionID int64, beforeID int64) (int64, error) {
	var count int64
	err := DB.QueryRow(`SELECT COUNT(*) FROM conversation_logs WHERE session_id = ? AND id < ?`, sessionID, beforeID).Scan(&count)
	return count, err
}

func SearchConversationLogs(sessionID int64, keyword string, limit int) ([]model.ConversationLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := DB.Query(
		`SELECT id, session_id, message_id, role, content, created_at FROM conversation_logs
		 WHERE session_id = ? AND content LIKE ? ORDER BY id DESC LIMIT ?`,
		sessionID, "%"+keyword+"%", limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConversationLogs(rows)
}

func scanConversationLogs(rows interface {
	Next() bool
	Scan(...interface{}) error
}) ([]model.ConversationLog, error) {
	var list []model.ConversationLog
	for rows.Next() {
		var item model.ConversationLog
		if err := rows.Scan(&item.ID, &item.SessionID, &item.MessageID, &item.Role, &item.Content, &item.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, nil
}
