# AGENTS.md — 中转站运营 工作区说明

> 本文件是 NewMax / Claude Code 在本工作区工作时必须遵循的说明。每次在本工作区做操作前，先读一遍。目标是让 agent 知道：连哪台服务器、管什么服务、能干什么、**绝对不能干什么**。

---

## 1. 本工作区是什么

「中转站运营」：管理**美国服务器**上运行的中转站服务。

| 服务          | 角色                                              |
| ----------- | ----------------------------------------------- |
| **sub2api** | 中转站**主服务**（AI API Gateway 后端），Docker Compose 部署 |
| **sshzyu-ui** | 对外**品牌前端**（静态站点 + 发布管理），nginx 直接出静态文件 |

- 对外域名：`sshzyu.com` / `www.sshzyu.com`
- **mpay 支付服务已于 2026-09-27 弃用**（已切第三方支付），不迁移、不在新机部署，仅旧机残留（见第 5 节）。

工作区本地几乎没有业务文件，绝大部分运维工作在**远程服务器**上，通过 SSH 执行。

> ⚠️ **本工作区只服务一台服务器：us-server（美国，`64.83.2.153`，hostname `RainYun-LFS4goAS`）。2026-09-28 本地 SSH 别名已由历史旧名 `hk-server` 改为 `us-server`（服务器在美国）；旧文档、旧脚本或对话记录里出现的 `hk-server` 均指本机。** 旧机 `hk-server-old`（`45.192.105.96`）仅观察期保留、只读参考；其它任何服务器（含 `Ravi-server`）**不在本工作区职责范围**，不要对它们做任何运维操作。

### 1.1 迁移背景（2026-09-27）

- 2026-09-27 已完成 us-server 整机迁移：旧机 `45.192.105.96` → 新机 `64.83.2.153`。
- 迁移范围：sub2api 全套（镜像/data/postgres/redis/.env/compose）+ sshzyu-ui + nginx 配置 + 证书。mpay **整块不迁**（已弃用）。
- DNS 已切至新机，证书已续期（含 `www.sshzyu.com` SAN）。
- 完整方案与步骤见 `docs/us-server-迁移方案-20260927.md`。
- **旧机保留 3-7 天观察期**：容器已停、数据完整，异常时可切 DNS 回退。观察期满后随旧机清理。

---

## 2. 服务器与 SSH 连接（本机 `~/.ssh/config` 已配好别名，免密一键连接）

| 别名                | IP              | 用户名  | 认证钥匙                              | 说明                                                    |
| ----------------- | --------------- | ---- | --------------------------------- | ----------------------------------------------------- |
| **us-server**     | `64.83.2.153`   | root | `~/.ssh/new-server_ed25519`       | **新机**（hostname `RainYun-LFS4goAS`，Ubuntu 24.04），sub2api + sshzyu-ui 都在这台 |
| hk-server-old     | `45.192.105.96` | root | `~/.ssh/hk-2h4g_ed25519`          | 旧机（hostname `HKE278301`），观察期保留；frp 穿透仍在此机        |

**连接命令：**

- `ssh us-server` → 进新机（本工作区主目标）
- `ssh hk-server-old` → 进旧机（仅观察期回退/取数据用；确认稳定后此别名随旧机一起清理）

本机已配置为**密钥认证、免密登录**（`IdentitiesOnly yes` 强制用指定钥匙）。

### ❌ 边界声明：不服务 Ravi-server

`Ravi-server`（**199.68.217.212**）本机 `~/.ssh/config` 有别名、也能连，但**它不承载本工作区任何服务，不属于本工作区**。

- 本工作区任务中**不要**对 `Ravi-server` 做任何操作（排查、改动、重启、升级等）。
- 若用户**明确指定**要动它，属跨出本工作区的操作，需先单独确认用途再行动。
- 其私钥 `199.68.217.212_id_ed25519` 不属于本工作区管理对象。

---

## 3. sub2api 服务（中转站主服务，us-server 新机）

### 3.1 部署形态

Docker Compose 部署，compose 文件在：

```
/opt/sub2api-deploy/docker-compose.yml
```

三容器（`docker compose ps` 可查，均 healthy）：

