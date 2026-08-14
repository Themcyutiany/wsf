package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookie = "wsf_session"
const sessionTTL = 7 * 24 * time.Hour

// newAuthSecret 生成进程级随机密钥，用于给会话 Cookie 签名。
func newAuthSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return []byte("wsf-insecure-fallback-secret")
	}
	return b
}

// sign 计算 payload 的 HMAC-SHA256 签名（十六进制）。
func (a *App) sign(payload string) string {
	mac := hmac.New(sha256.New, a.authSecret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// issueSession 签发会话 Cookie。
func (a *App) issueSession(w http.ResponseWriter) {
	payload := strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 10)
	value := payload + "." + a.sign(payload)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   a.https,
		SameSite: http.SameSiteLaxMode,
	})
}

// validSession 校验请求携带的会话 Cookie 是否有效（签名 + 有效期）。
func (a *App) validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	payload, sig := parts[0], parts[1]
	want := a.sign(payload)
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return false
	}
	expires, err := strconv.ParseInt(payload, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	return true
}

func (a *App) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.https,
		SameSite: http.SameSiteLaxMode,
	})
}

// handleLogin 校验密码并签发会话。
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !a.authEnabled {
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	ip := remoteIP(r)
	if d := a.limiter.locked(ip); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Round(time.Second)/time.Second)))
		httpError(w, http.StatusTooManyRequests, "尝试次数过多，请稍后再试")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(a.password)) != 1 {
		a.limiter.fail(ip)
		httpError(w, http.StatusUnauthorized, "密码错误")
		return
	}
	a.limiter.success(ip)
	a.issueSession(w)
	writeJSON(w, map[string]any{"ok": true})
}

// handleLogout 清除会话。
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	a.clearSession(w)
	writeJSON(w, map[string]any{"ok": true})
}