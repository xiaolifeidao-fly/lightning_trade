#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib.sh"
argus_init_paths
argus_acquire_lock
trap argus_release_lock EXIT

case "${1:-}" in
    start)
        "$SCRIPT_DIR/start.sh"
        ;;
    stop)
        "$SCRIPT_DIR/stop.sh"
        ;;
    restart)
        "$SCRIPT_DIR/stop.sh"
        "$SCRIPT_DIR/start.sh"
        ;;
    status)
        if pid="$(argus_read_pid)"; then
            echo "running pid=$pid"
        else
            echo "stopped"
        fi
        ;;
    *)
        echo "用法: $0 {start|stop|restart|status}" >&2
        exit 64
        ;;
esac
