package main

import (
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// securityCSP 基础内容安全策略：仅允许本站资源，禁止被嵌入第三方页面。
const securityCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'"

// securityHeaders 为所有响应附加安全头。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", securityCSP)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin 校验浏览器跨站请求（CSRF 防护）：请求若携带 Origin/Referer，
// 其主机必须与请求的 Host 一致；不携带则放行（curl 等非浏览器客户端）。
func sameOrigin(r *http.Request) bool {
	host := strings.ToLower(r.Host)
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || !strings.EqualFold(u.Host, host) {
			return false
		}
		return true
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		u, err := url.Parse(ref)
		if err != nil || !strings.EqualFold(u.Host, host) {
			return false
		}
	}
	return true
}

// parseAllowList 解析逗号分隔的 IP / CIDR 白名单。
func parseAllowList(s string) []netip.Prefix {
	var out []netip.Prefix
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			if p, err := netip.ParsePrefix(part); err == nil {
				out = append(out, p)
			}
			continue
		}
		if ip, err := netip.ParseAddr(part); err == nil {
			bits := 128
			if ip.Is4() {
				bits = 32
			}
			out = append(out, netip.PrefixFrom(ip, bits))
		}
	}
	return out
}

// ipAllowed 白名单为空时不限制；否则要求来源 IP 命中任一网段。
func ipAllowed(allow []netip.Prefix, ipStr string) bool {
	if len(allow) == 0 {
		return true
	}
	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}
	for _, p := range allow {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
