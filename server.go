package main

import (
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type App struct {
	root        string
	port        int
	proxy       string
	noProxy     bool
	addrs       []string
	startTime   time.Time
	tasks       *TaskManager
	password    string
	authEnabled bool
	authSecret  []byte
	ffmpeg      *ffmpegInfo
}

func NewApp(root, proxy string, noProxy bool, port int, password string) *App {
	return &App{
		root:      root,
		port:      port,
		proxy:     proxy,
		noProxy:   noProxy,
		addrs:       lanIPs(),
		startTime:   time.Now(),
		tasks:       NewTaskManager(),
		password:    password,
		authEnabled: password != "",
		authSecret:  newAuthSecret(),
		ffmpeg:      detectFFmpeg(),
	}
}

func (a *App) handler() http.Handler {
	mux := http.NewServeMux()

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("嵌入资源加载失败: %v", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))

	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/login", a.handleLogin)
	mux.HandleFunc("/api/logout", a.handleLogout)
	mux.HandleFunc("/api/info", a.requireAuthFunc(a.handleInfo))
	mux.HandleFunc("/api/list", a.requireAuthFunc(a.handleList))
	mux.HandleFunc("/api/download", a.requireAuthFunc(a.handleDownload))
	mux.HandleFunc("/api/preview", a.requireAuthFunc(a.handlePreview))
	mux.HandleFunc("/api/zip", a.requireAuthFunc(a.handleZip))
	mux.HandleFunc("/api/url-download", a.requireAuthFunc(a.handleURLDownload))
	mux.HandleFunc("/api/tasks", a.requireAuthFunc(a.handleTasks))
	mux.HandleFunc("/api/tasks/", a.requireAuthFunc(a.handleTaskAction))

	return logRequests(mux)
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		httpError(w, http.StatusInternalServerError, "内部错误")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (a *App) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, map[string]any{
		"version":   version,
		"root":      a.root,
		"port":      a.port,
		"proxy":     a.proxy,
		"noProxy":   a.noProxy,
		"auth":      a.authEnabled,
		"preview": map[string]any{
			"ffmpeg": a.transcodeOK(),
			"codec":  a.videoCodecName(),
		},
		"addrs":     a.addrs,
		"uptime":    int64(time.Since(a.startTime).Seconds()),
		"startedAt": a.startTime.Format(time.RFC3339),
	})
}

// requireAuthFunc 启用访问密码后，未通过会话校验的请求一律返回 401。
func (a *App) requireAuthFunc(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.authEnabled && !a.validSession(r) {
			httpError(w, http.StatusUnauthorized, "需要访问密码")
			return
		}
		h(w, r)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/tasks") && rec.status == http.StatusOK {
			return // 轮询不刷屏
		}
		log.Printf("%s %s %s -> %d (%s)", remoteIP(r), r.Method, r.URL.RequestURI(), rec.status, time.Since(start).Round(time.Millisecond))
	})
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func methodNotAllowed(w http.ResponseWriter) {
	httpError(w, http.StatusMethodNotAllowed, "不支持的请求方法")
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
