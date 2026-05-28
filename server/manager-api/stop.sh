#!/bin/sh
set -eu

APP_NAME="manager-api"
PORT="8291"
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

if command -v lsof >/dev/null 2>&1; then
  PIDS="$(lsof -ti ":$PORT" || true)"
  if [ -n "$PIDS" ]; then
    for PID in $PIDS; do
      stop_pid "$PID"
    done
    echo "$APP_NAME stopped by port $PORT, pid: $PIDS"
    exit 0
  fi
fi

echo "$APP_NAME is not running"