| 容器名                | 镜像                            | 端口                        | 用途  |
| ------------------ | ----------------------------- | ------------------------- | --- |
| `sub2api`          | `local/sub2api-batch:0.2.7-r2` | 宿主机 `127.0.0.1:8080→8080` | 主应用 |
| `sub2api-postgres` | `postgres:18-alpine`          | 内网 5432（不暴露宿主机）           | 数据库 |
| `sub2api-redis`    | `redis:8-alpine`              | 内网 6379（不暴露宿主机）           | 缓存  |

**⚠️ 镜像是本地构建**（`local/sub2api-batch`，193MB，**无 registry**）：升级/回滚只能靠 `docker save/load` 搬运镜像 tar，或在本机构建后传上去。**服务器上没有 `weishaw/sub2api:latest` 官方镜像**，不要 `docker compose pull`（会拉不到 `local/` 镜像）。

### 3.2 目录与数据

- 部署目录：`/opt/sub2api-deploy/`
  - `.env`（root 600）—— 配置（数据库/Redis/JWT/TOTP/OAuth 等）**含敏感凭证，禁止外泄**
  - `docker-compose.yml`（及多个历史 `.bak-*` 备份）
  - `data/`（挂载容器 `/app/data`）：`config.yaml`、`model_pricing.json`、`logs/`、`plugins/`、`pages/`
  - `postgres_data/`、`redis_data/`
  - `backups/` 及根下两个 2026-09-21 的备份包（`.tar.gz` / `.sql`）—— **迁移前旧数据备份，不要删**
- 凭证文件：`/root/sub2api-credentials.txt`（root，600）—— 记录后台 email / admin 密码 / Redis 密码。**只引用路径，不把值带进对话或工作区文件。**

### 3.3 对外入口（nginx + sshzyu-ui 前端）

**⚠️ 线上不是简单的「nginx 反代容器」——品牌前端是独立静态站点，nginx 按路径分流：**

```
https://sshzyu.com (443)
  ├─ /          → /opt/sshzyu-ui/current/ 静态文件（SSHZYU 品牌 UI，SPA 路由）
  ├─ /assets/   → /opt/sshzyu-ui/shared/assets/（共享资源，防白屏）
  ├─ /docs/     → /opt/sshzyu-docs/
  └─ /api /v1 /health /models 等 → proxy_pass http://127.0.0.1:8080（sub2api 容器）
```

- **前端源码 ≠ 后端内嵌前端**：sub2api Go 二进制内嵌的前端（旧版 UI）与线上品牌 UI 是两回事。改线上 UI = 改 `sshzyu-frontend` fork 分支 → 构建静态产物 → 发布到 `/opt/sshzyu-ui/releases/` → `switch-release.sh` 切 current（见第 4 节）。
- nginx 关键文件（**结构比一般站点复杂，改前先看全**）：
  - `/etc/nginx/sites-enabled/sub2api` —— 站点主配置（UI 静态 + API 反代 + SPA 路由 + 防白屏守卫）
  - `/etc/nginx/snippets/sub2api-proxy.conf` —— 反代参数
  - `/etc/nginx/snippets/sshzyu-ui/` —— UI 版本相关 patch 配置
  - `/etc/nginx/conf.d/00-sub2api-map.conf`、`20-sshzyu-ui-map.conf` —— map 定义
  - `99-sub2api-performance.conf.bak` —— 已停用备份
- HTTPS 443 由 **Certbot** 管理（`/etc/letsencrypt/live/sshzyu.com/`），证书 SAN 含 `sshzyu.com` + `www.sshzyu.com`，**2026-12-25 到期**，自动续期任务已配。改 DNS 后需手动跑一次 `certbot renew --nginx` 验证。
- 新机还装有 fail2ban（曾把旧机 IP 封过，SSH 连不上先查 `fail2ban-client status sshd`）。

### 3.4 健康检查

- `curl http://127.0.0.1:8080/health` 应返回 `HTTP 200`（`{"status":"ok"}`）
- 看三容器：`cd /opt/sub2api-deploy && docker compose ps`

### 3.5 版本溯源（**无本地 git 仓库**）

