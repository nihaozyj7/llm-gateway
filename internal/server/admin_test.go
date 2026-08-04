package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gateway/internal/config"
	"gateway/internal/store"
)

// newTestAdminHandler 创建用于测试的 AdminHandler
func newTestAdminHandler(t *testing.T) *AdminHandler {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewAdminHandler(st, config.Default())
}

// TestRequireLocal 验证管理 API 仅允许本机访问
func TestRequireLocal(t *testing.T) {
	mux := http.NewServeMux()
	h := newTestAdminHandler(t)
	// 用一个最小路由验证中间件(直接挂 handleMe 已不存在,改挂 handleChannels)
	h.Mount(mux)

	cases := []struct {
		name string
		ip   string
		want int
	}{
		{"ipv4 loopback", "127.0.0.1:12345", http.StatusOK},
		{"ipv6 loopback", "[::1]:12345", http.StatusOK},
		{"mapped ipv4 loopback", "[::ffff:127.0.0.1]:12345", http.StatusOK},
		{"private ip", "192.168.1.10:12345", http.StatusForbidden},
		{"public ip", "8.8.8.8:12345", http.StatusForbidden},
		{"empty", "", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/channels", nil)
			req.RemoteAddr = c.ip
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("ip=%q got %d, want %d", c.ip, rec.Code, c.want)
			}
		})
	}
}
