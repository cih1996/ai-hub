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
		`INSERT INTO sessions (title, provider_id, claude_session_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		s.Title, s.ProviderID, s.ClaudeSessionID, s.CreatedAt, s.UpdatedAt,
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
	rows, err := DB.Query(`SELECT id, title, provider_id, claude_session_id, created_at, updated_at FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Session
	for rows.Next() {
		var s model.Session
		if err := rows.Scan(&s.ID, &s.Title, &s.ProviderID, &s.ClaudeSessionID, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

func GetSession(id int64) (*model.Session, error) {
	var s model.Session
	err := DB.QueryRow(
		`SELECT id, title, provider_id, claude_session_id, created_at, updated_at FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.Title, &s.ProviderID, &s.ClaudeSessionID, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func UpdateSession(s *model.Session) error {
	s.UpdatedAt = time.Now()
	_, err := DB.Exec(
		`UPDATE sessions SET title=?, provider_id=?, updated_at=? WHERE id=?`,
		s.Title, s.ProviderID, s.UpdatedAt, s.ID,
	)
	return err
}

func DeleteSession(id int64) error {
	_, err := DB.Exec(`DELETE FROM messages WHERE session_id = ?`, id)
	if err != nil {
		return err
	}
	_, err = DB.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func AddMessage(m *model.Message) error {
	m.CreatedAt = time.Now()
	result, err := DB.Exec(
		`INSERT INTO messages (session_id, role, content, created_at) VALUES (?, ?, ?, ?)`,
		m.SessionID, m.Role, m.Content, m.CreatedAt,
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

func GetMessages(sessionID int64) ([]model.Message, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, role, content, created_at FROM messages WHERE session_id = ? ORDER BY created_at`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Message
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

func CreateSessionWithMessage(providerID string, content string) (*model.Session, error) {
	s := &model.Session{
		Title:      truncateTitle(content),
		ProviderID: providerID,
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
