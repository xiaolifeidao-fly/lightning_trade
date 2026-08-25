#!/usr/bin/env bash

argus_init_paths() {
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
    ARGUS_ROOT="$(cd "$SCRIPT_DIR/.." && pwd -P)"
    APP_NAME="argus_single"
    APP_PATH="$ARGUS_ROOT/$APP_NAME"
    PID_FILE="$ARGUS_ROOT/$APP_NAME.pid"
    LOG_FILE="$ARGUS_ROOT/server.log"
    LOCK_DIR="$ARGUS_ROOT/.argus_single.control.lock"
}

argus_acquire_lock() {
    if [ "${ARGUS_CONTROL_LOCK_HELD:-}" = "1" ]; then
        return 0
    fi
    if ! mkdir "$LOCK_DIR" 2>/dev/null; then
        local lock_pid=""
        if [ -f "$LOCK_DIR/pid" ]; then
            lock_pid="$(tr -d '[:space:]' < "$LOCK_DIR/pid")"
        fi
        if [[ "$lock_pid" =~ ^[0-9]+$ ]] && ! kill -0 "$lock_pid" 2>/dev/null; then
            rm -f "$LOCK_DIR/pid"
            rmdir "$LOCK_DIR" 2>/dev/null || true
        fi
        if ! mkdir "$LOCK_DIR" 2>/dev/null; then
            echo "❌ Argus 控制请求正在执行，请稍后重试" >&2
            return 1
        fi
    fi
    printf '%s\n' "$$" > "$LOCK_DIR/pid"
    ARGUS_CONTROL_LOCK_HELD=1
    export ARGUS_CONTROL_LOCK_HELD
}

argus_release_lock() {
    if [ "${ARGUS_CONTROL_LOCK_HELD:-}" = "1" ]; then
        rm -f "$LOCK_DIR/pid"
        rmdir "$LOCK_DIR" 2>/dev/null || true
        unset ARGUS_CONTROL_LOCK_HELD
    fi
}

argus_pid_is_target() {
    local pid="$1"
    [[ "$pid" =~ ^[0-9]+$ ]] || return 1
    kill -0 "$pid" 2>/dev/null || return 1

    local command
    command="$(ps -p "$pid" -o command= 2>/dev/null || true)"
    case "$command" in
        "$APP_PATH"|"$APP_PATH "*) return 0 ;;
        *) return 1 ;;
    esac
}

argus_read_pid() {
    [ -f "$PID_FILE" ] || return 1
    local pid
    pid="$(tr -d '[:space:]' < "$PID_FILE")"
    if argus_pid_is_target "$pid"; then
        printf '%s\n' "$pid"
        return 0
    fi
    rm -f "$PID_FILE"
    return 1
}
