package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gateway/internal/config"
	"gateway/internal/model"
	"gateway/internal/store"
)

// Result 单次上游请求结果
type Result struct {
	Status       int    // 上游 HTTP 状态
	Body         []byte // 非流式响应体
	ChannelFail  bool   // 是否算渠道失败(可降级)
	BizError     bool   // 业务错误(直接返回,不降级)
	ErrorMessage string
	Usage        *Usage
	LatencyMs    int64
}

// Usage token 用量
type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// Router 路由决策器
type Router struct {
	store  *store.Store
	cfg    *config.Config
	client *http.Client
}

// NewRouter 创建路由决策器
func NewRouter(st *store.Store, cfg *config.Config) *Router {
	return &Router{
		store: st,
		cfg:   cfg,
		client: &http.Client{
			Timeout: cfg.UpstreamTimeout,
		},
	}
}

// ChannelCandidate 渠道候选
type ChannelCandidate struct {
	ChannelID         int64
	ChannelName       string
	BaseURL           string
	APIKey            string
	AuthHeader        string
	UpstreamModelName string
	Status            string
}

// PickChannels 选出目标模型的候选渠道(启用且未冷静,按优先级排序)
func (r *Router) PickChannels(ctx context.Context, modelID string) ([]*ChannelCandidate, error) {
	if err := r.store.RefreshCoolDowns(); err != nil {
		return nil, err
	}
	m, err := r.store.GetModelWithChannels(modelID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	now := time.Now()
	var out []*ChannelCandidate
	for _, ref := range m.Channels {
		if !ref.Enabled {
			continue
		}
		if ref.Status == "cooldown" {
			// 冷静期未到则不参与路由
			ch, err := r.store.GetChannel(ref.ChannelID)
			if err != nil || ch == nil {
				continue
			}
			if ch.CooldownUntil.After(now) {
				continue
			}
		}
		ch, err := r.store.GetChannel(ref.ChannelID)
		if err != nil || ch == nil {
			continue
		}
		out = append(out, &ChannelCandidate{
			ChannelID:         ch.ID,
			ChannelName:       ch.Name,
			BaseURL:           ch.BaseURL,
			APIKey:            ch.APIKey,
			AuthHeader:        ch.AuthHeader,
			UpstreamModelName: ref.UpstreamModelName,
			Status:            ch.Status,
		})
	}
	// 已按 priority ASC 排序(见 ListModelsWithChannels 的 ORDER BY)
	return out, nil
}

// BuildUpstreamURL 拼接上游 URL:baseURL + 客户端路径去掉 /v1 前缀
// 裸路径 /v1 或 /v1/ 时直接返回 baseURL(避免产生尾部斜杠);
// 仅接受 /v1/xxx 或 /v1 形态的路径,防止畸形路径拼接
func BuildUpstreamURL(baseURL, clientPath string) (string, error) {
	trimmed := strings.TrimPrefix(clientPath, "/v1")
	if trimmed != "" && !strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("invalid client path %q", clientPath)
	}
	trimmed = strings.TrimRight(trimmed, "/")
	base := strings.TrimRight(baseURL, "/")
	u := base
	if trimmed != "" {
		u = base + trimmed
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid upstream url %q", u)
	}
	return u, nil
}

