package store

import (
	"database/sql"
	"time"
)

// InjectionRoute represents a keyword → categories mapping rule.
type InjectionRoute struct {
	ID               int64  `json:"id"`
	Keywords         string `json:"keywords"`          // pipe-separated: "开发|编程|代码"
	InjectCategories string `json:"inject_categories"` // comma-separated: "domain,lessons"
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// InitInjectionRouterTable creates the injection_router table if not exists.
// Called from migrate().
func InitInjectionRouterTable() {
	DB.Exec(`CREATE TABLE IF NOT EXISTS injection_router (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		keywords TEXT NOT NULL DEFAULT '',
		inject_categories TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
}

// ListInjectionRoutes returns all injection router rules.
func ListInjectionRoutes() ([]InjectionRoute, error) {
	rows, err := DB.Query(`SELECT id, keywords, inject_categories, created_at, updated_at FROM injection_router ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []InjectionRoute
	for rows.Next() {
		var r InjectionRoute
		if err := rows.Scan(&r.ID, &r.Keywords, &r.InjectCategories, &r.CreatedAt, &r.UpdatedAt); err != nil {
			continue
		}
		routes = append(routes, r)
	}
	return routes, nil
}

// GetInjectionRoute returns a single injection route by ID.
func GetInjectionRoute(id int64) (*InjectionRoute, error) {
	var r InjectionRoute
	err := DB.QueryRow(`SELECT id, keywords, inject_categories, created_at, updated_at FROM injection_router WHERE id = ?`, id).
		Scan(&r.ID, &r.Keywords, &r.InjectCategories, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// CreateInjectionRoute inserts a new injection route.
func CreateInjectionRoute(keywords, injectCategories string) (*InjectionRoute, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	result, err := DB.Exec(
		`INSERT INTO injection_router (keywords, inject_categories, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		keywords, injectCategories, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()
	return &InjectionRoute{
		ID:               id,
		Keywords:         keywords,
		InjectCategories: injectCategories,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

// UpdateInjectionRoute updates an existing injection route.
func UpdateInjectionRoute(id int64, keywords, injectCategories string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(
		`UPDATE injection_router SET keywords = ?, inject_categories = ?, updated_at = ? WHERE id = ?`,
		keywords, injectCategories, now, id,
	)
	return err
}

// DeleteInjectionRoute deletes an injection route by ID.
func DeleteInjectionRoute(id int64) error {
	_, err := DB.Exec(`DELETE FROM injection_router WHERE id = ?`, id)
	return err
}
