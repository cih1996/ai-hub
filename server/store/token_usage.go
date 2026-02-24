package store

import (
	"ai-hub/server/model"
	"time"
)

func AddTokenUsage(t *model.TokenUsage) error {
	t.CreatedAt = time.Now()
	result, err := DB.Exec(
		`INSERT INTO token_usage (session_id, message_id, input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.SessionID, t.MessageID, t.InputTokens, t.OutputTokens, t.CacheCreationInputTokens, t.CacheReadInputTokens, t.CreatedAt,
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
		`SELECT id, session_id, message_id, input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens, created_at FROM token_usage WHERE message_id = ?`,
		messageID,
	).Scan(&t.ID, &t.SessionID, &t.MessageID, &t.InputTokens, &t.OutputTokens, &t.CacheCreationInputTokens, &t.CacheReadInputTokens, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func GetSessionTokenStats(sessionID int64) (*model.TokenUsageStats, error) {
	var s model.TokenUsageStats
	err := DB.QueryRow(
		`SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cache_creation_input_tokens),0), COALESCE(SUM(cache_read_input_tokens),0), COUNT(*) FROM token_usage WHERE session_id = ?`,
		sessionID,
	).Scan(&s.TotalInput, &s.TotalOutput, &s.TotalCacheCreation, &s.TotalCacheRead, &s.Count)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func GetSystemTokenStats(startTime, endTime string) (*model.TokenUsageStats, error) {
	var s model.TokenUsageStats
	query := `SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cache_creation_input_tokens),0), COALESCE(SUM(cache_read_input_tokens),0), COUNT(*) FROM token_usage WHERE 1=1`
	var args []interface{}
	if startTime != "" {
		query += ` AND DATE(created_at, '+8 hours') >= ?`
		args = append(args, startTime)
	}
	if endTime != "" {
		query += ` AND DATE(created_at, '+8 hours') <= ?`
		args = append(args, endTime)
	}
	err := DB.QueryRow(query, args...).Scan(&s.TotalInput, &s.TotalOutput, &s.TotalCacheCreation, &s.TotalCacheRead, &s.Count)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func GetSessionTokenUsageList(sessionID int64) ([]model.TokenUsage, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, message_id, input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens, created_at FROM token_usage WHERE session_id = ? ORDER BY created_at DESC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.TokenUsage
	for rows.Next() {
		var t model.TokenUsage
		if err := rows.Scan(&t.ID, &t.SessionID, &t.MessageID, &t.InputTokens, &t.OutputTokens, &t.CacheCreationInputTokens, &t.CacheReadInputTokens, &t.CreatedAt); err != nil {
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

// DailyTokenUsage represents aggregated token usage for a single day.
type DailyTokenUsage struct {
	Date                     string `json:"date"`
	InputTokens              int64  `json:"input_tokens"`
	OutputTokens             int64  `json:"output_tokens"`
	CacheCreationInputTokens int64  `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64  `json:"cache_read_input_tokens"`
}

// GetDailyTokenUsage returns token usage aggregated by day within a date range.
func GetDailyTokenUsage(start, end string) ([]DailyTokenUsage, error) {
	query := `SELECT DATE(created_at, '+8 hours') as date, COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(cache_creation_input_tokens),0), COALESCE(SUM(cache_read_input_tokens),0) FROM token_usage WHERE 1=1`
	var args []interface{}
	if start != "" {
		query += ` AND DATE(created_at, '+8 hours') >= ?`
		args = append(args, start)
	}
	if end != "" {
		query += ` AND DATE(created_at, '+8 hours') <= ?`
		args = append(args, end)
	}
	query += ` GROUP BY DATE(created_at, '+8 hours') ORDER BY date`
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []DailyTokenUsage
	for rows.Next() {
		var d DailyTokenUsage
		if err := rows.Scan(&d.Date, &d.InputTokens, &d.OutputTokens, &d.CacheCreationInputTokens, &d.CacheReadInputTokens); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, nil
}

// SessionTokenRanking represents a session's total token consumption.
type SessionTokenRanking struct {
	SessionID                int64  `json:"session_id"`
	Title                    string `json:"title"`
	InputTokens              int64  `json:"input_tokens"`
	OutputTokens             int64  `json:"output_tokens"`
	CacheCreationInputTokens int64  `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64  `json:"cache_read_input_tokens"`
	Total                    int64  `json:"total"`
}

// GetTokenUsageRanking returns top N sessions by total token consumption.
func GetTokenUsageRanking(start, end string, limit int) ([]SessionTokenRanking, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `SELECT t.session_id, COALESCE(s.title,''), COALESCE(SUM(t.input_tokens),0), COALESCE(SUM(t.output_tokens),0), COALESCE(SUM(t.cache_creation_input_tokens),0), COALESCE(SUM(t.cache_read_input_tokens),0), COALESCE(SUM(t.input_tokens)+SUM(t.output_tokens)+SUM(t.cache_creation_input_tokens)+SUM(t.cache_read_input_tokens),0) as total FROM token_usage t LEFT JOIN sessions s ON t.session_id = s.id WHERE 1=1`
	var args []interface{}
	if start != "" {
		query += ` AND DATE(t.created_at, '+8 hours') >= ?`
		args = append(args, start)
	}
	if end != "" {
		query += ` AND DATE(t.created_at, '+8 hours') <= ?`
		args = append(args, end)
	}
	query += ` GROUP BY t.session_id ORDER BY total DESC LIMIT ?`
	args = append(args, limit)
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []SessionTokenRanking
	for rows.Next() {
		var r SessionTokenRanking
		if err := rows.Scan(&r.SessionID, &r.Title, &r.InputTokens, &r.OutputTokens, &r.CacheCreationInputTokens, &r.CacheReadInputTokens, &r.Total); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

// HourlyTokenUsage represents aggregated token usage for a single hour.
type HourlyTokenUsage struct {
	Hour                     string `json:"hour"`
	InputTokens              int64  `json:"input_tokens"`
	OutputTokens             int64  `json:"output_tokens"`
	CacheCreationInputTokens int64  `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64  `json:"cache_read_input_tokens"`
}

// GetHourlyTokenUsage returns token usage aggregated by hour, optionally filtered by session.
func GetHourlyTokenUsage(start, end string, sessionID int64) ([]HourlyTokenUsage, error) {
	query := `SELECT strftime('%Y-%m-%d %H:00', created_at, '+8 hours') as hour,
		COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
		COALESCE(SUM(cache_creation_input_tokens),0), COALESCE(SUM(cache_read_input_tokens),0)
		FROM token_usage WHERE 1=1`
	var args []interface{}
	if sessionID > 0 {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	if start != "" {
		query += ` AND datetime(created_at, '+8 hours') >= ?`
		args = append(args, start)
	}
	if end != "" {
		query += ` AND datetime(created_at, '+8 hours') <= ?`
		args = append(args, end)
	}
	query += ` GROUP BY strftime('%Y-%m-%d %H:00', created_at, '+8 hours') ORDER BY hour`
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []HourlyTokenUsage
	for rows.Next() {
		var h HourlyTokenUsage
		if err := rows.Scan(&h.Hour, &h.InputTokens, &h.OutputTokens,
			&h.CacheCreationInputTokens, &h.CacheReadInputTokens); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, nil
}
