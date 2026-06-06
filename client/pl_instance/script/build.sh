#!/bin/bash

# 构建 pl-instance Node.js 服务

APP_NAME="pl-instance"
BUILD_DIR="$(cd "$(dirname "$0")/.." && pwd)"

echo "开始构建 $APP_NAME..."
echo "构建目录: $BUILD_DIR"

cd "$BUILD_DIR" || exit 1

if [ ! -d "node_modules" ]; then
    echo "未找到 node_modules，开始安装依赖..."
    npm install
fi

rm -rf dist
npm run build

if [ -f "dist/server.js" ]; then
    echo "✅ 构建成功: dist/server.js"
    ls -lh dist/server.js
else
    echo "❌ 构建失败: 找不到 dist/server.js"
    exit 1
fi
