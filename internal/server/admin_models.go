package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gateway/internal/model"
	"gateway/internal/store"
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
			PriceInput     *float64 `json:"price_input"`
			PriceOutput    *float64 `json:"price_output"`
			PriceCacheRead *float64 `json:"price_cache_read"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if err := h.store.UpdateModelPrice(id, req.PriceInput, req.PriceOutput, req.PriceCacheRead); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case "reorder":
		// POST 调整该模型的渠道顺序(模型级优先级,不影响渠道全局优先级)
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		var req struct {
			Items []store.ChannelPriority `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if len(req.Items) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "items 不能为空"})
			return
		}
		if err := h.store.ReorderModelChannels(id, req.Items); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
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

// ---- 模型测试(可用性 / 首字延迟 / 回复速度) ----

// modelTestResult 单个渠道的模型测试结果
type modelTestResult struct {
	ChannelID        int64   `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	Priority         int     `json:"priority"`
	ChannelStatus    string  `json:"channel_status"` // normal / cooldown
	OK               bool    `json:"ok"`
	Skipped          bool    `json:"skipped"`
	SkipReason       string  `json:"skip_reason,omitempty"`
	HTTPStatus       int     `json:"http_status,omitempty"`
	Error            string  `json:"error,omitempty"`
	TTFTMs           int64   `json:"ttft_ms"`           // 首字延迟(第一个 SSE chunk 到达时间)
	CompletionTokens int64   `json:"completion_tokens"` // 输出 token(usage;无则按字符估算)
	TokensEstimated  bool    `json:"tokens_estimated"`
	TotalMs          int64   `json:"total_ms"`
	SpeedTPS         float64 `json:"speed_tps"` // token/s
}

// POST /api/admin/models/test — 测试模型在关联渠道上的可用性、首字延迟、回复速度
func (h *AdminHandler) handleModelTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.ModelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "model_id 必填"})
		return
	}
	mw, err := h.store.GetModelWithChannels(req.ModelID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if mw == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "model not found"})
		return
	}
	if len(mw.Channels) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "模型未关联任何渠道,请先在「关联渠道」中添加"})
		return
	}
	results := make([]modelTestResult, 0, len(mw.Channels))
	client := &http.Client{Timeout: 20 * time.Second}
	for _, ref := range mw.Channels {
		res := modelTestResult{
			ChannelID:     ref.ChannelID,
			ChannelName:   ref.ChannelName,
			Priority:      ref.Priority,
			ChannelStatus: ref.Status,
		}
		if !ref.Enabled {
			res.Skipped = true
			res.SkipReason = "渠道已禁用"
			results = append(results, res)
			continue
		}
		ch, err := h.store.GetChannel(ref.ChannelID)
		if err != nil || ch == nil {
			res.Skipped = true
			res.SkipReason = "渠道不存在"
			results = append(results, res)
			continue
		}
		upstream := ref.UpstreamModelName
		if upstream == "" {
			upstream = req.ModelID
		}
		results = append(results, testModelOnChannel(r.Context(), client, ch, upstream))
	}
	writeJSON(w, http.StatusOK, map[string]any{"model_id": req.ModelID, "results": results})
}

// testModelOnChannel 对单个渠道发送最小流式测试请求,测量:连通性、首字延迟(TTFT)、回复速度(token/s)
// ctx 取消(如前端关闭弹窗)会中断测试;client 由调用方共享以复用连接。
func testModelOnChannel(ctx context.Context, client *http.Client, ch *model.Channel, upstreamModel string) modelTestResult {
	res := modelTestResult{ChannelID: ch.ID, ChannelName: ch.Name, Priority: ch.Priority, ChannelStatus: ch.Status}
	payload, err := json.Marshal(map[string]any{
		"model":      upstreamModel,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 16,
		"stream":     true,
	})
	if err != nil {
		res.Error = err.Error()
		return res
	}
	target, err := buildURL(ch.BaseURL, "/chat/completions")
	if err != nil {
		res.Error = "invalid base_url: " + err.Error()
		return res
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if ch.AuthHeader == "" || ch.AuthHeader == "Authorization" {
		req.Header.Set("Authorization", "Bearer "+ch.APIKey)
	} else {
		req.Header.Set(ch.AuthHeader, ch.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		res.HTTPStatus = resp.StatusCode
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		res.Error = firstLine(errBody)
		if res.Error == "" {
			res.Error = "HTTP " + strconv.Itoa(resp.StatusCode)
		}
		return res
	}

	// 流式读取:记录首字延迟,解析 usage(无则按输出字符估算)
	br := bufio.NewReader(resp.Body)
	var firstChunkAt *time.Time
	var contentChars int
	var usageTokens int64
	usageSeen := false
	var streamErr error
	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if strings.HasPrefix(trimmed, "data:") {
				now := time.Now()
				if firstChunkAt == nil {
					firstChunkAt = &now
					res.TTFTMs = now.Sub(start).Milliseconds()
				}
				if ct, ok := streamTestUsage(trimmed); ok {
					usageSeen = true
					usageTokens = ct
				}
				contentChars += streamChunkContentLen(trimmed)
			}
		}
		if rerr != nil {
			if rerr != io.EOF {
				// 超时 / 连接重置等真实错误:标记失败,不按成功返回
				streamErr = rerr
			}
			break
		}
	}
	res.TotalMs = time.Since(start).Milliseconds()
	if streamErr != nil {
		res.Error = streamErr.Error()
		return res
	}
	res.OK = true
	if usageSeen {
		res.CompletionTokens = usageTokens
	} else {
		// 上游未返回 usage:按输出字符数粗略估算 token(中英混合约 3 字符/token)
		res.CompletionTokens = int64(contentChars / 3)
		res.TokensEstimated = true
	}
	if res.TotalMs > 0 && res.CompletionTokens > 0 {
		res.SpeedTPS = float64(res.CompletionTokens) / (float64(res.TotalMs) / 1000.0)
	}
	return res
}

// streamTestUsage 从单个 SSE data 行提取 usage 的 completion_tokens
func streamTestUsage(dataLine string) (completionTokens int64, ok bool) {
	payload := strings.TrimSpace(strings.TrimPrefix(dataLine, "data:"))
	if payload == "" || payload == "[DONE]" {
		return 0, false
	}
	var chunk struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return 0, false
	}
	if chunk.Usage.TotalTokens == 0 && chunk.Usage.PromptTokens == 0 && chunk.Usage.CompletionTokens == 0 {
		return 0, false
	}
	return chunk.Usage.CompletionTokens, true
}

// streamChunkContentLen 统计单个 SSE chunk 的输出字符数(content + reasoning_content)
func streamChunkContentLen(dataLine string) int {
	payload := strings.TrimSpace(strings.TrimPrefix(dataLine, "data:"))
	if payload == "" || payload == "[DONE]" {
		return 0
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return 0
	}
	n := 0
	for _, c := range chunk.Choices {
		n += len([]rune(c.Delta.Content)) + len([]rune(c.Delta.ReasoningContent))
	}
	return n
}

var _ = model.Model{}
