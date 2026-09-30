# AGENTS.md — sshzy 统一仓库 工作规范

> 本文件约束所有在本仓库（中转站运营 / sshzy）中的操作。每次动工前先读一遍。目标：让 agent 明确「仓库里有什么、怎么构建、怎么部署、能干什么、绝不能干什么」。

---

## 0. 统一仓库结构

本仓库把所有来源（sub2api 主仓库、sshzyu 前端 fork、sshzyu-docs、gpt_image_playground）整理为一个统一的 sshzy 仓库。**所有路径以下列相对路径为准，禁止引用已合并的旧路径。**

| 路径 | 内容 | 角色 |
| --- | --- | --- |
| `frontend/` | Vue 前端源码（`sub2api-frontend`） | 品牌 UI 源码，`pnpm build` 产出 |
| `backend/` | Go 后端源码（+ `backend/Dockerfile` 纯后端构建） | 服务端源码 |
| `Dockerfile` | 仓库根多阶段 Dockerfile（前端 build → Go 编译 embed → 运行时） | 本地后端镜像构建 |
| `docker-compose.yml` | 本地统一栈编排（sshzyu-ui + sub2api + postgres + redis） | 本地一键启动 |
| `nginx/` | 本地 nginx 配置（`default.conf` 等，与线上同构、无 TLS） | 本地站点入口 |
| `ui/` | UI 构建产物装配（`current/` + `shared/` + `releases/`） | 站点静态资源，nginx 直读 |
| `docs/` | **线上文档站**（前端 nginx 挂到 `/docs/`） | 对外公开 |
| `playground/` | 生图应用静态产物（nginx 挂到 `/image/`） | 生图工作台 |
| `scripts/` | `rebuild.sh` + 构建/校验脚本 | 本地重建 |
| `deploy/` | 本地运行时数据卷（`data/`、`postgres_data/`、`redis_data/`，已被 `.gitignore` 忽略） | 本地数据 |
| `records/` | 运营问答梳理记录（`.gitignore` 忽略，不入版本库、不对外） | 内部记录 |

**构建链**：`frontend/`(源码) → `scripts/rebuild.sh` → `ui/current`、`ui/shared`（静态产物）。

---

## 1. 服务器与 SSH

本工作区只服务一台服务器：**us-server**（`64.83.2.153`，hostname `RainYun-LFS4goAS`）。

- **SSH 连接：使用你本地的 SSH 配置**。本仓库为多人协作，主机别名（`us-server`）、密钥等连接凭据由各人按自己本机 `~/.ssh/config` 维护，本文件不写死具体的密钥路径或本机专属配置。
- 服务器上**没有源码、没有 git 仓库**，sub2api 使用本地构建镜像 + `docker save/load` 部署。

> ❌ **边界声明**：其它任何服务器（尤其 `Ravi-server` `199.68.217.212`）不在本工作区职责范围，不对其做任何操作。

---

## 2. 线上服务拓扑（us-server）

```
sshzyu.com / www.sshzyu.com (443, Certbot 管理)
  ├─ /          → /opt/sshzyu-ui/current/  静态 UI（SPA 路由）
  ├─ /assets/   → /opt/sshzyu-ui/shared/assets/  共享资源（防白屏）
  ├─ /docs/     → /opt/sshzyu-docs/        文档站
  ├─ /image/    → /opt/imageplayground/    生图工作台
  └─ /api /v1 /health /models 等 → proxy_pass http://127.0.0.1:8080 (sub2api 容器)
```

### 2.1 sub2api 后端（Docker Compose，`/opt/sub2api-deploy/docker-compose.yml`）

| 容器 | 镜像 | 端口 | 说明 |
| --- | --- | --- | --- |
| `sub2api` | `local/sub2api-batch:<tag>` | `127.0.0.1:8080→8080` | 主应用，**纯 Go 后端，不内嵌前端** |
| `sub2api-postgres` | `postgres:18-alpine` | 内网 5432 | 数据库 |
| `sub2api-redis` | `redis:8-alpine` | 内网 6379 | 缓存 |

- **⚠️ 镜像为本地构建（`local/` 前缀，无 registry）**。服务器上 `docker compose pull` 拉不到，升级/回滚只能 `docker save/load` 搬运镜像 tar。
- `.env`（`/opt/sub2api-deploy/.env`）：数据库/Redis/JWT/账号等敏感凭证，**含值不外传**，只用路径引用。
- `data/`（挂载 `/app/data`）：`config.yaml`、日志、插件、页面；`backups/` 等历史备份**不要删**
- 健康检查：`curl http://127.0.0.1:8080/health` → `{"status":"ok"}`

### 2.2 sshzyu-ui（品牌前端，nginx 直出静态文件）

