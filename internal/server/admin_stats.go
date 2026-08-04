package server

import (
	"net/http"
	"strconv"
)

// GET /api/admin/stats?table=daily|hourly&period=2025-05-20&group_by=channel|model|time&channel_id=&model=
func (h *AdminHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	q := r.URL.Query()
	table := q.Get("table")
	if table != "stat_hourly" {
		table = "stat_daily"
	}
	groupBy := q.Get("group_by")
	if groupBy != "channel" && groupBy != "model" && groupBy != "time" {
		groupBy = "time"
	}
	channelID, _ := strconv.ParseInt(q.Get("channel_id"), 10, 64)
	rows, err := h.store.QueryStat(table, q.Get("period"), groupBy, channelID, q.Get("model"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// POST /api/admin/stats/reset — 清空统计聚合数据(仅统计数字,保留请求日志)
func (h *AdminHandler) handleStatsReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if err := h.store.ResetStats(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /api/admin/dashboard?period=2025-05-20(可空=全部)
func (h *AdminHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	period := r.URL.Query().Get("period")
	sum, err := h.store.Summarize("stat_daily", period)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// 渠道冷静状态概览
	channels, err := h.store.ListChannels()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	cooldown := []map[string]any{}
	for _, c := range channels {
		if c.Status == "cooldown" {
			cooldown = append(cooldown, map[string]any{
				"id":             c.ID,
				"name":           c.Name,
				"cooldown_until": c.CooldownUntil,
				"last_error":     c.LastError,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":  sum,
		"cooldown": cooldown,
		"channels": channels,
	})
}
