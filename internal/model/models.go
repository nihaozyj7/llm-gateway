package model

import "time"

// Channel 上游渠道(OpenAI 兼容端点)
type Channel struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	BaseURL       string    `json:"base_url"`
	APIKey        string    `json:"api_key"`
	AuthHeader    string    `json:"auth_header"` // 自定义鉴权头名,默认 Authorization
	Priority      int       `json:"priority"`    // 越小越优先
	Enabled       bool      `json:"enabled"`
	Status        string    `json:"status"` // normal / cooldown
	CooldownUntil time.Time `json:"cooldown_until"`
	FailureCount  int       `json:"failure_count"`
	LastError     string    `json:"last_error"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Model 聚合模型(同名全局唯一)
type Model struct {
	ID             int64     `json:"id"`
	ModelID        string    `json:"model_id"` // 如 gpt-4o
	DisplayName    string    `json:"display_name"`
	PriceInput     *float64  `json:"price_input"`     // 元/百万 token,可空
	PriceOutput    *float64  `json:"price_output"`    // 元/百万 token,可空
	PriceCacheRead *float64  `json:"price_cache_read"` // 缓存读取价,元/百万 token,可空
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ChannelModel 渠道-模型关联(含渠道内模型名映射与模型级优先级)
type ChannelModel struct {
	ID                int64     `json:"id"`
	ChannelID         int64     `json:"channel_id"`
	ModelID           int64     `json:"model_id"`
	UpstreamModelName string    `json:"upstream_model_name"` // 空=同名透传
	Priority          int       `json:"priority"`           // 模型级优先级,越小越优先;默认继承渠道全局优先级
	CreatedAt         time.Time `json:"created_at"`
}

// APIKey 网关 API 密钥
type APIKey struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	KeyHash    string    `json:"-"`
	KeyPrefix  string    `json:"key_prefix"`  // 明文前缀 8 位用于展示
	KeySecret  string    `json:"key_secret"`  // 完整密钥明文(本地自用,允许随时查看/复制)
	Enabled    bool      `json:"enabled"`
	UsageCount int64     `json:"usage_count"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// Admin 管理员
type Admin struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// RequestLog 请求日志
type RequestLog struct {
	ID               int64     `json:"id"`
	RequestTime      time.Time `json:"request_time"`
	RequestID        string    `json:"request_id"`
	ChannelID        int64     `json:"channel_id"`
	ChannelName      string    `json:"channel_name"`
	Model            string    `json:"model"`
	Status           string    `json:"status"` // success / fail / biz_error / retry_success
	LatencyMs        int64     `json:"latency_ms"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	TotalTokens      int64     `json:"total_tokens"`
	CacheReadTokens  int64     `json:"cache_read_tokens"` // prompt 中命中缓存的部分(成本按缓存价计)
	Cost             float64   `json:"cost"`
	Error            string    `json:"error"`
	SourceIP         string    `json:"source_ip"`
	PayloadRequest   string    `json:"payload_request,omitempty"`  // 请求体(可选)
	PayloadResponse  string    `json:"payload_response,omitempty"` // 响应体(可选)
}

// StatRow 聚合统计行(日/小时)
type StatRow struct {
	Period           string  `json:"period"`
	ChannelID        int64   `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	Model            string  `json:"model"`
	RequestCount     int64   `json:"request_count"`
	SuccessCount     int64   `json:"success_count"`
	FailCount        int64   `json:"fail_count"`
	BizErrorCount    int64   `json:"biz_error_count"`
	TotalLatencyMs   int64   `json:"total_latency_ms"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Cost             float64 `json:"cost"`
}
