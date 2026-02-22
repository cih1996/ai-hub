package store

import (
	"ai-hub/server/model"
	"strings"
	"time"
)

func CreateChannel(ch *model.Channel) error {
	now := time.Now()
	ch.CreatedAt = now
	ch.UpdatedAt = now
	if ch.Config == "" {
		ch.Config = "{}"
	}
	result, err := DB.Exec(
		`INSERT INTO channels (name, platform, session_id, config, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ch.Name, ch.Platform, ch.SessionID, ch.Config, ch.Enabled, ch.CreatedAt, ch.UpdatedAt,
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	ch.ID = id
	return nil
}

func ListChannels() ([]model.Channel, error) {
	rows, err := DB.Query(`SELECT id, name, platform, session_id, config, enabled, created_at, updated_at FROM channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Channel
	for rows.Next() {
		var ch model.Channel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Platform, &ch.SessionID, &ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, ch)
	}
	return list, nil
}

func GetChannel(id int64) (*model.Channel, error) {
	var ch model.Channel
	err := DB.QueryRow(`SELECT id, name, platform, session_id, config, enabled, created_at, updated_at FROM channels WHERE id = ?`, id).
		Scan(&ch.ID, &ch.Name, &ch.Platform, &ch.SessionID, &ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func GetChannelByPlatformConfig(platform, key, value string) (*model.Channel, error) {
	rows, err := DB.Query(`SELECT id, name, platform, session_id, config, enabled, created_at, updated_at FROM channels WHERE platform = ? AND enabled = 1`, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ch model.Channel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Platform, &ch.SessionID, &ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			continue
		}
		needle := `"` + key + `":"` + value + `"`
		if strings.Contains(ch.Config, needle) {
			return &ch, nil
		}
	}
	return nil, nil
}

func GetEnabledChannelByPlatform(platform string) (*model.Channel, error) {
	var ch model.Channel
	err := DB.QueryRow(`SELECT id, name, platform, session_id, config, enabled, created_at, updated_at FROM channels WHERE platform = ? AND enabled = 1 LIMIT 1`, platform).
		Scan(&ch.ID, &ch.Name, &ch.Platform, &ch.SessionID, &ch.Config, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func UpdateChannel(ch *model.Channel) error {
	ch.UpdatedAt = time.Now()
	_, err := DB.Exec(
		`UPDATE channels SET name=?, platform=?, session_id=?, config=?, enabled=?, updated_at=? WHERE id=?`,
		ch.Name, ch.Platform, ch.SessionID, ch.Config, ch.Enabled, ch.UpdatedAt, ch.ID,
	)
	return err
}

func DeleteChannel(id int64) error {
	_, err := DB.Exec(`DELETE FROM channels WHERE id = ?`, id)
	return err
}
