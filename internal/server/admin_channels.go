package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gateway/internal/model"
)

// maskKey 遮罩 API key:保留前4后4
func maskKey(k string) string {
	if len(k) <= 8 {
		return "***"
	}
	return k[:4] + "***" + k[len(k)-4:]
}

// GET /api/admin/channels — 渠道列表(api_key 遮罩)
// POST /api/admin/channels — 新建渠道
func (h *AdminHandler) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		chs, err := h.store.ListChannels()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		for _, c := range chs {
			c.APIKey = maskKey(c.APIKey)
		}
		writeJSON(w, http.StatusOK, map[string]any{"channels": chs})
	case http.MethodPost:
		var c model.Channel
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if c.Name == "" || c.BaseURL == "" || c.APIKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name/base_url/api_key 必填"})
			return
		}
		if c.AuthHeader == "" {
			c.AuthHeader = "Authorization"
		}
		if !c.Enabled {
			c.Enabled = true // 新建渠道默认启用
		}
		id, err := h.store.CreateChannel(&c)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// /api/admin/channels/{id}[/action]
func (h *AdminHandler) handleChannelByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/channels/")
	parts := strings.SplitN(rest, "/", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid channel id"})
		return
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	switch action {
	case "test":
		h.testChannel(w, r, id)
		return
	case "recover":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		if err := h.store.ResetChannelFailure(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	switch r.Method {
	case http.MethodGet:
		c, err := h.store.GetChannel(id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if c == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "channel not found"})
			return
		}
		c.APIKey = maskKey(c.APIKey)
		writeJSON(w, http.StatusOK, c)
	case http.MethodPut:
		var c model.Channel
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		c.ID = id
		if c.AuthHeader == "" {
			c.AuthHeader = "Authorization"
		}
		if c.APIKey == "" || strings.Contains(c.APIKey, "***") {
			// 留空或遮罩值表示不修改密钥
			orig, err := h.store.GetChannel(id)
			if err == nil && orig != nil {
				c.APIKey = orig.APIKey
			}
		}
		if err := h.store.UpdateChannel(&c); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodDelete:
		if err := h.store.DeleteChannel(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleTestChannel HTTP 包装:POST /api/admin/test?channel_id=
func (h *AdminHandler) handleTestChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("channel_id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid channel_id"})
		return
	}
	h.testChannel(w, r, id)
}

// testChannel 测试渠道连通性:调 ${baseURL}/models
func (h *AdminHandler) testChannel(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	c, err := h.store.GetChannel(id)
	if err != nil || c == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "channel not found"})
		return
	}
	client := http.Client{Timeout: 15 * time.Second}
	target, err := buildURL(c.BaseURL, "/models")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid base_url: " + err.Error()})
		return
	}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if c.AuthHeader == "" || c.AuthHeader == "Authorization" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	} else {
		req.Header.Set(c.AuthHeader, c.APIKey)
	}
	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latency_ms": latency, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency_ms": latency, "status": resp.StatusCode})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latency_ms": latency, "status": resp.StatusCode, "error": "HTTP " + strconv.Itoa(resp.StatusCode)})
}

// buildURL 拼接 baseURL + path(与网关转发规则一致:去掉 /v1 前缀拼接)
func buildURL(baseURL, path string) (string, error) {
	trimmed := strings.TrimPrefix(path, "/v1")
	base := strings.TrimRight(baseURL, "/")
	return base + trimmed, nil
}
