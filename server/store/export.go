package store

import "ai-hub/server/model"

// ListSessionsByGroup returns all sessions with the given group_name, ordered by updated_at DESC.
func ListSessionsByGroup(groupName string) ([]model.Session, error) {
	rows, err := DB.Query(
		`SELECT id, title, provider_id, claude_session_id, work_dir, group_name, created_at, updated_at
		 FROM sessions WHERE group_name = ? ORDER BY updated_at DESC`, groupName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Session
	for rows.Next() {
		var s model.Session
		if err := rows.Scan(&s.ID, &s.Title, &s.ProviderID, &s.ClaudeSessionID, &s.WorkDir, &s.GroupName, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}
