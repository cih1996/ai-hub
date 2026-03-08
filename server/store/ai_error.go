package store

import (
	"fmt"
	"strings"

	"ai-hub/server/model"
)

// AddAIError inserts an AI error/warning record.
func AddAIError(e *model.AIError) error {
	_, err := DB.Exec(
		`INSERT INTO ai_errors (session_id, message_id, level, summary) VALUES (?, ?, ?, ?)`,
		e.SessionID, e.MessageID, e.Level, e.Summary,
	)
	return err
}

// GetSessionErrors returns errors for a session, optionally filtered by level.
func GetSessionErrors(sessionID int64, level string) ([]model.AIError, error) {
	query := `SELECT id, session_id, message_id, level, summary, created_at FROM ai_errors WHERE session_id = ?`
	args := []interface{}{sessionID}
	if level != "" {
		query += ` AND level = ?`
		args = append(args, level)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.AIError
	for rows.Next() {
		var e model.AIError
		if err := rows.Scan(&e.ID, &e.SessionID, &e.MessageID, &e.Level, &e.Summary, &e.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, e)
	}
	return results, nil
}

// GetErrorStats returns error/warning counts aggregated by session.
// If sessionID > 0, only returns stats for that session.
func GetErrorStats(sessionID int64) ([]model.ErrorStat, error) {
	query := `SELECT e.session_id, COALESCE(s.title, ''),
		SUM(CASE WHEN e.level = 'error' THEN 1 ELSE 0 END),
		SUM(CASE WHEN e.level = 'warning' THEN 1 ELSE 0 END)
		FROM ai_errors e LEFT JOIN sessions s ON e.session_id = s.id`
	var args []interface{}
	if sessionID > 0 {
		query += ` WHERE e.session_id = ?`
		args = append(args, sessionID)
	}
	query += ` GROUP BY e.session_id ORDER BY SUM(CASE WHEN e.level = 'error' THEN 1 ELSE 0 END) DESC`

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.ErrorStat
	for rows.Next() {
		var s model.ErrorStat
		if err := rows.Scan(&s.SessionID, &s.SessionTitle, &s.ErrorCount, &s.WarningCount); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, nil
}

// GetSessionErrorCounts returns error/warning counts for multiple sessions in batch.
func GetSessionErrorCounts(sessionIDs []int64) (map[int64]model.ErrorCount, error) {
	result := make(map[int64]model.ErrorCount)
	if len(sessionIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(sessionIDs))
	args := make([]interface{}, len(sessionIDs))
	for i, id := range sessionIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT session_id,
		SUM(CASE WHEN level = 'error' THEN 1 ELSE 0 END),
		SUM(CASE WHEN level = 'warning' THEN 1 ELSE 0 END)
		FROM ai_errors WHERE session_id IN (%s) GROUP BY session_id`,
		strings.Join(placeholders, ","))

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sid int64
		var ec model.ErrorCount
		if err := rows.Scan(&sid, &ec.ErrorCount, &ec.WarningCount); err != nil {
			return nil, err
		}
		result[sid] = ec
	}
	return result, nil
}
