#!/bin/bash

# Oracle 本地重新部署脚本：本地原生编译 -> 停止 -> 启动
# 按当前机器（如 macOS darwin/arm64）原生编译，便于本地调试运行。

set -e

APP_NAME="oracle"
ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT_DIR="$ROOT_DIR/script"

echo "=========================================="
echo "🔧 步骤1: 本地编译 ($(go env GOOS)/$(go env GOARCH))"
echo "=========================================="
cd "$ROOT_DIR" || exit 1
rm -f "$APP_NAME"
# 不指定 GOOS/GOARCH，按本机环境编译；oracle go.mod 已声明 go 1.25.0，
# GOTOOLCHAIN=auto 会自动选用匹配的工具链。
go build -o "$APP_NAME" ./cmd.go

if [ ! -f "$ROOT_DIR/$APP_NAME" ]; then
    echo "❌ 编译失败"
    exit 1
fi
echo "✅ 编译成功: $ROOT_DIR/$APP_NAME"
ls -lh "$ROOT_DIR/$APP_NAME"

echo ""
echo "=========================================="
echo "🛑 步骤2: 停止旧进程"
echo "=========================================="
bash "$SCRIPT_DIR/stop.sh"

echo ""
echo "=========================================="
echo "🚀 步骤3: 启动新进程"
echo "=========================================="
bash "$SCRIPT_DIR/start.sh"
