package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gateway/internal/config"
	"gateway/internal/model"
	"gateway/internal/store"
)

func newTestRouter(t *testing.T, cfg *config.Config) *Router {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewRouter(st, cfg)
}

// TestStreamNotKilledByTTFBTimeout 核心回归(任务8):
// 流式输出超过 TTFB 超时(upstream_timeout)不应被中断——TTFB 超时只约束「首次响应」,
// 一旦收到首包,输出应持续到流结束(受 stream_max_duration 约束)。
func TestStreamNotKilledByTTFBTimeout(t *testing.T) {
	// 上游:立即返回响应头,然后慢速输出(每 200ms 一块,共 5 块,总输出约 1s > TTFB 500ms)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		for i := 0; i < 5; i++ {
			time.Sleep(200 * time.Millisecond)
			if _, err := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")); err != nil {
				return
			}
			fl.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		fl.Flush()
	}))
	defer up.Close()

	cfg := config.Default()
	cfg.UpstreamTimeout = 500 * time.Millisecond // TTFB 很短
	cfg.StreamMaxDuration = 10 * time.Second     // 流式最长持续很长
	r := newTestRouter(t, cfg)

	rec := httptest.NewRecorder()
	res := r.doStreamOnce(context.Background(), rec, up.URL+"/chat/completions", []byte(`{"model":"m","stream":true}`), "sk-test", "", 0, nil)

	if res.ChannelFail {
		t.Fatalf("stream was wrongly killed by TTFB timeout: %v", res.ErrorMessage)
	}
	if !res.Started {
		t.Fatal("stream not started")
	}
	if res.ErrorMessage != "" {
		t.Fatalf("stream error: %v", res.ErrorMessage)
	}
	out := rec.Body.String()
	if strings.Count(out, "delta") != 5 || !strings.Contains(out, "[DONE]") {
		t.Errorf("expected 5 chunks + [DONE], got: %q", out)
	}
	if res.FirstResponseMs <= 0 || res.FirstResponseMs >= 500 {
		t.Errorf("FirstResponseMs = %d, want 0 < t < 500", res.FirstResponseMs)
	}
}

// TestStreamTTFBTimeout 首次响应超时:上游迟迟不返回响应头(慢于 TTFB)→ 判定失败且可降级
func TestStreamTTFBTimeout(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // 慢于 TTFB 300ms
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
	}))
	defer up.Close()

	cfg := config.Default()
	cfg.UpstreamTimeout = 300 * time.Millisecond
	cfg.StreamMaxDuration = 10 * time.Second
	r := newTestRouter(t, cfg)

	rec := httptest.NewRecorder()
	res := r.doStreamOnce(context.Background(), rec, up.URL+"/chat/completions", []byte(`{"model":"m","stream":true}`), "sk-test", "", 0, nil)
	if !res.ChannelFail || res.Started {
		t.Fatalf("expected TTFB timeout failure (fail=%v started=%v err=%q)", res.ChannelFail, res.Started, res.ErrorMessage)
	}
	if !strings.Contains(res.ErrorMessage, "first response not received") {
		t.Errorf("TTFB timeout message unclear: %q", res.ErrorMessage)
	}
}

// TestStreamMaxDuration 流式最长持续时间:上游持续输出超过 stream_max_duration → 判定超时(即使首包正常)
func TestStreamMaxDuration(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		for {
			time.Sleep(100 * time.Millisecond)
			if _, err := w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"y\"}}]}\n\n")); err != nil {
				return
			}
			fl.Flush()
		}
	}))
	defer up.Close()

	cfg := config.Default()
	cfg.UpstreamTimeout = 5 * time.Second      // TTFB 宽松
	cfg.StreamMaxDuration = 500 * time.Millisecond // 流式最长 500ms
	r := newTestRouter(t, cfg)

	rec := httptest.NewRecorder()
	res := r.doStreamOnce(context.Background(), rec, up.URL+"/chat/completions", []byte(`{"model":"m","stream":true}`), "sk-test", "", 0, nil)
	if res.ChannelFail {
		t.Fatalf("stream should have started before max duration: %v", res.ErrorMessage)
	}
	if !res.Started {
		t.Fatal("stream should have started")
	}
	if res.ErrorMessage == "" {
		t.Fatal("expected max-duration timeout error")
	}
}

// TestNonStreamUsesNonStreamTimeout 非流式完整超时由 non_stream_timeout 约束(任务2):
// 上游 200ms 才返回响应头+body,若仍受 upstream_timeout(100ms)约束会失败;新逻辑应成功。
func TestNonStreamUsesNonStreamTimeout(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // 慢于 upstream_timeout 100ms
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer up.Close()

	cfg := config.Default()
	cfg.UpstreamTimeout = 100 * time.Millisecond // 若用于整个非流式请求会失败
	cfg.NonStreamTimeout = 5 * time.Second       // 非流式完整超时足够长
	r := newTestRouter(t, cfg)

	res := r.doOnce(context.Background(), up.URL+"/chat/completions", []byte(`{"model":"m"}`), "sk-test", "")
	if res.ChannelFail {
		t.Fatalf("non-stream failed: %v", res.ErrorMessage)
	}
	if res.FirstResponseMs <= 0 || res.LatencyMs < res.FirstResponseMs {
		t.Errorf("timing wrong: first=%d latency=%d", res.FirstResponseMs, res.LatencyMs)
	}
	if res.Usage == nil || res.Usage.TotalTokens != 3 {
		t.Errorf("usage = %+v, want total 3", res.Usage)
	}
}

