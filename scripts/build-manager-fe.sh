#!/usr/bin/env bash
#
# 构建管理端前端并打成可发布的 tar.gz（Next standalone 根）。
#
# 与 scripts/deploy-manager-fe.sh 配套：
#   scripts/build-manager-fe.sh            # 产出 dist/manager-fe-<BUILD_ID>.tar.gz
#   scripts/deploy-manager-fe.sh dist/...  # 上传、切换、校验、失败回滚
#
# 为什么要有它：此前这套动作是手贴 docker build / docker cp / tar 三条命令，
# Dockerfile 还放在会话临时目录。2026-09-13 那个临时文件没了，docker build 以
# rc=1 失败，但外层脚本退出码是 0 —— 于是**拿上一次的镜像导出了产物**，
# 改动没进构建，BUILD_ID 和线上一模一样，差点当成"发布成功"。
# 所以这里每一步都检查退出码，并在最后核对 BUILD_ID 确实变了。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_DIR="$ROOT/client/manager"
OUT_DIR="${OUT_DIR:-$ROOT/dist}"
IMAGE="manager-fe-build:latest"

die() { echo "❌ $*" >&2; exit 1; }

[ -f "$APP_DIR/Dockerfile.build" ] || die "缺 $APP_DIR/Dockerfile.build"
command -v docker >/dev/null || die "需要 docker"

echo "▶ 构建镜像（linux/amd64）"
docker build --platform linux/amd64 -f "$APP_DIR/Dockerfile.build" -t "$IMAGE" "$APP_DIR" \
  || die "docker build 失败"

echo "▶ 导出 standalone 产物"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
CID="$(docker create --platform linux/amd64 "$IMAGE")"
docker cp "$CID:/app/.next/standalone/." "$WORK/" >/dev/null || { docker rm "$CID" >/dev/null; die "导出 standalone 失败"; }
docker cp "$CID:/app/.next/static" "$WORK/.next/static" >/dev/null || { docker rm "$CID" >/dev/null; die "导出 static 失败"; }
docker rm "$CID" >/dev/null

[ -f "$WORK/server.js" ] || die "产物里没有 server.js"
[ -s "$WORK/.next/BUILD_ID" ] || die "产物里没有 BUILD_ID"
BUILD_ID="$(cat "$WORK/.next/BUILD_ID")"

mkdir -p "$OUT_DIR"
TARBALL="$OUT_DIR/manager-fe-$BUILD_ID.tar.gz"
tar czf "$TARBALL" -C "$WORK" .

echo "✅ $TARBALL"
echo "   BUILD_ID = $BUILD_ID"
echo "   下一步:   scripts/deploy-manager-fe.sh \"$TARBALL\""
