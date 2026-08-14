package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// extractBuiltinFFmpeg 把内置 ffmpeg 释放到系统临时目录并返回可执行文件路径。
// 未嵌入或释放失败时返回空字符串。目录名由数据哈希决定，数据更新后会自动换新目录。
func extractBuiltinFFmpeg() string {
	data := builtinFFmpegData()
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	dir := filepath.Join(os.TempDir(), "wsf-ffmpeg-"+hex.EncodeToString(sum[:6]))
	exeName := "ffmpeg"
	if runtime.GOOS == "windows" {
		exeName = "ffmpeg.exe"
	}
	exe := filepath.Join(dir, exeName)
	if st, err := os.Stat(exe); err == nil && st.Size() > 1000000 {
		return exe
	}
	_ = os.MkdirAll(dir, 0o755)
	tmp := filepath.Join(dir, exeName+".tmp")
	if err := writeExtractedFFmpeg(tmp, data); err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	_ = os.Chmod(tmp, 0o755)
	if err := os.Rename(tmp, exe); err != nil {
		if st, err2 := os.Stat(exe); err2 == nil && st.Size() > 1000000 {
			_ = os.Remove(tmp)
			return exe
		}
		return ""
	}
	return exe
}

// writeExtractedFFmpeg 流式解压 gzip 数据到目标文件。
func writeExtractedFFmpeg(dst string, data []byte) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	_, err = io.Copy(f, gz)
	return err
}
