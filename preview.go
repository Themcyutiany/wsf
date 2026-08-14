package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// 浏览器原生可播放/展示的扩展名（直接原样输出）
var nativeImageExt = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".svg": true, ".bmp": true, ".avif": true, ".ico": true, ".jfif": true}
var nativeVideoExt = map[string]bool{".mp4": true, ".m4v": true, ".webm": true, ".ogv": true, ".mov": true}
var nativeAudioExt = map[string]bool{".mp3": true, ".wav": true, ".ogg": true, ".oga": true, ".opus": true, ".flac": true, ".m4a": true, ".weba": true}

// 浏览器不支持、需要 ffmpeg 转码的媒体扩展名
var transcodeImageExt = map[string]bool{".tif": true, ".tiff": true, ".heic": true, ".heif": true, ".jp2": true, ".ppm": true, ".pgm": true, ".pbm": true, ".pnm": true}
var transcodeVideoExt = map[string]bool{".avi": true, ".mkv": true, ".flv": true, ".wmv": true, ".ts": true, ".m2ts": true, ".mts": true, ".mpg": true, ".mpeg": true, ".vob": true, ".3gp": true, ".asf": true, ".rm": true, ".rmvb": true, ".divx": true, ".mxf": true}
var transcodeAudioExt = map[string]bool{".aac": true, ".ape": true, ".wma": true, ".mka": true, ".mid": true, ".midi": true, ".amr": true, ".caf": true, ".aiff": true, ".aif": true, ".ac3": true, ".ra": true, ".au": true, ".dts": true}

var extContentType = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/mp4", ".webm": "video/webm", ".ogv": "video/ogg", ".mov": "video/quicktime",
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg", ".oga": "audio/ogg", ".opus": "audio/ogg", ".flac": "audio/flac", ".m4a": "audio/mp4", ".weba": "audio/webm",
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml", ".bmp": "image/bmp", ".avif": "image/avif", ".ico": "image/x-icon", ".jfif": "image/jpeg",
}

// ffmpegInfo 记录 ffmpeg 路径与可用编码器。
type ffmpegInfo struct {
	path    string
	hasX264 bool
	hasVP9  bool
	hasAAC  bool
	hasLame bool
	hasOpus bool
}

// detectFFmpeg 探测系统 ffmpeg 及关键编码器，找不到返回 nil。
func detectFFmpeg() *ffmpegInfo {
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil
	}
	info := &ffmpegInfo{path: p}
	out, err := exec.Command(p, "-hide_banner", "-encoders").Output()
	if err != nil {
		return info
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[1] {
		case "libx264":
			info.hasX264 = true
		case "libvpx-vp9":
			info.hasVP9 = true
		case "aac":
			info.hasAAC = true
		case "libmp3lame":
			info.hasLame = true
		case "libopus":
			info.hasOpus = true
		}
	}
	return info
}

// transcodeOK 是否可用 ffmpeg 转码。
func (a *App) transcodeOK() bool { return a.ffmpeg != nil }

// videoCodecName 返回视频转码主编码器名，用于横幅与前端提示。
func (a *App) videoCodecName() string {
	if a.ffmpeg == nil {
		return ""
	}
	if a.ffmpeg.hasX264 {
		return "H.264"
	}
	if a.ffmpeg.hasVP9 {
		return "VP9"
	}
	return "AAC"
}

// mediaKind 根据扩展名判断媒体类型：image / video / audio，否则返回空串。
func mediaKind(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if nativeImageExt[ext] || transcodeImageExt[ext] {
		return "image"
	}
	if nativeVideoExt[ext] || transcodeVideoExt[ext] {
		return "video"
	}
	if nativeAudioExt[ext] || transcodeAudioExt[ext] {
		return "audio"
	}
	return ""
}

// isNativeMedia 是否属于浏览器原生支持的媒体格式。
func isNativeMedia(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return nativeImageExt[ext] || nativeVideoExt[ext] || nativeAudioExt[ext]
}

