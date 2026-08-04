package store

import (
	"testing"
	"time"

	"gateway/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// TestLogUpstreamModelRoundTrip 验证日志 upstream_model 落库与读取往返(含渠道映射场景)
func TestLogUpstreamModelRoundTrip(t *testing.T) {
	st := newTestStore(t)
	now := time.Now()
	l := &model.RequestLog{
		RequestTime:      now,
		RequestID:        "req_test_001",
		APIKeyName:       "测试密钥",
		ChannelID:        7,
		ChannelName:      "chan-b",
		Model:            "a",
		UpstreamModel:    "b", // 渠道映射后实际转发模型
		Status:           "success",
		LatencyMs:        123,
		FirstResponseMs:  45,
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
		Cost:             0.42,
	}
	id, err := st.InsertLog(l)
	if err != nil {
		t.Fatalf("InsertLog: %v", err)
	}

	got, err := st.GetLog(id)
	if err != nil {
		t.Fatalf("GetLog: %v", err)
	}
	if got == nil {
		t.Fatal("GetLog returned nil")
	}
	if got.UpstreamModel != "b" {
		t.Errorf("UpstreamModel = %q, want %q", got.UpstreamModel, "b")
	}
	if got.Model != "a" {
		t.Errorf("Model = %q, want %q", got.Model, "a")
	}
	if got.APIKeyName != "测试密钥" {
		t.Errorf("APIKeyName = %q, want %q", got.APIKeyName, "测试密钥")
	}
	if got.FirstResponseMs != 45 {
		t.Errorf("FirstResponseMs = %d, want 45", got.FirstResponseMs)
	}

	// ListLogs 也带出新列
	logs, total, err := st.ListLogs(nil, "", "", "", "", 0, 10)
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].UpstreamModel != "b" {
		t.Errorf("ListLogs = %d logs (total %d), first upstream=%q; want 1 log with upstream 'b'", len(logs), total, logs[0].UpstreamModel)
	}

	// 按密钥名称筛选
	logs, total, err = st.ListLogs(nil, "", "", "测试密钥", "", 0, 10)
	if err != nil {
		t.Fatalf("ListLogs keyName: %v", err)
	}
	if total != 1 || logs[0].APIKeyName != "测试密钥" {
		t.Errorf("keyName filter = %d logs (total %d); want 1 matching log", len(logs), total)
	}
	logs, total, err = st.ListLogs(nil, "", "", "不存在的密钥", "", 0, 10)
	if err != nil {
		t.Fatalf("ListLogs keyName no-match: %v", err)
	}
	if total != 0 {
		t.Errorf("keyName no-match filter = total %d; want 0", total)
	}
}

// TestChannelTimeoutFieldsRoundTrip 验证渠道级超时/冷静期毫秒字段落库与读取往返
func TestChannelTimeoutFieldsRoundTrip(t *testing.T) {
	st := newTestStore(t)
	id, err := st.CreateChannel(&model.Channel{
		Name:       "chan-test",
		BaseURL:    "https://api.example.com/v1",
		APIKey:     "sk-test",
		AuthHeader: "Authorization",
		Priority:   1,
		Enabled:    true,
		TimeoutMs:  3000,   // 3 秒
		CooldownMs: 120000, // 2 分钟
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	c, err := st.GetChannel(id)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if c == nil {
		t.Fatal("GetChannel returned nil")
	}
	if c.TimeoutMs != 3000 || c.CooldownMs != 120000 {
		t.Errorf("TimeoutMs=%d CooldownMs=%d, want 3000/120000", c.TimeoutMs, c.CooldownMs)
	}

	// 更新后仍保留
	c.TimeoutMs = 5000
	c.CooldownMs = 0 // 0 = 回退全局
	if err := st.UpdateChannel(c); err != nil {
		t.Fatalf("UpdateChannel: %v", err)
	}
	c2, err := st.GetChannel(id)
	if err != nil {
		t.Fatalf("GetChannel after update: %v", err)
	}
	if c2.TimeoutMs != 5000 || c2.CooldownMs != 0 {
		t.Errorf("after update TimeoutMs=%d CooldownMs=%d, want 5000/0", c2.TimeoutMs, c2.CooldownMs)
	}
}
