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

// TrailStep 单次渠道尝试记录(用于请求日志展示完整渠道链路:渠道1(失败)→渠道2(成功))
type TrailStep struct {
	ChannelID   int64  `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	OK          bool   `json:"ok"`              // 渠道是否正常响应(2xx / 业务错误也算响应成功)
	Reason      string `json:"reason,omitempty"` // 未成功原因(渠道故障 / 业务错误 / 取消)
}

// Result 单次上游请求结果
type Result struct {
	Status          int    // 上游 HTTP 状态
	Body            []byte // 非流式响应体
	ChannelFail     bool   // 是否算渠道失败(可降级)
	BizError        bool   // 业务错误(直接返回,不降级)
	ClientCanceled  bool   // 客户端断开/上层取消(非渠道故障,不重试不降级不计数)
	ErrorMessage    string
	Usage           *Usage
	Trail           []TrailStep // 本次请求渠道尝试链路(含最终结果)
	LatencyMs       int64 // 请求发起 → 读完响应体 总耗时
	FirstResponseMs int64 // 请求发起 → 收到首次响应(响应头)耗时
}

// Usage token 用量
type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"` // prompt_tokens_details.cached_tokens(命中缓存部分)
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
		// 超时不在此处全局设置:改为按渠道/请求通过 context 控制(支持渠道级超时)
		client: &http.Client{},
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
	Timeout           time.Duration // 渠道级超时(0 = 使用全局 upstream_timeout)
	Cooldown          time.Duration // 渠道级冷静时长(0 = 使用全局 cooldown_duration)
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
			Timeout:           time.Duration(ch.TimeoutMs) * time.Millisecond,
			Cooldown:          time.Duration(ch.CooldownMs) * time.Millisecond,
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

// doOnce 单次上游请求(非流式)。
// 超时一律使用全局 cfg.NonStreamTimeout(<=0 则不设超时),约束整个非流式请求:
// 首次响应 + 读取完整响应体(默认 5 分钟)。渠道级 timeout_ms 仅对流式 TTFB 生效
// (见 doStreamOnce),不作用于非流式请求。
func (r *Router) doOnce(ctx context.Context, target string, body []byte, apiKey, authHeader string) *Result {
	start := time.Now()
	total := r.cfg.NonStreamTimeout
	reqCtx, cancel := context.WithCancel(ctx)
	if total > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, total)
	}
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, target, bytes.NewReader(body))
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
	firstMs := time.Since(start).Milliseconds()
	if err != nil {
		// 父 ctx 已取消(客户端断开/上层取消):不是渠道故障,不触发重试/降级/失败计数
		if ctx.Err() != nil {
			return &Result{ClientCanceled: true, ErrorMessage: "client canceled", LatencyMs: firstMs, FirstResponseMs: firstMs}
		}
		return &Result{ChannelFail: true, ErrorMessage: err.Error(), LatencyMs: firstMs, FirstResponseMs: firstMs}
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	latency := time.Since(start).Milliseconds()
	if err != nil {
		if ctx.Err() != nil {
			return &Result{ClientCanceled: true, ErrorMessage: "client canceled", LatencyMs: latency, FirstResponseMs: firstMs}
		}
		return &Result{ChannelFail: true, ErrorMessage: err.Error(), LatencyMs: latency, FirstResponseMs: firstMs}
	}
	if len(bodyBytes) >= 64<<20 {
		// 响应体超限,视为渠道失败(避免截断 JSON 返回给客户端)
		return &Result{ChannelFail: true, ErrorMessage: "upstream response too large (>64MB)", LatencyMs: latency, FirstResponseMs: firstMs}
	}
	res := &Result{
		Status:          resp.StatusCode,
		Body:            bodyBytes,
		LatencyMs:       latency,
		FirstResponseMs: firstMs,
		ChannelFail:     isChannelFailure(resp.StatusCode, nil),
		BizError:        resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests,
	}
	res.Usage = parseUsage(bodyBytes)
	return res
}

