package store

import (
	"database/sql"
	"time"

	"gateway/internal/model"
)

// UpsertStat 累加统计(日/小时)。period 形如 "2025-05-20" 或 "2025-05-20T14"
func (s *Store) UpsertStat(table, period string, l *model.RequestLog) error {
	success, fail, biz := 0, 0, 0
	switch l.Status {
	case "success", "retry_success":
		success = 1
	case "fail":
		fail = 1
	case "biz_error":
		biz = 1
	}
	q := `INSERT INTO ` + table + ` (period, channel_id, channel_name, model, request_count, success_count, fail_count, biz_error_count,
		total_latency_ms, prompt_tokens, completion_tokens, total_tokens, cost)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(period, channel_id, model) DO UPDATE SET
		channel_name=excluded.channel_name,
		request_count = request_count + 1,
		success_count = success_count + excluded.success_count,
		fail_count = fail_count + excluded.fail_count,
		biz_error_count = biz_error_count + excluded.biz_error_count,
		total_latency_ms = total_latency_ms + excluded.total_latency_ms,
		prompt_tokens = prompt_tokens + excluded.prompt_tokens,
		completion_tokens = completion_tokens + excluded.completion_tokens,
		total_tokens = total_tokens + excluded.total_tokens,
		cost = cost + excluded.cost`
	_, err := s.db.Exec(q, period, l.ChannelID, l.ChannelName, l.Model, success, fail, biz,
		l.LatencyMs, l.PromptTokens, l.CompletionTokens, l.TotalTokens, l.Cost)
	return err
}

// RecordStat 同时更新日/小时聚合
func (s *Store) RecordStat(l *model.RequestLog) error {
	day := l.RequestTime.Format("2006-01-02")
	hour := l.RequestTime.Format("2006-01-02T15")
	if err := s.UpsertStat("stat_daily", day, l); err != nil {
		return err
	}
	return s.UpsertStat("stat_hourly", hour, l)
}

// QueryStat 查询聚合统计
// table: stat_daily / stat_hourly
// groupBy: "" | "channel" | "model" | "all"(按维度汇总)
// periodPrefix: 日="2006-01-02" 前缀,小时="2006-01-02T15" 前缀;空=全部
func (s *Store) QueryStat(table, periodPrefix, groupBy string, channelID int64, modelFilter string) ([]*model.StatRow, error) {
	where := "WHERE 1=1"
	args := []any{}
	if periodPrefix != "" {
		where += " AND period LIKE ?"
		args = append(args, periodPrefix+"%")
	}
	if channelID > 0 {
		where += " AND channel_id = ?"
		args = append(args, channelID)
	}
	if modelFilter != "" {
		where += " AND model = ?"
		args = append(args, modelFilter)
	}

	sel := "SUM(request_count) AS request_count, SUM(success_count) AS success_count, SUM(fail_count) AS fail_count, SUM(biz_error_count) AS biz_error_count, SUM(total_latency_ms) AS total_latency_ms, SUM(prompt_tokens) AS prompt_tokens, SUM(completion_tokens) AS completion_tokens, SUM(total_tokens) AS total_tokens, SUM(cost) AS cost"

	var query string
	switch groupBy {
	case "channel":
		query = "SELECT channel_id, MAX(channel_name) AS channel_name, '' AS model, " + sel + " FROM " + table + " " + where + " GROUP BY channel_id ORDER BY request_count DESC"
	case "model":
		query = "SELECT 0 AS channel_id, '' AS channel_name, model, " + sel + " FROM " + table + " " + where + " GROUP BY model ORDER BY request_count DESC"
	default:
		// 按时间序列
		query = "SELECT 0 AS channel_id, '' AS channel_name, period AS model, " + sel + " FROM " + table + " " + where + " GROUP BY period ORDER BY period ASC"
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.StatRow, 0)
	for rows.Next() {
		var r model.StatRow
		if err := rows.Scan(&r.ChannelID, &r.ChannelName, &r.Model, &r.RequestCount, &r.SuccessCount,
			&r.FailCount, &r.BizErrorCount, &r.TotalLatencyMs, &r.PromptTokens, &r.CompletionTokens,
			&r.TotalTokens, &r.Cost); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// Summary 汇总指标(用于 Dashboard 顶部卡片)
type Summary struct {
	RequestCount int64   `json:"request_count"`
	SuccessCount int64   `json:"success_count"`
	FailCount    int64   `json:"fail_count"`
	TotalTokens  int64   `json:"total_tokens"`
	Cost         float64 `json:"cost"`
}

// Summarize 汇总指定时间段(periodPrefix 为空=全部)
func (s *Store) Summarize(table, periodPrefix string) (*Summary, error) {	where := ""
	args := []any{}
	if periodPrefix != "" {
		where = "WHERE period LIKE ?"
		args = append(args, periodPrefix+"%")
	}
	var sum Summary
	err := s.db.QueryRow(`SELECT COALESCE(SUM(request_count),0), COALESCE(SUM(success_count),0), COALESCE(SUM(fail_count),0),
		COALESCE(SUM(total_tokens),0), COALESCE(SUM(cost),0) FROM `+table+` `+where, args...).
		Scan(&sum.RequestCount, &sum.SuccessCount, &sum.FailCount, &sum.TotalTokens, &sum.Cost)
	if err == sql.ErrNoRows {
		return &sum, nil
	}
	return &sum, err
}

// ResetStats 清空统计聚合表(不删请求日志;供概览「重置」按钮使用)
func (s *Store) ResetStats() error {
	if _, err := s.db.Exec("DELETE FROM stat_daily"); err != nil {
		return err
	}
	_, err := s.db.Exec("DELETE FROM stat_hourly")
	return err
}

// _ 占位:防止未使用导入
var _ = time.Now
