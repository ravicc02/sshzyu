#!/usr/bin/env bash
# 本地 sshzyu 站点一键重建 —— 完全从 GitHub 分支源码构建
#
#   bash sshzyu-local-site/rebuild.sh
#
# 构建链：fetch 分支 → LF 规范化 → 前端 build → 复现线上补丁 → docs build → 装配
# nginx 通过 bind-mount 直读站点目录，重建后无需重启容器，浏览器刷新即可（建议 Ctrl+Shift+R）。
set -euo pipefail

SITE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WS=/f/中转站运营
WT="$WS/_build/sshzyu-frontend"
FE="$WT/frontend"
DIST="$WT/backend/internal/web/dist"
DOCSDIST="$WT/web-docs/dist"
BRANCH=fork/codex/sshzyu-frontend

echo "== 1/6 同步源码：$BRANCH =="
git -C "$WS/sub2api" fetch --prune fork
git -C "$WT" reset --hard --quiet
git -C "$WT" checkout --detach "$BRANCH" --quiet
git -C "$WT" log -1 --format='   HEAD %h %ad %s' --date=short

echo "== 2/6 LF 规范化（Windows CRLF 会让产物 hash 与线上不一致）=="
node "$SITE/build/normalize-lf.mjs"

echo "== 3/6 安装依赖 =="
pnpm -C "$FE" install --frozen-lockfile

echo "== 4/6 构建前端 + 复现线上补丁 =="
pnpm -C "$FE" run build
node "$SITE/build/build-online-equivalent.mjs"

echo "== 5/6 构建文档站 =="
node "$WT/web-docs/build.mjs"

echo "== 6/6 装配到站点目录 =="
mkdir -p "$SITE/sshzyu-ui/current/assets" "$SITE/sshzyu-ui/shared/assets" "$SITE/sshzyu-docs/assets"
cp "$DIST/index.html" "$DIST/logo.svg" "$SITE/sshzyu-ui/current/"
cp "$DIST/assets/"* "$SITE/sshzyu-ui/current/assets/"
cp "$DIST/assets/"* "$SITE/sshzyu-ui/shared/assets/"
cp "$DOCSDIST/index.html" "$SITE/sshzyu-docs/"
cp "$DOCSDIST/assets/"* "$SITE/sshzyu-docs/assets/"

echo "== 校验 =="
node "$SITE/build/verify/check-chunks.mjs"
node "$SITE/build/verify/compare-build.mjs" | grep -E 'only local|identical:|IDENTICAL|DIFFERENT' || true

echo
echo "完成 → http://127.0.0.1:8080/"
echo "  /image/  生图应用    /docs/  接入文档    /image-studio  创作画布"