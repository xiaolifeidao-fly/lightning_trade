#!/bin/bash

# Oracle 停止入口（根目录便捷封装，实际逻辑在 script/stop.sh）
ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
exec bash "$ROOT_DIR/script/stop.sh" "$@"
