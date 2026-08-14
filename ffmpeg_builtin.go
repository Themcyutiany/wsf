//go:build !embedded_ffmpeg

package main

// builtinFFmpegData 返回内置的 ffmpeg 二进制（gzip 压缩），未嵌入时返回 nil。
// 该文件为默认构建（不带 embedded_ffmpeg 标签），嵌入实现见 *_embed_*.go。
func builtinFFmpegData() []byte { return nil }
