package store

import (
	"ai-hub/server/model"
	"strconv"
)

// GetSetting returns the value for a key, or "" if not found.
func GetSetting(key string) (string, error) {
	row := DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key)
	var val string
	if err := row.Scan(&val); err != nil {
		// Not found is treated as empty string (not an error)
		return "", nil
	}
	return val, nil
}

// SetSetting upserts a key-value pair.
func SetSetting(key, value string) error {
	_, err := DB.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`, key, value)
	return err
}

// GetCompressSettings reads compress.* keys and returns a CompressSettings struct.
// Defaults: auto_enabled=false, threshold=80000, mode="auto", min_turns=10.
func GetCompressSettings() (*model.CompressSettings, error) {
	s := &model.CompressSettings{
		AutoEnabled: false,
		Threshold:   80000,
		Mode:        "auto",
		MinTurns:    10,
	}

	if v, _ := GetSetting("compress.auto_enabled"); v == "true" {
		s.AutoEnabled = true
	}
	if v, _ := GetSetting("compress.threshold"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.Threshold = n
		}
	}
	if v, _ := GetSetting("compress.mode"); v != "" {
		s.Mode = v
	}
	if v, _ := GetSetting("compress.min_turns"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			s.MinTurns = n
		}
	}

	return s, nil
}

// SaveCompressSettings writes all compress settings atomically.
func SaveCompressSettings(s *model.CompressSettings) error {
	enabled := "false"
	if s.AutoEnabled {
		enabled = "true"
	}
	if err := SetSetting("compress.auto_enabled", enabled); err != nil {
		return err
	}
	if err := SetSetting("compress.threshold", strconv.Itoa(s.Threshold)); err != nil {
		return err
	}
	mode := s.Mode
	if mode == "" {
		mode = "auto"
	}
	if err := SetSetting("compress.mode", mode); err != nil {
		return err
	}
	return SetSetting("compress.min_turns", strconv.Itoa(s.MinTurns))
}

// CountUserMessages returns the number of user messages in a session (each = 1 turn).
func CountUserMessages(sessionID int64) int {
	row := DB.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ? AND role = 'user'`, sessionID)
	var count int
	row.Scan(&count)
	return count
}
