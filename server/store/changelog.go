package store

import (
	"ai-hub/server/model"
)

// InitChangelogTable creates the memory_changelog table (called from migrate).
func InitChangelogTable() {
	DB.Exec(`CREATE TABLE IF NOT EXISTS memory_changelog (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_name TEXT NOT NULL DEFAULT '',
		scope TEXT NOT NULL DEFAULT '',
		change_type TEXT NOT NULL DEFAULT '',
		session_id INTEGER NOT NULL DEFAULT 0,
		diff TEXT NOT NULL DEFAULT '',
		schema TEXT NOT NULL DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		content TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_changelog_file ON memory_changelog(file_name, scope)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_changelog_session ON memory_changelog(session_id)`)
}

// AddChangelog inserts a new changelog entry. Version is auto-calculated.
func AddChangelog(cl *model.MemoryChangelog) error {
	// Get the next version number for this file+scope
	var maxVersion int
	DB.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM memory_changelog WHERE file_name = ? AND scope = ?`,
		cl.FileName, cl.Scope).Scan(&maxVersion)
	cl.Version = maxVersion + 1
	cl.CreatedAt = now()

	result, err := DB.Exec(
		`INSERT INTO memory_changelog (file_name, scope, change_type, session_id, diff, schema, version, content, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cl.FileName, cl.Scope, cl.ChangeType, cl.SessionID, cl.Diff, cl.Schema, cl.Version, cl.Content, cl.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	cl.ID = id
	return nil
}

// ListChangelog returns changelog entries for a specific file+scope, ordered by version DESC.
func ListChangelog(fileName, scope string, limit int) ([]model.MemoryChangelog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := DB.Query(
		`SELECT id, file_name, scope, change_type, session_id, diff, schema, version, content, created_at
		 FROM memory_changelog WHERE file_name = ? AND scope = ? ORDER BY version DESC LIMIT ?`,
		fileName, scope, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.MemoryChangelog
	for rows.Next() {
		var cl model.MemoryChangelog
		if err := rows.Scan(&cl.ID, &cl.FileName, &cl.Scope, &cl.ChangeType, &cl.SessionID, &cl.Diff, &cl.Schema, &cl.Version, &cl.Content, &cl.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, cl)
	}
	return list, nil
}

// GetChangelogByVersion returns a specific version of a file.
func GetChangelogByVersion(fileName, scope string, version int) (*model.MemoryChangelog, error) {
	var cl model.MemoryChangelog
	err := DB.QueryRow(
		`SELECT id, file_name, scope, change_type, session_id, diff, schema, version, content, created_at
		 FROM memory_changelog WHERE file_name = ? AND scope = ? AND version = ?`,
		fileName, scope, version,
	).Scan(&cl.ID, &cl.FileName, &cl.Scope, &cl.ChangeType, &cl.SessionID, &cl.Diff, &cl.Schema, &cl.Version, &cl.Content, &cl.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &cl, nil
}

// GetLatestChangelog returns the most recent changelog entry for a file.
func GetLatestChangelog(fileName, scope string) (*model.MemoryChangelog, error) {
	var cl model.MemoryChangelog
	err := DB.QueryRow(
		`SELECT id, file_name, scope, change_type, session_id, diff, schema, version, content, created_at
		 FROM memory_changelog WHERE file_name = ? AND scope = ? ORDER BY version DESC LIMIT 1`,
		fileName, scope,
	).Scan(&cl.ID, &cl.FileName, &cl.Scope, &cl.ChangeType, &cl.SessionID, &cl.Diff, &cl.Schema, &cl.Version, &cl.Content, &cl.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &cl, nil
}
