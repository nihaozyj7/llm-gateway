package route

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// StreamResult 流式转发结果
type StreamResult struct {
	ChannelFail     bool   // 首包前失败(可降级)
	Started         bool   // 是否已向客户端写入首字节
	ClientCanceled  bool   // 客户端断开/上层取消(非渠道故障,不重试不降级不计数)
	ErrorMessage    string
	Usage           *Usage
	Trail           []TrailStep // 本次请求渠道尝试链路(含最终结果)
	FirstResponseMs int64 // 请求发起 → 收到首次响应(响应头)耗时
}

// doStreamOnce 单次流式转发:透传 SSE,同时截获 usage。
// 超时语义(两类独立):
//   - TTFB(首次响应):渠道级 timeout,否则全局 cfg.UpstreamTimeout;
//     指「请求发起后到收到第一个响应的时间」,超过即判首包前失败(可降级重试);
//     一旦收到响应头,该超时不再生效,流式输出不会被它打断。
//   - 流式最长持续时间:cfg.StreamMaxDuration(默认 6 分钟);
//     指「整个流式请求允许的最长时长」,超过后即使仍在输出也判定超时。
//
// timeout 为渠道级超时;首包前失败返回 ChannelFail=true(调用方可降级重试);已开始输出则不再重试
func (r *Router) doStreamOnce(ctx context.Context, w http.ResponseWriter, target string, body []byte, apiKey, authHeader string, timeout time.Duration) *StreamResult {
	start := time.Now()
	ttfb := timeout
	if ttfb <= 0 {
		ttfb = r.cfg.UpstreamTimeout
	}
	maxDur := r.cfg.StreamMaxDuration

	// 请求 context:以流式最长持续时间为 deadline,覆盖整个流式过程(含 body 读取)。
	// 未配置(<=0)时仅用 WithCancel,便于 TTFB 定时器中断等待响应头。
	reqCtx, cancel := context.WithCancel(ctx)
	if maxDur > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, maxDur)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return &StreamResult{ChannelFail: true, ErrorMessage: err.Error()}
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
	req.Header.Set("Accept", "text/event-stream")

	// TTFB 定时器:等待首个响应期间超时则取消请求(首次响应超时)。
	// Do 返回后立即 Stop;若定时器恰在返回瞬间已触发,则请求已被取消,按 TTFB 超时处理。
	var ttfbTimer *time.Timer
	if ttfb > 0 {
		ttfbTimer = time.AfterFunc(ttfb, cancel)
	}
	resp, err := r.client.Do(req)
	firstMs := time.Since(start).Milliseconds()
	ttfbStopped := true
	if ttfbTimer != nil {
		ttfbStopped = ttfbTimer.Stop()
	}
	if err != nil {
		if !ttfbStopped {
			// TTFB 定时器已触发:首次响应超时
			return &StreamResult{ChannelFail: true, ErrorMessage: "upstream timeout: first response not received within " + ttfb.String(), FirstResponseMs: firstMs}
		}
		// 父 ctx 已取消(客户端断开/上层取消):不是渠道故障,不重试不降级
		if ctx.Err() != nil {
			return &StreamResult{ClientCanceled: true, ErrorMessage: "client canceled", FirstResponseMs: firstMs}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return &StreamResult{ChannelFail: true, ErrorMessage: "stream duration exceeded (max " + maxDur.String() + ")", FirstResponseMs: firstMs}
		}
		return &StreamResult{ChannelFail: true, ErrorMessage: err.Error(), FirstResponseMs: firstMs}
	}
	if !ttfbStopped {
		// 极小竞态:响应头到达与 TTFB 定时器触发同时发生,请求已被取消,无法继续读取
		resp.Body.Close()
		return &StreamResult{ChannelFail: true, ErrorMessage: "upstream timeout: first response not received within " + ttfb.String(), FirstResponseMs: firstMs}
	}
	defer resp.Body.Close()

	// 非 2xx:读 body 作为错误内容,首包前失败
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		msg := string(errBody)
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		// 脱敏:仅保留状态码,不上游原始错误体(避免泄漏内部细节)
		return &StreamResult{
			ChannelFail:     isChannelFailure(resp.StatusCode, nil),
			ErrorMessage:    fmt.Sprintf("upstream HTTP %d", resp.StatusCode),
			FirstResponseMs: firstMs,
		}
	}

	// 透传响应头(过滤 hop-by-hop 头与 CORS 头,避免 Content-Length/Connection 干扰流式,
	// 也避免上游的 Access-Control-Allow-Origin 等与网关统一设置的 CORS 头合并成多值)
	for k, vv := range resp.Header {
		lowerK := strings.ToLower(k)
		if isHopByHopHeader(k) || lowerK == "content-length" || strings.HasPrefix(lowerK, "access-control-") {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(resp.StatusCode)

	// 逐行透传,扫描 data: 块截获 usage
	br := bufio.NewReader(resp.Body)
	var usage *Usage
	var writeErr error
	canceled := false // 客户端断开/上层取消(区别于渠道/上游故障)

	for {
		// 流式最长持续时间检查:超时后即使仍在输出也判定超时;
		// 客户端断开(父 ctx 取消)与 maxDur 到期需区分,避免误报超时
		if err := reqCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				writeErr = fmt.Errorf("stream duration exceeded (max %s)", maxDur)
			} else {
				canceled = true
				writeErr = fmt.Errorf("request canceled") // context.Canceled:客户端断开/上层取消
			}
			break
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				// 写失败只可能是客户端侧问题(断开/代理关闭),与上游故障无关;
				// 渠道已成功开始输出(Started),一律按"请求取消"处理,不记渠道失败
				canceled = true
				writeErr = fmt.Errorf("request canceled")
				break
			}
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			// 收集 data: 行用于 usage 解析(只保留最近若干行,防止无限增长)
			trimmed := strings.TrimSpace(string(line))
			if strings.HasPrefix(trimmed, "data:") {
				if u := parseStreamUsage(trimmed); u != nil {
					usage = u
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				writeErr = err
			}
			break
		}
	}

	if writeErr != nil {
		return &StreamResult{Started: true, ClientCanceled: canceled, ErrorMessage: writeErr.Error(), Usage: usage, FirstResponseMs: firstMs}
	}
	return &StreamResult{Started: true, Usage: usage, FirstResponseMs: firstMs}
}

