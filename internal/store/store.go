package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store 数据访问层
type Store struct {
	db *sql.DB
}

// Open 打开 SQLite 数据库并执行迁移
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(0)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS channels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			auth_header TEXT NOT NULL DEFAULT 'Authorization',
			priority INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'normal',
			cooldown_until INTEGER NOT NULL DEFAULT 0,
			failure_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model_id TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL DEFAULT '',
			price_input REAL,
			price_output REAL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS channel_models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
			model_id INTEGER NOT NULL REFERENCES models(id) ON DELETE CASCADE,
			upstream_model_name TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			UNIQUE(channel_id, model_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cm_channel ON channel_models(channel_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cm_model ON channel_models(model_id)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_prefix TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			usage_count INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			last_used_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS admins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_time INTEGER NOT NULL,
			request_id TEXT NOT NULL DEFAULT '',
			channel_id INTEGER NOT NULL DEFAULT 0,
			channel_name TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			latency_ms INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			source_ip TEXT NOT NULL DEFAULT '',
			payload_request TEXT NOT NULL DEFAULT '',
			payload_response TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_time ON request_logs(request_time)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_channel ON request_logs(channel_id)`,
		`CREATE INDEX IF NOT EXISTS idx_logs_model ON request_logs(model)`,
		`CREATE TABLE IF NOT EXISTS stat_daily (
			period TEXT NOT NULL,
			channel_id INTEGER NOT NULL DEFAULT 0,
			channel_name TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			request_count INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			fail_count INTEGER NOT NULL DEFAULT 0,
			biz_error_count INTEGER NOT NULL DEFAULT 0,
			total_latency_ms INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			PRIMARY KEY (period, channel_id, model)
		)`,
		`CREATE TABLE IF NOT EXISTS stat_hourly (
			period TEXT NOT NULL,
			channel_id INTEGER NOT NULL DEFAULT 0,
			channel_name TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			request_count INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			fail_count INTEGER NOT NULL DEFAULT 0,
			biz_error_count INTEGER NOT NULL DEFAULT 0,
			total_latency_ms INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			PRIMARY KEY (period, channel_id, model)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// Close 关闭数据库
func (s *Store) Close() error { return s.db.Close() }

func ts(t time.Time) int64     { return t.Unix() }
func fromTS(v int64) time.Time { return time.Unix(v, 0) }
