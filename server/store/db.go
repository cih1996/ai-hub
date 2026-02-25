package store

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func Init(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}
	dbPath := filepath.Join(dataDir, "ai-hub.db")
	var err error
	DB, err = sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return err
	}
	return migrate()
}

func migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS providers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'openai-compatible',
		base_url TEXT NOT NULL DEFAULT '',
		api_key TEXT NOT NULL DEFAULT '',
		model_id TEXT NOT NULL DEFAULT '',
		is_default INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL DEFAULT 'New Chat',
		provider_id TEXT NOT NULL DEFAULT '',
		claude_session_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);
	`
	_, err := DB.Exec(schema)
	if err != nil {
		return err
	}
	// Safe column migration: add work_dir to sessions (SQLite ignores duplicate ALTER)
	DB.Exec(`ALTER TABLE sessions ADD COLUMN work_dir TEXT NOT NULL DEFAULT ''`)

	// Messages: add metadata column
	DB.Exec(`ALTER TABLE messages ADD COLUMN metadata TEXT NOT NULL DEFAULT ''`)

	// Sessions: add group_name column
	DB.Exec(`ALTER TABLE sessions ADD COLUMN group_name TEXT NOT NULL DEFAULT ''`)

	// Triggers table
	DB.Exec(`CREATE TABLE IF NOT EXISTS triggers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		trigger_time TEXT NOT NULL DEFAULT '',
		max_fires INTEGER NOT NULL DEFAULT 1,
		enabled INTEGER NOT NULL DEFAULT 1,
		fired_count INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'active',
		next_fire_at TEXT NOT NULL DEFAULT '',
		last_fired_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_triggers_session ON triggers(session_id)`)

	// Token usage table
	DB.Exec(`CREATE TABLE IF NOT EXISTS token_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		message_id INTEGER DEFAULT 0,
		input_tokens INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_token_usage_session ON token_usage(session_id)`)
	DB.Exec(`CREATE INDEX IF NOT EXISTS idx_token_usage_created ON token_usage(created_at)`)

	// Safe column migration: add cache token columns
	DB.Exec(`ALTER TABLE token_usage ADD COLUMN cache_creation_input_tokens INTEGER NOT NULL DEFAULT 0`)
	DB.Exec(`ALTER TABLE token_usage ADD COLUMN cache_read_input_tokens INTEGER NOT NULL DEFAULT 0`)

	// Channels table (IM gateway)
	DB.Exec(`CREATE TABLE IF NOT EXISTS channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL DEFAULT '',
		platform TEXT NOT NULL DEFAULT '',
		session_id INTEGER DEFAULT 0,
		config TEXT NOT NULL DEFAULT '{}',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)

	// Provider: add auth_mode column (api_key | oauth)
	DB.Exec(`ALTER TABLE providers ADD COLUMN auth_mode TEXT NOT NULL DEFAULT 'api_key'`)

	return nil
}

func Close() {
	if DB != nil {
		DB.Close()
	}
}