- sub2api 服务器上**没有源码、没有 `.git`**，无法 `git log/status/diff`。
- 当前镜像 `local/sub2api-batch:0.2.7-r2` 为本地构建产物；版本号规则 `<上游版本>-r<n>。
- 源码与构建链在本工作区本地（见第 6 节 `_build/` 与 `sub2api/`）。
- 想确认上游更新 → 看 `github.com/Wei-Shaw/sub2api` 的 releases/tags，**不能**在服务器上 `git pull`。

---

## 4. sshzyu-ui（品牌前端，us-server 新机）

- 目录：`/opt/sshzyu-ui/`
  - `releases/` —— 历史版本（`20260908-ui-r1` … `20260922-ui-r10`，共 10 个）
  - `current` —— 软链 → `releases/20260922-ui-r10`（线上生效版本）
  - `shared/assets/` —— 跨版本共享资源（nginx `/assets/` 指向这里）
  - `switch-release.sh` —— 切版本脚本；`patch-workspace-embed.sh`、`patches/`、`backups/`、`README.md`
- **切版本 = 改 `current` 软链 + nginx reload**，用现成的 `switch-release.sh`，不要手动 ln。
- 历史版本 `releases/` 是回退保障，**不要清理**。
- UI 源码与构建链在本地工作区（第 6 节），发布流程：本地构建 → 传新 release 目录 → 切 current。

---

## 5. mpay（已弃用，仅旧机残留）

**2026-09-27 起弃用**：支付已切第三方平台，mpay 整块**未迁移**到新机。

- 新机上**没有** mpay 的任何东西（无容器、无目录、无证书）。
- 旧机 `hk-server-old` 上原样保留：`/opt/mpay-deploy/`（含 `.env`、`.pem` 密钥、git 仓库）、`mpay_mysql-data` volume、`pay.sshzyu.com` 证书、容器（已停）。观察期后随旧机一起清理。
- 若发现第三方支付仍有依赖 mpay 的地方（如回调 URL 指向 `pay.sshzyu.com`），**旧机数据就是退路**——先恢复旧机再排查，不要试图在新机重建 mpay。
- 旧机上的 mpay 凭证（`.env`、`*.pem`、`*credentials`）**仍是敏感物**：不打印、不入库、不外传。

---

## 6. 本地开发与部署（本机 Windows Docker，与 us-server 线上无关）

> ⚠️ **端口语义与线上不同，动手前先看本节**：线上新机的 `127.0.0.1:8080` 是 sub2api 容器；**本机的 `127.0.0.1:8080` 是 sshzyu-ui-local（nginx 站点）**。历史上曾因混淆端口把本地拓扑改乱过。

### 6.1 拓扑（2026-09-22 定稿，唯一正确形态）

| 端口 | 容器 | 角色 | 归属编排 |
| --- | --- | --- | --- |
| `127.0.0.1:8080` | `sshzyu-ui-local`（nginx） | **本地站点唯一入口**：sshzyu 新 UI 静态资源 + `/api /v1 /health /models` 等反代 | `sshzyu-local-site/docker-compose.yml` |
| `127.0.0.1:8081` | `sub2api-dev`（`local/sub2api-batch:<tag>`） | sub2api 后端，仅本机可见（nginx 上游 + 调试口） | `sub2api/deploy/docker-compose.dev.yml` |
| 无外部端口 | `sub2api-postgres-dev` / `sub2api-redis-dev` | 数据库 / 缓存（bind mount 在宿主机，重建不丢数据） | 同上 |

- nginx 反代 sub2api 走**容器名** `proxy_pass http://sub2api-dev:8080`（同一 docker 网络），与宿主机端口映射无关。
- 两栈共用外部网络 `sub2api-network`，由 dev.yml 显式命名（`networks.name:`）并创建；nginx 以 external 挂入。**没有这行网络名就是两个网络，nginx 连不上后端。**

### 6.2 文件 / 目录职责（改前对照）

