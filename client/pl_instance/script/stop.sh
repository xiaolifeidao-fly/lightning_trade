#!/bin/bash

# pl-instance 停止脚本

APP_NAME="pl-instance"
PORT="${PORT:-8765}"
PID_FILE="pl_instance.pid"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ "$(basename "$SCRIPT_DIR")" = "script" ]; then
    SCRIPT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
fi

APP_PATH="$SCRIPT_DIR/dist/server.js"

echo "🛑 停止 $APP_NAME..."

PID=""
if [ -f "$SCRIPT_DIR/$PID_FILE" ]; then
    PID=$(cat "$SCRIPT_DIR/$PID_FILE")
    if ! ps -p "$PID" > /dev/null 2>&1; then
        PID=""
    fi
fi

if [ -z "$PID" ]; then
    PID=$(ps -ef | grep "node $APP_PATH" | grep -v grep | awk '{print $2}' | head -n 1)
fi

if [ -z "$PID" ] && command -v lsof >/dev/null 2>&1; then
    PORT_PID=$(lsof -ti :"$PORT" 2>/dev/null | head -n 1)
    if [ -n "$PORT_PID" ]; then
        PROC_CMD=$(ps -p "$PORT_PID" -o cmd=)
        if [[ "$PROC_CMD" =~ "$APP_PATH" ]] || [[ "$PROC_CMD" =~ "dist/server.js" ]]; then
            PID="$PORT_PID"
        fi
    fi
fi

if [ -z "$PID" ]; then
    echo "ℹ️  没有找到运行中的 $APP_NAME 进程"
    rm -f "$SCRIPT_DIR/$PID_FILE"
    exit 0
fi

echo "找到进程ID: $PID"
ps -p "$PID" -o pid,cmd

kill "$PID"
sleep 2

if ps -p "$PID" > /dev/null 2>&1; then
    echo "⚠️  进程未停止，强制杀死..."
    kill -9 "$PID"
    sleep 1
fi

rm -f "$SCRIPT_DIR/$PID_FILE"
echo "✅ $APP_NAME 已停止"
