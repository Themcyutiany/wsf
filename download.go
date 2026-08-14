package main

import (
	"archive/zip"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (a *App) handleDownload(w http.ResponseWriter, r *http.Request) {
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
	if err != nil {
		httpError(w, http.StatusNotFound, "文件不存在")
		return
	}
	if info.IsDir() {
		httpError(w, http.StatusBadRequest, "文件夹请使用 ZIP 下载")
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "打开文件失败")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (a *App) handleZip(w http.ResponseWriter, r *http.Request) {
	var paths []string
	switch r.Method {
	case http.MethodGet:
		p := r.URL.Query().Get("path")
		if p == "" {
			httpError(w, http.StatusBadRequest, "缺少 path 参数")
			return
		}
		paths = []string{p}
	case http.MethodPost:
		var req struct {
			Paths []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, http.StatusBadRequest, "请求格式错误")
			return
		}
		paths = req.Paths
	default:
		methodNotAllowed(w)
		return
	}
	if len(paths) == 0 || len(paths) > 200 {
		httpError(w, http.StatusBadRequest, "请选择 1~200 个项目")
		return
	}
	type item struct {
		abs  string
		name string
	}
	items := make([]item, 0, len(paths))
	for _, vp := range paths {
		abs, _, err := a.resolve(vp, false)
		if err != nil {
			httpError(w, http.StatusNotFound, "路径不存在: "+vp)
			return
		}
		info, err := os.Stat(abs)
		if err != nil {
			httpError(w, http.StatusNotFound, "路径不存在: "+vp)
			return
		}
		items = append(items, item{abs: abs, name: info.Name()})
	}

	zipName := "wsf-下载"
	if len(items) == 1 {
		zipName = items[0].name
	}
	if !strings.HasSuffix(strings.ToLower(zipName), ".zip") {
		zipName += ".zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": zipName}))

	pr, pw := io.Pipe()
	zw := zip.NewWriter(pw)
	go func() {
		var werr error
		for _, it := range items {
			if werr = addToZip(zw, it.abs, it.name, ""); werr != nil {
				break
			}
		}
		if cerr := zw.Close(); werr == nil {
			werr = cerr
		}
		_ = pw.CloseWithError(werr)
	}()
	_, _ = io.Copy(w, pr)
}

// addToZip 递归地把 abs 指向的文件/文件夹写入 zip，name 为包内顶层名称。
func addToZip(zw *zip.Writer, abs, name, prefix string) error {
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return err
		}
		zipName := path.Join(prefix, name)
		if len(entries) == 0 {
			hdr, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			hdr.Name = zipName + "/"
			hdr.Method = zip.Store
			_, err = zw.CreateHeader(hdr)
			return err
		}
		for _, e := range entries {
			if e.Type()&os.ModeSymlink != 0 {
				continue // 跳过符号链接，防止目录循环
			}
			if err := addToZip(zw, filepath.Join(abs, e.Name()), e.Name(), zipName); err != nil {
				return err
			}
		}
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = path.Join(prefix, name)
	hdr.Method = zip.Deflate
	fw, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	f, err := os.Open(abs)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(fw, f)
	return err
}
