package route

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// StreamResult 流式转发结果
type StreamResult struct {
	ChannelFail  bool // 首包前失败(可降级)
	Started      bool // 是否已向客户端写入首字节
	ErrorMessage string
	Usage        *Usage
}

// doStreamOnce 单次流式转发:透传 SSE,同时截获 usage
// 首包前失败返回 ChannelFail=true(调用方可降级重试);已开始输出则不再重试
func (r *Router) doStreamOnce(ctx context.Context, w http.ResponseWriter, target string, body []byte, apiKey, authHeader string) *StreamResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
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

	resp, err := r.client.Do(req)
	if err != nil {
		return &StreamResult{ChannelFail: true, ErrorMessage: err.Error()}
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
			ChannelFail:  isChannelFailure(resp.StatusCode, nil),
			ErrorMessage: fmt.Sprintf("upstream HTTP %d", resp.StatusCode),
		}
	}

	// 透传响应头(过滤 hop-by-hop 头,避免 Content-Length/Connection 干扰流式)
	for k, vv := range resp.Header {
		if isHopByHopHeader(k) || strings.EqualFold(k, "Content-Length") {
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
	buf := make([]byte, 0, 4096)
	var usage *Usage
	var writeErr error

	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				writeErr = werr
				break
			}
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			// 收集 data: 行用于 usage 解析(只保留最近若干行,防止无限增长)
			trimmed := strings.TrimSpace(string(line))
			if strings.HasPrefix(trimmed, "data:") {
				buf = append(buf, trimmed...)
				if len(buf) > 64*1024 {
					buf = buf[len(buf)-64*1024:]
				}
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

	_ = buf
	if writeErr != nil {
		return &StreamResult{Started: true, ErrorMessage: writeErr.Error(), Usage: usage}
	}
	return &StreamResult{Started: true, Usage: usage}
}

// parseStreamUsage 从单行 SSE data 中解析 usage
func parseStreamUsage(dataLine string) *Usage {
	payload := strings.TrimSpace(strings.TrimPrefix(dataLine, "data:"))
	if payload == "" || payload == "[DONE]" {
		return nil
	}
	var chunk struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
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
	for _, cand := range candidates {
		target, err := BuildUpstreamURL(cand.BaseURL, clientPath)
		if err != nil {
			continue
		}
		sendBody := body
		if cand.UpstreamModelName != "" {
			sendBody = ApplyModelMapping(body, cand.UpstreamModelName)
		}
		res := r.doStreamOnce(ctx, w, target, sendBody, cand.APIKey, authHeader)
		attempts++
		if !res.ChannelFail {
			return cand, res, attempts, nil // 已开始输出或成功
		}
		// 首包前失败:重试同一渠道一次
		if r.cfg.RetrySameChannel {
			res2 := r.doStreamOnce(ctx, w, target, sendBody, cand.APIKey, authHeader)
			attempts++
			if !res2.ChannelFail {
				return cand, res2, attempts, nil
			}
			res = res2
		}
		// 降级到下一渠道
		r.markChannelFail(cand.ChannelID, &Result{ChannelFail: true, ErrorMessage: res.ErrorMessage})
	}
	last := &StreamResult{ChannelFail: true, ErrorMessage: "all channels failed for stream request"}
	return nil, last, attempts, nil
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
