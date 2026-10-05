# AGENTS.md — sshzy 统一仓库 工作规范

> 本文件约束所有在本仓库（中转站运营 / sshzy）中的操作。每次动工前先读一遍。目标：让 agent 明确「仓库里有什么、怎么构建、怎么部署、能干什么、绝不能干什么」。

---

## 0. 统一仓库结构

本仓库把所有来源（sub2api 主仓库、sshzyu 前端 fork、sshzyu-docs、gpt_image_playground）整理为一个统一的 sshzy 仓库。**所有路径以下列相对路径为准，禁止引用已合并的旧路径。**


| 路径                        | 内容                                                                  | 角色                       |
| ------------------------- | ------------------------------------------------------------------- | ------------------------ |
| `frontend/`               | Vue 前端源码（`sub2api-frontend`）                                        | 品牌 UI 源码，`pnpm build` 产出 |
| `backend/`                | Go 后端源码（+ `backend/Dockerfile` 纯后端构建）                               | 服务端源码                    |
| `Dockerfile`              | 仓库根多阶段 Dockerfile（前端 build → Go 编译 embed → 运行时）                     | 本地后端镜像构建                 |
| `docker-compose.yml`      | 本地统一栈编排（sshzyu-ui + sub2api + postgres + redis）                     | 本地一键启动                   |
| `nginx/`                  | 本地 nginx 配置（`default.conf` 等，与线上同构、无 TLS）                           | 本地站点入口                   |
| `ui/`                     | UI 构建产物装配（`current/` + `shared/` + `releases/`）                     | 站点静态资源，nginx 直读          |
| `docs/`                   | **线上文档站**（前端 nginx 挂到 `/docs/`）                                     | 对外公开                     |
| `playground/`             | 生图应用静态产物（nginx 挂到 `/image/`）                                        | 生图工作台                    |
| `scripts/`                | `rebuild.sh` + 构建/校验脚本                                              | 本地重建                     |
| `scripts/release/`        | 发布准入、migration 审阅记录                                                 | GitHub 发布辅助              |
| `backend/pkg/release/`    | 共享发布契约、版本比较和验签                                                      | 后端/执行器共用                 |
| `backend/pkg/deployment/` | 维护屏障、在途计数                                                           | 更新期间业务保护                 |
| `tools/sshzy-updater/`    | 独立 Go module、宿主机执行器及发布 CLI                                          | 不在网站进程内运行                |
| `deploy/updater/`         | 执行器配置、unit、挂载样例和手册                                                  | 样例，不代表已接入生产              |
| `.github/workflows/`      | 验证与定制发布工作流                                                          | 不自动上线                    |
| `deploy/`                 | 本地运行时数据卷（`data/`、`postgres_data/`、`redis_data/`，已被 `.gitignore` 忽略） | 本地数据                     |
| `records/`                | 运营问答梳理记录（`.gitignore` 忽略，不入版本库、不对外）                                 | 内部记录                     |


**构建链**：`frontend/`(源码) → `scripts/rebuild.sh` → `ui/current`、`ui/shared`（静态产物）。

---

## 1. 服务器与 SSH

本工作区只服务一台服务器：**us-server**。其地址、hostname 与连接凭据由各人本机 SSH 配置维护，本文件不写死。

- **SSH 连接：使用你本地的 SSH 配置**。本仓库为多人协作，主机别名（`us-server`）、密钥等连接凭据由各人按自己本机 `~/.ssh/config` 维护，本文件不写死具体的密钥路径或本机专属配置。
- 服务器上**没有源码、没有 git 仓库**，sub2api 使用本地构建镜像 + `docker save/load` 部署。
- **定制发布主链已源码化**：本地开发 → GitHub 检查/构建 → GHCR 镜像与签名 Release → 网站人工准备/确认。生产仍需按 `deploy/updater/README.md` 完成一次性接入；接入前继续使用人工发布，不把代码改造当成服务已启用。
- **所有上传统一使用 rsync，不使用 scp**，目标主机使用 `us-server` 别名。目录上传使用 `-a --partial --info=progress2`；镜像归档使用 `--partial --append-verify --info=progress2`，完成后核对本地与服务器 SHA256。`.tar.gz` 已压缩，不再加 `-z`；不得使用 `--delete` 删除远端历史资源、日志或备份。
- **Windows 传输**：本机未安装原生 rsync 时，使用 WSL Ubuntu 的 rsync；Windows 文件路径转换为 `/mnt/<盘符>/...`。SSH 必须复用已有本机配置，可通过 rsync 的 `-e` 指定 Git for Windows 的 `ssh.exe`；不要为传输复制私钥到仓库或放宽凭证文件权限。

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