| 路径 | 作用 | 红线 |
| --- | --- | --- |
| `sub2api/deploy/docker-compose.dev.yml` | **本地栈唯一的 sub2api 编排**（sub2api 服务 `image: local/sub2api-batch:${SUB2API_TAG:-dev}`，可直接跑预构建镜像，也可 `--build`） | 禁止再复制出第二份编排；禁止删 `networks.name: sub2api-network` |
| `sub2api/deploy/.env` | `SERVER_PORT=8081`（后端宿主机端口）、`SUB2API_TAG`（镜像版本，切版本只改这一行） | `SERVER_PORT` 不得改为 8080（8080 属于站点 nginx，冲突） |
| `sub2api/deploy/docker-compose.yml / local.yml / standalone.yml` | 项目自带的其他场景编排，**本地栈不用** | 不要对本地栈使用 |
| `sshzyu-local-site/docker-compose.yml` | 本地站点 nginx，端口 `127.0.0.1:8080:80`（容器内监听 80） | 映射格式必须是 `8080:80`，写成 `8080:8080` 容器内没人监听 |
| `sshzyu-local-site/nginx/` | 站点配置，与线上同构（`listen 80`，无 TLS） | `proxy_pass` 指容器名，不改 |
| `sshzyu-local-site/sshzyu-ui/current|shared/` | 新 UI 静态产物，bind-mount 直读，**重建后无需重启容器** | 产物由构建链生成，不要手改 |
| `sshzyu-local-site/rebuild.sh` | 一键重建站点：fetch → checkout 本地 fork 分支 → 前端 build → 装配 | 它会 `git reset --hard`：功能改动必须**先 commit 到本地分支**再跑 |
| `_build/sshzyu-frontend/` | 新 UI 构建工作副本，分支 `fork/codex/sshzyu-frontend`（本地含功能提交 `fc8773476`，未 push） | 上游 GitHub 无此提交；push 需用户拍板 |
| `_build/online-reference/` | 线上产物参考快照，只读比对用 | 不改 |

### 6.3 前端代码与两个 UI 的关系（最容易搞错的一点）

- sub2api 内嵌前端（Go embed，r2 镜像里的）**≠ 线上/本地站点用的 sshzyu 新 UI**。
- 新 UI 源码 = sub2api 仓库的 **`fork/codex/sshzyu-frontend` 分支**，由 `rebuild.sh` 独立构建成静态产物。**改 UI 功能：改 fork 分支 → rebuild.sh → 刷新 8080**；只改内嵌前端并打进镜像，用户在 8080 上看不到。
- 本地栈后端用 `SUB2API_TAG` 钉预构建镜像（构建时必须传 `--build-arg VERSION=<tag> COMMIT=<sha>`，否则版本自报回退成 `commit: docker`）。管理端新参数（如 `api_key_provider`）需要后端与前端**同时**具备才生效。

### 6.4 日常操作速查

```bash
# 切后端版本：改 .env 的 SUB2API_TAG 后执行
cd sub2api/deploy && docker compose -f docker-compose.dev.yml up -d --no-build sub2api

# 重建本地站点前端（fork 分支源码 → 静态产物 → 装配，nginx 直读即生效）
bash sshzyu-local-site/rebuild.sh

# 健康检查
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/health   # 站点→反代→后端
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8081/health   # 后端直连
```

### 6.5 历史教训（本节存在的原因）

- 曾同时存在三套编排（dev + image-pin overlay + 复制的 sshzyu.yml）抢同一组容器名，反复互相顶掉、端口漂移、网络错位（`deploy_sub2api-network` vs `sub2api-network`）。2026-09-22 收敛为：**sub2api 只有 dev.yml 一个编排 + 站点只有 sshzyu-local-site 一个编排**。归档/复制文件已删除，不要再造。
- `rebuild.sh` 引用的 `build-online-equivalent.mjs` 三处补丁与当前源码已脱节（对最新产物会 FAIL），构建时若报「产物结构变化需人工复核」，先核对 `build/` 下脚本再决定，不要盲目重跑。
- **2026-09-27 迁移教训**：线上 UI 是独立静态站点而非镜像内嵌，最初迁移时漏掉了 `/opt/sshzyu-ui/` 与配套 nginx 配置，导致用户看到旧 UI。以后任何「整机/整站迁移」先对照本文件第 3.3/4 节的完整入口清单。

---

## 7. 本工作区要做什么（授权范围内的职责）

