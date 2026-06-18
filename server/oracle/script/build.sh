#!/bin/bash

# 构建 oracle 应用
# 输出 Linux amd64 二进制文件

APP_NAME="oracle"
BUILD_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "开始构建 $APP_NAME..."
echo "构建目录: $BUILD_DIR"

cd "$BUILD_DIR" || exit 1

# 删除旧的二进制文件
rm -f $APP_NAME

# 构建 Linux amd64 版本
GOOS=linux GOARCH=amd64 go build -o $APP_NAME ./cmd.go

if [ -f "$APP_NAME" ]; then
    echo "✅ 构建成功: $APP_NAME"
    ls -lh $APP_NAME
else
    echo "❌ 构建失败"
    exit 1
fi