// PrepareRequest 构造上游请求:替换 baseurl/apikey/模型映射
func (r *Router) PrepareRequest(baseURL, clientPath, apiKey, authHeader, upstreamModel string, body []byte) (*http.Request, error) {
	target, err := BuildUpstreamURL(baseURL, clientPath)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if authHeader == "" {
		authHeader = "Authorization"
	}
	if strings.EqualFold(authHeader, "Authorization") {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		req.Header.Set(authHeader, apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	return req, nil
}

// ApplyModelMapping 若配置了上游模型名映射则替换 body 中 model 字段
func ApplyModelMapping(body []byte, upstreamModel string) []byte {
	if upstreamModel == "" {
		return body
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	if cur, ok := m["model"].(string); ok && cur != upstreamModel {
		m["model"] = upstreamModel
		if b, err := json.Marshal(m); err == nil {
			return b
		}
	}
	return body
}

// isChannelFailure 判断响应是否算渠道失败(可降级)
func isChannelFailure(status int, err error) bool {
	if err != nil {
		return true
	}
	// 5xx 与 429 视为渠道失败;其余 4xx 为业务错误
	return status >= 500 || status == http.StatusTooManyRequests
}

// classifyError 网络错误分类
func (r *Router) classifyError(err error) (fail bool, msg string) {
	if err == nil {
		return false, ""
	}
	return true, err.Error()
}

// doOnce 单次上游请求(非流式),返回结果
func (r *Router) doOnce(ctx context.Context, target string, body []byte, apiKey, authHeader string) *Result {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return &Result{ChannelFail: true, ErrorMessage: err.Error()}
	}
	if authHeader == "" {
		authHeader = "Authorization"
	}
	if strings.EqualFold(authHeader, "Authorization") {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		req.Header.Set(authHeader, apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := r.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &Result{ChannelFail: true, ErrorMessage: err.Error(), LatencyMs: latency}
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return &Result{ChannelFail: true, ErrorMessage: err.Error(), LatencyMs: latency}
	}
	if len(bodyBytes) >= 64<<20 {
		// 响应体超限,视为渠道失败(避免截断 JSON 返回给客户端)
		return &Result{ChannelFail: true, ErrorMessage: "upstream response too large (>64MB)", LatencyMs: latency}
	}
	res := &Result{
		Status:      resp.StatusCode,
		Body:        bodyBytes,
		LatencyMs:   latency,
		ChannelFail: isChannelFailure(resp.StatusCode, nil),
		BizError:    resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests,
	}
	res.Usage = parseUsage(bodyBytes)
	return res
}

// parseUsage 从非流式响应提取 usage
func parseUsage(body []byte) *Usage {
	var payload struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	if payload.Usage.TotalTokens == 0 && payload.Usage.PromptTokens == 0 && payload.Usage.CompletionTokens == 0 {
		return nil
	}
	u := &Usage{
		PromptTokens:     payload.Usage.PromptTokens,
		CompletionTokens: payload.Usage.CompletionTokens,
		TotalTokens:      payload.Usage.TotalTokens,
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

// Handle 主入口:路由 + 重试 + 降级(非流式路径)
// 返回:最终渠道、结果、是否发生降级/重试
func (r *Router) Handle(ctx context.Context, modelID, clientPath, apiKey, authHeader string, body []byte, isStream bool) (*ChannelCandidate, *Result, int, error) {
	candidates, err := r.PickChannels(ctx, modelID)
	if err != nil {
		return nil, nil, 0, err
	}
	if len(candidates) == 0 {
		return nil, nil, 0, fmt.Errorf("no available channel for model %q", modelID)
	}

	attempts := 0
	maxAttempts := r.cfg.MaxAttemptsPerRequest
	if maxAttempts <= 0 {
		maxAttempts = len(candidates) * 2 // 每渠道最多 2 次(首次+重试)
	}

	for _, cand := range candidates {
		// 首次尝试
		target, err := BuildUpstreamURL(cand.BaseURL, clientPath)
		if err != nil {
			continue
		}
		sendBody := body
		if cand.UpstreamModelName != "" {
			sendBody = ApplyModelMapping(body, cand.UpstreamModelName)
		}
		res := r.doOnce(ctx, target, sendBody, cand.APIKey, authHeader)
		attempts++
		if res.BizError {
			return cand, res, attempts, nil // 业务错误:直接返回,不降级
		}
		if !res.ChannelFail {
			return cand, res, attempts, nil // 成功
		}
		// 渠道失败:重试同一渠道一次
		if r.cfg.RetrySameChannel && attempts < maxAttempts {
			res2 := r.doOnce(ctx, target, sendBody, cand.APIKey, authHeader)
			attempts++
			if res2.BizError {
				return cand, res2, attempts, nil
			}
			if !res2.ChannelFail {
				return cand, res2, attempts, nil // 重试成功
			}
			res = res2
		}
		// 记录渠道失败并降级
		r.markChannelFail(cand.ChannelID, res)
	}
	// 全部失败:返回最后一次错误
	last := &Result{ChannelFail: true, ErrorMessage: "all channels failed"}
	return nil, last, attempts, nil
}

// markChannelFail 渠道失败计数,达阈值进入冷静
func (r *Router) markChannelFail(channelID int64, res *Result) {
	msg := res.ErrorMessage
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", res.Status)
	}
	count, err := r.store.IncrementChannelFailure(channelID, msg)
	if err != nil {
		return
	}
	if count >= r.cfg.CooldownThreshold {
		until := time.Now().Add(r.cfg.CooldownDuration)
		_ = r.store.SetChannelStatus(channelID, "cooldown", until, msg)
	}
}

// Cost 计算成本:元 = tokens/1000 * 单价
func Cost(inputTokens, outputTokens int64, priceIn, priceOut *float64) float64 {
	var cost float64
	if priceIn != nil {
		cost += float64(inputTokens) / 1000 * *priceIn
	}
	if priceOut != nil {
		cost += float64(outputTokens) / 1000 * *priceOut
	}
	return cost
}

// ExtractModelID 从请求体提取 model 字段
func ExtractModelID(body []byte) string {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	if s, ok := m["model"].(string); ok {
		return s
	}
	return ""
}

// IsStreamRequest 判断是否为流式请求
func IsStreamRequest(body []byte) bool {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	if v, ok := m["stream"].(bool); ok {
		return v
	}
	return false
}

var _ = model.Channel{}
