package main

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Hidden  bool      `json:"hidden"`
}

// resolve 把网页传入的虚拟路径（如 /docs/a）解析为共享目录内的绝对路径，
// 返回 (绝对路径, 相对共享目录的虚拟路径)，并阻止任何越界访问。
func (a *App) resolve(vp string, mustDir bool) (string, string, error) {
	vp = strings.TrimSpace(vp)
	if vp == "" {
		vp = "/"
	}
	if !strings.HasPrefix(vp, "/") {
		vp = "/" + vp
	}
	rel := strings.TrimPrefix(path.Clean(vp), "/")
	abs := a.root
	if rel != "" {
		abs = filepath.Join(a.root, filepath.FromSlash(rel))
	}

	rootRes, err := filepath.EvalSymlinks(a.root)
	if err != nil {
		return "", "", err
	}
	res, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", err
	}
	rr, err := filepath.Rel(rootRes, res)
	if err != nil {
		return "", "", err
	}
	if rr == ".." || strings.HasPrefix(rr, ".."+string(filepath.Separator)) {
		return "", "", errors.New("路径越界")
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", "", err
	}
	if mustDir && !info.IsDir() {
		return "", "", errors.New("不是文件夹")
	}
	return abs, rel, nil
}

func (a *App) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	dir, rel, err := a.resolve(r.URL.Query().Get("path"), true)
	if err != nil {
		httpError(w, http.StatusNotFound, "文件夹不存在或不可访问")
		return
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "读取文件夹失败: "+err.Error())
		return
	}
	entries := make([]Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			continue
		}
		name := de.Name()
		entries = append(entries, Entry{
			Name:    name,
			Path:    toVirtual(path.Join(rel, name)),
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Hidden:  strings.HasPrefix(name, "."),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	writeJSON(w, map[string]any{
		"path":    toVirtual(rel),
		"entries": entries,
		"count":   len(entries),
	})
}

func toVirtual(rel string) string {
	return path.Join("/", filepath.ToSlash(rel))
}