| 容器                 | 镜像                                                                                     | 端口                    | 说明                    |
| ------------------ | -------------------------------------------------------------------------------------- | --------------------- | --------------------- |
| `sub2api`          | 人工部署 `local/sub2api-batch:<tag>`；接入后 `ghcr.io/ravicc02/sshzyu-backend@sha256:<digest>` | `127.0.0.1:8080→8080` | 主应用，**纯 Go 后端，不内嵌前端** |
| `sub2api-postgres` | `postgres:18-alpine`                                                                   | 内网 5432               | 数据库                   |
| `sub2api-redis`    | `redis:8-alpine`                                                                       | 内网 6379               | 缓存                    |


- **⚠️ 区分部署模式**：`local/` 人工镜像不能从 registry pull，继续用 save/load；接入后的定制版从自己的 GHCR 按签名清单 digest 拉取，禁止使用官方镜像或浮动 `latest` 替换定制版。源码工作流存在不等于镜像已经发布。
- `.env`（`/opt/sub2api-deploy/.env`）：数据库/Redis/JWT/账号等敏感凭证，**含值不外传**，只用路径引用。
- `data/`（挂载 `/app/data`）：`config.yaml`、日志、插件、页面；`backups/` 等历史备份**不要删**
- 健康检查：`curl http://127.0.0.1:8080/health` → `{"status":"ok"}`

### 2.2 sshzyu-ui（品牌前端，nginx 直出静态文件）

- 部署目录：`/opt/sshzyu-ui/`
  - `releases/<版本>/`：历史版本（回退保障，**不要清理**）
  - `current`：软链指向当前生效 release
  - `shared/assets/`：跨版本累积资产（nginx `/assets/` 指向这里）
  - `switch-release.sh`：切版本（改 `current` 软链 + nginx reload），**不要手动** `ln`
- **改线上 UI = 走本地构建 → 上传新 release 目录 → 切 current**（见第 5 节）。

### 2.3 关键 nginx 文件（改前先看全，结构复杂）

- `/etc/nginx/sites-enabled/sub2api`：站点主配置（UI 静态 + API 反代 + SPA 路由 + 防白屏守卫）
- `/etc/nginx/snippets/sub2api-proxy.conf`：反代参数
- `/etc/nginx/snippets/sshzyu-ui/`：UI 版本 patch 配置
- `/etc/nginx/conf.d/00-sub2api-map.conf`、`20-sshzyu-ui-map.conf`：map 定义
- **改 nginx 配置必须**：备份 → `nginx -t` → `nginx -s reload`（**不 restart**）。

---

## 3. 端到端开发部署流程

### 3.0 定制版日常发布主链（完成首次接入后）

