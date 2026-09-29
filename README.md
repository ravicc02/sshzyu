# SSHZYU — 统一本地栈

把原先分散的三个仓库（前端站点栈、后端源码、文档站点）合并到一个仓库里，
**不含任何远程业务镜像**，克隆后用一条命令即可在本地跑起完整服务。

## 快速启动

```bash
cp .env.example .env        # 编辑 POSTGRES_PASSWORD（必需）
docker compose up -d --build
```

首次启动会：

1. 用仓库根的 `Dockerfile` **从本地源码**构建后端镜像（前端也一并构建并嵌入）；
2. 启动 PostgreSQL / Redis，后端自动执行数据库迁移；
3. 按 `.env` 里的 `ADMIN_EMAIL` / `ADMIN_PASSWORD` 创建管理员账号。

启动完成后访问：

| 地址 | 内容 |
|------|------|
| <http://localhost:8080> | 前端主界面（nginx 托管） |
| <http://localhost:8080/docs/> | API 接入文档 |
| <http://localhost:8081/health> | 后端健康检查（直连，调试用） |

## 服务与端口

| 服务 | 容器名 | 宿主端口 | 镜像 | 说明 |
|------|--------|---------|------|------|
| sshzyu-ui | `sshzyu-ui-local` | `127.0.0.1:8080` | `nginx:alpine` | 前端 + 文档站 + API 反代 |
| sub2api | `sub2api-dev` | `127.0.0.1:8081` | `local/sshzyu:<SUB2API_TAG>` | 后端（**本地构建**） |
| postgres | `sub2api-postgres-dev` | 内部 | `postgres:18-alpine` | 数据库 |
| redis | `sub2api-redis-dev` | 内部 | `redis:8-alpine` | 缓存 |

nginx 通过共享网络 `sub2api-network` 反代到 `sub2api-dev:8080`。

## 目录结构

```
sshzy/
├── docker-compose.yml     ← 统一编排（前端 + 后端 + 数据库 + 缓存）
├── Dockerfile             ← 后端多阶段构建（构建前端 → 嵌前端 → 编译 Go）
├── .env.example           ← 环境变量模板（cp 成 .env）
├── Makefile               ← 开发辅助（构建 / 测试）
│
├── backend/               ← Go 后端源码
│   ├── cmd/ internal/ pkg/ ent/ migrations/ scripts/ resources/
│   └── go.mod  go.sum  Dockerfile(快速版)
│
├── frontend/              ← 前端源码（Vue + Vite + TS）
│   ├── src/ public/ package.json pnpm-lock.yaml
│   └── vite.config.ts  tailwind.config.js
│
├── ui/                    ← 已构建的前端产物（nginx 直接托管，无需构建）
│   ├── current/           ← 当前版本
│   ├── shared/            ← 共享静态资源
│   └── releases/          ← 历史版本
│
├── docs/                  ← 文档与合规
│   ├── index.html         ← 文档站点（nginx 托管 /docs/）
│   ├── content.md
│   ├── iamge/
│   └── legal/             ← 合规文档（前端构建时读取）
│
├── nginx/                 ← nginx 配置
│   ├── default.conf
│   ├── 00-sub2api-map.conf  20-sshzyu-ui-map.conf
│   └── sub2api-proxy.conf
│
├── deploy/                ← 部署辅助
│   ├── docker-entrypoint.sh  (被根 Dockerfile 引用)
│   ├── .env.example          (完整环境变量参考，23KB)
│   └── DOCKER.md  Makefile
│
├── records/operational/   ← 运营发布记录 & 排障日志
└── scripts/rebuild.sh     ← 本地 UI 重建脚本
```

## 镜像策略

| 镜像 | 来源 | 说明 |
|------|------|------|
| `local/sshzyu:<tag>` | **本地构建** | 业务镜像，从本仓库源码构建，不依赖任何远程业务镜像 |
| `nginx:alpine` | 官方 | 反向代理（通用基础设施） |
| `postgres:18-alpine` | 官方 | 数据库 |
| `redis:8-alpine` | 官方 | 缓存 |

原则：**只对自有业务代码做本地构建，通用基础设施使用官方镜像**。
这样既能保证"不依赖官方业务镜像、可自包含启动"，又不必重造 postgres/redis 这类轮子。

## 数据持久化

所有运行时数据都落在 `deploy/` 下的宿主机目录（不是 Docker 命名卷）：

| 目录 | 内容 |
|------|------|
| `deploy/postgres_data/` | PostgreSQL 数据 |
| `deploy/redis_data/` | Redis 数据 |
| `deploy/data/` | 后端应用数据（config.yaml 等） |

### 从旧的分散目录迁移数据

如果你已有运行中的数据，想延续到新仓库：

```bash
# 停掉旧栈
docker compose -f <旧路径>/sub2api/deploy/docker-compose.dev.yml down
docker compose -f <旧路径>/sshzyu-local-site/docker-compose.yml down

# 复制数据
cp -r <旧路径>/sub2api/deploy/postgres_data  deploy/
cp -r <旧路径>/sub2api/deploy/redis_data     deploy/
cp -r <旧路径>/sub2api/deploy/data           deploy/

# 用新仓库启动
docker compose up -d --build
```

若不需要历史数据，直接启动即可（会初始化一套干净数据库）。

## 重新构建

```bash
# 只重建后端镜像
docker compose build sub2api
docker compose up -d sub2api

# 重建前端产物（ui/ 里的静态文件）
cd frontend
pnpm install
pnpm run build
# 将 dist/ 内容同步到 ../ui/current/
```

## 备份数据库

```bash
docker compose exec postgres pg_dump -U sub2api sub2api > backup.sql
```

## 合并来源

| 原仓库 | 现位置 | 远程 |
|--------|--------|------|
| `sshzyu-local-site` | `nginx/` `ui/` `scripts/` | `ravicc02/sshzyu-local-site` |
| `sshzyu-sub2api` | `backend/` `frontend/` `docs/legal/` `deploy/` `Dockerfile` | `ravicc02/sshzyu-sub2api` |
| `sshzyu-docs` | `docs/`（站点部分） | `ravicc02/sshzyu-docs` |
| 运营记录 | `records/operational/` | （原先无远程） |

> 注：`/image/` 路由指向可选的外部目录（生图工作室产物，不属于本仓库）。
> 缺失时该路由返回 404，不影响其它功能；如需启用，在 `.env` 里设置
> `IMAGE_PLAYGROUND_DIST` 指向对应 `dist` 目录。
