package route

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gateway/internal/config"
)

// TestStreamStripsUpstreamCORSHeaders 回归(CORS 双值头问题):
// 上游返回 Access-Control-Allow-Origin 等 CORS 头时,网关不得透传,
// 避免与网关 handleV1 统一设置的 CORS 头合并成多值,被浏览器以
// "The 'Access-Control-Allow-Origin' header contains multiple values" 拦截。
// 非 CORS 头(Content-Type、自定义头)仍应正常透传。
func TestStreamStripsUpstreamCORSHeaders(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
		w.Header().Set("Access-Control-Expose-Headers", "X-Trace")
		w.Header().Set("X-Upstream-Marker", "yes")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer up.Close()

	r := newTestRouter(t, config.Default())
	rec := httptest.NewRecorder()
	res := r.doStreamOnce(context.Background(), rec, up.URL+"/chat/completions", []byte(`{"model":"m","stream":true}`), "sk-test", "", 0)

	if res.ChannelFail || res.ErrorMessage != "" {
		t.Fatalf("stream failed: %v", res.ErrorMessage)
	}
	if !res.Started {
		t.Fatal("stream not started")
	}
	for _, h := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Expose-Headers",
		"Access-Control-Allow-Credentials",
		"Access-Control-Max-Age",
	} {
		if v := rec.Header().Get(h); v != "" {
			t.Errorf("upstream CORS header %q must not be passed through, got %q", h, v)
		}
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type not passed through: %q", got)
	}
	if got := rec.Header().Get("X-Upstream-Marker"); got != "yes" {
		t.Errorf("non-CORS header not passed through: %q", got)
	}
}