// handlePreview 媒体预览：原生格式直接输出，其余格式用 ffmpeg 实时转码。
func (a *App) handlePreview(w http.ResponseWriter, r *http.Request) {
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
		httpError(w, http.StatusUnsupportedMediaType, "该文件类型不支持预览，请直接下载")
		return
	}
	if isNativeMedia(info.Name()) {
		a.serveNativePreview(w, r, abs, info)
		return
	}
	if !a.transcodeOK() {
		httpError(w, http.StatusNotImplemented, "该格式需要 ffmpeg 转码才能预览，请安装 ffmpeg 后重试")
		return
	}
	a.serveTranscoded(w, r, abs, info, kind)
}

// serveNativePreview 直接输出原生媒体，支持 Range 断点。
func (a *App) serveNativePreview(w http.ResponseWriter, r *http.Request, abs string, info os.FileInfo) {
	f, err := os.Open(abs)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "打开文件失败")
		return
	}
	defer f.Close()
	if ct := extContentType[strings.ToLower(filepath.Ext(info.Name()))]; ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": info.Name()}))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// serveTranscoded 用 ffmpeg 实时转码并流式输出给浏览器。
func (a *App) serveTranscoded(w http.ResponseWriter, r *http.Request, abs string, info os.FileInfo, kind string) {
	args, contentType, ok := a.transcodeArgs(abs, info.Name(), kind)
	if !ok {
		httpError(w, http.StatusInternalServerError, "暂无可用的转码编码器，请安装完整版 ffmpeg")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	cmd := exec.CommandContext(ctx, a.ffmpeg.path, args...)
	cmd.Dir = filepath.Dir(abs)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		httpError(w, http.StatusInternalServerError, "启动 ffmpeg 失败")
		return
	}
	go func() {
		werr := cmd.Wait()
		_ = pw.CloseWithError(werr)
	}()

	br := bufio.NewReaderSize(pr, 64*1024)
	first := make([]byte, 32*1024)
	n, rerr := br.Read(first)
	if rerr != nil && rerr != io.EOF {
		_ = cmd.Process.Kill()
		httpError(w, http.StatusInternalServerError, "转码失败："+strings.TrimSpace(stderr.String()))
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Transcoded", "1")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if n > 0 {
		_, _ = w.Write(first[:n])
	}
	_, _ = io.Copy(w, br)
}

// transcodeArgs 根据媒体类型与可用编码器生成 ffmpeg 参数。
func (a *App) transcodeArgs(abs, name, kind string) ([]string, string, bool) {
	base := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-i", abs}
	switch kind {
	case "video":
		if a.ffmpeg.hasX264 {
			args := append(base,
				"-map", "0:v:0?", "-map", "0:a:0?",
				"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p",
				"-c:a", "aac", "-b:a", "128k",
				"-movflags", "frag_keyframe+empty_moov+default_base_moof",
				"-f", "mp4", "pipe:1")
			return args, "video/mp4", true
		}
		if a.ffmpeg.hasVP9 && a.ffmpeg.hasOpus {
			args := append(base,
				"-map", "0:v:0?", "-map", "0:a:0?",
				"-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-crf", "34", "-b:v", "0",
				"-c:a", "libopus", "-b:a", "96k",
				"-f", "webm", "pipe:1")
			return args, "video/webm", true
		}
	case "audio":
		if a.ffmpeg.hasAAC {
			args := append(base,
				"-vn", "-c:a", "aac", "-b:a", "192k",
				"-movflags", "frag_keyframe+empty_moov+default_base_moof",
				"-f", "mp4", "pipe:1")
			return args, "audio/mp4", true
		}
		if a.ffmpeg.hasLame {
			args := append(base, "-vn", "-c:a", "libmp3lame", "-b:a", "192k", "-f", "mp3", "pipe:1")
			return args, "audio/mpeg", true
		}
		if a.ffmpeg.hasOpus {
			args := append(base, "-vn", "-c:a", "libopus", "-b:a", "128k", "-f", "ogg", "pipe:1")
			return args, "audio/ogg", true
		}
	case "image":
		args := append(base, "-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "3", "pipe:1")
		return args, "image/jpeg", true
	}
	return nil, "", false
}
