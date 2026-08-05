package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gateway/internal/config"
	"gateway/internal/model"
)

// TestHandleTrailOnFallback 降级后成功:链路应记录 渠道1(失败)→渠道2(成功),且渠道不重复
func TestHandleTrailOnFallback(t *testing.T) {
	up1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503) // 渠道1 失败
	}))
	defer up1.Close()
	up2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200) // 渠道2 成功
		_, _ = w.Write([]byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer up2.Close()

	cfg := config.Default()
	cfg.NonStreamTimeout = 5 * time.Second
	cfg.RetrySameChannel = true // 开启同渠道重试:验证重试失败后降级,链路中渠道不重复
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
	mID, err := r.store.UpsertModel("m-trail", "trail model")
	if err != nil {
		t.Fatalf("UpsertModel: %v", err)
	}
	if err := r.store.AddChannelModel(ch1, mID, ""); err != nil {
		t.Fatalf("AddChannelModel: %v", err)
	}
	if err := r.store.AddChannelModel(ch2, mID, ""); err != nil {
		t.Fatalf("AddChannelModel: %v", err)
	}

	cand, res, attempts, err := r.Handle(context.Background(), "m-trail", "/v1/chat/completions", "", "", []byte(`{"model":"m-trail"}`), false)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res == nil || res.ChannelFail || res.BizError {
		t.Fatalf("expected success after fallback, got %+v", res)
	}
	if cand == nil || cand.ChannelName != "chan-b" {
		t.Fatalf("expected final channel chan-b, got %+v", cand)
	}
	if attempts != 3 { // chan-a 首试失败 + 同渠道重试失败 + chan-b 成功
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	// 链路:chan-a(失败,重试覆盖)→ chan-b(成功),渠道不重复
	if len(res.Trail) != 2 {
		t.Fatalf("trail length = %d, want 2 (渠道链路不重复): %+v", len(res.Trail), res.Trail)
	}
	if res.Trail[0].ChannelName != "chan-a" || res.Trail[0].OK {
		t.Fatalf("trail[0] should be chan-a failed, got %+v", res.Trail[0])
	}
	if res.Trail[0].Reason == "" {
		t.Fatal("trail[0] should carry failure reason")
	}
	if res.Trail[1].ChannelName != "chan-b" || !res.Trail[1].OK {
		t.Fatalf("trail[1] should be chan-b ok, got %+v", res.Trail[1])
	}
}

// TestHandleTrailAllFail 全部渠道失败:链路记录所有渠道失败步骤
func TestHandleTrailAllFail(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
	}))
	defer up.Close()

	cfg := config.Default()
	cfg.NonStreamTimeout = 5 * time.Second
	cfg.RetrySameChannel = false
	r := newTestRouter(t, cfg)

	ch1, err := r.store.CreateChannel(&model.Channel{
		Name: "chan-a", BaseURL: up.URL, APIKey: "sk-1", AuthHeader: "Authorization",
		Priority: 1, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	ch2, err := r.store.CreateChannel(&model.Channel{
		Name: "chan-b", BaseURL: up.URL, APIKey: "sk-2", AuthHeader: "Authorization",
		Priority: 2, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	mID, err := r.store.UpsertModel("m-trail-fail", "trail fail model")
	if err != nil {
		t.Fatalf("UpsertModel: %v", err)
	}
	if err := r.store.AddChannelModel(ch1, mID, ""); err != nil {
		t.Fatalf("AddChannelModel: %v", err)
	}
	if err := r.store.AddChannelModel(ch2, mID, ""); err != nil {
		t.Fatalf("AddChannelModel: %v", err)
	}

	_, res, _, err := r.Handle(context.Background(), "m-trail-fail", "/v1/chat/completions", "", "", []byte(`{"model":"m-trail-fail"}`), false)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res == nil || !res.ChannelFail {
		t.Fatalf("expected all-fail, got %+v", res)
	}
	if len(res.Trail) != 2 {
		t.Fatalf("trail length = %d, want 2: %+v", len(res.Trail), res.Trail)
	}
	for i, s := range res.Trail {
		if s.OK || s.Reason == "" {
			t.Fatalf("trail[%d] should be failed with reason, got %+v", i, s)
		}
	}
}
