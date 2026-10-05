#!/usr/bin/env bash
# sshzy 统一仓库 —— 生图工作台（image-playground）构建与产物同步
#
#   bash scripts/build-playground.sh
#
# 源码：image-playground/        统一仓库内，唯一源（外部目录已归档停用）
# 产物：image-playground/dist → playground/   nginx 直读并挂到 /image/
#
# ⚠️ playground/ 是入库目录，docker-compose 以 ./playground 只读挂载，
#    因此本脚本只替换目录内容，绝不删除 playground/ 目录本身。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/image-playground"
OUT="$ROOT/playground"

echo "== 1/3 安装依赖（npm ci）=="
( cd "$SRC" && npm ci )

echo "== 2/3 构建（tsc -b && vite build）=="
( cd "$SRC" && npm run build )

if [ ! -f "$SRC/dist/index.html" ]; then
  echo "构建失败：缺少产物 $SRC/dist/index.html" >&2
  exit 1
fi

echo "== 3/3 同步产物到 playground/ =="
# 替换式同步：先清空旧产物（含上一版哈希 chunk，避免残留），再整体复制
find "$OUT" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
cp -a "$SRC/dist/." "$OUT/"

echo
echo "完成 → /image/"
echo "  · 与 UI 重建是两条独立链路：UI 用 scripts/rebuild.sh，生图应用用本脚本"
echo "  · Service Worker 会缓存入口，浏览器请 Ctrl+Shift+R 强刷"