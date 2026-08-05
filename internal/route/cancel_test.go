package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gateway/internal/config"
	"gateway/internal/model"
)

// setupCancelTest 构建两个渠道(chan-a 挂起、chan-b 记录调用)绑定到同一模型。
// 返回 router、chan-a 渠道 ID、chan-b 调用计数、上游服务器关闭函数。
func setupCancelTest(t *testing.T, cfg *config.Config) (*Router, int64, *int32, func()) {
	t.Helper()
	// 渠道 A:挂起不返回响应头(模拟上游慢,等待客户端取消)。
	// 注意:不依赖 r.Context().Done() 退出——Go/Windows 下带 body 请求取消时
	// Transport 可能不关闭连接(服务器端 ctx 不触发),故用固定超时兜底避免测试僵局。
	upA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	// 渠道 B:记录被调用次数(降级到它即为 bug)
	var callsB int32
	upB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callsB, 1)
		w.WriteHeader(503)
	}))

	r := newTestRouter(t, cfg)

	ch1, err := r.store.CreateChannel(&model.Channel{
		Name: "chan-a", BaseURL: upA.URL, APIKey: "sk-1", AuthHeader: "Authorization",
		Priority: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateChannel chan-a: %v", err)
	}
	ch2, err := r.store.CreateChannel(&model.Channel{
		Name: "chan-b", BaseURL: upB.URL, APIKey: "sk-2", AuthHeader: "Authorization",
		Priority: 2, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateChannel chan-b: %v", err)
	}
	mID, err := r.store.UpsertModel("m-cancel", "cancel model")
	if err != nil {
		t.Fatalf("UpsertModel: %v", err)
	}
	if err := r.store.AddChannelModel(ch1, mID, ""); err != nil {
		t.Fatalf("AddChannelModel ch1: %v", err)
	}
	if err := r.store.AddChannelModel(ch2, mID, ""); err != nil {
		t.Fatalf("AddChannelModel ch2: %v", err)
	}
	cleanup := func() { upA.Close(); upB.Close() }
	return r, ch1, &callsB, cleanup
}

// assertNoChannelFail 校验客户端取消结果:不判渠道失败、不重试不降级、失败计数不增加
func assertNoChannelFail(t *testing.T, r *Router, ch1 int64, callsB *int32, attempts int) {
	t.Helper()
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1(客户端取消不应重试/降级)", attempts)
	}
	if atomic.LoadInt32(callsB) != 0 {
		t.Fatalf("chan-b 不应被调用,实际 %d 次(客户端取消不应降级)", atomic.LoadInt32(callsB))
	}
	chAfter, err := r.store.GetChannel(ch1)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if chAfter.FailureCount != 0 || chAfter.Status == "cooldown" {
		t.Fatalf("客户端取消不应累计渠道失败计数/进入冷却: %+v", chAfter)
	}
}

// TestNonStreamClientCancelNoFallback 回归:客户端主动断开(请求 ctx 取消)时,
// context.Canceled 不得被当作渠道失败触发同渠道重试/降级,也不得累计渠道失败计数。
func TestNonStreamClientCancelNoFallback(t *testing.T) {
	cfg := config.Default()
	cfg.NonStreamTimeout = 10 * time.Second // 足够长,确保是客户端取消而非超时
	r, ch1, callsB, cleanup := setupCancelTest(t, cfg)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel) // 模拟客户端在等待响应期间断开

	cand, res, attempts, err := r.Handle(ctx, "m-cancel", "/v1/chat/completions", "", "", []byte(`{"model":"m-cancel"}`), false)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res == nil || !res.ClientCanceled {
		t.Fatalf("expected ClientCanceled, got %+v", res)
	}
	if res.ChannelFail {
		t.Fatalf("客户端取消不得标记为渠道失败: %+v", res)
	}
	if cand == nil || cand.ChannelName != "chan-a" {
		t.Fatalf("应停在首次尝试的 chan-a,实际 %+v", cand)
	}
	assertNoChannelFail(t, r, ch1, callsB, attempts)
}

// TestStreamClientCancelNoFallback 回归(流式):首包前客户端断开,不得重试/降级。
func TestStreamClientCancelNoFallback(t *testing.T) {
	cfg := config.Default()
	cfg.UpstreamTimeout = 10 * time.Second     // TTFB 足够长,确保是客户端取消而非 TTFB 超时
	cfg.StreamMaxDuration = 30 * time.Second   // 流式最长足够长
	cfg.RetrySameChannel = true                // 显式开启同渠道重试,验证取消时不触发
	r, ch1, callsB, cleanup := setupCancelTest(t, cfg)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	rec := httptest.NewRecorder()
	cand, res, attempts, err := r.HandleStream(ctx, rec, "m-cancel", "/v1/chat/completions", "", "", []byte(`{"model":"m-cancel","stream":true}`))
	if err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	if res == nil || !res.ClientCanceled {
		t.Fatalf("expected ClientCanceled, got %+v", res)
	}
	if res.ChannelFail || res.Started {
		t.Fatalf("首包前客户端取消不得判为渠道失败/已开始: %+v", res)
	}
	if cand == nil || cand.ChannelName != "chan-a" {
		t.Fatalf("应停在首次尝试的 chan-a,实际 %+v", cand)
	}
	assertNoChannelFail(t, r, ch1, callsB, attempts)
}
