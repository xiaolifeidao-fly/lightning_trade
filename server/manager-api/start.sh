#!/bin/sh
set -eu

APP_NAME="manager-api"
PORT="8491"
STARTUP_TIMEOUT="${STARTUP_TIMEOUT:-90}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$SCRIPT_DIR/$APP_NAME.pid"
LOG_DIR="${LOG_DIR:-$SCRIPT_DIR/logs}"
LOG_FILE="${LOG_FILE:-$LOG_DIR/$APP_NAME.log}"

is_listening() {
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$PORT" -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi
  if command -v nc >/dev/null 2>&1; then
    nc -z 127.0.0.1 "$PORT" >/dev/null 2>&1
    return $?
  fi
  return 1
}

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

elapsed=0
while [ "$elapsed" -lt "$STARTUP_TIMEOUT" ]; do
  if ! kill -0 "$PID" 2>/dev/null; then
    rm -f "$PID_FILE"
    echo "$APP_NAME failed to start, see log: $LOG_FILE" >&2
    exit 1
  fi
  if is_listening; then
    echo "$APP_NAME started, pid: $PID, port: $PORT, log: $LOG_FILE"
    exit 0
  fi
  sleep 1
  elapsed=$((elapsed + 1))
done

if ! kill -0 "$PID" 2>/dev/null; then
  rm -f "$PID_FILE"
  echo "$APP_NAME failed to start, see log: $LOG_FILE" >&2
fi

echo "$APP_NAME did not listen on port $PORT within ${STARTUP_TIMEOUT}s, see log: $LOG_FILE" >&2
exit 1
