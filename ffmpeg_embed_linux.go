//go:build embedded_ffmpeg && linux

package main

import _ "embed"

//go:embed ffmpegdata/ffmpeg-linux-amd64.gz
var embeddedFFmpegLinux []byte

func builtinFFmpegData() []byte { return embeddedFFmpegLinux }
