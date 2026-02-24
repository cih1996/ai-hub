package store

import (
	"ai-hub/server/model"
	"time"
)

func AddTokenUsage(t *model.TokenUsage) error {
	t.CreatedAt = time.Now()
	result, err := DB.Exec(
		`INSERT INTO token_usage (session_id, message_id, input_tokens, output_tokens, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.SessionID, t.MessageID, t.InputTokens, t.OutputTokens, t.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = id
	return nil
}

func GetTokenUsageByMessage(messageID int64) (*model.TokenUsage, error) {
	var t model.TokenUsage
	err := DB.QueryRow(
		`SELECT id, session_id, message_id, input_tokens, output_tokens, created_at FROM token_usage WHERE message_id = ?`,
		messageID,
	).Scan(&t.ID, &t.SessionID, &t.MessageID, &t.InputTokens, &t.OutputTokens, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func GetSessionTokenStats(sessionID int64) (*model.TokenUsageStats, error) {
	var s model.TokenUsageStats
	err := DB.QueryRow(
		`SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COUNT(*) FROM token_usage WHERE session_id = ?`,
		sessionID,
	).Scan(&s.TotalInput, &s.TotalOutput, &s.Count)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func GetSystemTokenStats(startTime, endTime string) (*model.TokenUsageStats, error) {
	var s model.TokenUsageStats
	query := `SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COUNT(*) FROM token_usage WHERE 1=1`
	var args []interface{}
	if startTime != "" {
		query += ` AND created_at >= ?`
		args = append(args, startTime)
	}
	if endTime != "" {
		query += ` AND created_at <= ?`
		args = append(args, endTime)
	}
	err := DB.QueryRow(query, args...).Scan(&s.TotalInput, &s.TotalOutput, &s.Count)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func GetSessionTokenUsageList(sessionID int64) ([]model.TokenUsage, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, message_id, input_tokens, output_tokens, created_at FROM token_usage WHERE session_id = ? ORDER BY created_at DESC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.TokenUsage
	for rows.Next() {
		var t model.TokenUsage
		if err := rows.Scan(&t.ID, &t.SessionID, &t.MessageID, &t.InputTokens, &t.OutputTokens, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

// DeleteSessionTokenUsage removes all token usage records for a session (called on session delete).
func DeleteSessionTokenUsage(sessionID int64) error {
	_, err := DB.Exec(`DELETE FROM token_usage WHERE session_id = ?`, sessionID)
	return err
}
