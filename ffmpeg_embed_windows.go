//go:build embedded_ffmpeg && windows

package main

import _ "embed"

//go:embed ffmpegdata/ffmpeg-windows-amd64.exe.gz
var embeddedFFmpegWindows []byte

func builtinFFmpegData() []byte { return embeddedFFmpegWindows }
