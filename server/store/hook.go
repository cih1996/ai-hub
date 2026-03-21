package store

import (
	"ai-hub/server/model"
	"time"
)

// InitHooksTable creates the hooks table (called from migrate).
func InitHooksTable() {
	DB.Exec(`CREATE TABLE IF NOT EXISTS hooks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event TEXT NOT NULL DEFAULT '',
		condition TEXT NOT NULL DEFAULT '',
		target_session INTEGER NOT NULL DEFAULT 0,
		payload TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		fired_count INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_hooks_event ON hooks(event)`)
}

func CreateHook(h *model.Hook) error {
	n := now()
	h.CreatedAt = n
	h.UpdatedAt = n
	if !h.Enabled {
		h.Enabled = true
	}
	result, err := DB.Exec(
		`INSERT INTO hooks (event, condition, target_session, payload, enabled, fired_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		h.Event, h.Condition, h.TargetSession, h.Payload, h.Enabled, h.FiredCount, h.CreatedAt, h.UpdatedAt,
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	h.ID = id
	return nil
}

func ListHooks() ([]model.Hook, error) {
	rows, err := DB.Query(`SELECT id, event, condition, target_session, payload, enabled, fired_count, created_at, updated_at FROM hooks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Hook
	for rows.Next() {
		var h model.Hook
		if err := rows.Scan(&h.ID, &h.Event, &h.Condition, &h.TargetSession, &h.Payload, &h.Enabled, &h.FiredCount, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, nil
}

func ListHooksByEvent(event string) ([]model.Hook, error) {
	rows, err := DB.Query(`SELECT id, event, condition, target_session, payload, enabled, fired_count, created_at, updated_at FROM hooks WHERE event = ? AND enabled = 1 ORDER BY id`, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Hook
	for rows.Next() {
		var h model.Hook
		if err := rows.Scan(&h.ID, &h.Event, &h.Condition, &h.TargetSession, &h.Payload, &h.Enabled, &h.FiredCount, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, nil
}

func GetHook(id int64) (*model.Hook, error) {
	var h model.Hook
	err := DB.QueryRow(`SELECT id, event, condition, target_session, payload, enabled, fired_count, created_at, updated_at FROM hooks WHERE id = ?`, id).
		Scan(&h.ID, &h.Event, &h.Condition, &h.TargetSession, &h.Payload, &h.Enabled, &h.FiredCount, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func UpdateHook(h *model.Hook) error {
	h.UpdatedAt = time.Now().In(time.FixedZone("CST", 8*3600)).Format(triggerTimeLayout)
	_, err := DB.Exec(
		`UPDATE hooks SET event=?, condition=?, target_session=?, payload=?, enabled=?, fired_count=?, updated_at=? WHERE id=?`,
		h.Event, h.Condition, h.TargetSession, h.Payload, h.Enabled, h.FiredCount, h.UpdatedAt, h.ID,
	)
	return err
}

func DeleteHook(id int64) error {
	_, err := DB.Exec(`DELETE FROM hooks WHERE id = ?`, id)
	return err
}

func IncrementHookFiredCount(id int64) error {
	_, err := DB.Exec(`UPDATE hooks SET fired_count = fired_count + 1, updated_at = ? WHERE id = ?`, now(), id)
	return err
}
