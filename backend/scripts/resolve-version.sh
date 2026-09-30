#!/bin/sh
set -eu

# 本地定制版总是以受审查的 VERSION 文件为默认来源，不能被 checkout 上的官方 tag 覆盖。
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
VERSION_FILE="$SCRIPT_DIR/../cmd/server/VERSION"
tr -d '\r\n' < "$VERSION_FILE"
printf '\n'
