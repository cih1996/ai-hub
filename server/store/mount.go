package store

import (
	"ai-hub/server/model"
	"time"
)

// CreateMount 创建挂载
func CreateMount(m *model.Mount) error {
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now
	result, err := DB.Exec(`
		INSERT INTO mounts (alias, local_path, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, m.Alias, m.LocalPath, m.CreatedAt, m.UpdatedAt)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	m.ID = id
	return nil
}

// GetMountByAlias 根据别名获取挂载
func GetMountByAlias(alias string) (*model.Mount, error) {
	var m model.Mount
	err := DB.QueryRow(`
		SELECT id, alias, local_path, created_at, updated_at
		FROM mounts WHERE alias = ?
	`, alias).Scan(&m.ID, &m.Alias, &m.LocalPath, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListMounts 列出所有挂载
func ListMounts() ([]model.Mount, error) {
	rows, err := DB.Query(`
		SELECT id, alias, local_path, created_at, updated_at
		FROM mounts ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mounts []model.Mount
	for rows.Next() {
		var m model.Mount
		if err := rows.Scan(&m.ID, &m.Alias, &m.LocalPath, &m.CreatedAt, &m.UpdatedAt); err != nil {
			continue
		}
		mounts = append(mounts, m)
	}
	return mounts, nil
}

// DeleteMountByAlias 根据别名删除挂载
func DeleteMountByAlias(alias string) error {
	_, err := DB.Exec(`DELETE FROM mounts WHERE alias = ?`, alias)
	return err
}
