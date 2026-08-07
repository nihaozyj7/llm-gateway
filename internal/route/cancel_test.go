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

// TestStreamClientCancelAfterStart 回归:流式已开始输出后客户端断开。
// 客户端断开发生在 ReadBytes 阻塞等待上游数据期间:reqCtx 被取消 → Transport 关闭连接
// → ReadBytes 返回 context.Canceled。此前该错误未识别为客户端取消,被记成 ChannelFail/失败,
// 导致日志状态显示「失败」而非「客户端断开」(同错误文本两种状态)。修复后应判 ClientCanceled。
func TestStreamClientCancelAfterStart(t *testing.T) {
	cfg := config.Default()
	cfg.UpstreamTimeout = 10 * time.Second   // TTFB 足够长
	cfg.StreamMaxDuration = 30 * time.Second // 流式最长足够长
	// 渠道 A:返回 200 后挂起,不再输出(ReadBytes 阻塞等待中);客户端断开后连接被关闭。
	upA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		// 挂起:不写更多数据;客户端断开后 Transport 会关闭连接,本 handler 的 ctx 随之取消
		<-r.Context().Done()
	}))
	defer upA.Close()

	r := newTestRouter(t, cfg)
	ch, err := r.store.CreateChannel(&model.Channel{
		Name: "chan-a", BaseURL: upA.URL, APIKey: "sk-1", AuthHeader: "Authorization",
		Priority: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	mID, err := r.store.UpsertModel("m-cancel-after", "cancel after start")
	if err != nil {
		t.Fatalf("UpsertModel: %v", err)
	}
	if err := r.store.AddChannelModel(ch, mID, ""); err != nil {
		t.Fatalf("AddChannelModel: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 自定义 ResponseWriter:网关侧 WriteHeader(Do 已返回、正阻塞 ReadBytes)时触发客户端断开,
	// 确保稳定命中「ReadBytes 阻塞期间 reqCtx 取消」路径而非 Do 阶段取消。
	started := make(chan struct{})
	rec := &signalRecorder{rec: httptest.NewRecorder(), onHeader: func() {
		select {
		case <-started:
		default:
			close(started)
		}
	}}
	go func() {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Error("网关未在超时内开始写响应头")
			return
		}
		cancel() // 已开始输出后客户端断开
	}()

	cand, res, attempts, err := r.HandleStream(ctx, rec, "m-cancel-after", "/v1/chat/completions", "", "", []byte(`{"model":"m-cancel-after","stream":true}`))
	if err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	if res == nil {
		t.Fatalf("res 为空")
	}
	if !res.ClientCanceled {
		t.Fatalf("输出中途客户端断开应判 ClientCanceled,实际 %+v (ErrorMessage=%q)", res, res.ErrorMessage)
	}
	if res.ChannelFail {
		t.Fatalf("客户端取消不得判为渠道失败: %+v", res)
	}
	if !res.Started {
		t.Fatalf("已开始输出后取消应保持 Started=true: %+v", res)
	}
	if cand == nil || cand.ChannelName != "chan-a" {
		t.Fatalf("应停在 chan-a,实际 %+v", cand)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

// signalRecorder 包装 httptest.ResponseRecorder,首次 WriteHeader 时回调(用于感知网关开始写响应)。
type signalRecorder struct {
	rec       *httptest.ResponseRecorder
	onHeader  func()
	headerSet bool
}

func (s *signalRecorder) Header() http.Header { return s.rec.Header() }
func (s *signalRecorder) WriteHeader(code int) {
	if !s.headerSet {
		s.headerSet = true
		s.onHeader()
	}
	s.rec.WriteHeader(code)
}
func (s *signalRecorder) Write(p []byte) (int, error) { return s.rec.Write(p) }
func (s *signalRecorder) Flush()                      { s.rec.Flush() }
