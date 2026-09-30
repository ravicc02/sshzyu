#!/usr/bin/env bash
# sshzy 统一仓库 —— 本地站点一键重建
#
#   bash scripts/rebuild.sh
#
# 构建链：pnpm install → 前端 build → 生成 fw-cachebust.js → 装配
# nginx 通过 bind-mount 直读站点目录，重建后无需重启容器，浏览器刷新即可（建议 Ctrl+Shift+R）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FE="$ROOT/frontend"
DIST="$ROOT/backend/internal/web/dist"

echo "== 1/4 安装依赖 =="
pnpm --dir "$FE" install --frozen-lockfile

echo "== 2/4 构建前端 =="
pnpm --dir "$FE" run build

echo "== 3/4 生成 fw-cachebust.js + 修正 index.html =="
node "$ROOT/scripts/build-online-equivalent.mjs"

echo "== 4/4 装配到 ui/ =="
mkdir -p "$ROOT/ui/current/assets" "$ROOT/ui/shared/assets"
cp "$DIST/index.html" "$DIST/logo.svg" "$ROOT/ui/current/"
cp "$DIST/assets/"* "$ROOT/ui/current/assets/"
# fw-cachebust.js 使用固定文件名，但 Nginx 从 shared/assets 提供它；必须每次覆盖，
# 否则入口仍会加载上一版 immutable chunk，导致新页面逻辑或 i18n 未生效。
cp "$DIST/assets/fw-cachebust.js" "$ROOT/ui/shared/assets/fw-cachebust.js"
# shared 累积所有版本（保持与线上部署的版本命名习惯一致）
for f in "$DIST/assets/"*; do
  name="$(basename "$f")"
  if [ ! -f "$ROOT/ui/shared/assets/$name" ]; then
    cp "$f" "$ROOT/ui/shared/assets/"
  fi
done

echo "== 校验 =="
node "$ROOT/scripts/check-chunks.mjs"

echo
echo "完成 → http://127.0.0.1:8080/"
echo "  /            SSHZYU 新 UI（前端面板）"
echo "  /image/      生图应用"
echo "  /docs/       接入文档"
echo "  /image-studio  创作画布"