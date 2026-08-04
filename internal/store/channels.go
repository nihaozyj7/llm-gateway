package store

import (
	"database/sql"
	"fmt"
	"time"

	"gateway/internal/model"
)

const channelCols = "id, name, base_url, api_key, auth_header, priority, enabled, timeout_ms, cooldown_ms, status, cooldown_until, failure_count, last_error, created_at, updated_at"

func scanChannel(row interface{ Scan(...any) error }) (*model.Channel, error) {
	var c model.Channel
	var enabled, cooldownUntil, createdAt, updatedAt int64
	err := row.Scan(&c.ID, &c.Name, &c.BaseURL, &c.APIKey, &c.AuthHeader, &c.Priority,
		&enabled, &c.TimeoutMs, &c.CooldownMs, &c.Status, &cooldownUntil, &c.FailureCount, &c.LastError, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled == 1
	c.CooldownUntil = fromTS(cooldownUntil)
	c.CreatedAt = fromTS(createdAt)
	c.UpdatedAt = fromTS(updatedAt)
	return &c, nil
}

// ListChannels 列出全部渠道(按优先级排序)
func (s *Store) ListChannels() ([]*model.Channel, error) {
	rows, err := s.db.Query("SELECT " + channelCols + " FROM channels ORDER BY priority ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.Channel, 0)
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetChannel 按 ID 取渠道
func (s *Store) GetChannel(id int64) (*model.Channel, error) {
	row := s.db.QueryRow("SELECT "+channelCols+" FROM channels WHERE id = ?", id)
	c, err := scanChannel(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// CreateChannel 新建渠道
func (s *Store) CreateChannel(c *model.Channel) (int64, error) {
	now := time.Now()
	res, err := s.db.Exec(`INSERT INTO channels (name, base_url, api_key, auth_header, priority, enabled, timeout_ms, cooldown_ms, status, cooldown_until, failure_count, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'normal', 0, 0, '', ?, ?)`,
		c.Name, c.BaseURL, c.APIKey, c.AuthHeader, c.Priority, boolInt(c.Enabled), c.TimeoutMs, c.CooldownMs, ts(now), ts(now))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateChannel 更新渠道
func (s *Store) UpdateChannel(c *model.Channel) error {
	_, err := s.db.Exec(`UPDATE channels SET name=?, base_url=?, api_key=?, auth_header=?, priority=?, enabled=?, timeout_ms=?, cooldown_ms=?, updated_at=?
		WHERE id=?`,
		c.Name, c.BaseURL, c.APIKey, c.AuthHeader, c.Priority, boolInt(c.Enabled), c.TimeoutMs, c.CooldownMs, ts(time.Now()), c.ID)
	return err
}

// ChannelPriority 优先级批量调整项
type ChannelPriority struct {
	ID       int64 `json:"id"`
	Priority int   `json:"priority"`
}

// ReorderChannels 批量更新渠道优先级(拖拽排序保存)。
// 要求:priority >= 1,items 内不允许重复渠道 ID。
func (s *Store) ReorderChannels(items []ChannelPriority) error {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(items))
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, it := range items {
		if it.ID <= 0 {
			return fmt.Errorf("invalid channel id %d", it.ID)
		}
		if it.Priority < 1 {
			return fmt.Errorf("priority 必须 >= 1(渠道 %d)", it.ID)
		}
		if _, dup := seen[it.ID]; dup {
			return fmt.Errorf("重复的渠道 id %d", it.ID)
		}
		seen[it.ID] = struct{}{}
		if _, err := tx.Exec("UPDATE channels SET priority=?, updated_at=? WHERE id=?", it.Priority, ts(time.Now()), it.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteChannel 删除渠道(级联删除 channel_models)
func (s *Store) DeleteChannel(id int64) error {
	_, err := s.db.Exec("DELETE FROM channels WHERE id = ?", id)
	return err
}

// SetChannelStatus 设置渠道状态(冷静/正常)与冷静截止时间
func (s *Store) SetChannelStatus(id int64, status string, cooldownUntil time.Time, lastError string) error {
	_, err := s.db.Exec("UPDATE channels SET status=?, cooldown_until=?, last_error=?, updated_at=? WHERE id=?",
		status, ts(cooldownUntil), lastError, ts(time.Now()), id)
	return err
}

// ResetChannelFailure 重置失败计数并恢复正常
func (s *Store) ResetChannelFailure(id int64) error {
	_, err := s.db.Exec("UPDATE channels SET status='normal', cooldown_until=0, failure_count=0, last_error='', updated_at=? WHERE id=?",
		ts(time.Now()), id)
	return err
}

// IncrementChannelFailure 失败计数 +1,返回新计数
func (s *Store) IncrementChannelFailure(id int64, errMsg string) (int, error) {
	res, err := s.db.Exec("UPDATE channels SET failure_count = failure_count + 1, last_error=?, updated_at=? WHERE id=?",
		errMsg, ts(time.Now()), id)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, fmt.Errorf("channel %d not found", id)
	}
	var count int
	if err := s.db.QueryRow("SELECT failure_count FROM channels WHERE id = ?", id).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ClearChannelFailure 请求成功时清零该渠道失败计数。
// 仅在 failure_count > 0 时执行,避免每次成功请求都无谓写库。
// 这样 cooldown_threshold 的语义为"连续失败 N 次才冷却",中间有成功则重新计数。
func (s *Store) ClearChannelFailure(id int64) error {
	_, err := s.db.Exec("UPDATE channels SET failure_count=0, last_error='', updated_at=? WHERE id=? AND failure_count > 0",
		ts(time.Now()), id)
	return err
}

// RefreshCoolDowns 把已过期的冷静渠道恢复正常(启动与路由时惰性调用)
func (s *Store) RefreshCoolDowns() error {
	_, err := s.db.Exec("UPDATE channels SET status='normal', failure_count=0 WHERE status='cooldown' AND cooldown_until > 0 AND cooldown_until <= ?", ts(time.Now()))
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
