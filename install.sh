#!/usr/bin/env bash
# wsf 一键安装脚本（Linux）
# 用法：curl -fsSL https://raw.githubusercontent.com/Themcyutiany/wsf/main/install.sh | bash
# 说明：不需要 sudo；自动获取最新版本，按 CPU 架构下载对应程序，
#       校验 SHA-256 后安装到 ~/.local/bin，并把该目录加入 PATH。

set -euo pipefail

REPO='Themcyutiany/wsf'
TAG=''
if command -v curl >/dev/null 2>&1; then
  TAG="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed -n 's#.*/tag/##p')" || true
fi
[ -n "$TAG" ] || TAG='latest'

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) SUFFIX='amd64' ;;
  aarch64|arm64) SUFFIX='arm64' ;;
  *) echo "不支持的架构：$ARCH" >&2; exit 1 ;;
esac
ASSET="wsf-linux-$SUFFIX"
URL="https://github.com/$REPO/releases/latest/download/$ASSET"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "正在下载 $ASSET（版本 $TAG）..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$TMP/$ASSET" "$URL"
else
  wget -q -O "$TMP/$ASSET" "$URL"
fi

# 校验 SHA-256（对照发布页 sha256sums.txt）
if curl -fsSL -o "$TMP/sha256sums.txt" "https://github.com/$REPO/releases/latest/download/sha256sums.txt" 2>/dev/null; then
  WANT="$(awk -v a="$ASSET" '$2==a {print $1; exit}' "$TMP/sha256sums.txt" | tr 'A-F' 'a-f')"
  if [ -n "$WANT" ]; then
    GOT="$(sha256sum "$TMP/$ASSET" | awk '{print $1}')"
    if [ "$GOT" != "$WANT" ]; then
      echo "校验失败：$ASSET 的 SHA-256 与发布页不一致，请重新运行安装。" >&2
      exit 1
    fi
    echo "校验通过：$ASSET 与发布页 SHA-256 一致"
  else
    echo "警告：发布页未列出 $ASSET 的校验值，跳过校验。"
  fi
else
  echo "警告：无法获取校验文件，跳过校验。"
fi

DEST_DIR="$HOME/.local/bin"
mkdir -p "$DEST_DIR"
DEST="$DEST_DIR/wsf"
install -m 0755 "$TMP/$ASSET" "$DEST"
echo "已安装到：$DEST"

case ":$PATH:" in
  *":$DEST_DIR:"*) ;;
  *) echo "已把 $DEST_DIR 加入 PATH（请重新打开终端生效）" ;;
esac

"$DEST" --version
echo ''
echo '安装完成！请重新打开终端，然后在任意目录输入 wsf 即可。'
echo '快速开始：wsf -f ~/share        # 共享文件夹，默认端口 5665'
