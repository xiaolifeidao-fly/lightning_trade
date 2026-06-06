#!/bin/bash

# Argus Single 启动脚本

APP_NAME="argus_single"
LOG_FILE="server.log"
PID_FILE="argus_single.pid"

# 获取脚本所在目录的绝对路径
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# 如果脚本在 script/ 目录下，则项目根目录是父目录
if [ "$(basename "$SCRIPT_DIR")" = "script" ]; then
    SCRIPT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
fi
APP_PATH="$SCRIPT_DIR/$APP_NAME"

# 检查应用是否存在
if [ ! -f "$APP_PATH" ]; then
    echo "❌ 错误: 找不到 $APP_NAME 文件: $APP_PATH"
    exit 1
fi

# 检查是否已经在运行（使用完整路径匹配，避免匹配到其他argus进程）
PID=$(ps -ef | grep "$APP_PATH" | grep -v grep | awk '{print $2}')
if [ -z "$PID" ]; then
    # 如果没找到，尝试通过PID文件查找
    if [ -f "$SCRIPT_DIR/$PID_FILE" ]; then
        OLD_PID=$(cat "$SCRIPT_DIR/$PID_FILE")
        if ps -p $OLD_PID > /dev/null 2>&1; then
            PID=$OLD_PID
        fi
    fi
fi

if [ -n "$PID" ]; then
    echo "⚠️  警告: $APP_NAME 已经在运行，进程ID: $PID"
    exit 1
fi

# 启动应用
echo "🚀 启动 $APP_NAME..."
cd "$SCRIPT_DIR" || exit 1
nohup ./$APP_NAME > $LOG_FILE 2>&1 &
NEW_PID=$!

# 保存PID到文件
echo $NEW_PID > "$SCRIPT_DIR/$PID_FILE"

# 等待一下确保启动
sleep 2

# 检查是否启动成功
if ps -p $NEW_PID > /dev/null 2>&1; then
    echo "✅ $APP_NAME 启动成功，进程ID: $NEW_PID"
    echo "📊 日志文件: $SCRIPT_DIR/$LOG_FILE"
    echo "📝 PID文件: $SCRIPT_DIR/$PID_FILE"
else
    echo "❌ $APP_NAME 启动失败，请查看日志: $SCRIPT_DIR/$LOG_FILE"
    rm -f "$SCRIPT_DIR/$PID_FILE"
    exit 1
fi
