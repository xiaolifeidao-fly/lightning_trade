#!/bin/sh
set -eu

APP_NAME="manager-api"
PORT="8291"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$SCRIPT_DIR/$APP_NAME.pid"
LOG_DIR="${LOG_DIR:-$SCRIPT_DIR/logs}"
LOG_FILE="${LOG_FILE:-$LOG_DIR/$APP_NAME.log}"

cd "$SCRIPT_DIR"
mkdir -p "$LOG_DIR"

if [ -f "$PID_FILE" ]; then
  PID="$(cat "$PID_FILE")"
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    echo "$APP_NAME is already running, pid: $PID"
    exit 0
  fi
  rm -f "$PID_FILE"
fi

if command -v lsof >/dev/null 2>&1; then
  PORT_PID="$(lsof -ti ":$PORT" || true)"
  if [ -n "$PORT_PID" ]; then
    echo "port $PORT is already in use by pid: $PORT_PID"
    exit 1
  fi
fi

if [ -x "$SCRIPT_DIR/$APP_NAME" ]; then
  nohup "$SCRIPT_DIR/$APP_NAME" > "$LOG_FILE" 2>&1 &
elif [ -f "$SCRIPT_DIR/cmd.go" ]; then
  nohup go run cmd.go > "$LOG_FILE" 2>&1 &
else
  echo "no executable '$APP_NAME' or cmd.go found in $SCRIPT_DIR" >&2
  exit 1
fi

PID="$!"
echo "$PID" > "$PID_FILE"

sleep 1
if kill -0 "$PID" 2>/dev/null; then
  echo "$APP_NAME started, pid: $PID, port: $PORT, log: $LOG_FILE"
else
  rm -f "$PID_FILE"
  echo "$APP_NAME failed to start, see log: $LOG_FILE" >&2
  exit 1
fi
