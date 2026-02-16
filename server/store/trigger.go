package store

import (
	"ai-hub/server/model"
	"time"
)

const triggerTimeLayout = "2006-01-02 15:04:05"

func now() string {
	return time.Now().In(time.FixedZone("CST", 8*3600)).Format(triggerTimeLayout)
}

func CreateTrigger(t *model.Trigger) error {
	t.CreatedAt = now()
	t.UpdatedAt = t.CreatedAt
	if t.Status == "" {
		t.Status = "active"
	}
	result, err := DB.Exec(
		`INSERT INTO triggers (session_id, content, trigger_time, max_fires, enabled, fired_count, status, next_fire_at, last_fired_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.SessionID, t.Content, t.TriggerTime, t.MaxFires, t.Enabled, t.FiredCount, t.Status, t.NextFireAt, t.LastFiredAt, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	t.ID = id
	return nil
}

func ListTriggers() ([]model.Trigger, error) {
	rows, err := DB.Query(`SELECT id, session_id, content, trigger_time, max_fires, enabled, fired_count, status, next_fire_at, last_fired_at, created_at, updated_at FROM triggers ORDER BY session_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Trigger
	for rows.Next() {
		var t model.Trigger
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Content, &t.TriggerTime, &t.MaxFires, &t.Enabled, &t.FiredCount, &t.Status, &t.NextFireAt, &t.LastFiredAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

func ListTriggersBySession(sessionID int64) ([]model.Trigger, error) {
	rows, err := DB.Query(`SELECT id, session_id, content, trigger_time, max_fires, enabled, fired_count, status, next_fire_at, last_fired_at, created_at, updated_at FROM triggers WHERE session_id = ? ORDER BY id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Trigger
	for rows.Next() {
		var t model.Trigger
		if err := rows.Scan(&t.ID, &t.SessionID, &t.Content, &t.TriggerTime, &t.MaxFires, &t.Enabled, &t.FiredCount, &t.Status, &t.NextFireAt, &t.LastFiredAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

func GetTrigger(id int64) (*model.Trigger, error) {
	var t model.Trigger
	err := DB.QueryRow(`SELECT id, session_id, content, trigger_time, max_fires, enabled, fired_count, status, next_fire_at, last_fired_at, created_at, updated_at FROM triggers WHERE id = ?`, id).
		Scan(&t.ID, &t.SessionID, &t.Content, &t.TriggerTime, &t.MaxFires, &t.Enabled, &t.FiredCount, &t.Status, &t.NextFireAt, &t.LastFiredAt, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func UpdateTrigger(t *model.Trigger) error {
	t.UpdatedAt = now()
	_, err := DB.Exec(
		`UPDATE triggers SET session_id=?, content=?, trigger_time=?, max_fires=?, enabled=?, fired_count=?, status=?, next_fire_at=?, last_fired_at=?, updated_at=? WHERE id=?`,
		t.SessionID, t.Content, t.TriggerTime, t.MaxFires, t.Enabled, t.FiredCount, t.Status, t.NextFireAt, t.LastFiredAt, t.UpdatedAt, t.ID,
	)
	return err
}

func DeleteTrigger(id int64) error {
	_, err := DB.Exec(`DELETE FROM triggers WHERE id = ?`, id)
	return err
}

// SessionsWithTriggers 返回拥有触发器的会话ID集合
func SessionsWithTriggers() (map[int64]bool, error) {
	rows, err := DB.Query(`SELECT DISTINCT session_id FROM triggers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[int64]bool)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		m[id] = true
	}
	return m, nil
}
