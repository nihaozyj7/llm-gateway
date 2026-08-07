package route

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"gateway/internal/config"
	"gateway/internal/model"
)

// TestStreamDoneThenClientDisconnect 回归:客户端在收到 data: [DONE] 后立即断开,
// 而上游在 DONE 之后仍发送尾部数据(空行/多余行)时,不得被误判为「客户端断开」。
// 旧逻辑会继续透传 DONE 之后的尾部行,写入已断开的连接失败 → ClientCanceled=true,
// 导致日志出现大量「客户端断开」。修复后读到 [DONE] 即终止转发,视为正常成功结束。
func TestStreamDoneThenClientDisconnect(t *testing.T) {
	cfg := config.Default()
	cfg.UpstreamTimeout = 5 * time.Second
	cfg.StreamMaxDuration = 30 * time.Second
	// 上游:正常 chunk → [DONE] → 尾部数据(模拟部分厂商在 DONE 后再发空行)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		fl.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n"))
		fl.Flush()
		_, _ = w.Write([]byte("\n")) // 尾部数据:DONE 之后仍有内容
		fl.Flush()
	}))
	defer up.Close()

	r := newTestRouter(t, cfg)
	ch, err := r.store.CreateChannel(&model.Channel{
		Name: "chan-a", BaseURL: up.URL, APIKey: "sk-1", AuthHeader: "Authorization",
		Priority: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	mID, err := r.store.UpsertModel("m-done", "done model")
	if err != nil {
		t.Fatalf("UpsertModel: %v", err)
	}
	if err := r.store.AddChannelModel(ch, mID, ""); err != nil {
		t.Fatalf("AddChannelModel: %v", err)
	}

	rec := &failAfterDoneRecorder{rec: httptest.NewRecorder()}
	var startedCount int32
	_, res, attempts, err := r.HandleStream(context.Background(), rec, "m-done", "/v1/chat/completions", "", "", []byte(`{"model":"m-done","stream":true}`), func() {
		atomic.AddInt32(&startedCount, 1)
	})
	if err != nil {
		t.Fatalf("HandleStream: %v", err)
	}
	if res == nil {
		t.Fatal("res 为空")
	}
	if res.ClientCanceled {
		t.Fatalf("DONE 之后客户端断开不得判为客户端断开: %+v (ErrorMessage=%q)", res, res.ErrorMessage)
	}
	if res.ChannelFail {
		t.Fatalf("不得判为渠道失败: %+v", res)
	}
	if !res.Started {
		t.Fatal("流应已开始输出")
	}
	if res.ErrorMessage != "" {
		t.Fatalf("不应有错误信息: %q", res.ErrorMessage)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if atomic.LoadInt32(&startedCount) != 1 {
		t.Fatalf("onStarted 回调应恰好触发 1 次,实际 %d", atomic.LoadInt32(&startedCount))
	}
	if !rec.done {
		t.Fatal("测试前提失效:客户端应已模拟断开")
	}
}

// failAfterDoneRecorder 模拟「收到 [DONE] 后立即断开」的客户端:
// 一旦写出过含 [DONE] 的内容,后续 Write 一律返回错误。
type failAfterDoneRecorder struct {
	rec  *httptest.ResponseRecorder
	done bool
}

func (f *failAfterDoneRecorder) Header() http.Header { return f.rec.Header() }
func (f *failAfterDoneRecorder) WriteHeader(code int) {
	f.rec.WriteHeader(code)
}
func (f *failAfterDoneRecorder) Write(p []byte) (int, error) {
	if f.done {
		return 0, errors.New("client disconnected after DONE")
	}
	if bytes.Contains(p, []byte("[DONE]")) {
		f.done = true
	}
	return f.rec.Write(p)
}
func (f *failAfterDoneRecorder) Flush() {}
