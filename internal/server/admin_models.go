package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gateway/internal/model"
)

// GET /api/admin/models — 聚合模型列表(含渠道引用)
// POST /api/admin/models — 手动添加模型
func (h *AdminHandler) handleModels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		models, err := h.store.ListModelsWithChannels()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
	case http.MethodPost:
		var req struct {
			ModelID     string `json:"model_id"`
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if req.ModelID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "model_id 必填"})
			return
		}
		id, err := h.store.UpsertModel(req.ModelID, req.DisplayName)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// /api/admin/models/{id}[/action]
func (h *AdminHandler) handleModelByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/models/")
	parts := strings.SplitN(rest, "/", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid model id"})
		return
	}
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	switch action {
	case "price":
		if r.Method != http.MethodPut {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		var req struct {
			PriceInput  *float64 `json:"price_input"`
			PriceOutput *float64 `json:"price_output"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if err := h.store.UpdateModelPrice(id, req.PriceInput, req.PriceOutput); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case "channels":
		// POST 添加渠道关联;DELETE 移除
		if r.Method == http.MethodPost {
			var req struct {
				ChannelID         int64  `json:"channel_id"`
				UpstreamModelName string `json:"upstream_model_name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
				return
			}
			if req.ChannelID <= 0 {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "channel_id 必填"})
				return
			}
			if err := h.store.AddChannelModel(req.ChannelID, id, req.UpstreamModelName); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		if r.Method == http.MethodDelete {
			var req struct {
				ChannelID int64 `json:"channel_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
				return
			}
			if err := h.store.RemoveChannelModel(req.ChannelID, id); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := h.store.DeleteModel(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// POST /api/admin/models/sync — 同步某渠道的模型列表
func (h *AdminHandler) handleModelSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		ChannelID int64 `json:"channel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	c, err := h.store.GetChannel(req.ChannelID)
	if err != nil || c == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "channel not found"})
		return
	}
	models, err := fetchModels(c.BaseURL, c.APIKey, c.AuthHeader)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "同步失败: " + err.Error()})
		return
	}
	added := 0
	existing := 0
	for _, mid := range models {
		mk, err := h.store.UpsertModel(mid, "")
		if err != nil {
			continue
		}
		if err := h.store.AddChannelModel(req.ChannelID, mk, ""); err != nil {
			continue
		}
		added++
	}
	_ = existing
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "total": len(models), "added": added})
}

// fetchModels 调用渠道 ${baseURL}/models 拉取模型列表
func fetchModels(baseURL, apiKey, authHeader string) ([]string, error) {
	client := http.Client{Timeout: 15 * time.Second}
	target, err := buildURL(baseURL, "/v1/models")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if authHeader == "" || authHeader == "Authorization" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		req.Header.Set(authHeader, apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httpError(resp.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(payload.Data))
	for _, d := range payload.Data {
		if d.ID != "" {
			out = append(out, d.ID)
		}
	}
	return out, nil
}

type httpStatusError int

func (e httpStatusError) Error() string { return "HTTP " + strconv.Itoa(int(e)) }
func httpError(code int) error          { return httpStatusError(code) }

var _ = model.Model{}
