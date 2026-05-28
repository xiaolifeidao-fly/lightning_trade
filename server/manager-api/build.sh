#!/usr/bin/env bash
set -euo pipefail

APP_NAME="manager-api"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT="${OUTPUT:-$SCRIPT_DIR/$APP_NAME}"

cd "$SCRIPT_DIR"

go mod tidy
go build -o "$OUTPUT" cmd.go

echo "$APP_NAME built: $OUTPUT"
