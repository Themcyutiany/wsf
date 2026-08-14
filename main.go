package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var version = "0.6.0"

func main() {
	var (
		dir     = flag.String("f", "", "要共享的文件夹（默认：当前目录）")
		port    = flag.Int("p", 5665, "监听端口")
		addr    = flag.String("a", "[::]", "监听地址（[::] 同时支持 IPv4 与 IPv6）")
		proxy   = flag.String("proxy", "http://127.0.0.1:7897", "远程下载使用的 HTTP 代理")
		noProxy = flag.Bool("no-proxy", false, "禁用代理，远程下载直连")
		pws       = flag.String("pws", "", "Web 访问密码（设置后浏览/下载需输入密码）")
		apiKey    = flag.String("api", "", "API 密钥（设置后启用 /api/v1 接口，脚本用 Bearer 密钥调用）")
		public    = flag.Bool("public", false, "公网模式：必须设置 -pws 密码，并输出安全提示")
		certFile  = flag.String("cert", "", "HTTPS 证书文件（与 -key 一起提供后启用 HTTPS）")
		keyFile   = flag.String("key", "", "HTTPS 私钥文件")
		allowList = flag.String("allow", "", "仅允许这些 IP/CIDR 访问（逗号分隔，如 1.2.3.4,10.0.0.0/8）")
		showVer   = flag.Bool("version", false, "显示版本号")
	)
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "wsf %s — 局域网网页文件共享\n\n用法: wsf [-f 文件夹] [-p 端口] [选项]\n\n选项:\n", version)
		flag.PrintDefaults()
		fmt.Fprintf(out, "\n示例:\n  wsf -f D:\\share -p 5665\n  wsf -f D:\\share -p 5665 -pws 123456\n  wsf -f . -p 8080 --no-proxy\n  wsf -f . -p 8443 -pws 密码 -public -cert cert.pem -key key.pem\n  wsf -f . -p 5665 -api 我的密钥   # 开启 API\n")
	}
	flag.Parse()

	if *showVer {
		fmt.Printf("wsf %s\n", version)
		return
	}

	root := *dir
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatalf("无法获取当前目录: %v", err)
		}
		root = wd
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		log.Fatalf("共享目录无效: %v", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil || !info.IsDir() {
		log.Fatalf("共享目录不是有效的文件夹: %s", absRoot)
	}

	if *public && *pws == "" {
		log.Fatal("公网模式（-public）必须同时设置访问密码（-pws），否则任何人都能访问你的文件")
	}
	useTLS := *certFile != "" || *keyFile != ""
	if *certFile == "" || *keyFile == "" {
		if useTLS {
			log.Fatal("HTTPS 需要同时提供 -cert 和 -key 两个文件")
		}
	}

	allow := parseAllowList(*allowList)
	if *allowList != "" && len(allow) == 0 {
		log.Fatal("IP 白名单格式无效，示例：-allow 1.2.3.4,10.0.0.0/8")
	}

	app := NewApp(absRoot, *proxy, *noProxy, *port, *pws, *apiKey, *public, useTLS, allow)
	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", *addr, *port),
		Handler:           app.handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	printBanner(app)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	var serveErr error
	if useTLS {
		serveErr = server.ListenAndServeTLS(*certFile, *keyFile)
	} else {
		serveErr = server.ListenAndServe()
	}
	if serveErr != nil && serveErr != http.ErrServerClosed {
		log.Fatalf("服务启动失败: %v", serveErr)
	}
	fmt.Println("已退出")
}

func printBanner(app *App) {
	sep := strings.Repeat("─", 56)
	fmt.Println(sep)
	fmt.Printf("  wsf %s  ·  局域网网页文件共享\n", version)
	fmt.Println(sep)
	fmt.Printf("  共享目录  %s\n", app.root)
	fmt.Printf("  本地访问  http://127.0.0.1:%d\n", app.port)
	for _, ip := range app.addrs {
		fmt.Printf("  局域网访问 http://%s:%d\n", formatHost(ip), app.port)
	}
	if app.noProxy {
		fmt.Println("  代理      已禁用（远程下载直连）")
	} else {
		fmt.Printf("  代理      %s（用于网页“远程下载”）\n", app.proxy)
	}
	if app.authEnabled {
		fmt.Println("  访问密码  已启用（连续输错 5 次将锁定 15 分钟）")
	} else {
		fmt.Println("  访问密码  未启用（任何人可访问）")
	}
	if app.publicMode {
		fmt.Println("  公网模式  已开启")
	} else {
		fmt.Println("  公网模式  未开启（默认仅适合可信局域网）")
	}
	if app.apiKey != "" {
		fmt.Println("  API        已启用（/api/v1，脚本用 Authorization: Bearer 密钥调用）")
		if len(app.apiKey) < 8 {
			fmt.Println("  API 提示  API 密钥过短，建议至少 8 位")
		}
		if !app.authEnabled {
			fmt.Println("  API 提示  未设置 -pws，网页与接口均免密，建议同时设置访问密码")
		}
	} else {
		fmt.Println("  API        未启用（-api 密钥可开启 /api/v1 接口）")
	}
	if app.https {
		fmt.Println("  HTTPS      已启用（密码与流量加密传输）")
	} else {
		fmt.Println("  HTTPS      未启用（密码将以明文传输，公网建议启用）")
	}
	if len(app.allowList) > 0 {
		fmt.Printf("  IP 白名单  仅允许 %d 个网段访问\n", len(app.allowList))
	} else {
		fmt.Println("  IP 白名单  未限制（默认允许所有来源）")
	}
	if app.ffmpeg == nil {
		fmt.Println("  媒体预览  原生格式（未检测到 ffmpeg，其他格式需安装 ffmpeg）")
	} else {
		fmt.Printf("  媒体预览  原生格式 + ffmpeg 转码（%s）\n", app.videoCodecName())
	}
	fmt.Println(sep)
	fmt.Println("  按 Ctrl+C 退出")
	fmt.Println(sep)
}

// lanIPs 收集本机可被局域网访问的 IPv4 与 IPv6 地址（排除回环与链路本地）。
func lanIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				out = append(out, ip4.String())
			} else if ip.IsGlobalUnicast() || isULA(ip) {
				out = append(out, ip.String())
			}
		}
	}
	return out
}

// isULA 判断是否为 IPv6 唯一本地地址（fc00::/7 的 fd00::/8 段）。
func isULA(ip net.IP) bool {
	return len(ip) == net.IPv6len && ip[0] == 0xfd
}

// formatHost 把 IP 格式化为 URL 主机部分，IPv6 需加方括号。
func formatHost(ip string) string {
	if strings.Contains(ip, ":") {
		return "[" + ip + "]"
	}
	return ip
}
