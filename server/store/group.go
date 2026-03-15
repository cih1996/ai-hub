package store

import (
	"ai-hub/server/model"
	"time"
)

// ListGroups returns all groups ordered by name
func ListGroups() ([]model.Group, error) {
	rows, err := DB.Query(`SELECT id, name, icon, description, created_at, updated_at FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []model.Group
	for rows.Next() {
		var g model.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Icon, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// GetGroupByName returns a group by name
func GetGroupByName(name string) (*model.Group, error) {
	var g model.Group
	err := DB.QueryRow(`SELECT id, name, icon, description, created_at, updated_at FROM groups WHERE name = ?`, name).
		Scan(&g.ID, &g.Name, &g.Icon, &g.Description, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// CreateGroup creates a new group
func CreateGroup(name, description string) (*model.Group, error) {
	now := time.Now().Format(time.RFC3339)
	result, err := DB.Exec(`INSERT INTO groups (name, description, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		name, description, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &model.Group{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// DeleteGroup deletes a group by name
func DeleteGroup(name string) error {
	_, err := DB.Exec(`DELETE FROM groups WHERE name = ?`, name)
	return err
}

// UpdateGroup updates a group's icon and description
func UpdateGroup(name string, icon string, description string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := DB.Exec(`UPDATE groups SET icon = ?, description = ?, updated_at = ? WHERE name = ?`,
		icon, description, now, name)
	return err
}

// CountSessionsByGroup returns the number of sessions in a group
func CountSessionsByGroup(groupName string) (int, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE group_name = ?`, groupName).Scan(&count)
	return count, err
}

// UpdateSessionGroup updates a session's group_name
func UpdateSessionGroup(sessionID int64, groupName string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := DB.Exec(`UPDATE sessions SET group_name = ?, updated_at = ? WHERE id = ?`, groupName, now, sessionID)
	return err
}

// ListUniqueGroupNames returns all unique non-empty group_name values from sessions
func ListUniqueGroupNames() ([]string, error) {
	rows, err := DB.Query(`SELECT DISTINCT group_name FROM sessions WHERE group_name != '' ORDER BY group_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}
