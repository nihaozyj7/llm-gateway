package store

import (
	"database/sql"
	"time"

	"gateway/internal/model"
)

const logCols = "id, request_time, request_id, channel_id, channel_name, model, upstream_model, status, is_stream, latency_ms, first_response_ms, prompt_tokens, completion_tokens, total_tokens, cache_read_tokens, cost, error, source_ip, api_key_name, payload_request, payload_response, channel_trail"

// InsertLog 插入一条请求日志(一次性写入,旧逻辑)
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

// InsertPendingLog 请求到达时插入一条「等待中」日志并返回其 id;
// 请求结束时用 UpdateLogFinal 更新同一条记录,实现实时状态流转
// (等待中 → 传输中(流式首包) → 成功/失败/客户端断开/业务错误)。
// 字段:仅插入请求阶段已知的信息,其余待最终更新。
func (s *Store) InsertPendingLog(l *model.RequestLog) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO request_logs (request_time, request_id, api_key_name, model, source_ip, payload_request, is_stream, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')`,
		ts(l.RequestTime), l.RequestID, l.APIKeyName, l.Model, l.SourceIP, l.PayloadRequest, l.IsStream)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateLogStatus 更新进行中日志的状态(流式请求收到首包后置为 streaming)
func (s *Store) UpdateLogStatus(id int64, status string) error {
	_, err := s.db.Exec("UPDATE request_logs SET status = ? WHERE id = ?", status, id)
	return err
}

// UpdateLogFinal 请求结束时用最终结果更新同一条日志记录(由 InsertPendingLog 创建的 id)。
// id <= 0 时跳过(未成功创建等待日志时保持旧行为,直接插入完整记录)。
func (s *Store) UpdateLogFinal(l *model.RequestLog) error {
	if l.ID <= 0 {
		_, err := s.InsertLog(l)
		return err
	}
	_, err := s.db.Exec(`UPDATE request_logs SET
		channel_id = ?, channel_name = ?, model = ?, upstream_model = ?, status = ?, latency_ms = ?,
		first_response_ms = ?, prompt_tokens = ?, completion_tokens = ?, total_tokens = ?, cache_read_tokens = ?,
		cost = ?, error = ?, payload_request = ?, payload_response = ?, channel_trail = ? WHERE id = ?`,
		l.ChannelID, l.ChannelName, l.Model, l.UpstreamModel, l.Status, l.LatencyMs,
		l.FirstResponseMs, l.PromptTokens, l.CompletionTokens, l.TotalTokens, l.CacheReadTokens,
		l.Cost, l.Error, l.PayloadRequest, l.PayloadResponse, l.ChannelTrail, l.ID)
	return err
}

// FinishStaleLogs 网关启动时把上次进程残留的进行中日志(等待中/传输中)标记为失败,
// 避免异常退出(断电/强杀)后前端长期悬挂在中间状态。
func (s *Store) FinishStaleLogs() error {
	_, err := s.db.Exec(`UPDATE request_logs SET status = 'fail', error = '进程中断,请求未完成' WHERE status IN ('pending', 'streaming')`)
	return err
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
