package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"gateway/internal/config"
	"gateway/internal/route"
	"gateway/internal/store"
)

// GatewayHandler OpenAI 兼容转发网关
type GatewayHandler struct {
	store  *store.Store
	cfg    *config.Config
	router *route.Router
}

// NewGatewayHandler 创建网关处理器
func NewGatewayHandler(st *store.Store, cfg *config.Config, router *route.Router) *GatewayHandler {
	return &GatewayHandler{store: st, cfg: cfg, router: router}
}

// Mount 注册 /v1/* 路由
func (h *GatewayHandler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("/v1/", h.handleV1)
}

func (h *GatewayHandler) handleV1(w http.ResponseWriter, r *http.Request) {
	// CORS:允许浏览器端应用(如本地网页、Web 工具)跨域调用网关 API。
	// 仅对 /v1/* 开放;管理接口 /api/admin/* 不开 CORS,避免远程页面借本机浏览器越权。
	// 固定返回 *:/v1/* 为带 API key 鉴权的公开转发接口,允许任意来源调用;
	// 上游返回的 CORS 头会在流式透传时被过滤(见 route.doStreamOnce),不会与本头合并成多值。
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
	w.Header().Set("Access-Control-Max-Age", "86400")
	if r.Method == http.MethodOptions {
		// 预检请求:直接放行,不校验 API key(预检不带 Authorization 头)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 鉴权:Authorization: Bearer <api key>
	key := extractBearer(r)
	if key == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": map[string]any{"message": "missing API key", "type": "invalid_request_error"},
		})
		return
	}
	ak, err := h.store.GetAPIKeyByHash(hashKey(key))
	if err != nil || ak == nil || !ak.Enabled {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": map[string]any{"message": "invalid API key", "type": "invalid_request_error"},
		})
		return
	}
	_ = h.store.TouchAPIKey(ak.ID)

	// GET /v1/models → 聚合模型列表
	if r.Method == http.MethodGet && strings.TrimSuffix(r.URL.Path, "/") == "/v1/models" {
		h.handleListModels(w, r)
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": map[string]any{"message": "method not allowed", "type": "invalid_request_error"},
		})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{"message": "failed to read body: " + err.Error(), "type": "invalid_request_error"},
		})
		return
	}
	modelID := route.ExtractModelID(body)
	if modelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{"message": "missing model field", "type": "invalid_request_error"},
		})
		return
	}
	clientPath := r.URL.Path
	isStream := route.IsStreamRequest(body)
	start := time.Now()
	sourceIP := clientIP(r)
	requestID := newRequestID()

	logEntry := &storeLogEntry{
		RequestID:  requestID,
		KeyName:    ak.Name,
		Model:      modelID,
		SourceIP:   sourceIP,
		PayloadReq: string(body),
	}

	if isStream {
		h.handleStreamRequest(w, r, body, modelID, clientPath, start, logEntry)
	} else {
		h.handleJSONRequest(w, r, body, modelID, clientPath, start, logEntry)
	}
}

func (h *GatewayHandler) handleJSONRequest(w http.ResponseWriter, r *http.Request, body []byte, modelID, clientPath string, start time.Time, logEntry *storeLogEntry) {
	cand, res, attempts, err := h.router.Handle(r.Context(), modelID, clientPath, "", "", body, false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{"message": err.Error(), "type": "gateway_error"},
		})
		logEntry.Status = "fail"
		logEntry.Error = err.Error()
		fillUpstreamModel(logEntry, modelID, nil)
		h.writeLog(start, logEntry, 0, nil, 0, 0, 0, 0, 0)
		return
	}
	fillUpstreamModel(logEntry, modelID, cand)
	if res.BizError {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.Status)
		_, _ = w.Write(res.Body)
		logEntry.Status = "biz_error"
		logEntry.Error = firstLine(res.Body)
		logEntry.ChannelID = cand.ChannelID
		logEntry.ChannelName = cand.ChannelName
		logEntry.FirstResponseMs = res.FirstResponseMs
		h.writeLog(start, logEntry, res.LatencyMs, res.Body, 0, 0, 0, 0, 0)
		return
	}
	if res.ChannelFail {
		// 所有渠道失败
		status := http.StatusBadGateway
		msg := res.ErrorMessage
		if msg == "" {
			msg = "upstream error"
		}
		writeJSON(w, status, map[string]any{
			"error": map[string]any{"message": msg, "type": "upstream_error"},
		})
		logEntry.Status = "fail"
		logEntry.Error = msg
		if cand != nil {
			logEntry.ChannelID = cand.ChannelID
			logEntry.ChannelName = cand.ChannelName
		}
		logEntry.FirstResponseMs = res.FirstResponseMs
		h.writeLog(start, logEntry, res.LatencyMs, nil, 0, 0, 0, 0, 0)
		return
	}

	// 成功
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.Status)
	_, _ = w.Write(res.Body)

	status := "success"
	if attempts > 1 {
		status = "retry_success" // 发生过重试或降级后成功
	}
	var pt, ct, cache, tt int64
	if res.Usage != nil {
		pt, ct, cache, tt = res.Usage.PromptTokens, res.Usage.CompletionTokens, res.Usage.CacheReadTokens, res.Usage.TotalTokens
	}
	logEntry.Status = status
	logEntry.ChannelID = cand.ChannelID
	logEntry.ChannelName = cand.ChannelName
	logEntry.FirstResponseMs = res.FirstResponseMs
	h.writeLog(start, logEntry, res.LatencyMs, res.Body, pt, ct, cache, tt, res.Status)
}

