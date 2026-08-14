package main

import (
	"sync"
	"time"
)

// loginRate 记录某个 IP 的登录失败情况。
type loginRate struct {
	fails       int
	windowStart time.Time
	lockedUntil time.Time
}

// loginLimiter 对密码登录做失败次数限制，防止公网暴力破解：
// 同一 IP 在窗口期内失败超过阈值即锁定一段时间。
type loginLimiter struct {
	mu       sync.Mutex
	byIP     map[string]*loginRate
	maxFails int
	window   time.Duration
	lockFor  time.Duration
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		byIP:     map[string]*loginRate{},
		maxFails: 5,
		window:   10 * time.Minute,
		lockFor:  15 * time.Minute,
	}
}

// locked 返回该 IP 是否处于锁定状态及剩余锁定时间（0 表示未锁定）。
func (l *loginLimiter) locked(ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.byIP[ip]
	if !ok {
		return 0
	}
	now := time.Now()
	if now.Before(r.lockedUntil) {
		return r.lockedUntil.Sub(now)
	}
	if now.Sub(r.windowStart) > l.window {
		delete(l.byIP, ip)
	}
	return 0
}

// fail 记录一次失败；达到阈值后锁定该 IP。
func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	r, ok := l.byIP[ip]
	if !ok {
		r = &loginRate{windowStart: now}
		l.byIP[ip] = r
	}
	if now.Before(r.lockedUntil) {
		return
	}
	if now.Sub(r.windowStart) > l.window {
		r.windowStart = now
		r.fails = 0
	}
	r.fails++
	if r.fails >= l.maxFails {
		r.lockedUntil = now.Add(l.lockFor)
		r.fails = 0
	}
}

// success 登录成功后清除该 IP 的失败记录。
func (l *loginLimiter) success(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byIP, ip)
}
