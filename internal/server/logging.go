package server

import (
	"encoding/json"
	"time"

	"gateway/internal/model"
	"gateway/internal/route"
)

// storeLogEntry 日志记录中间态
type storeLogEntry struct {
	RequestID       string
	KeyName         string // 调用所使用的 API Key 名称
	ChannelID       int64
	ChannelName     string
	Model           string
	UpstreamModel   string // 实际转发给上游的模型(渠道映射后,未映射时与 Model 相同)
	Status          string
	Error           string
	SourceIP        string
	PayloadReq      string
	FirstResponseMs int64 // 请求发起 → 收到首次响应耗时(毫秒)
	ChannelTrail    string // 渠道尝试链路 JSON(概览展示 渠道1(失败)→渠道2(成功))
}

// trailJSON 将渠道尝试链路序列化为 JSON 文本(空链路返回空串)
func trailJSON(t []route.TrailStep) string {
	if len(t) == 0 {
		return ""
	}
	b, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return string(b)
}

// writeLog 落库日志 + 更新统计,并按模型价格计算成本
// pt/ct/cache/tt 分别为 prompt/completion/缓存读取/total tokens;价格为元/百万 token
func (h *GatewayHandler) writeLog(start time.Time, e *storeLogEntry, latencyMs int64, respBody []byte, pt, ct, cache, tt int64, httpStatus int) {
	// 计算成本:按模型价格
	var cost float64
	m, _ := h.store.GetModelByID(e.Model)
	if m != nil {
		cost = route.Cost(pt, cache, ct, m.PriceInput, m.PriceCacheRead, m.PriceOutput)
	}
	payloadResp := ""
	if h.cfg.LogPayloads && respBody != nil && len(respBody) < 64*1024 {
		payloadResp = string(respBody)
	}
	if h.cfg.LogPayloads && len(e.PayloadReq) > 64*1024 {
		e.PayloadReq = e.PayloadReq[:64*1024]
	}
	l := &model.RequestLog{
		RequestTime:      start,
		RequestID:        e.RequestID,
		APIKeyName:       e.KeyName,
		ChannelID:        e.ChannelID,
		ChannelName:      e.ChannelName,
		Model:            e.Model,
		UpstreamModel:    e.UpstreamModel,
		Status:           e.Status,
		LatencyMs:        latencyMs,
		FirstResponseMs:  e.FirstResponseMs,
		PromptTokens:     pt,
		CompletionTokens: ct,
		TotalTokens:      tt,
		CacheReadTokens:  cache,
		Cost:             cost,
		Error:            e.Error,
		SourceIP:         e.SourceIP,
		PayloadRequest:   e.PayloadReq,
		PayloadResponse:  payloadResp,
		ChannelTrail:     e.ChannelTrail,
	}
	if _, err := h.store.InsertLog(l); err == nil {
		_ = h.store.RecordStat(l)
	}
}