func (h *GatewayHandler) handleStreamRequest(w http.ResponseWriter, r *http.Request, body []byte, modelID, clientPath string, start time.Time, logEntry *storeLogEntry) {
	cand, res, attempts, err := h.router.HandleStream(r.Context(), w, modelID, clientPath, "", "", body)
	if err != nil {
		if !streamStarted(w) {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": map[string]any{"message": err.Error(), "type": "gateway_error"},
			})
		}
		logEntry.Status = "fail"
		logEntry.Error = err.Error()
		fillUpstreamModel(logEntry, modelID, nil)
		h.writeLog(start, logEntry, 0, nil, 0, 0, 0, 0, 0)
		return
	}
	fillUpstreamModel(logEntry, modelID, cand)
	if res == nil {
		return
	}
	if res.ChannelFail && !res.Started {
		// 所有渠道首包前失败
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]any{"message": res.ErrorMessage, "type": "upstream_error"},
		})
		logEntry.Status = "fail"
		logEntry.Error = res.ErrorMessage
		if cand != nil {
			logEntry.ChannelID = cand.ChannelID
			logEntry.ChannelName = cand.ChannelName
		}
		h.writeLog(start, logEntry, 0, nil, 0, 0, 0, 0, 0)
		return
	}
	// 已开始流式输出(成功或中途断开)
	var pt, ct, cache, tt int64
	if res.Usage != nil {
		pt, ct, cache, tt = res.Usage.PromptTokens, res.Usage.CompletionTokens, res.Usage.CacheReadTokens, res.Usage.TotalTokens
	}
	status := "success"
	if attempts > 1 {
		status = "retry_success"
	}
	if res.ErrorMessage != "" {
		status = "fail"
	}
	logEntry.Status = status
	logEntry.Error = res.ErrorMessage
	if cand != nil {
		logEntry.ChannelID = cand.ChannelID
		logEntry.ChannelName = cand.ChannelName
	}
	logEntry.FirstResponseMs = res.FirstResponseMs
	latency := time.Since(start).Milliseconds()
	h.writeLog(start, logEntry, latency, nil, pt, ct, cache, tt, 200)
}

// handleListModels 返回聚合模型列表(OpenAI 格式)
func (h *GatewayHandler) handleListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.store.ListModels()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	type modelObj struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}
	data := make([]modelObj, 0, len(models))
	for _, m := range models {
		data = append(data, modelObj{ID: m.ModelID, Object: "model", Created: m.CreatedAt.Unix(), OwnedBy: "gateway"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// ---- 工具 ----

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(h)
}

// hashKey 密钥哈希(SHA-256 十六进制)
func hashKey(key string) string {
	return sha256Hex(key)
}

func sha256Hex(s string) string {
	return newSHA256Hex(s)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func firstLine(b []byte) string {
	s := string(b)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// fillUpstreamModel 记录实际转发到上游的模型名:渠道配置了映射则用映射名,否则与请求模型相同
func fillUpstreamModel(e *storeLogEntry, modelID string, cand *route.ChannelCandidate) {
	if cand != nil && cand.UpstreamModelName != "" {
		e.UpstreamModel = cand.UpstreamModelName
	} else {
		e.UpstreamModel = modelID
	}
}

var _ = log.Printf