- 部署目录：`/opt/sshzyu-ui/`
  - `releases/<版本>/`：历史版本（回退保障，**不要清理**）
  - `current`：软链指向当前生效 release
  - `shared/assets/`：跨版本累积资产（nginx `/assets/` 指向这里）
  - `switch-release.sh`：切版本（改 `current` 软链 + nginx reload），**不要手动 `ln`**
- **改线上 UI = 走本地构建 → 上传新 release 目录 → 切 current**（见第 5 节）。

### 2.3 关键 nginx 文件（改前先看全，结构复杂）

- `/etc/nginx/sites-enabled/sub2api`：站点主配置（UI 静态 + API 反代 + SPA 路由 + 防白屏守卫）
- `/etc/nginx/snippets/sub2api-proxy.conf`：反代参数
- `/etc/nginx/snippets/sshzyu-ui/`：UI 版本 patch 配置
- `/etc/nginx/conf.d/00-sub2api-map.conf`、`20-sshzyu-ui-map.conf`：map 定义
- **改 nginx 配置必须**：备份 → `nginx -t` → `nginx -s reload`（**不 restart**）。

---

## 3. 端到端开发部署流程

### 3.1 修改前端 → 本地验证 → 线上发布（UI）

```bash
# 1. 改源码：frontend/src/ ...
cd /f/中转站运营/sshzy

# 2. 本地重建站点（install → build → fw-cachebust → 装配到 ui/）
bash scripts/rebuild.sh

# 3. 本地浏览器验证（确认 UI / image / docs 正常）
#    http://127.0.0.1:8080/

# 4. 提交源码（正源是 frontend/，构建链自动产出 ui/）
git add -A && git commit

# 5. 构建线上产物体积小，直接上传 current 目录
scp -r ui/current/* root@64.83.2.153:/opt/sshzyu-ui/current/   # 或做成新 release
```

> 若需保留旧版本用于回退：先 `mkdir /opt/sshzyu-ui/releases/<new>` 上传，再 `bash /opt/sshzyu-ui/switch-release.sh` 切 `current`。
> 前端产物 `ui/` 由构建生成，**不要手改**；只改 `frontend/` 源码再重建。

### 3.2 修改文档站（docs/）

```bash
# docs/ 直接是构建产物且无构建链，改完上传即可
scp docs/index.html docs/content.md docs/iamge/* root@64.83.2.153:/opt/sshzyu-docs/
```

### 3.3 修改后端 → 构建镜像 → 线上部署

后端是纯 Go，**改源码后必须重建镜像**并重新部署。

```bash
# 1.（可选）更新后端版本号（决定二进制内嵌的 --version / UI 版本标签）
#    backend/cmd/server/VERSION 是完整本地构建号（X.Y.Z-rN）。
#    必须与仓库根 upstream-baseline.json 的 local_build_version 一致。

# 2. 正式发布镜像（线上纯后端，须显式注入构建版本和提交 SHA）
VERSION="$(tr -d '\r\n' < backend/cmd/server/VERSION)"
COMMIT="$(git rev-parse HEAD)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
python scripts/upstream_release_guard.py --validate-clean-build --build-target backend
(cd backend && docker build --build-arg VERSION="$VERSION" --build-arg COMMIT="$COMMIT" --build-arg DATE="$DATE" -t "local/sub2api-batch:$VERSION" .)

# 3. 本地验证版本和 Git SHA
# docker run --rm "local/sub2api-batch:$VERSION" /app/main --version
# 输出必须含完整本地版本号及预期提交 SHA；推送/部署之前还须核对业务测试和线上变更流程。

# 4. 导出镜像 tar（若服务器无法直接访问本地 Docker）
docker save local/sub2api-batch:<tag> | gzip -6 > /tmp/sub2api-<tag>.tar.gz

# 5. 上传（用你本地的 scp/rsync，连接方式按本机 SSH 配置）
scp /tmp/sub2api-<tag>.tar.gz root@64.83.2.153:/tmp/

# 6. 服务器：加载镜像 + 更新 compose tag + 重建容器
ssh us-server '
docker load -i /tmp/sub2api-<tag>.tar.gz
sed -i "s|local/sub2api-batch:[^ ]*|local/sub2api-batch:<tag>|g" /opt/sub2api-deploy/docker-compose.yml
cd /opt/sub2api-deploy && docker compose up -d --no-build --force-recreate sub2api
'

# 7. 验证
ssh us-server 'docker exec sub2api /app/main --version && curl -s http://127.0.0.1:8080/health'
```

