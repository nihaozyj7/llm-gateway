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
	// 上次进程异常退出时遗留的「等待中/传输中」日志标记为失败,避免前端悬挂中间状态
	_ = s.FinishStaleLogs()
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
			timeout_ms INTEGER NOT NULL DEFAULT 0,
			cooldown_ms INTEGER NOT NULL DEFAULT 0,
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
			price_cache_read REAL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS channel_models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel_id INTEGER NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
			model_id INTEGER NOT NULL REFERENCES models(id) ON DELETE CASCADE,
			upstream_model_name TEXT NOT NULL DEFAULT '',
			priority INTEGER NOT NULL DEFAULT 0,
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
			key_secret TEXT NOT NULL DEFAULT '',
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
			upstream_model TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			latency_ms INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens INTEGER NOT NULL DEFAULT 0,
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
	// 旧库迁移:api_keys 补充 key_secret 列(明文密钥,本地自用可随时查看)
	if err := ensureColumn(s.db, "api_keys", "key_secret", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// 旧库迁移:channels 补充渠道级超时/冷静期列(毫秒,0 = 使用全局配置)
	if err := ensureColumn(s.db, "channels", "timeout_ms", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := ensureColumn(s.db, "channels", "cooldown_ms", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// 旧库迁移:request_logs 补充实际转发模型列(渠道映射后)
	if err := ensureColumn(s.db, "request_logs", "upstream_model", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// 旧库迁移:request_logs 补充调用密钥名称列(用于按密钥筛选日志)
	if err := ensureColumn(s.db, "request_logs", "api_key_name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// 旧库迁移:request_logs 补充首次响应耗时列(毫秒,用于计算输出 token 速度)
	if err := ensureColumn(s.db, "request_logs", "first_response_ms", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// 旧库迁移:request_logs 补充渠道尝试链路列(JSON,概览展示 渠道1(失败)→渠道2(成功) 链路)
	if err := ensureColumn(s.db, "request_logs", "channel_trail", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// 旧库迁移:request_logs 补充流式请求标记列(非流式请求不显示输出速度)
	if err := ensureColumn(s.db, "request_logs", "is_stream", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return s.migrateV1()
}

// migrateV1 一次性迁移(以 PRAGMA user_version 幂等标记,全部包在单个事务内保证原子性):
//  1. models 增加缓存读取单价(price_cache_read);
//  2. request_logs 增加 cache_read_tokens;
//  3. channel_models 增加模型级优先级 priority(0 = 继承渠道全局优先级,可拖拽单独调整);
//  4. 价格单位从「元/千 token」切换为「元/百万 token」,存量价格 ×1000 保持成本等价。
func (s *Store) migrateV1() error {
	var v int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("migrate v1: %w", err)
	}
	if v >= 1 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("migrate v1: %w", err)
	}
	defer tx.Rollback()
	if err := ensureColumn(tx, "models", "price_cache_read", "REAL"); err != nil {
		return fmt.Errorf("migrate v1: %w", err)
	}
	if err := ensureColumn(tx, "request_logs", "cache_read_tokens", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate v1: %w", err)
	}
	if err := ensureColumn(tx, "channel_models", "priority", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("migrate v1: %w", err)
	}
	// 价格单位切换:元/千 → 元/百万(×1000,成本公式同步改为除以 1e6,结果等价)
	if _, err := tx.Exec("UPDATE models SET price_input = price_input * 1000 WHERE price_input IS NOT NULL"); err != nil {
		return fmt.Errorf("migrate v1: %w", err)
	}
	if _, err := tx.Exec("UPDATE models SET price_output = price_output * 1000 WHERE price_output IS NOT NULL"); err != nil {
		return fmt.Errorf("migrate v1: %w", err)
	}
	if _, err := tx.Exec("PRAGMA user_version = 1"); err != nil {
		return fmt.Errorf("migrate v1: %w", err)
	}
	return tx.Commit()
}

// sqlExecer DB 与事务共有的执行接口(用于迁移内统一执行)
type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
}

// ensureColumn 检查列是否存在,不存在则 ALTER TABLE 添加
func ensureColumn(db sqlExecer, table, column, ddl string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + ddl)
	return err
}

// Close 关闭数据库
func (s *Store) Close() error { return s.db.Close() }

func ts(t time.Time) int64     { return t.Unix() }
func fromTS(v int64) time.Time { return time.Unix(v, 0) }