1. 保留官方基线与本地定制的三方整合规则；每次发布递增 `backend/cmd/server/VERSION` 的 `rN` 并同步 `upstream-baseline.json`，不得复用旧版本。定制 tag 前缀为 `sshzy-v`，不用官方 `vX.Y.Z`。
2. 本地定向验证和 pre-push 守卫通过后，按用户授权提交/推送；不得擅自提交未归属的改动。GitHub 对 main 执行全量门禁，成功且版号递增才发布；失败阻断，不能伪造通过。
3. `.github/workflows/publish-custom.yml` 只发布签名 manifest、同提交 UI、精简后端镜像和执行器产物，不持生产 SSH 私钥，不重启站点。
4. 网站只更新 `ravicc02/sshzyu` / `ghcr.io/ravicc02/sshzyu-backend` 的签名定制版。官方 `Wei-Shaw/sub2api` 仅用于基线核验与提醒，不提供本站安装/回退按钮。
5. 准备阶段只下载、验签和预检；激活需要真人管理员、近期 TOTP、明确 migration 列表与停机确认。未配置执行器/信任链/权限时禁止回退旧的原地二进制更新逻辑。
6. 独立宿主机执行器通过 Unix socket 控制，不暴露公网、不挂 Docker socket 给网站。首次安装、凭证配置、权限、服务启停及生产迁移均需单独授权。
7. 首版维护窗口包含优雅退出和离线数据库备份，不能保证几秒完成。失败保留记录/备份，不自动还原数据库；人工恢复见 `deploy/updater/README.md`。

下述 3.1–3.3 保留为首次接入、人工应急和离线发布方式；上传继续只用 rsync。

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

# 5. 上传到新 release（将 <new> 替换为本次发布版本，不覆盖 current）
RELEASE="<new>"
ssh us-server "mkdir -p '/opt/sshzyu-ui/releases/$RELEASE'"
rsync -a --partial --info=progress2 ui/current/ "us-server:/opt/sshzyu-ui/releases/$RELEASE/"
```

> 上传与生效分开：校验新 release 后，按已授权的发布流程运行 `bash /opt/sshzyu-ui/switch-release.sh <new>` 切 `current`，保留旧 release 用于回退。固定 `shared/assets/fw-cachebust.js` 的更新与备份仍须按发布流程处理，不能因上传而提前切换。 前端产物 `ui/` 由构建生成，**不要手改**；只改 `frontend/` 源码再重建。

### 3.2 修改文档站（docs/）

```bash
# docs/ 直接是构建产物且无构建链，改完上传即可
rsync -a --partial --info=progress2 docs/index.html docs/content.md us-server:/opt/sshzyu-docs/
rsync -a --partial --info=progress2 docs/iamge/ us-server:/opt/sshzyu-docs/iamge/
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

# 5. 上传（统一使用 rsync，连接方式按本机 SSH 配置）
rsync --partial --append-verify --info=progress2 "/tmp/sub2api-<tag>.tar.gz" us-server:/tmp/
# 核对本地与远端归档 SHA256 一致后，才继续加载和部署

# 6. 服务器：加载镜像 + 更新 compose tag + 重建容器
ssh us-server '
docker load -i /tmp/sub2api-<tag>.tar.gz
sed -i "s|local/sub2api-batch:[^ ]*|local/sub2api-batch:<tag>|g" /opt/sub2api-deploy/docker-compose.yml
cd /opt/sub2api-deploy && docker compose up -d --no-build --force-recreate sub2api
'