// parseUsage 从非流式响应提取 usage
func parseUsage(body []byte) *Usage {
	var payload struct {
		Usage struct {
			PromptTokens        int64 `json:"prompt_tokens"`
			CompletionTokens    int64 `json:"completion_tokens"`
			TotalTokens         int64 `json:"total_tokens"`
			PromptTokensDetails struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
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
		CacheReadTokens:  payload.Usage.PromptTokensDetails.CachedTokens,
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

	// 收集各渠道失败原因,便于返回详细错误信息
	var failReasons []string
	lastCand := candidates[0] // 全部失败时返回最后尝试的渠道,供日志展示渠道名
	var trail []TrailStep     // 渠道尝试链路(每个尝试过的渠道 + 结果)

	for _, cand := range candidates {
		// 客户端已断开/上层取消:立即停止,不重试不降级(避免把取消误判为渠道故障)
		if ctx.Err() != nil {
			return nil, &Result{ClientCanceled: true, ErrorMessage: "client canceled"}, attempts, nil
		}
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
		trail = append(trail, trailStep(cand, res))
		if res.ClientCanceled {
			res.Trail = trail
			return cand, res, attempts, nil // 客户端取消:直接结束,不算渠道失败
		}
		if res.BizError {
			res.Trail = trail
			return cand, res, attempts, nil // 业务错误:直接返回,不降级
		}
		if !res.ChannelFail {
			// 成功:清零该渠道失败计数,使冷却阈值为"连续失败 N 次"
			_ = r.store.ClearChannelFailure(cand.ChannelID)
			res.Trail = trail
			return cand, res, attempts, nil // 成功
		}
		// 渠道失败:重试同一渠道一次
		if r.cfg.RetrySameChannel && attempts < maxAttempts {
			res2 := r.doOnce(ctx, target, sendBody, cand.APIKey, authHeader)
			attempts++
			trail[len(trail)-1] = trailStep(cand, res2) // 同渠道重试结果覆盖该渠道步骤(链路中渠道不重复)
			if res2.ClientCanceled {
				res2.Trail = trail
				return cand, res2, attempts, nil // 重试期间客户端取消
			}
			if res2.BizError {
				res2.Trail = trail
				return cand, res2, attempts, nil
			}
			if !res2.ChannelFail {
				_ = r.store.ClearChannelFailure(cand.ChannelID)
				res2.Trail = trail
				return cand, res2, attempts, nil // 重试成功
			}
			res = res2
		}
		// 记录渠道失败并降级
		r.markChannelFail(cand, res)
		lastCand = cand
		failReasons = append(failReasons, fmt.Sprintf("%s(%s)", cand.ChannelName, shortErr(res)))
	}
	// 全部失败:返回汇总错误与最后尝试的渠道(日志可展示失败渠道名)
	detail := ""
	if len(failReasons) > 0 {
		detail = ":" + strings.Join(failReasons, "; ")
	}
	last := &Result{ChannelFail: true, ErrorMessage: "all channels failed" + detail, Trail: trail}
	return lastCand, last, attempts, nil
}

// trailStep 将单次渠道尝试转为链路步骤:2xx/业务错误视为渠道正常响应,故障与取消记录原因
func trailStep(cand *ChannelCandidate, res *Result) TrailStep {
	s := TrailStep{ChannelID: cand.ChannelID, ChannelName: cand.ChannelName}
	switch {
	case res.ClientCanceled:
		s.Reason = "client canceled"
	case res.BizError:
		s.OK = true
		s.Reason = fmt.Sprintf("HTTP %d", res.Status)
	case res.ChannelFail:
		s.Reason = shortErr(res)
	default:
		s.OK = true
	}
	return s
}

// shortErr 生成渠道失败的简短描述(错误信息或 HTTP 状态)
func shortErr(res *Result) string {
	if res.ErrorMessage != "" {
		return res.ErrorMessage
	}
	if res.Status > 0 {
		return fmt.Sprintf("HTTP %d", res.Status)
	}
	return "unknown error"
}

// markChannelFail 渠道失败计数,达阈值进入冷静(冷静时长优先渠道级配置)
func (r *Router) markChannelFail(cand *ChannelCandidate, res *Result) {
	msg := res.ErrorMessage
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", res.Status)
	}
	count, err := r.store.IncrementChannelFailure(cand.ChannelID, msg)
	if err != nil {
		return
	}
	if count >= r.cfg.CooldownThreshold {
		cooldown := r.cfg.CooldownDuration
		if cand.Cooldown > 0 {
			cooldown = cand.Cooldown
		}
		until := time.Now().Add(cooldown)
		_ = r.store.SetChannelStatus(cand.ChannelID, "cooldown", until, msg)
	}
}

// Cost 计算成本(单位:元;价格为元/百万 token)。
// 命中缓存的部分按缓存单价计,其余输入按输入单价计,输出按输出单价计;
// 未配置缓存单价时缓存部分按输入单价计;cacheTokens 超过输入时按输入量截断。
func Cost(inputTokens, cacheTokens, outputTokens int64, priceIn, priceCache, priceOut *float64) float64 {
	if cacheTokens < 0 {
		cacheTokens = 0
	}
	if cacheTokens > inputTokens {
		cacheTokens = inputTokens
	}
	var cost float64
	if priceIn != nil {
		cost += float64(inputTokens-cacheTokens) / 1e6 * *priceIn
	}
	cachePrice := priceCache
	if cachePrice == nil {
		cachePrice = priceIn
	}
	if cachePrice != nil && cacheTokens > 0 {
		cost += float64(cacheTokens) / 1e6 * *cachePrice
	}
	if priceOut != nil {
		cost += float64(outputTokens) / 1e6 * *priceOut
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
