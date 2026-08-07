package store

import (
	"database/sql"
	"time"

	"gateway/internal/model"
)

const logCols = "id, request_time, request_id, channel_id, channel_name, model, upstream_model, status, is_stream, latency_ms, first_response_ms, prompt_tokens, completion_tokens, total_tokens, cache_read_tokens, cost, error, source_ip, api_key_name, payload_request, payload_response, channel_trail"

// InsertLog 插入一条请求日志
func (s *Store) InsertLog(l *model.RequestLog) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO request_logs (request_time, request_id, channel_id, channel_name, model, upstream_model, status, is_stream, latency_ms,
		first_response_ms, prompt_tokens, completion_tokens, total_tokens, cache_read_tokens, cost, error, source_ip, api_key_name, payload_request, payload_response, channel_trail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts(l.RequestTime), l.RequestID, l.ChannelID, l.ChannelName, l.Model, l.UpstreamModel, l.Status, l.IsStream, l.LatencyMs,
		l.FirstResponseMs, l.PromptTokens, l.CompletionTokens, l.TotalTokens, l.CacheReadTokens, l.Cost, l.Error, l.SourceIP,
		l.APIKeyName, l.PayloadRequest, l.PayloadResponse, l.ChannelTrail)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListLogs 分页查询日志(支持过滤),返回日志与总数
func (s *Store) ListLogs(channelID *int64, modelFilter string, status string, keyName string, keyword string, offset, limit int) ([]*model.RequestLog, int64, error) {
	where := "WHERE 1=1"
	args := []any{}
	if channelID != nil {
		where += " AND channel_id = ?"
		args = append(args, *channelID)
	}
	if modelFilter != "" {
		where += " AND model = ?"
		args = append(args, modelFilter)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if keyName != "" {
		where += " AND api_key_name = ?"
		args = append(args, keyName)
	}
	if keyword != "" {
		where += " AND (request_id LIKE ? OR source_ip LIKE ?)"
		kw := "%" + keyword + "%"
		args = append(args, kw, kw)
	}

	var total int64
	if err := s.db.QueryRow("SELECT COUNT(*) FROM request_logs "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query("SELECT "+logCols+" FROM request_logs "+where+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*model.RequestLog, 0)
	for rows.Next() {
		l, err := scanLog(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// GetLog 单条日志
func (s *Store) GetLog(id int64) (*model.RequestLog, error) {
	row := s.db.QueryRow("SELECT "+logCols+" FROM request_logs WHERE id = ?", id)
	l, err := scanLog(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return l, err
}

func scanLog(row interface{ Scan(...any) error }) (*model.RequestLog, error) {
	var l model.RequestLog
	var rt int64
	err := row.Scan(&l.ID, &rt, &l.RequestID, &l.ChannelID, &l.ChannelName, &l.Model, &l.UpstreamModel, &l.Status,
		&l.IsStream, &l.LatencyMs, &l.FirstResponseMs, &l.PromptTokens, &l.CompletionTokens, &l.TotalTokens, &l.CacheReadTokens, &l.Cost, &l.Error, &l.SourceIP,
		&l.APIKeyName, &l.PayloadRequest, &l.PayloadResponse, &l.ChannelTrail)
	if err != nil {
		return nil, err
	}
	l.RequestTime = fromTS(rt)
	return &l, nil
}

// PruneLogs 清理超过 N 天的日志
func (s *Store) PruneLogs(days int) error {
	cutoff := ts(time.Now().AddDate(0, 0, -days))
	_, err := s.db.Exec("DELETE FROM request_logs WHERE request_time < ?", cutoff)
	return err
}

// ClearLogs 清空全部请求日志(不涉及统计聚合表)
func (s *Store) ClearLogs() error {
	_, err := s.db.Exec("DELETE FROM request_logs")
	return err
}

// Vacuum 回收数据库文件空间:SQLite 的 DELETE 只标记删除,文件页不会自动释放;
// VACUUM 重建数据库文件,把已删除行占用的空闲页真正归还给文件系统。
func (s *Store) Vacuum() error {
	_, err := s.db.Exec("VACUUM")
	return err
}
