#!/bin/bash

# Oracle 启动入口（根目录便捷封装，实际逻辑在 script/start.sh）
ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
exec bash "$ROOT_DIR/script/start.sh" "$@"
