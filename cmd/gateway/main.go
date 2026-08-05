package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gateway/internal/config"
	"gateway/internal/route"
	"gateway/internal/server"
	"gateway/internal/store"
	"gateway/web"
)

var version = "0.7.0"

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.json", "配置文件路径(不存在则使用默认并创建)")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dbPath, err := cfg.ResolveDBPath()
	if err != nil {
		log.Fatalf("resolve db path: %v", err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	router := route.NewRouter(st, cfg)

	adminHandler := server.NewAdminHandler(st, cfg)

	gatewayHandler := server.NewGatewayHandler(st, cfg, router)

	mux := http.NewServeMux()
	adminHandler.Mount(mux)
	gatewayHandler.Mount(mux)
	mountFrontend(mux)

	adminURL := localAdminURL(cfg.Listen)
	log.Printf("大模型转发网关 v%s 启动,监听 %s", version, cfg.Listen)
	log.Printf("SQLite: %s", dbPath)
	if cfg.OpenBrowser {
		log.Printf("管理界面: %s (仅本机可访问,启动后将自动打开浏览器;设置 open_browser=false 可关闭)", adminURL)
		go openBrowserWhenReady(cfg.Listen, adminURL)
	} else {
		log.Printf("管理界面: %s (仅本机可访问,已通过 open_browser=false 关闭自动打开)", adminURL)
	}
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       0, // 流式请求需要长连接,body 读取交给上游超时控制
		IdleTimeout:       120 * time.Second,
		WriteTimeout:      0, // SSE 长连接,不设写超时
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Printf("listen: %v", err)
		// 启动失败也保持控制台窗口驻留,便于用户查看原因
		waitOnWindows()
		os.Exit(1)
	}
}

// localAdminURL 从监听地址推导管理界面 URL(始终用 localhost 访问)
func localAdminURL(listen string) string {
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" || port == "0" {
		port = "8080"
	}
	return "http://localhost:" + port
}

// openBrowserWhenReady 等待端口就绪后自动打开管理界面浏览器
func openBrowserWhenReady(listen, url string) {
	_, port, err := net.SplitHostPort(listen)
	addr := "127.0.0.1:" + port
	if err != nil || port == "" || port == "0" {
		addr = "127.0.0.1:8080"
	}
	for i := 0; i < 50; i++ {
		conn, derr := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if derr == nil {
			_ = conn.Close()
			openBrowser(url)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	log.Printf("等待 %s 就绪超时,未自动打开浏览器(可手动访问 %s)", addr, url)
}

// openBrowser 调用系统默认浏览器打开 URL。
// 使用标准方式(cmd /c start)而非 rundll32:rundll32 打开浏览器存在已知窗口管理问题
// (窗口可能无法正常最小化/关闭),start 打开的浏览器窗口行为与用户手动打开一致。
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// start "" "url" —— 空字符串为窗口标题参数,必须保留
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("open browser: %v", err)
	}
}

// waitOnWindows 在 Windows 上等待回车,避免控制台窗口一闪而过
func waitOnWindows() {
	if runtime.GOOS != "windows" {
		return
	}
	log.Print("按回车键退出...")
	_, _ = fmt.Scanln()
}

// mountFrontend 挂载前端静态资源(embedded web/dist)
func mountFrontend(mux *http.ServeMux) {
	sub, err := fs.Sub(webassets.FS, "dist")
	if err != nil {
		// 前端未构建(无 dist):对 / 返回提示
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api" || r.URL.Path == "/v1" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><title>大模型转发网关</title></head>
<body style="background:#0a0a0a;color:#e5e5e5;font-family:monospace;display:flex;align-items:center;justify-content:center;height:100vh">
<div style="text-align:center"><h1>LLM GATEWAY</h1><p>前端尚未构建。请先执行 <code>npm run build</code>(web 目录)后重新编译。</p><p>API: /v1/models(需 API key),管理 API: /api/admin/*</p></div>
</body></html>`))
		})
		return
	}
	index, _ := fs.ReadFile(sub, "index.html")
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("/", spaFallback(fileServer, index))
}

// spaFallback 对不存在路径回退到 index.html(SPA history 路由)
func spaFallback(fs http.Handler, index []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/v1") {
			fs.ServeHTTP(w, r)
			return
		}
		// 判断扩展名是否像静态资源;否则回退 index.html
		ext := filepath.Ext(r.URL.Path)
		if ext != "" && (ext == ".js" || ext == ".css" || ext == ".svg" || ext == ".png" || ext == ".ico" || ext == ".woff" || ext == ".woff2" || ext == ".ttf") {
			fs.ServeHTTP(w, r)
			return
		}
		// 回退到 index.html
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}

// logRequests 简单访问日志(仅管理路径)
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) > 4 && r.URL.Path[:4] == "/v1/" {
			// 网关请求不打印 body,仅记录方法+路径
			log.Printf("v1 %s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}
