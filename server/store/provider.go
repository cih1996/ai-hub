package store

import (
	"ai-hub/server/model"
	"time"

	"github.com/google/uuid"
)

func CreateProvider(p *model.Provider) error {
	p.ID = uuid.New().String()
	p.Mode = p.DetectMode()
	if p.AuthMode == "" {
		p.AuthMode = "api_key"
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := DB.Exec(
		`INSERT INTO providers (id, name, type, base_url, api_key, model_id, is_default, auth_mode, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Mode, p.BaseURL, p.APIKey, p.ModelID, boolToInt(p.IsDefault), p.AuthMode, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func ListProviders() ([]model.Provider, error) {
	rows, err := DB.Query(`SELECT id, name, type, base_url, api_key, model_id, is_default, auth_mode, created_at, updated_at FROM providers ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Provider
	for rows.Next() {
		var p model.Provider
		var def int
		if err := rows.Scan(&p.ID, &p.Name, &p.Mode, &p.BaseURL, &p.APIKey, &p.ModelID, &def, &p.AuthMode, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.Mode = p.DetectMode()
		p.IsDefault = def == 1
		list = append(list, p)
	}
	return list, nil
}

func GetProvider(id string) (*model.Provider, error) {
	var p model.Provider
	var def int
	err := DB.QueryRow(
		`SELECT id, name, type, base_url, api_key, model_id, is_default, auth_mode, created_at, updated_at FROM providers WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.Mode, &p.BaseURL, &p.APIKey, &p.ModelID, &def, &p.AuthMode, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.Mode = p.DetectMode()
	p.IsDefault = def == 1
	return &p, nil
}

func UpdateProvider(p *model.Provider) error {
	p.Mode = p.DetectMode()
	if p.AuthMode == "" {
		p.AuthMode = "api_key"
	}
	p.UpdatedAt = time.Now()
	_, err := DB.Exec(
		`UPDATE providers SET name=?, type=?, base_url=?, api_key=?, model_id=?, is_default=?, auth_mode=?, updated_at=? WHERE id=?`,
		p.Name, p.Mode, p.BaseURL, p.APIKey, p.ModelID, boolToInt(p.IsDefault), p.AuthMode, p.UpdatedAt, p.ID,
	)
	return err
}

func DeleteProvider(id string) error {
	_, err := DB.Exec(`DELETE FROM providers WHERE id = ?`, id)
	return err
}

func GetDefaultProvider() (*model.Provider, error) {
	var p model.Provider
	var def int
	// Try is_default=1 first, fallback to first provider
	err := DB.QueryRow(
		`SELECT id, name, type, base_url, api_key, model_id, is_default, auth_mode, created_at, updated_at
		 FROM providers ORDER BY is_default DESC, created_at ASC LIMIT 1`,
	).Scan(&p.ID, &p.Name, &p.Mode, &p.BaseURL, &p.APIKey, &p.ModelID, &def, &p.AuthMode, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.Mode = p.DetectMode()
	p.IsDefault = def == 1
	return &p, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
