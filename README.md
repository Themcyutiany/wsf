# wsf — 局域网网页文件共享

`wsf` 是一个用 Go 编写的跨平台（Windows / Linux）网页文件共享工具：启动后在指定端口
开启一个美观的网页，映射指定文件夹的全部内容；局域网内任何设备打开网页即可浏览、
搜索并下载文件，支持文件夹一键打包 ZIP，还支持“远程下载”——粘贴链接，由本机通过
代理抓取后存进共享目录，供局域网内所有人下载。

## ⚡ 一键安装

**Windows（PowerShell 里粘贴这一行，回车）：**

```powershell
irm https://raw.githubusercontent.com/Themcyutiany/wsf/main/install.ps1 | iex
```

**Linux（终端里粘贴这一行，回车，无需 sudo）：**

```bash
curl -fsSL https://raw.githubusercontent.com/Themcyutiany/wsf/main/install.sh | bash
```

> 装完**重新打开终端**，任意目录输入 `wsf` 即可使用。安装脚本会自动获取最新版本并
> **校验下载包的 SHA-256**（与发布页 `sha256sums.txt` 对照），校验失败会自动报错。
> 直连 GitHub 慢时先设置代理：Windows 在 PowerShell 执行
> `$env:HTTPS_PROXY='http://你的代理地址:端口'`，Linux 执行
> `export HTTPS_PROXY=http://你的代理地址:端口`。
> 也可以直接从 [GitHub Releases](https://github.com/Themcyutiany/wsf/releases) 下载二进制。

## 快速开始

```bash
# 共享当前目录，默认端口 5665
wsf -f .

# 媒体预览：程序已内置 ffmpeg，MKV/AVI 等格式开箱即可转码播放，无需额外安装

# 共享指定文件夹，自定义端口
wsf -f D:\share -p 8080

# 设置 Web 访问密码（打开网页需先输密码才能浏览/下载）
wsf -f D:\share -p 5665 -pws 123456

# 开启 API（脚本用 Bearer 密钥调用 /api/v1/*）
wsf -f D:\share -p 5665 -api 你的密钥

# 远程下载不走代理（直连）
wsf -f . --no-proxy
```

启动后终端会打印局域网访问地址（如 `http://192.168.1.8:5665`），把地址发给别人，
对方用浏览器打开即可浏览和下载文件。

## 参数

| 参数 | 说明 | 默认值 |
| --- | --- | --- |
| `-f 文件夹` | 要共享的文件夹 | 当前目录 |
| `-p 端口` | 监听端口 | `5665` |
| `-pws 密码` | Web 访问密码（设置后浏览/下载需先输密码） | 空（不设密码） |
| `-api 密钥` | API 密钥（设置后启用 `/api/v1` 接口，脚本用 Bearer 密钥调用） | 空（不启用） |
| `-a 地址` | 监听地址（`[::]` 双栈） | `[::]` |
| `-public` | 公网模式：强制要求设置 `-pws` 密码，防止无密码暴露到公网 | 关 |
| `-cert 证书` | HTTPS 证书文件（与 `-key` 一起提供后启用 HTTPS） | 空（HTTP） |
| `-key 私钥` | HTTPS 私钥文件 | 空（HTTP） |
| `-allow IP/CIDR` | 仅允许这些 IP/CIDR 访问（逗号分隔，如 `1.2.3.4,10.0.0.0/8`） | 空（不限制） |
| `--proxy 地址` | 远程下载使用的 HTTP 代理 | `http://127.0.0.1:7897` |
| `--no-proxy` | 远程下载直连，不走代理 | 关 |
| `--version` | 显示版本号 |  |

## 功能

- 媒体在线预览：图片、视频、音频在网页里直接预览/播放（原生格式直接播放；其他格式如
  MKV / AVI / FLV / WMV / HEIC / APE 等会自动调用内置 ffmpeg 实时转码播放）
- 访问密码保护：`-pws` 设置密码后，打开网页需先输入密码才能浏览和下载
- 网页浏览共享目录：面包屑导航、搜索、按名称 / 大小 / 时间排序
- 单个文件下载（支持断点续传 Range）；文件夹一键打包 ZIP；多选打包 ZIP
- 远程下载：网页里粘贴 http/https 链接，服务器通过本地代理抓取后保存到共享目录
  （默认代理 `127.0.0.1:7897`，可用 `--proxy` 修改或 `--no-proxy` 关闭）
- 路径穿越防护：任何请求都无法访问共享目录之外的内容
- 纯 Go 标准库实现，零第三方依赖，单文件二进制，Windows / Linux 通用
- 媒体预览说明：jpg/png/gif/webp/svg、mp4/webm/ogv/mov、mp3/wav/flac/m4a/ogg 等常用格式
  由浏览器原生播放；mkv/avi/flv/wmv/heic/ape 等其余格式由程序内置的 `ffmpeg` 自动
  转码播放（正式版二进制已内置 ffmpeg，无需安装；启动横幅会显示检测状态）

## ⚙️ HTTP API（`-api 密钥`）

启动时加 `-api 你的密钥` 即开启 `/api/v1/*` 接口，供脚本 / 程序调用（上传、下载、
远程下载、打包、预览等）。调用时在请求头带密钥：

```text
Authorization: Bearer 你的密钥
# 或
X-API-Key: 你的密钥
```

常用接口（`BASE` 为 `http://服务器:端口`）：

```bash
KEY='你的密钥'
BASE='http://192.168.1.8:5665'

# 服务信息
curl -H "Authorization: Bearer $KEY" $BASE/api/v1/info

# 列目录
curl -H "Authorization: Bearer $KEY" "$BASE/api/v1/list?path=/"

# 下载文件
curl -H "Authorization: Bearer $KEY" -OJ "$BASE/api/v1/download?path=/docs/报告.pdf"

# 上传文件到 /uploads（同名自动加 (1)(2)，不覆盖）
curl -H "Authorization: Bearer $KEY" -F "file=@本地文件.zip" "$BASE/api/v1/upload?path=/uploads"

# 打包下载文件夹
curl -H "Authorization: Bearer $KEY" -OJ "$BASE/api/v1/zip?path=/photos"

# 远程下载（服务器通过代理抓取链接存入共享目录）
curl -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/a.iso"}' $BASE/api/v1/url-download

# 查看 / 取消远程下载任务
curl -H "Authorization: Bearer $KEY" $BASE/api/v1/tasks
curl -H "Authorization: Bearer $KEY" -X POST $BASE/api/v1/tasks/<任务ID>/cancel
```

> 接口一览：`info` 服务信息 · `list` 列目录 · `download` 下载 ·
> `preview` / `thumb` 媒体预览与缩略图 · `zip` 打包 · `upload` 上传 ·
> `url-download` 远程下载 · `tasks` / `tasks/{id}/cancel` 任务管理。
> API 用请求头密钥认证（不走 Cookie），不受浏览器跨站限制；建议配合 `-pws`
> 一起使用，避免网页免密时接口被他人直接调用。

## 🔐 公网安全

把 wsf 暴露到公网（端口映射 / 云服务器）前，请务必开启以下防护：

- **必须设置密码**：`-pws 密码`，否则任何人都能浏览下载你的文件
- **强制公网模式**：`-public`，未设置密码时程序会拒绝启动
- **HTTPS 加密**：`-cert 证书 -key 私钥` 提供后流量与密码均加密传输
  （可用 Caddy / Nginx 反向代理自动签发证书，或使用自签名证书）
- **IP 白名单**：`-allow 1.2.3.4,10.0.0.0/8`，仅允许指定来源访问
- **登录防爆破**：同一 IP 连续输错 5 次密码自动锁定 15 分钟
- **跨站请求防护**：所有写操作校验来源，恶意网页无法借用你的浏览器操作
- **安全响应头**：CSP / X-Frame-Options / nosniff / API 禁止缓存

公网部署示例：

```bash
wsf -f /data/share -p 8443 -pws 你的强密码 -public -allow 202.96.134.0/24 -cert fullchain.pem -key privkey.pem
```

## 构建

需要 Go 1.24+：

```bash
go build .                  # 构建当前平台（不带内置 ffmpeg）
go build -tags embedded_ffmpeg .   # 构建并内置 ffmpeg（正式版方式）
make build                  # 构建当前平台（make 构建默认内置 ffmpeg）
make release                # 交叉编译 4 个平台二进制到 dist/ 并生成 sha256sums.txt
make test                   # go vet + go test
```

> amd64 版本内置 ffmpeg（开箱即转码）；arm64 版本未内置 ffmpeg，需要系统安装
> ffmpeg 才能转码非原生格式。
> 打 `v*` 标签推送到 GitHub 后，`.github/workflows/release.yml` 会自动构建并发布
> Release（含 sha256sums.txt），一键安装脚本随后即可安装到该版本。

## 安全提示

- 局域网内可信环境可直接使用；跨网段或公网分享务必开启 `-pws` + `-public`，
  公网建议再加 HTTPS 与 IP 白名单（见上方“公网安全”）。
- 即使有密码保护，也不建议把任意文件目录长期暴露到公网；分享完及时退出。
