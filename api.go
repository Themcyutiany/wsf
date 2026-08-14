package main

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// apiAuth API 密钥校验中间件：-api 设置后，/api/v1/* 必须携带有效密钥
// （Authorization: Bearer <密钥> 或 X-API-Key: <密钥>），并做防爆破限流。
func (a *App) apiAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.apiKey == "" {
			httpError(w, http.StatusForbidden, "API 未启用（启动时用 -api 密钥开启）")
			return
		}
		key := ""
		if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
			key = strings.TrimPrefix(ah, "Bearer ")
		} else if xk := r.Header.Get("X-API-Key"); xk != "" {
			key = xk
		}
		if key == "" {
			httpError(w, http.StatusUnauthorized, "缺少 API 密钥（Authorization: Bearer <密钥> 或 X-API-Key: <密钥>）")
			return
		}
		ip := remoteIP(r)
		if d := a.apiLimiter.locked(ip); d > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(d.Round(time.Second)/time.Second)))
			httpError(w, http.StatusTooManyRequests, "尝试次数过多，请稍后再试")
			return
		}
		if subtle.ConstantTimeCompare([]byte(key), []byte(a.apiKey)) != 1 {
			a.apiLimiter.fail(ip)
			httpError(w, http.StatusUnauthorized, "API 密钥无效")
			return
		}
		a.apiLimiter.success(ip)
		h(w, r)
	}
}

// handleAPIUpload 上传文件到共享目录（multipart/form-data，字段名 file，可多个）。
// 目标目录用 ?path=/xxx 指定，默认共享目录根；同名文件自动加 (1)、(2) 后缀，不覆盖。
func (a *App) handleAPIUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	dir, dirRel, err := a.resolve(r.URL.Query().Get("path"), true)
	if err != nil {
		httpError(w, http.StatusNotFound, "目标文件夹不存在或不可访问")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpError(w, http.StatusBadRequest, "不是有效的 multipart 表单")
		return
	}
	files, ok := r.MultipartForm.File["file"]
	if !ok || len(files) == 0 {
		httpError(w, http.StatusBadRequest, "缺少 file 字段（curl -F \"file=@本地文件\"）")
		return
	}
	type saved struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Size int64  `json:"size"`
	}
	results := make([]saved, 0, len(files))
	for _, fh := range files {
		src, err := fh.Open()
		if err != nil {
			httpError(w, http.StatusInternalServerError, "读取上传内容失败")
			return
		}
		name := sanitizeName(filepath.Base(fh.Filename))
		if name == "" {
			src.Close()
			httpError(w, http.StatusBadRequest, "文件名无效")
			return
		}
		dest := filepath.Join(dir, name)
		ext := filepath.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		for i := 1; ; i++ {
			if _, err := os.Stat(dest); os.IsNotExist(err) {
				break
			}
			dest = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		}
		dst, err := os.Create(dest)
		if err != nil {
			src.Close()
			httpError(w, http.StatusInternalServerError, "写入文件失败")
			return
		}
		n, cerr := io.Copy(dst, src)
		dst.Close()
		src.Close()
		if cerr != nil {
			httpError(w, http.StatusInternalServerError, "写入文件失败")
			return
		}
		base := filepath.Base(dest)
		results = append(results, saved{
			Name: base,
			Path: toVirtual(path.Join(dirRel, base)),
			Size: n,
		})
	}
	writeJSON(w, map[string]any{"uploaded": results})
}

// handleAPITaskAction API 版任务操作：/api/v1/tasks/{id}/cancel
func (a *App) handleAPITaskAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/"), "/"), "/")
	if len(parts) != 2 || parts[1] != "cancel" {
		httpError(w, http.StatusNotFound, "未知操作")
		return
	}
	id := parts[0]
	if id == "" {
		httpError(w, http.StatusNotFound, "任务不存在")
		return
	}
	snap, ok := a.cancelTask(id)
	if !ok {
		httpError(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeJSON(w, snap)
}