# 7. 验证
ssh us-server 'docker exec sub2api /app/main --version && curl -s http://127.0.0.1:8080/health'
```

> **版本号规则**：本地定制版完整版本号由 `backend/cmd/server/VERSION` 决定，格式为 `X.Y.Z-rN`；`resolve-version.sh` 不再从本地/官方 tag 自动推断。`upstream-baseline.json` 必须与版本文件及官方稳定 Release/tag/commit 对齐。根 Dockerfile 默认构建本地 `source` 内嵌前端镜像；发布根镜像需先运行 `python scripts/upstream_release_guard.py --validate-clean-build --build-target root`，然后以 `--build-arg BUILD_TYPE=release` 及 VERSION、COMMIT、DATE 显式构建。`backend/Dockerfile` 只构建线上纯后端 `release` 镜像，发布前应使用 `--build-target backend` 验证。正式发布须核验镜像 tag、二进制输出及管理端 `build_type`。 **推送门禁**：每次向 `origin` 推送前运行 `python scripts/upstream_release_guard.py`，在当前机器执行一次 `git config core.hooksPath .githooks` 启用自动 `pre-push`。守卫读取待推送提交中的基线、核对官方最新稳定 Release 及 tag SHA；上游状态未知或有新版本时阻止 push 并向用户汇报，未经确认不得自动合并。钩子是本机机制，可被跳过；团队级强制保护已由服务端 `main` 分支保护承接（见 §3.4）。完整三方增量合并、migration 和部署边界见 `plan_docs/official-upstream-versioning-workflow.md`。 **回滚**：compose 改回旧 tag → `docker compose up -d --force-recreate`。 **定制更新回滚**：接入后的回滚使用已部署 history 中的签名版本，恢复后端、UI 和固定 bootstrap；必须先核对当前 schema 的兼容性。不能用容器内 `.backup`、官方 install.sh 或官方 Docker Hub 镜像替换定制版。

### 3.4 多人协作与提交规范

本仓库多人 + agent 协作，所有改动要可追溯、可验证。规则：

1. **改动走 PR，不直推** `main`：本地建分支（`feat/` `fix/` `docs/` `chore/` `build/`）→ push → 开 PR → `verify.yml`（后端测试 / migration / 编译 + 前端 typecheck / test / build）全绿才 merge。`main` **已启用分支保护**（仓库已公开）：要求 PR、**必需检查 `backend` 与 `frontend`** 通过，禁止 force-push 与删除分支；管理员可在紧急时绕过（`enforce_admins=false`）。**不要求审批**（`required_approving_review_count=0`）：GitHub 不允许 PR 作者批准自己的 PR，当前无第二名 reviewer，保留「1 人批准」会让每个 PR 永久 `REVIEW_REQUIRED`，故审批降为 0，门禁只由 CI 承担。**必需检查的 context 必须用 `verify.yml` 的 job 名（`backend`/`frontend`），不要写成 `verify / backend` 这种「workflow 名 / job 名」形式**——写错会让 required check 永远停在 expected 未满足状态，导致**所有 PR 永久无法合并**（症状：CI 全绿且已批准，`mergeStateStatus` 却一直是 `BLOCKED`）。另外注意**批准（Approve）≠ 合并（Merge）**：批准只是审查通过、不改动 `main`；合并才把改动写入 `main`（并可能触发发布）。本仓库当前不设审批，正常路径只有「CI 全绿 → Merge」一步；日后若启用审批才需区分这两步。
2. **提交信息用 conventional commits**：`feat(scope): …` / `fix(scope): …` / `docs: …` / `chore: …`，一次提交只做一件事。
3. **发版前先同步**：`git fetch origin && git pull --rebase origin main`，再本地验证。发版人各自独立，但**先 rebase 再递增版本**，避免 `main` 分叉。
4. `VERSION` **是唯一发版入口**：递增 `backend/cmd/server/VERSION` 的 `rN` 并同步 `upstream-baseline.json`，放在**最后一步**；只有版本严格递增 `publish-custom.yml` 才真正发布，未递增则自动跳过（不会误发）。两人都可发版，但同一时刻只由一人递增版本。**递增只触发「构建 + 签名 + 发布 Release 产物」，不会部署到线上**——`publish-custom.yml` 只 `gh release create`（说明文字明确 "Site activation remains a manual operation"），站点切换**始终是人工操作**（管理员登录控制台 + 近期 TOTP，或宿主机受控 CLI）。**不要把「递增版本 / 合并 PR」误当成「部署上线」**。
5. **启用本机守卫**：每台机器执行一次 `git config core.hooksPath .githooks`，push 前自动核对官方上游 Release 状态；未启用则该守卫不生效。
6. **禁止**：直接 push 到 `main`、force-push 已发布分支、复用已发布的版本号、用官方镜像替换定制版。

> 详细三方合并、migration 安全与发布边界见 `plan_docs/official-upstream-versioning-workflow.md`。

### 3.5 定制更新与磁盘运维要点

> 首次接入后定制更新由宿主机执行器 `sshzy-updater` 驱动。以下要点均已源码核验，供日常运维与排障。

**更新门控（**`/etc/sshzy-updater/config.json`**）**

- 两个部署策略开关默认 `false`，**网页不提供修改入口**，只能以 root 编辑该文件：
  - `activation_enabled`：是否启用"网站定制更新入口"（决定后端 `CanUpdate`）。
  - `payment_callbacks_reviewed`：支付回调是否已复核。**本站确有支付业务**，开启前须确认支付回调具备重试与本地幂等，不得仅为解锁而翻转。
- **改后必须** `systemctl restart sshzy-updater`：daemon 在启动时读取并缓存该配置，不重启不生效。
- **门是设计内的，不是故障**：`payment_callbacks_reviewed=false` 时 activate 直接返回 `PAYMENT_CALLBACK_REVIEW_REQUIRED`。此时应如实告知"更新器有效开关仍关闭"，不得绕过。
- **两条激活路径的门不同**：网站控制台路径需真人管理员 + 近期 TOTP（step-up；TOTP 服务不可用会返回 `503 STEP_UP_UNAVAILABLE`）；宿主机 socket 路径（root + control token）不经过控制台 TOTP。二者勿混淆。
- **该标记无审计、无时效**：纯布尔，无"谁在何时复核"的记录；翻转前后建议补一次真复核。

**维护屏障与监控盲区**

- `maintenance.json` 属组必须是执行器的 `socket_gid`（容器内 `app` 的 gid，通常 1000）。属组错误会让后端读不到 → `DeploymentMaintenance` **fail-closed 全站 503**。
- `/health` **是放行路径**，维护态下仍返回 200——"健康检查全绿、业务 API 全 503"会静默发生。**必须另加一条对业务端点（如** `/v1/...`**）的外部拨测**。

**磁盘运维（发布前先看** `/` **使用率）**

- activate 的 backup 环节会写 `pg_dump -Fc`（未压缩，可能数百 MB）。空间紧张时先清理再发布。
- **可清理（非红线）**：`/tmp` 的旧镜像 tar 与脚本残留、`/var/cache/apt`（`apt-get clean`）、悬空镜像（`docker image prune -f`）、确认无用的历史 fat 镜像。
- **绝不能清（红线，见 §5）**：`backups/`、`ui/releases/`、`data/`、`*.bak*`，以及**当前与上一版的回滚镜像**（`local/sub2api-batch:<上一版>`、`ghcr…@<上一版 digest>`）。
- **不要用** `docker image prune -a`：会连同回滚镜像一起删除；必须按 ID 定向 `docker rmi`。
- 上传用的镜像 tar 部署后即从 `/tmp` 清掉，避免堆积（曾累积到 2.7G）。

---

## 4. 本地开发栈

本地 `docker-compose.yml` 一键启动（**与线上无关**，端口语义不同）。


| 端口               | 容器                                           | 角色                                          |
| ---------------- | -------------------------------------------- | ------------------------------------------- |
| `127.0.0.1:8080` | `sshzyu-ui-local`（nginx）                     | 本地站点唯一入口：UI + `/docs/` + `/image/` + API 反代 |
| `127.0.0.1:8081` | `sub2api-dev`（`local/sshzyu:dev`）            | 后端，仅本机可见                                    |
| 无外部端口            | `sub2api-postgres-dev` / `sub2api-redis-dev` | 数据库 / 缓存                                    |


```bash
# 启动（首次或改镜像后）
cp .env.example .env && docker compose up -d --build

# 日常：前端改动直接用 rebuild.sh，nginx bind-mount 直读，刷新即可
bash scripts/rebuild.sh

# 健康检查
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/health
```

- **⚠️ 本地** `8080` **= 站点 nginx；线上** `8080` **= sub2api 容器**。端口语义务必区分。
- `rebuild.sh` 是纯本地构建（`install → build → fw-cachebust → 装配`），**无** `git reset --hard` **风险**，可放心跑。

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
- **把** `8080` **暴露公网**：只绑 `127.0.0.1`，经 nginx 反代。
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

&nbsp;