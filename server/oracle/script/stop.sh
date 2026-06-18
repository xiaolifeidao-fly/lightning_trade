#!/bin/bash

# Oracle 停止脚本

APP_NAME="oracle"
PID_FILE="oracle.pid"

# 获取脚本所在目录的绝对路径
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# 如果脚本在 script/ 目录下，则项目根目录是父目录
if [ "$(basename "$SCRIPT_DIR")" = "script" ]; then
    SCRIPT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
fi
APP_PATH="$SCRIPT_DIR/$APP_NAME"

echo "🛑 停止 $APP_NAME..."

# 方法1: 通过PID文件查找进程
PID=""
if [ -f "$SCRIPT_DIR/$PID_FILE" ]; then
    PID=$(cat "$SCRIPT_DIR/$PID_FILE")
    # 验证PID是否有效且确实是我们的进程
    if ! ps -p $PID > /dev/null 2>&1; then
        PID=""
    else
        # 验证进程路径是否匹配
        PROC_PATH=$(ps -p $PID -o command= | awk '{print $1}')
        if [ "$PROC_PATH" != "$APP_PATH" ] && [ "$PROC_PATH" != "./$APP_NAME" ] && [[ ! "$PROC_PATH" =~ "$SCRIPT_DIR" ]]; then
            PID=""
        fi
    fi
fi

# 方法2: 通过完整路径查找进程（避免匹配到其他进程）
if [ -z "$PID" ]; then
    PID=$(ps -ef | grep "$APP_PATH" | grep -v grep | awk '{print $2}' | head -n 1)
fi

# 方法3: 如果方法2没找到，通过当前目录下的进程查找
if [ -z "$PID" ]; then
    PID=$(ps -ef | grep "$SCRIPT_DIR/$APP_NAME" | grep -v grep | awk '{print $2}' | head -n 1)
fi

# 检查是否找到了进程ID
if [ -z "$PID" ]; then
    echo "ℹ️  没有找到运行中的 $APP_NAME 进程"
    # 清理可能存在的无效PID文件
    rm -f "$SCRIPT_DIR/$PID_FILE"
    exit 0
else
    echo "找到进程ID: $PID"
    # 显示进程信息
    ps -p $PID -o pid,command

    # 优雅停止
    kill $PID

    # 等待进程停止
    sleep 2

    # 检查是否还在运行
    if ps -p $PID > /dev/null 2>&1; then
        echo "⚠️  进程未停止，强制杀死..."
        kill -9 $PID
        sleep 1
    fi

    # 清理PID文件
    rm -f "$SCRIPT_DIR/$PID_FILE"

    echo "✅ $APP_NAME 已停止"
fi
