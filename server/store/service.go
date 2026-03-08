package store

import (
	"ai-hub/server/model"
	"time"
)

func CreateService(s *model.Service) error {
	s.Status = "stopped"
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	res, err := DB.Exec(
		`INSERT INTO services (name, command, work_dir, port, log_path, pid, status, auto_start, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`,
		s.Name, s.Command, s.WorkDir, s.Port, s.LogPath, s.Status, s.AutoStart, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return err
	}
	s.ID, _ = res.LastInsertId()
	return nil
}

func GetService(id int64) (*model.Service, error) {
	var s model.Service
	err := DB.QueryRow(
		`SELECT id, name, command, work_dir, port, log_path, pid, status, auto_start, created_at, updated_at
		 FROM services WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.Command, &s.WorkDir, &s.Port, &s.LogPath, &s.PID, &s.Status, &s.AutoStart, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func GetServiceByName(name string) (*model.Service, error) {
	var s model.Service
	err := DB.QueryRow(
		`SELECT id, name, command, work_dir, port, log_path, pid, status, auto_start, created_at, updated_at
		 FROM services WHERE name = ?`, name,
	).Scan(&s.ID, &s.Name, &s.Command, &s.WorkDir, &s.Port, &s.LogPath, &s.PID, &s.Status, &s.AutoStart, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func ListServices() ([]model.Service, error) {
	rows, err := DB.Query(
		`SELECT id, name, command, work_dir, port, log_path, pid, status, auto_start, created_at, updated_at
		 FROM services ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Service
	for rows.Next() {
		var s model.Service
		if err := rows.Scan(&s.ID, &s.Name, &s.Command, &s.WorkDir, &s.Port, &s.LogPath, &s.PID, &s.Status, &s.AutoStart, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

func UpdateService(s *model.Service) error {
	s.UpdatedAt = time.Now()
	_, err := DB.Exec(
		`UPDATE services SET name=?, command=?, work_dir=?, port=?, log_path=?, auto_start=?, updated_at=? WHERE id=?`,
		s.Name, s.Command, s.WorkDir, s.Port, s.LogPath, s.AutoStart, s.UpdatedAt, s.ID,
	)
	return err
}

func UpdateServiceStatus(id int64, status string, pid int) error {
	_, err := DB.Exec(
		`UPDATE services SET status=?, pid=?, updated_at=? WHERE id=?`,
		status, pid, time.Now(), id,
	)
	return err
}

func DeleteService(id int64) error {
	_, err := DB.Exec(`DELETE FROM services WHERE id = ?`, id)
	return err
}
