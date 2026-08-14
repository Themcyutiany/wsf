//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetFinalPathName = kernel32.NewProc("GetFinalPathNameByHandleW")
)

// realPath 返回路径解析链接（符号链接 / 目录联接 junction）后的真实绝对路径。
// 标准库 filepath.EvalSymlinks 在 Windows 上不解析 junction，
// 这里直接调用 GetFinalPathNameByHandle 获取真实路径，用于越界防护。
func realPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	pathp, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return "", err
	}
	h, err := syscall.CreateFile(
		pathp,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return "", err
	}
	defer syscall.CloseHandle(h)

	size := uint32(1024)
	for {
		buf := make([]uint16, size)
		r1, _, e1 := procGetFinalPathName.Call(
			uintptr(h),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(size),
			0, // FILE_NAME_NORMALIZED：解析链接
		)
		if r1 == 0 {
			return "", e1
		}
		if r1 < uintptr(size) {
			s := syscall.UTF16ToString(buf[:r1])
			if strings.HasPrefix(s, `\\?\UNC\`) {
				s = `\\` + s[len(`\\?\UNC\`):]
			} else if strings.HasPrefix(s, `\\?\`) {
				s = s[len(`\\?\`):]
			}
			return s, nil
		}
		size = uint32(r1) + 1
	}
}
