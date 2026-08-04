package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// GET /api/admin/logs?channel_id=&model=&status=&key_name=&keyword=&page=&page_size=
func (h *AdminHandler) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	q := r.URL.Query()
	var channelID *int64
	if v := q.Get("channel_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			channelID = &id
		}
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize < 1 || pageSize > 200 {
		pageSize = 25
	}
	logs, total, err := h.store.ListLogs(channelID, q.Get("model"), q.Get("status"), q.Get("key_name"), q.Get("keyword"), (page-1)*pageSize, pageSize)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs, "total": total, "page": page, "page_size": pageSize})
}

// GET /api/admin/logs/{id}
func (h *AdminHandler) handleLogByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/logs/")
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid log id"})
		return
	}
	l, err := h.store.GetLog(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if l == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "log not found"})
		return
	}
	writeJSON(w, http.StatusOK, l)
}

var _ = json.Marshal
