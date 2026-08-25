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

if [ ! -f "$APP_PATH" ] || [ ! -x "$APP_PATH" ]; then
    echo "❌ 找不到可执行 Argus 文件: $APP_PATH" >&2
    exit 1
fi

if pid="$(argus_read_pid)"; then
    echo "⚠️  $APP_NAME 已在运行，进程 ID: $pid" >&2
    exit 1
fi

echo "🚀 启动 $APP_NAME..."
cd "$ARGUS_ROOT"
nohup "$APP_PATH" > "$LOG_FILE" 2>&1 &
pid=$!
printf '%s\n' "$pid" > "$PID_FILE"

for _ in $(seq 1 5); do
    if argus_pid_is_target "$pid"; then
        echo "✅ $APP_NAME 启动成功，进程 ID: $pid"
        echo "📊 日志文件: $LOG_FILE"
        exit 0
    fi
    sleep 1
done

rm -f "$PID_FILE"
echo "❌ $APP_NAME 启动失败，请查看日志: $LOG_FILE" >&2
exit 1