> **版本号规则**：本地定制版完整版本号由 `backend/cmd/server/VERSION` 决定，格式为 `X.Y.Z-rN`；`resolve-version.sh` 不再从本地/官方 tag 自动推断。`upstream-baseline.json` 必须与版本文件及官方稳定 Release/tag/commit 对齐。根 Dockerfile 默认构建本地 `source` 内嵌前端镜像；发布根镜像需先运行 `python scripts/upstream_release_guard.py --validate-clean-build --build-target root`，然后以 `--build-arg BUILD_TYPE=release` 及 VERSION、COMMIT、DATE 显式构建。`backend/Dockerfile` 只构建线上纯后端 `release` 镜像，发布前应使用 `--build-target backend` 验证。正式发布须核验镜像 tag、二进制输出及管理端 `build_type`。
> **推送门禁**：每次向 `origin` 推送前运行 `python scripts/upstream_release_guard.py`，在当前机器执行一次 `git config core.hooksPath .githooks` 启用自动 `pre-push`。守卫读取待推送提交中的基线、核对官方最新稳定 Release 及 tag SHA；上游状态未知或有新版本时阻止 push 并向用户汇报，未经确认不得自动合并。钩子是本机机制，可被跳过；团队级强制保护仍需服务端分支保护/必需 CI。完整三方增量合并、migration 和部署边界见 `plan_docs/official-upstream-versioning-workflow.md`。
> **回滚**：compose 改回旧 tag → `docker compose up -d --force-recreate`。

---

## 4. 本地开发栈

本地 `docker-compose.yml` 一键启动（**与线上无关**，端口语义不同）。

| 端口 | 容器 | 角色 |
| --- | --- | --- |
| `127.0.0.1:8080` | `sshzyu-ui-local`（nginx） | 本地站点唯一入口：UI + `/docs/` + `/image/` + API 反代 |
| `127.0.0.1:8081` | `sub2api-dev`（`local/sshzyu:dev`） | 后端，仅本机可见 |
| 无外部端口 | `sub2api-postgres-dev` / `sub2api-redis-dev` | 数据库 / 缓存 |

```bash
# 启动（首次或改镜像后）
cp .env.example .env && docker compose up -d --build

# 日常：前端改动直接用 rebuild.sh，nginx bind-mount 直读，刷新即可
bash scripts/rebuild.sh

# 健康检查
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/health
```

- **⚠️ 本地 `8080` = 站点 nginx；线上 `8080` = sub2api 容器**。端口语义务必区分。
- `rebuild.sh` 是纯本地构建（`install → build → fw-cachebust → 装配`），**无 `git reset --hard` 风险**，可放心跑。

---

## 5. 能力边界

### ✅ 可以做

- 所有只读检查：`docker compose ps`、`docker logs`、`curl /health`、`cat` 配置（脱敏后）。
- 管理 sub2api 与 sshzyu-ui：重启容器、看日志、切 UI release。
- 备份数据目录。

### ❌ 绝对不能做（除非用户逐项明确授权）

- **改回 SSH 密码登录**：us-server 已禁密码、仅公钥（`PasswordAuthentication no`）。不得恢复、不得动 `sshd_config` 及 hardening 文件、不重复追加 authorized_keys。
- **改核心网络/SSH 配置前**：先备份 → 校验语法 → 留有另一条登录途径，避免锁死。
- **重启/停止 nginx、fail2ban、cron** 等非 sub2api 服务（nginx 改配置先 `nginx -t` 再 `reload`）。
- **操作数据库数据**：清库、migrate、删表、改 superuser。
- **把 `8080` 暴露公网**：只绑 `127.0.0.1`，经 nginx 反代。
- **删除任何日志/数据/备份**：`data/`、`postgres_data/`、`redis_data/`、`backups/`、`ui/releases/`、部署目录 `*.bak*`。
- **对其它服务器（含 Ravi-server）做任何操作**。

### 🔒 安全红线

- **绝不把** sub2api 的 `.env`、`config.yaml`、凭证文件内容**打印到对话或写进工作区文件**；只引用路径，用值时在服务器上看。
- SSH 私钥泄漏 = 服务器失守；各人保护好自己本地的 SSH 私钥，只读权限不得放宽。
- 命令输出含敏感值先脱敏再展示。

---

## 6. 常用速查

```bash
# 进服务器（us-server 别名在你的本地 SSH 配置中定义）
ssh us-server

# sub2api 后端
ssh us-server 'cd /opt/sub2api-deploy && docker compose ps'
ssh us-server 'curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/health'
ssh us-server 'docker exec sub2api /app/main --version'

# UI
ssh us-server 'readlink /opt/sshzyu-ui/current && ls /opt/sshzyu-ui/releases/'

# 站点可达性
curl -s -o /dev/null -w "%{http_code}\n" https://sshzyu.com/health

# 本地栈
docker compose ps
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/health
```

> 拿不准就停下来问，不要凭猜测动手。本工作区只服务 us-server。