// TestNonStreamIgnoresChannelTimeout 回归:渠道级 timeout_ms 只约束流式 TTFB,
// 非流式请求必须使用全局 non_stream_timeout,不能被渠道级短超时掐死
// (MCP/图片等慢速非流式请求曾因此全部渠道超时失败)。
func TestNonStreamIgnoresChannelTimeout(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond) // 慢于渠道级 200ms,但远快于 non_stream_timeout
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer up.Close()

	cfg := config.Default()
	cfg.NonStreamTimeout = 5 * time.Second
	cfg.UpstreamTimeout = 100 * time.Millisecond // 流式 TTFB 很短,不影响非流式
	r := newTestRouter(t, cfg)

	// 渠道配置极短超时 200ms:若被错误用于非流式整体,请求必然超时
	chID, err := r.store.CreateChannel(&model.Channel{
		Name:       "slow-up",
		BaseURL:    up.URL,
		APIKey:     "sk-test",
		AuthHeader: "Authorization",
		Priority:   1,
		Enabled:    true,
		TimeoutMs:  200,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	mID, err := r.store.UpsertModel("m-slow", "slow model")
	if err != nil {
		t.Fatalf("UpsertModel: %v", err)
	}
	if err := r.store.AddChannelModel(chID, mID, ""); err != nil {
		t.Fatalf("AddChannelModel: %v", err)
	}

	cand, res, _, err := r.Handle(context.Background(), "m-slow", "/v1/chat/completions", "", "", []byte(`{"model":"m-slow"}`), false)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res == nil || res.ChannelFail {
		t.Fatalf("non-stream must not be killed by channel-level timeout: %+v (cand=%v)", res, cand)
	}
}

// TestHandleAllFailReturnsChannelDetail 全部渠道失败时:
// 1) 返回最后尝试的渠道(供日志展示失败渠道名);
// 2) 错误信息汇总各渠道具体失败原因(而非空泛的 all channels failed)。
func TestHandleAllFailReturnsChannelDetail(t *testing.T) {
	// 两个上游都返回 503,触发渠道失败
	up1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer up1.Close()
	up2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer up2.Close()

	cfg := config.Default()
	cfg.NonStreamTimeout = 5 * time.Second
	cfg.RetrySameChannel = false // 避免同渠道重试,聚焦降级链路
	r := newTestRouter(t, cfg)

	ch1, err := r.store.CreateChannel(&model.Channel{
		Name: "chan-a", BaseURL: up1.URL, APIKey: "sk-1", AuthHeader: "Authorization",
		Priority: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	ch2, err := r.store.CreateChannel(&model.Channel{
		Name: "chan-b", BaseURL: up2.URL, APIKey: "sk-2", AuthHeader: "Authorization",
		Priority: 2, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	mID, err := r.store.UpsertModel("m-fail", "fail model")
	if err != nil {
		t.Fatalf("UpsertModel: %v", err)
	}
	if err := r.store.AddChannelModel(ch1, mID, ""); err != nil {
		t.Fatalf("AddChannelModel: %v", err)
	}
	if err := r.store.AddChannelModel(ch2, mID, ""); err != nil {
		t.Fatalf("AddChannelModel: %v", err)
	}

	cand, res, _, err := r.Handle(context.Background(), "m-fail", "/v1/chat/completions", "", "", []byte(`{"model":"m-fail"}`), false)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res == nil || !res.ChannelFail {
		t.Fatalf("expected all-channels failure, got %+v", res)
	}
	// 应返回最后尝试的渠道(chan-b),供日志展示失败渠道名
	if cand == nil || cand.ChannelName != "chan-b" {
		t.Fatalf("expected last attempted channel chan-b, got %+v", cand)
	}
	// 错误信息应包含两个渠道的具体失败原因(HTTP 503)
	if !strings.Contains(res.ErrorMessage, "chan-a") || !strings.Contains(res.ErrorMessage, "chan-b") {
		t.Errorf("error message should list per-channel reasons, got %q", res.ErrorMessage)
	}
	if !strings.Contains(res.ErrorMessage, "503") {
		t.Errorf("error message should include HTTP status, got %q", res.ErrorMessage)
	}
}

// TestNonStreamTTFBTimeout 非流式首次响应超时:上游迟迟不返回响应头(慢于 non_stream_timeout 上限)→ 失败
func TestNonStreamTTFBTimeout(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
	}))
	defer up.Close()

	cfg := config.Default()
	cfg.NonStreamTimeout = 300 * time.Millisecond
	r := newTestRouter(t, cfg)

	res := r.doOnce(context.Background(), up.URL+"/chat/completions", []byte(`{"model":"m"}`), "sk-test", "")
	if !res.ChannelFail {
		t.Fatal("expected non-stream timeout failure")
	}
}
