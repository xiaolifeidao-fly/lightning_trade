#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib.sh"
argus_init_paths
owns_lock=0
if [ "${ARGUS_CONTROL_LOCK_HELD:-}" != "1" ]; then
    argus_acquire_lock
    owns_lock=1
fi
if [ "$owns_lock" = "1" ]; then
    trap argus_release_lock EXIT
fi

echo "🛑 停止 $APP_NAME..."
if ! pid="$(argus_read_pid)"; then
    echo "ℹ️  没有找到由 $PID_FILE 管理且路径匹配的 $APP_NAME 进程"
    exit 0
fi

echo "找到受控进程 ID: $pid"
kill -TERM "$pid"
for _ in $(seq 1 10); do
    if ! argus_pid_is_target "$pid"; then
        rm -f "$PID_FILE"
        echo "✅ $APP_NAME 已优雅停止"
        exit 0
    fi
    sleep 1
done

if argus_pid_is_target "$pid"; then
    echo "⚠️  进程在 10 秒内未退出，发送 SIGKILL" >&2
    kill -KILL "$pid"
    sleep 1
fi
if argus_pid_is_target "$pid"; then
    echo "❌ 无法停止受控进程 $pid" >&2
    exit 1
fi
rm -f "$PID_FILE"
echo "✅ $APP_NAME 已停止"
