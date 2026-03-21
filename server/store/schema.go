package store

import (
	"ai-hub/server/model"
	"database/sql"
)

// CreateSchema inserts a new JSON Schema definition.
func CreateSchema(s *model.Schema) error {
	result, err := DB.Exec(
		`INSERT INTO schemas (name, definition) VALUES (?, ?)`,
		s.Name, s.Definition,
	)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	s.ID = id
	// Read back timestamps
	row := DB.QueryRow(`SELECT created_at, updated_at FROM schemas WHERE id = ?`, id)
	row.Scan(&s.CreatedAt, &s.UpdatedAt)
	return nil
}

// GetSchema retrieves a schema by name.
func GetSchema(name string) (*model.Schema, error) {
	var s model.Schema
	err := DB.QueryRow(
		`SELECT id, name, definition, created_at, updated_at FROM schemas WHERE name = ?`, name,
	).Scan(&s.ID, &s.Name, &s.Definition, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

// ListSchemas returns all schema definitions.
func ListSchemas() ([]model.Schema, error) {
	rows, err := DB.Query(`SELECT id, name, definition, created_at, updated_at FROM schemas ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []model.Schema
	for rows.Next() {
		var s model.Schema
		if err := rows.Scan(&s.ID, &s.Name, &s.Definition, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

// DeleteSchema removes a schema by name.
func DeleteSchema(name string) error {
	_, err := DB.Exec(`DELETE FROM schemas WHERE name = ?`, name)
	return err
}

// UpdateSchema updates a schema's definition by name (read-then-merge pattern).
func UpdateSchema(name, definition string) (*model.Schema, error) {
	_, err := DB.Exec(
		`UPDATE schemas SET definition = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?`,
		definition, name,
	)
	if err != nil {
		return nil, err
	}
	return GetSchema(name)
}
