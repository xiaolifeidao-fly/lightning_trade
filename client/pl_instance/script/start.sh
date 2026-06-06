#!/bin/bash

# pl-instance 启动脚本

APP_NAME="pl-instance"
PORT="${PORT:-8765}"
LOG_FILE="server.log"
PID_FILE="pl_instance.pid"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ "$(basename "$SCRIPT_DIR")" = "script" ]; then
    SCRIPT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
fi

APP_PATH="$SCRIPT_DIR/dist/server.js"

if [ ! -f "$APP_PATH" ]; then
    echo "❌ 错误: 找不到 $APP_PATH，请先执行 script/build.sh"
    exit 1
fi

PID=""
if [ -f "$SCRIPT_DIR/$PID_FILE" ]; then
    OLD_PID=$(cat "$SCRIPT_DIR/$PID_FILE")
    if ps -p "$OLD_PID" > /dev/null 2>&1; then
        PID="$OLD_PID"
    fi
fi

if [ -z "$PID" ]; then
    PID=$(ps -ef | grep "node $APP_PATH" | grep -v grep | awk '{print $2}' | head -n 1)
fi

if [ -n "$PID" ]; then
    echo "⚠️  警告: $APP_NAME 已经在运行，进程ID: $PID"
    exit 1
fi

echo "🚀 启动 $APP_NAME..."
cd "$SCRIPT_DIR" || exit 1
PORT="$PORT" nohup node "$APP_PATH" > "$LOG_FILE" 2>&1 &
NEW_PID=$!
echo "$NEW_PID" > "$SCRIPT_DIR/$PID_FILE"

sleep 2

if ps -p "$NEW_PID" > /dev/null 2>&1; then
    echo "✅ $APP_NAME 启动成功，进程ID: $NEW_PID，端口: $PORT"
    echo "📊 日志文件: $SCRIPT_DIR/$LOG_FILE"
    echo "📝 PID文件: $SCRIPT_DIR/$PID_FILE"
else
    echo "❌ $APP_NAME 启动失败，请查看日志: $SCRIPT_DIR/$LOG_FILE"
    rm -f "$SCRIPT_DIR/$PID_FILE"
    exit 1
fi
