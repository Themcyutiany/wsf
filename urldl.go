package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Task struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Filename   string    `json:"filename"`
	Status     string    `json:"status"` // downloading | done | error | canceled
	Progress   float64   `json:"progress"`
	Total      int64     `json:"total"`
	Downloaded int64     `json:"downloaded"`
	Speed      int64     `json:"speed"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	SavedPath  string    `json:"savedPath,omitempty"`

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (t *Task) snapshot() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return map[string]any{
		"id":         t.ID,
		"url":        t.URL,
		"filename":   t.Filename,
		"status":     t.Status,
		"progress":   t.Progress,
		"total":      t.Total,
		"downloaded": t.Downloaded,
		"speed":      t.Speed,
		"error":      t.Error,
		"createdAt":  t.CreatedAt,
		"savedPath":  t.SavedPath,
	}
}

func (t *Task) setStatus(s string) {
	t.mu.Lock()
	t.Status = s
	t.mu.Unlock()
}

type TaskManager struct {
	mu    sync.Mutex
	tasks map[string]*Task
	order []string
}

func NewTaskManager() *TaskManager {
	return &TaskManager{tasks: map[string]*Task{}}
}

func (tm *TaskManager) add(t *Task) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tasks[t.ID] = t
	tm.order = append(tm.order, t.ID)
}

func (tm *TaskManager) get(id string) (*Task, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	t, ok := tm.tasks[id]
	return t, ok
}

func (tm *TaskManager) list() []map[string]any {
	tm.mu.Lock()
	tasks := make([]*Task, 0, len(tm.order))
	for i := len(tm.order) - 1; i >= 0; i-- {
		if t, ok := tm.tasks[tm.order[i]]; ok {
			tasks = append(tasks, t)
		}
	}
	tm.mu.Unlock()
	out := make([]map[string]any, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.snapshot())
	}
	return out
}

func (a *App) handleURLDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		httpError(w, http.StatusBadRequest, "请输入合法的 http/https 链接")
		return
	}
	task := &Task{
		ID:        newTaskID(),
		URL:       u.String(),
		Filename:  sanitizeName(req.Filename),
		Status:    "downloading",
		CreatedAt: time.Now(),
	}
	a.tasks.add(task)
	go a.runTask(task)
	writeJSON(w, task.snapshot())
}

func (a *App) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, map[string]any{"tasks": a.tasks.list()})
}

func (a *App) handleTaskAction(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/")
	if id == "" {
		httpError(w, http.StatusNotFound, "任务不存在")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if _, ok := a.tasks.get(id); !ok {
		httpError(w, http.StatusNotFound, "任务不存在")
		return
	}
	if strings.HasSuffix(r.URL.Path, "/cancel") {
		snap, ok := a.cancelTask(id)
		if !ok {
			httpError(w, http.StatusNotFound, "任务不存在")
			return
		}
		writeJSON(w, snap)
		return
	}
	httpError(w, http.StatusNotFound, "未知操作")
}

// cancelTask 取消指定任务，返回任务快照与是否成功。
func (a *App) cancelTask(id string) (map[string]any, bool) {
	t, ok := a.tasks.get(id)
	if !ok {
		return nil, false
	}
	t.mu.Lock()
	if t.cancel != nil {
		t.cancel()
	}
	if t.Status == "downloading" {
		t.Status = "canceled"
	}
	t.mu.Unlock()
	return t.snapshot(), true
}

func (a *App) runTask(t *Task) {
	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	t.cancel = cancel
	t.mu.Unlock()

	setErr := func(err error) {
		t.mu.Lock()
		t.Status = "error"
		t.Error = err.Error()
		t.mu.Unlock()
	}

	client := a.httpClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		setErr(err)
		return
	}
	req.Header.Set("User-Agent", "wsf/"+version)

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			t.setStatus("canceled")
		} else {
			setErr(fmt.Errorf("连接失败: %v", err))
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		setErr(fmt.Errorf("服务器返回 %s", resp.Status))
		return
	}

	name := t.Filename
	if name == "" {
		name = deriveFilename(resp)
	}
	dest := a.uniquePath(name)

	f, err := os.Create(dest)
	if err != nil {
		setErr(fmt.Errorf("创建文件失败: %v", err))
		return
	}

	total := resp.ContentLength
	start := time.Now()
	buf := make([]byte, 128*1024)
	var written int64

	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			if _, ew := f.Write(buf[:nr]); ew != nil {
				_ = f.Close()
				_ = os.Remove(dest)
				setErr(fmt.Errorf("写入文件失败: %v", ew))
				return
			}
			written += int64(nr)
			t.mu.Lock()
			t.Downloaded = written
			t.Total = total
			if total > 0 {
				t.Progress = float64(written) / float64(total) * 100
			}
			if s := time.Since(start).Seconds(); s > 0 {
				t.Speed = int64(float64(written) / s)
			}
			t.mu.Unlock()
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			_ = f.Close()
			_ = os.Remove(dest)
			if ctx.Err() != nil {
				t.setStatus("canceled")
				return
			}
			setErr(fmt.Errorf("下载中断: %v", er))
			return
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(dest)
		setErr(fmt.Errorf("写入文件失败: %v", err))
		return
	}

	t.mu.Lock()
	t.Status = "done"
	t.Progress = 100
	t.Downloaded = written
	t.Total = written
	t.Speed = 0
	if rel, err := filepath.Rel(a.root, dest); err == nil {
		t.SavedPath = toVirtual(rel)
	}
	t.mu.Unlock()
}

func (a *App) httpClient() *http.Client {
	tr := &http.Transport{
		Proxy:               nil,
		MaxIdleConns:        16,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
	}
	if !a.noProxy {
		if pu, err := url.Parse(a.proxy); err == nil && pu.Host != "" {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	return &http.Client{Transport: tr}
}

func deriveFilename(resp *http.Response) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if n := sanitizeName(params["filename"]); n != "" {
				return n
			}
		}
	}
	if resp.Request == nil || resp.Request.URL == nil {
		return "download"
	}
	base := sanitizeName(path.Base(resp.Request.URL.Path))
	if base == "" {
		base = "download_" + time.Now().Format("20060102_150405")
	}
	return base
}

func (a *App) uniquePath(name string) string {
	base := sanitizeName(name)
	if base == "" {
		base = "download"
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	p := filepath.Join(a.root, base)
	for i := 1; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
		p = filepath.Join(a.root, fmt.Sprintf("%s (%d)%s", stem, i, ext))
	}
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." {
		return ""
	}
	s = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		if r < 0x20 {
			return '_'
		}
		return r
	}, s)
	return strings.Trim(s, ". ")
}

func newTaskID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
