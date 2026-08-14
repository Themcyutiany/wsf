package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// thumbEntry 缩略图缓存条目。
type thumbEntry struct {
	data      []byte
	content   string
	createdAt time.Time
}

// thumbCache 简单的内存缩略图缓存（LRU 近似：超过上限时清理最旧的一半）。
type thumbCache struct {
	mu    sync.Mutex
	items map[string]thumbEntry
	order []string
	cap   int
}

func newThumbCache(cap int) *thumbCache {
	return &thumbCache{items: make(map[string]thumbEntry), cap: cap}
}

func (c *thumbCache) get(key string) (thumbEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	return e, ok
}

func (c *thumbCache) put(key string, e thumbEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		c.items[key] = e
		return
	}
	c.items[key] = e
	c.order = append(c.order, key)
	if len(c.order) > c.cap {
		drop := c.cap / 2
		for _, k := range c.order[:drop] {
			delete(c.items, k)
		}
		c.order = append([]string{}, c.order[drop:]...)
	}
}

var thumbs = newThumbCache(512)

// thumbLocks 防止同一文件并发重复转码。
var thumbLocks sync.Map // path -> *sync.Mutex

// handleThumb 生成媒体缩略图（JPEG 小图）。
func (a *App) handleThumb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	abs, _, err := a.resolve(r.URL.Query().Get("path"), false)
	if err != nil {
		httpError(w, http.StatusNotFound, "文件不存在")
		return
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		httpError(w, http.StatusNotFound, "文件不存在")
		return
	}
	kind := mediaKind(info.Name())
	if kind == "" {
		httpError(w, http.StatusNotFound, "该文件没有缩略图")
		return
	}

	width := 128
	if v, err := strconv.Atoi(r.URL.Query().Get("w")); err == nil && v >= 48 && v <= 480 {
		width = v
	}
	key := strings.ToLower(abs) + "|" + strconv.Itoa(width)
	if e, ok := thumbs.get(key); ok && time.Since(e.createdAt) < 10*time.Minute {
		serveThumbCache(w, r, e)
		return
	}

	// 并发去重
	muI, _ := thumbLocks.LoadOrStore(key, &sync.Mutex{})
	mu := muI.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if e, ok := thumbs.get(key); ok && time.Since(e.createdAt) < 10*time.Minute {
		serveThumbCache(w, r, e)
		return
	}

	data, contentType, ok := a.makeThumb(abs, info.Name(), kind, width)
	if !ok {
		thumbs.put(key, thumbEntry{data: nil, createdAt: time.Now()}) // 负缓存，避免频繁重试
		httpError(w, http.StatusNotFound, "无法生成缩略图")
		return
	}
	e := thumbEntry{data: data, content: contentType, createdAt: time.Now()}
	thumbs.put(key, e)
	serveThumbCache(w, r, e)
}

func serveThumbCache(w http.ResponseWriter, r *http.Request, e thumbEntry) {
	if e.data == nil {
		httpError(w, http.StatusNotFound, "无法生成缩略图")
		return
	}
	w.Header().Set("Content-Type", e.content)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Transcoded", "1")
	_, _ = w.Write(e.data)
}

// makeThumb 生成缩略图。有 ffmpeg 时统一转 JPEG；无 ffmpeg 时仅原生图片可直出。
func (a *App) makeThumb(abs, name, kind string, width int) ([]byte, string, bool) {
	if a.ffmpeg != nil {
		if data, err := a.thumbViaFFmpeg(abs, name, kind, width); err == nil {
			return data, "image/jpeg", true
		}
	}
	// 无 ffmpeg 或转码失败：原生图片直接输出（浏览器缩放）
	if kind == "image" && isNativeMedia(name) {
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil, "", false
		}
		ct := extContentType[strings.ToLower(filepath.Ext(name))]
		if ct == "" {
			ct = "image/jpeg"
		}
		return data, ct, true
	}
	return nil, "", false
}

// thumbViaFFmpeg 用 ffmpeg 生成 JPEG 缩略图。
func (a *App) thumbViaFFmpeg(abs, name, kind string, width int) ([]byte, error) {
	scale := "scale='min(" + strconv.Itoa(width) + ",iw)':-2"
	var args []string
	switch kind {
	case "image":
		args = []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-i", abs,
			"-vf", scale, "-frames:v", "1", "-q:v", "4", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1"}
	case "video":
		args = []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-ss", "1", "-i", abs,
			"-vf", scale, "-frames:v", "1", "-q:v", "4", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1"}
	case "audio":
		args = []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-i", abs,
			"-an", "-vf", scale, "-frames:v", "1", "-q:v", "4", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1"}
	default:
		return nil, os.ErrNotExist
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.ffmpeg.path, args...)
	cmd.Dir = filepath.Dir(abs)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		// 视频取第 1 秒失败时退回到片头
		if kind == "video" {
			args = []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-i", abs,
				"-vf", scale, "-frames:v", "1", "-q:v", "4", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1"}
			cmd2 := exec.CommandContext(ctx, a.ffmpeg.path, args...)
			cmd2.Dir = filepath.Dir(abs)
			var out2 bytes.Buffer
			cmd2.Stdout = &out2
			cmd2.Stderr = io.Discard
			if err2 := cmd2.Run(); err2 == nil && out2.Len() > 0 {
				return out2.Bytes(), nil
			}
		}
		return nil, err
	}
	if out.Len() == 0 {
		return nil, io.EOF
	}
	return out.Bytes(), nil
}
