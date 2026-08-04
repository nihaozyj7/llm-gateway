package server

import (
	"net"
	"net/http"
	"strings"

	"gateway/internal/config"
	"gateway/internal/store"
)

// AdminHandler 管理后台 API
type AdminHandler struct {
	store *store.Store
	cfg   *config.Config
}

// NewAdminHandler 创建管理 API 处理器
func NewAdminHandler(st *store.Store, cfg *config.Config) *AdminHandler {
	return &AdminHandler{store: st, cfg: cfg}
}

// Mount 挂载 /api/admin/* 路由。
// 管理接口不再需要登录,安全模型为「仅允许本机(127.0.0.1 / ::1)访问」。
func (h *AdminHandler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("/api/admin/channels", h.requireLocal(h.handleChannels))
	mux.HandleFunc("/api/admin/channels/", h.requireLocal(h.handleChannelByID))
	mux.HandleFunc("/api/admin/channels/reorder", h.requireLocal(h.handleChannelsReorder))
	mux.HandleFunc("/api/admin/models", h.requireLocal(h.handleModels))
	mux.HandleFunc("/api/admin/models/", h.requireLocal(h.handleModelByID))
	mux.HandleFunc("/api/admin/models/sync", h.requireLocal(h.handleModelSync))
	mux.HandleFunc("/api/admin/models/test", h.requireLocal(h.handleModelTest))
	mux.HandleFunc("/api/admin/keys", h.requireLocal(h.handleKeys))
	mux.HandleFunc("/api/admin/keys/", h.requireLocal(h.handleKeyByID))
	mux.HandleFunc("/api/admin/logs", h.requireLocal(h.handleLogs))
	mux.HandleFunc("/api/admin/logs/clear", h.requireLocal(h.handleClearLogs))
	mux.HandleFunc("/api/admin/logs/", h.requireLocal(h.handleLogByID))
	mux.HandleFunc("/api/admin/stats", h.requireLocal(h.handleStats))
	mux.HandleFunc("/api/admin/stats/reset", h.requireLocal(h.handleStatsReset))
	mux.HandleFunc("/api/admin/dashboard", h.requireLocal(h.handleDashboard))
	mux.HandleFunc("/api/admin/test", h.requireLocal(h.handleTestChannel))
}

// requireLocal 限制管理接口仅允许本机(loopback)访问
func (h *AdminHandler) requireLocal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			ip = host
		}
		parsed := net.ParseIP(strings.TrimSpace(ip))
		if parsed == nil || !parsed.IsLoopback() {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "管理界面仅允许本机访问"})
			return
		}
		next(w, r)
	}
}
