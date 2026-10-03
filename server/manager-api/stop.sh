#!/bin/sh
set -eu

APP_NAME="manager-api"
PORT="8491"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$SCRIPT_DIR/$APP_NAME.pid"

stop_pid() {
  pid="$1"
  if [ -z "$pid" ] || ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi

  kill "$pid"
  i=1
  while [ "$i" -le 10 ]; do
    if ! kill -0 "$pid" 2>/dev/null; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done

  kill -9 "$pid" 2>/dev/null || true
}

if [ -f "$PID_FILE" ]; then
  PID="$(cat "$PID_FILE")"
  stop_pid "$PID"
  rm -f "$PID_FILE"
  echo "$APP_NAME stopped by pid file, pid: $PID"
fi

# 兜底：按端口找残留进程。只认**监听**该端口的进程，并核对进程名——
# `lsof -ti :PORT` 会把连着这个端口的客户端（例如 next-server）一起列出来，
# 10-03 就误杀过一次前端。
KILLED=0
if command -v lsof >/dev/null 2>&1; then
  PIDS="$(lsof -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null || true)"
  for PID in $PIDS; do
    COMM="$(ps -o comm= -p "$PID" 2>/dev/null || true)"
    case "$COMM" in
      "$APP_NAME"*)
        stop_pid "$PID"
        echo "$APP_NAME stopped by listening port $PORT, pid: $PID"
        KILLED=1
        ;;
      *)
        echo "port $PORT is listened by '$COMM' (pid $PID), not $APP_NAME; left untouched"
        ;;
    esac
  done
fi
if [ "$KILLED" = 1 ]; then
  exit 0
fi

echo "$APP_NAME is not running (or stopped by pid file)"
