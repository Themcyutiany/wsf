//go:build !windows

package main

import "path/filepath"

// realPath 返回路径解析符号链接后的真实绝对路径。
func realPath(p string) (string, error) {
	return filepath.EvalSymlinks(p)
}