1. **服务监控**：sub2api 三容器状态、日志、`ss` 端口、`curl /health`；sshzyu-ui current 指向与站点可达性。
2. **配置维护**：sub2api 上游/定价/限流；nginx 站点配置（改前备份、`nginx -t` 通过再 reload）。
3. **备份**：sub2api 的 `data/`、`postgres_data/`、`redis_data/`。
4. **升级**：sub2api 用本地构建镜像 + `docker save/load` 升级（先备份）；UI 走 releases 发布流程。
5. **记录**：在工作区 `docs/` 写运维记录（含关键版本、变更理由）。

---

## 8. 能力边界 —— 能干 / 不能干

### ✅ 可以做（且鼓励）

- 一切**只读检查**：`docker compose ps`、`docker logs`、`ss -tlnp`、`systemctl is-active`、`cat` 配置（脱敏后）、`curl /health`。
- 管理 **sub2api 与 sshzyu-ui**：重启对应容器、看日志、切 UI release。**不动 postgres/redis 数据、不 migrate、不清库。**
- 备份数据目录。

### ❌ 绝对不能做（除非用户逐项明确授权）

- **改回 SSH 密码登录**：2026-09-28 起新机已**禁用密码认证、仅允许公钥登录**（`/etc/ssh/sshd_config.d/00-hardening.conf`：`PasswordAuthentication no`、`KbdInteractiveAuthentication no`、`PermitRootLogin prohibit-password`；已实测密钥登录正常、纯密码认证被拒）。**不得恢复密码登录**；不要动主 `sshd_config`、`50-cloud-init.conf` 及该 hardening 文件（回滚仅限用户明确要求）；不要重复追加 authorized_keys。
- **重启/停止/改动 nginx、fail2ban、cron、或任何非 sub2api 的服务**（nginx 配置修改 = 先备份 → `nginx -t` → reload，不 restart）。
- **操作数据库数据**：清库、migrate、删表、改 superuser。
- **把** `8080` **暴露到公网**：只绑 `127.0.0.1`，经 nginx 反代，这是安全设计，不要改成 `0.0.0.0` 直暴露。
- **删除任何日志、数据、备份文件**（`data/logs/`、`postgres_data/`、`redis_data/`、`backups/`、`/opt/sshzyu-ui/releases/`、部署目录下的 `*.bak*`）。
- **操作旧机 `hk-server-old` 上的任何写操作**（观察期它只读；frp 通道与 mpay 残留都在上面，动了会失去退路）。
- **在没把握的情况下做会把服务器锁死的操作**。改任何核心网络/SSH 配置前必须：先备份 → 校验语法 → 留有另一条可登录途径。

### 🔒 安全红线（最重要）

- **绝不把** sub2api 的 `.env`、`config.yaml`、`/root/sub2api-credentials.txt` 内容，或旧机 mpay 的 `.env`、`*.pem`、`*credentials`、数据库备份内容**打印到对话或写进工作区文件。** 只引用路径，需要用值时在服务器上看，不外传。
- 任何命令输出若含敏感值，先脱敏再给用户看。
- 本机（Windows）权限等级 = 美国服务器**安全上限**。本机私钥 `new-server_ed25519`（新机）与 `hk-2h4g_ed25519`（旧机）一旦泄漏，对应整台服务器都可能被控制。**私钥已收紧为仅当前用户可读，不要放宽。**

---

## 9. 常用速查

```bash
# 进服务器（本工作区主目标）
ssh us-server

# sub2api
ssh us-server 'cd /opt/sub2api-deploy && docker compose ps'
ssh us-server 'curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/health'
ssh us-server 'docker logs --tail 100 sub2api'
ssh us-server 'cat /etc/nginx/sites-enabled/sub2api'

# sshzyu-ui
ssh us-server 'readlink /opt/sshzyu-ui/current && ls /opt/sshzyu-ui/releases/'
ssh us-server 'cat /opt/sshzyu-ui/README.md'

# 站点可达性（DNS/证书/全链路）
curl -s -o /dev/null -w "%{http_code}\n" https://sshzyu.com/health

# 旧机（仅观察期只读参考）
ssh hk-server-old 'hostname'
```

> 提交任何改动前，先把改动会影响的边界说清楚。**拿不准就停下来问，不要凭猜测动手。** 本工作区只服务 us-server（sub2api + sshzyu-ui），其它服务器（含 Ravi-server、hk-server-old 写操作）一律不动。
