package store

import (
	"database/sql"
	"time"

	"gateway/internal/model"
)

const apiKeyCols = "id, name, key_hash, key_prefix, key_secret, enabled, usage_count, created_at, last_used_at"

func scanAPIKey(row interface{ Scan(...any) error }) (*model.APIKey, error) {
	var k model.APIKey
	var enabled, createdAt, lastUsed int64
	err := row.Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.KeySecret, &enabled, &k.UsageCount, &createdAt, &lastUsed)
	if err != nil {
		return nil, err
	}
	k.Enabled = enabled == 1
	k.CreatedAt = fromTS(createdAt)
	k.LastUsedAt = fromTS(lastUsed)
	return &k, nil
}

// ListAPIKeys 列出全部密钥
func (s *Store) ListAPIKeys() ([]*model.APIKey, error) {
	rows, err := s.db.Query("SELECT " + apiKeyCols + " FROM api_keys ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.APIKey, 0)
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CreateAPIKey 创建密钥
func (s *Store) CreateAPIKey(name, hash, prefix, secret string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO api_keys (name, key_hash, key_prefix, key_secret, enabled, usage_count, created_at) VALUES (?, ?, ?, ?, 1, 0, ?)`,
		name, hash, prefix, secret, ts(time.Now()))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateAPIKeyEnabled 启用/停用
func (s *Store) UpdateAPIKeyEnabled(id int64, enabled bool) error {
	_, err := s.db.Exec("UPDATE api_keys SET enabled=? WHERE id=?", boolInt(enabled), id)
	return err
}

// DeleteAPIKey 删除密钥
func (s *Store) DeleteAPIKey(id int64) error {
	_, err := s.db.Exec("DELETE FROM api_keys WHERE id=?", id)
	return err
}

// GetAPIKeyByHash 按哈希查找密钥(用于网关鉴权),并更新最后使用时间
func (s *Store) GetAPIKeyByHash(hash string) (*model.APIKey, error) {
	row := s.db.QueryRow("SELECT "+apiKeyCols+" FROM api_keys WHERE key_hash = ?", hash)
	k, err := scanAPIKey(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return k, err
}

// TouchAPIKey 更新使用计数与最后使用时间
func (s *Store) TouchAPIKey(id int64) error {
	_, err := s.db.Exec("UPDATE api_keys SET usage_count = usage_count + 1, last_used_at = ? WHERE id=?", ts(time.Now()), id)
	return err
}