// parseStreamUsage 从单行 SSE data 中解析 usage
func parseStreamUsage(dataLine string) *Usage {
	payload := strings.TrimSpace(strings.TrimPrefix(dataLine, "data:"))
	if payload == "" || payload == "[DONE]" {
		return nil
	}
	var chunk struct {
		Usage struct {
			PromptTokens        int64 `json:"prompt_tokens"`
			CompletionTokens    int64 `json:"completion_tokens"`
			TotalTokens         int64 `json:"total_tokens"`
			PromptTokensDetails struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil
	}
	if chunk.Usage.TotalTokens == 0 && chunk.Usage.PromptTokens == 0 && chunk.Usage.CompletionTokens == 0 {
		return nil
	}
	u := &Usage{
		PromptTokens:     chunk.Usage.PromptTokens,
		CompletionTokens: chunk.Usage.CompletionTokens,
		TotalTokens:      chunk.Usage.TotalTokens,
		CacheReadTokens:  chunk.Usage.PromptTokensDetails.CachedTokens,
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	return u
}

// HandleStream 主入口:流式路由 + 重试 + 降级(仅首包前失败可降级)
func (r *Router) HandleStream(ctx context.Context, w http.ResponseWriter, modelID, clientPath, apiKey, authHeader string, body []byte) (*ChannelCandidate, *StreamResult, int, error) {
	candidates, err := r.PickChannels(ctx, modelID)
	if err != nil {
		return nil, nil, 0, err
	}
	if len(candidates) == 0 {
		return nil, nil, 0, fmt.Errorf("no available channel for model %q", modelID)
	}

	attempts := 0
	// 收集各渠道失败原因,便于返回详细错误信息
	var failReasons []string
	lastCand := candidates[0] // 全部失败时返回最后尝试的渠道,供日志展示渠道名
	var trail []TrailStep     // 渠道尝试链路(每个尝试过的渠道 + 结果)

	for _, cand := range candidates {
		// 客户端已断开/上层取消:立即停止,不重试不降级(避免把取消误判为渠道故障)
		if ctx.Err() != nil {
			return nil, &StreamResult{ClientCanceled: true, ErrorMessage: "client canceled"}, attempts, nil
		}
		target, err := BuildUpstreamURL(cand.BaseURL, clientPath)
		if err != nil {
			continue
		}
		sendBody := body
		if cand.UpstreamModelName != "" {
			sendBody = ApplyModelMapping(body, cand.UpstreamModelName)
		}
		res := r.doStreamOnce(ctx, w, target, sendBody, cand.APIKey, authHeader, cand.Timeout)
		attempts++
		trail = append(trail, streamTrailStep(cand, res))
		if res.ClientCanceled {
			res.Trail = trail
			return cand, res, attempts, nil // 客户端取消:直接结束,不算渠道失败
		}
		if !res.ChannelFail {
			_ = r.store.ClearChannelFailure(cand.ChannelID)
			res.Trail = trail
			return cand, res, attempts, nil // 已开始输出或成功
		}
		// 首包前失败:重试同一渠道一次
		if r.cfg.RetrySameChannel {
			res2 := r.doStreamOnce(ctx, w, target, sendBody, cand.APIKey, authHeader, cand.Timeout)
			attempts++
			trail[len(trail)-1] = streamTrailStep(cand, res2) // 同渠道重试结果覆盖该渠道步骤
			if res2.ClientCanceled {
				res2.Trail = trail
				return cand, res2, attempts, nil // 重试期间客户端取消
			}
			if !res2.ChannelFail {
				_ = r.store.ClearChannelFailure(cand.ChannelID)
				res2.Trail = trail
				return cand, res2, attempts, nil
			}
			res = res2
		}
		// 降级到下一渠道
		r.markChannelFail(cand, &Result{ChannelFail: true, ErrorMessage: res.ErrorMessage})
		lastCand = cand
		failReasons = append(failReasons, fmt.Sprintf("%s(%s)", cand.ChannelName, streamShortErr(res)))
	}
	detail := ""
	if len(failReasons) > 0 {
		detail = ":" + strings.Join(failReasons, "; ")
	}
	last := &StreamResult{ChannelFail: true, ErrorMessage: "all channels failed for stream request" + detail, Trail: trail}
	return lastCand, last, attempts, nil
}

// streamTrailStep 将单次流式渠道尝试转为链路步骤:已开始输出视为成功,首包前失败记录原因
func streamTrailStep(cand *ChannelCandidate, res *StreamResult) TrailStep {
	s := TrailStep{ChannelID: cand.ChannelID, ChannelName: cand.ChannelName}
	switch {
	case res.ClientCanceled:
		s.Reason = "client canceled"
	case res.ChannelFail:
		s.Reason = streamShortErr(res)
	default:
		s.OK = true
	}
	return s
}

// streamShortErr 生成流式渠道失败的简短描述
func streamShortErr(res *StreamResult) string {
	if res.ErrorMessage != "" {
		return res.ErrorMessage
	}
	return "unknown error"
}

// isHopByHopHeader 判断是否为逐跳头(不应透传)
func isHopByHopHeader(k string) bool {
	switch strings.ToLower(k) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailer", "transfer-encoding", "upgrade", "content-length":
		return true
	}
	return false
}

var _ = time.Now
