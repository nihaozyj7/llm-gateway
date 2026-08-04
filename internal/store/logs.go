package store

import (
	"database/sql"
	"time"

	"gateway/internal/model"
)

const logCols = "id, request_time, request_id, channel_id, channel_name, model, status, latency_ms, prompt_tokens, completion_tokens, total_tokens, cache_read_tokens, cost, error, source_ip, payload_request, payload_response"

// InsertLog 插入一条请求日志
func (s *Store) InsertLog(l *model.RequestLog) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO request_logs (request_time, request_id, channel_id, channel_name, model, status, latency_ms,
		prompt_tokens, completion_tokens, total_tokens, cache_read_tokens, cost, error, source_ip, payload_request, payload_response)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts(l.RequestTime), l.RequestID, l.ChannelID, l.ChannelName, l.Model, l.Status, l.LatencyMs,
		l.PromptTokens, l.CompletionTokens, l.TotalTokens, l.CacheReadTokens, l.Cost, l.Error, l.SourceIP,
		l.PayloadRequest, l.PayloadResponse)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListLogs 分页查询日志(支持过滤),返回日志与总数
func (s *Store) ListLogs(channelID *int64, modelFilter string, status string, keyword string, offset, limit int) ([]*model.RequestLog, int64, error) {
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
	err := row.Scan(&l.ID, &rt, &l.RequestID, &l.ChannelID, &l.ChannelName, &l.Model, &l.Status,
		&l.LatencyMs, &l.PromptTokens, &l.CompletionTokens, &l.TotalTokens, &l.CacheReadTokens, &l.Cost, &l.Error, &l.SourceIP,
		&l.PayloadRequest, &l.PayloadResponse)
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
