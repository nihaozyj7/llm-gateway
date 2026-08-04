package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config 网关配置
type Config struct {
	// Listen 监听地址
	Listen string `json:"listen"`
	// DBPath SQLite 文件路径
	DBPath string `json:"db_path"`
	// DataDir 数据目录(默认当前目录 .data)
	DataDir string `json:"data_dir"`
	// AdminUsername 初始管理员用户名(首次启动创建)
	AdminUsername string `json:"admin_username"`
	// AdminPassword 初始管理员密码(首次启动创建)
	AdminPassword string `json:"admin_password"`
	// CooldownDuration 渠道冷静时长
	CooldownDuration time.Duration `json:"cooldown_duration"`
	// CooldownThreshold 连续失败多少次触发冷静
	CooldownThreshold int `json:"cooldown_threshold"`
	// UpstreamTimeout 流式请求首次响应(TTFB)超时:仅对流式生效,
	// 指请求发起后到收到第一个响应的时间;收到首包后不再受此限制。
	// 非流式请求由 NonStreamTimeout 整体约束,不使用此值
	UpstreamTimeout time.Duration `json:"upstream_timeout"`
	// NonStreamTimeout 非流式请求完整超时(含读取完整响应体),默认 5 分钟
	NonStreamTimeout time.Duration `json:"non_stream_timeout"`
	// StreamMaxDuration 流式请求最长持续时间:超过后即使仍在输出也判定超时,默认 6 分钟
	StreamMaxDuration time.Duration `json:"stream_max_duration"`
	// MaxAttemptsPerRequest 单请求最多尝试渠道数(0=不限)
	MaxAttemptsPerRequest int `json:"max_attempts_per_request"`
	// RetrySameChannel 失败后是否重试同一渠道一次
	RetrySameChannel bool `json:"retry_same_channel"`
	// SessionSecret 管理会话签名密钥(自动生成持久化)
	SessionSecret string `json:"session_secret"`
	// LogPayloads 是否记录请求/响应体
	LogPayloads bool `json:"log_payloads"`
	// OpenBrowser 启动后是否自动打开浏览器管理界面
	OpenBrowser bool `json:"open_browser"`
}

// Default 返回默认配置
func Default() *Config {
	return &Config{
		Listen:                ":8080",
		DBPath:                "",
		DataDir:               ".data",
		AdminUsername:         "admin",
		AdminPassword:         "admin123",
		CooldownDuration:      10 * time.Minute,
		CooldownThreshold:     1,
		UpstreamTimeout:       60 * time.Second,
		NonStreamTimeout:      5 * time.Minute,
		StreamMaxDuration:     6 * time.Minute,
		MaxAttemptsPerRequest: 0,
		RetrySameChannel:      true,
		LogPayloads:           true,
		OpenBrowser:           true,
	}
}

// Load 加载配置:先读默认,再读 config.json(若存在)覆盖
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "gateway.db")
	}
	if cfg.SessionSecret == "" {
		// 生成并持久化,避免重启后会话失效
		cfg.SessionSecret = randomHex(32)
		_ = cfg.Save(path)
	}
	return cfg, nil
}

// Save 保存配置到文件
func (c *Config) Save(path string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ResolveDBPath 返回数据库绝对路径,并确保目录存在
func (c *Config) ResolveDBPath() (string, error) {
	if err := os.MkdirAll(filepath.Dir(c.DBPath), 0o755); err != nil {
		return "", err
	}
	return c.DBPath, nil
}
