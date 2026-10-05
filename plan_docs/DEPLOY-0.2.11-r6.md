# 线上部署文档 · sub2api 0.2.11-r6（官方 v0.2.11 + 本地定制）

> 目标服务器：**us-server**
> 交付物：后端镜像 `local/sub2api-batch:0.2.11-r6` + 前端 UI
> 说明：本文档是把「本地已合并并验证通过」的版本部署到线上 us-server 的完整操作手册。**支持无损升级（不丢线上数据）。**

---

## 0. 背景与版本对照

**版本对照：**

| 环境 | 版本 | HEAD | 工作区 | 说明 |
|---|---|---|---|---|
| 线上（部署前） | `0.2.10-r5` | — | 运行中 healthy | compose tag `0.2.10-r5`，前端 `current→20260929-ui-r1` |
| 本地 | `0.2.11-r6` | `1198128df` | 洁净 | 已合官方 v0.2.11，含 lottery 动态档位、content_moderation 增强、渠道推理倍率、联盟幂等、批量图片 BATJ |

> **部署实际使用的构建身份（已执行并核实）：**
> - VERSION = `0.2.11-r6`
> - COMMIT = `1198128dfeae5a85f7526c4f457bad063d76bb25`（**完整 40 位 SHA**，即本次部署时的 HEAD）
> - 线上镜像 ID = `2e8e7ae5422a`，`--version` 输出：`Sub2API 0.2.11-r6 (commit: 1198128dfeae5a85f7526c4f457bad063d76bb25, built: 2026-10-01T02:59:09Z)`
>
> 注意：`562710777` 是 UI 产物归档提交，**不是**后端镜像的构建身份。构建前务必用 `git rev-parse HEAD` 取当前 HEAD 的完整 SHA。

**本地 0.2.11-r6 的迁移文件总数：294 个 `.sql` 文件（线上 0.2.10-r5 已有 289 条记录）。**

**迁移增量（线上→本地，共 3 个改名对齐 + 5 个新增）：**

### 改名对齐组（线上记录 → 本地文件名，checksum 已逐字节验证一致）

| 线上旧记录（DB） | 本地新文件名 | TrimSpace+SHA256 checksum（前16位） | 结论 |
|---|---|---|---|
| `234_batch_image_idempotency_unique` | `241_batch_image_idempotency_unique.sql` | `a5a889db5bca79c3` | ✅ 一致 |
| `239_lottery` | `242_lottery.sql` | `7a0eacdeb81fcabf` | ✅ 一致 |
| `240_lottery_tier` | `243_lottery_tier.sql` | `0455bf4ac6460303` | ✅ 一致 |

> **验证方法：** 执行器用 `strings.TrimSpace(content) → sha256.Sum256 → hex.EncodeToString`。上述 checksum 已用同一算法从本地文件计算并与线上 `schema_migrations` 记录逐组比对确认。

> **幂等性备注：** 三个改名对齐的迁移内容均为完全幂等（`IF NOT EXISTS` / `CREATE TABLE IF NOT EXISTS` / 带条件 `DO $$` 块）。即使不做对齐直接部署，重执行也能安全通过。改名对齐是为了保持 `schema_migrations` 记录干净，非核心依赖。

### 新增迁移（线上无记录，启动时自动执行）

| 文件名 | 内容摘要 | 操作类型 | 幂等 |
|---|---|---|---|
| `238b_content_moderation_engine_meta.sql` | `content_moderation_logs` 增加 `engine_meta JSONB` | `ADD COLUMN IF NOT EXISTS` | ✅ |
| `239_channel_reasoning_effort_multipliers.sql` | `channel_model_pricing` 增加 `reasoning_effort_multipliers JSONB` + 老字段迁移 | 条件 ALTER + 条件 UPDATE | ✅ |
| `240_affiliate_ledger_operation_id.sql` | `user_affiliate_ledger` 增加 `operation_id` + 唯一索引 | `ADD COLUMN IF NOT EXISTS` + `CREATE UNIQUE INDEX IF NOT EXISTS` | ✅ |
| `244_lottery_activity_tier_config.sql` | `lottery_activities` 增加 `tier_mode`/`tier_thresholds` | `ADD COLUMN IF NOT EXISTS` × 2 | ✅ |
| `245_lottery_dynamic_tiers.sql` | `lottery_activities` 增加 `tier_definitions` | `ADD COLUMN IF NOT EXISTS` | ✅ |

> 所有新增迁移均已核实为幂等操作，引用目标表（`content_moderation_logs`、`channel_model_pricing`、`user_affiliate_ledger`、`lottery_activities`）均存在于线上（由更早期的迁移创建）。

**与旧方案 `0.2.10-r6` 相比变化：**
- 新增 `238b_content_moderation_engine_meta`、`239_channel_reasoning_effort_multipliers`、`240_affiliate_ledger_operation_id`、`245_lottery_dynamic_tiers` 共 4 个新迁移（旧方案只有 1 个）
- 改名对齐的 `234→241` 因版本变更仍保持不变（内容相同，线上市 `0.2.10-r5` 仍用旧编号）

---

## 1. 前置条件与安全红线

- 操作期间保持 **SSH 连接可用**，建议在维护窗口执行。
- **不得**：改回 SSH 密码登录、重启 nginx/fail2ban/cron 等非 sub2api 服务、删除任何日志/数据/备份。
- 关于迁移表：主机制是 `schema_migrations`；`atlas_schema_revisions` 仅用于无历史 baseline，本次无需处理（可选只读确认）。
- **前端与后端为两个独立交付物**，均可独立回滚。
- **本机 githooks 门禁（`pre-push`）不会阻塞部署流程。** 线上构建使用 `docker build`，不产生 `git push`。
- 本机 VERSION = `0.2.11-r6`，HEAD = `562710777`。**正式构建前需确保工作区洁净**（当前已验证为洁净）。

---

## 2. 部署流程图

```mermaid
flowchart TD
    A[Step 0 全库备份] --> B[Step 1 改名对齐<br/>UPDATE schema_migrations<br/>234→241 / 239→242 / 240→243]
    B --> C[Step 2 构建后端镜像<br/>backend/Dockerfile → 0.2.11-r6]
    C --> D[Step 3 save + 上传 + load]
    D --> E[Step 4 切 compose tag + force-recreate]
    E --> F[Step 5 后端验证<br/>health/version/全量8项迁移/路由]
    E -.-> G[Step 5a 回滚预案<br/>切回旧 tag 即恢复]
    C --> H[Step 6 UI 构建与发布<br/>果壳打包 → upload → switch-release]
    H --> I[Step 7 前端验证]
    I -.-> J[Step 7a 回滚预案<br/>switch-release 回旧版本]
```

---

## 3. 后端部署

### Step 0 — 全库备份（必须，不可跳过）

> **备份必须放在非易失目录 `/opt/sub2api-deploy/backups/`，不要用 `/tmp`**（可能被清理，且重启丢失）。

```bash
# 备份 schema_migrations（用于改名回退）
ssh us-server 'docker exec sub2api-postgres pg_dump -U sub2api -d sub2api -t schema_migrations > /opt/sub2api-deploy/backups/schema_migrations_pre-0.2.11-r6.sql && echo "migrations backed up"'

# 全库备份（压缩，141M 级；未压缩体积会大得多）
ssh us-server 'docker exec sub2api-postgres pg_dump -U sub2api -d sub2api | gzip -6 > /opt/sub2api-deploy/backups/sub2api_dump_pre-0.2.11-r6.sql.gz && echo "full db dumped"'

# 确认备份文件存在且非空（并校验 gzip 完整性）
ssh us-server 'ls -lh /opt/sub2api-deploy/backups/*pre-0.2.11-r6* && gzip -t /opt/sub2api-deploy/backups/sub2api_dump_pre-0.2.11-r6.sql.gz && echo "gzip OK"'
```

> 本次部署实际生成的备份（已核实存在）：
> - `/opt/sub2api-deploy/backups/schema_migrations_pre-0.2.11-r6.sql`（39K，349 行）
> - `/opt/sub2api-deploy/backups/sub2api_dump_pre-0.2.11-r6.sql.gz`（141M）

### Step 1 — 旧库迁移记录改名对齐

> 只更新 `filename`，**不动 checksum**。此步在旧镜像仍运行时执行，不影响运行中的 r5 容器。
> **如不想改名对齐**（改用重执行幂等方式），可直接跳过此步到 Step 2。本方案偏好改名对齐以保持记录干净。

```bash
# 改名对齐
ssh us-server 'docker exec sub2api-postgres psql -U sub2api -d sub2api -c "
UPDATE schema_migrations SET filename = '\''241_batch_image_idempotency_unique.sql'\'' WHERE filename = '\''234_batch_image_idempotency_unique.sql'\'';
UPDATE schema_migrations SET filename = '\''242_lottery.sql'\''              WHERE filename = '\''239_lottery.sql'\'';
UPDATE schema_migrations SET filename = '\''243_lottery_tier.sql'\''        WHERE filename = '\''240_lottery_tier.sql'\'';"'

# 确认改名结果：应有 241/242/243 三行，且 checksum 与上面表格一致
ssh us-server 'docker exec sub2api-postgres psql -U sub2api -d sub2api -c "SELECT filename, substring(checksum::text,1,16) AS cksum_short FROM schema_migrations WHERE filename LIKE '\''24%'\'' ORDER BY filename"'
# 期望输出（只展示 24x）：
# 241_batch_image_idempotency_unique.sql | a5a889db5bca79c3
# 242_lottery.sql                        | 7a0eacdeb81fcabf
# 243_lottery_tier.sql                   | 0455bf4ac6460303
# （尚无 244/245 —— 它们是新增迁移，启动时自动执行）

# 验证旧编号已不存在
ssh us-server 'docker exec sub2api-postgres psql -U sub2api -d sub2api -c "SELECT filename FROM schema_migrations WHERE filename IN ('\''234_batch_image_idempotency_unique.sql'\'','\''239_lottery.sql'\'','\''240_lottery_tier.sql'\'')"'
# 期望：空结果（0 rows）
```

### Step 2 — 本地构建后端镜像

**前置检查：工作区必须洁净，VERSION 必须符合预期。**

```bash
# 核验
cd /f/中转站运营/sshzy
git status --porcelain    # 应为空（或仅无关文件）
cat backend/cmd/server/VERSION   # 应为 0.2.11-r6
git rev-parse HEAD               # 取完整 40 位 SHA，供下方 COMMIT 使用
```

> ⚠️ **`COMMIT` 必须是完整 40 位 SHA。** `backend/Dockerfile` 会校验构建身份：`VERSION` 必须等于 `scripts/resolve-version.sh` 的输出，`COMMIT` 必须是 40 位十六进制。传 9 位短 SHA（如 `562710777`）会**直接构建失败**。
>
> ⚠️ **`backend/Dockerfile` 不生成 `/etc/build_type`。** 验证版本请只用 `--version`，不要 `cat /etc/build_type`（该文件在纯后端镜像中不存在）。

```bash
# 取完整 SHA（示例值，实际以命令输出为准）
COMMIT_SHA=$(git rev-parse HEAD)   # 本次部署时为 1198128dfeae5a85f7526c4f457bad063d76bb25

# 正式构建（backend/Dockerfile — 纯 Go 构建，不含前端）
docker build \
  --build-arg VERSION=0.2.11-r6 \
  --build-arg COMMIT=$COMMIT_SHA \
  --build-arg DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t local/sub2api-batch:0.2.11-r6 \
  backend/

# 验证版本注入（Git Bash 下需 MSYS_NO_PATHCONV=1 防止 /app/main 被转成 Windows 路径）
MSYS_NO_PATHCONV=1 docker run --rm local/sub2api-batch:0.2.11-r6 /app/main --version
# 期望：Sub2API 0.2.11-r6 (commit: <40位SHA>, built: ...)
```

> 构建前可选但推荐：`python scripts/upstream_release_guard.py --validate-clean-build --build-target backend`（检查工作区洁净与上游基线）。
>
> 当前环境为 Windows，`$(date -u +%Y-%m-%dT%H:%M:%SZ)` 在 Git Bash 中可用。若用 PowerShell，用 `Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ'`。

### Step 3 — 导出、上传、加载

> ⚠️ **Windows 上不要用管道式 `docker save ... | gzip`。** 实测该方式在 Docker Desktop for Windows 下会长时间输出 0 字节（管道在宿主与 WSL 之间不可靠）。改用 `docker save -o` 直接写 tar 文件，再用 gzip 压缩；且**不要导出到 C 盘**（Docker Desktop 数据盘与临时目录易被占满导致 daemon 卡死）。

```bash
cd /f/中转站运营/sshzy

# 1) 导出到空间充足的盘（本次用 F 盘；tar 约 3.4G，压缩后约 737M）
docker save -o /f/deploy-tmp/sub2api-0.2.11-r6.tar local/sub2api-batch:0.2.11-r6
gzip -6 /f/deploy-tmp/sub2api-0.2.11-r6.tar

# 2) 上传（先传压缩包，避免传输 3.4G 原始 tar）
scp /f/deploy-tmp/sub2api-0.2.11-r6.tar.gz us-server:/opt/sub2api-deploy/sub2api-0.2.11-r6.tar

# 3) 校验传输完整性（本地与线上 md5 必须一致）
md5sum /f/deploy-tmp/sub2api-0.2.11-r6.tar.gz
ssh us-server 'md5sum /opt/sub2api-deploy/sub2api-0.2.11-r6.tar'

# 4) 加载
ssh us-server 'docker load -i /opt/sub2api-deploy/sub2api-0.2.11-r6.tar'

# 5) 确认加载成功并校验版本
ssh us-server 'docker images local/sub2api-batch:0.2.11-r6 --format "{{.Repository}}:{{.Tag}} {{.ID}} ({{.Size}})"'
ssh us-server 'docker run --rm local/sub2api-batch:0.2.11-r6 /app/main --version'
```

> **加载完成后务必删除线上 tar 包**（`rm /opt/sub2api-deploy/sub2api-0.2.11-r6.tar`，737M）：镜像已进入 docker 本地存储，tar 仅是传输媒介，回滚不需要它。

### Step 4 — 切换 compose tag 并重建容器

```bash
ssh us-server '
sed -i "s|local/sub2api-batch:[^ ]*|local/sub2api-batch:0.2.11-r6|g" /opt/sub2api-deploy/docker-compose.yml
cd /opt/sub2api-deploy && docker compose up -d --no-build --force-recreate sub2api'
```

> 若旧 compose 中 service 名不同（如 `sub2api`），按实际加快通过 `docker compose ps` 确认。

### Step 5 — 后端验证

```bash
ssh us-server '
# 5a. 版本
echo "=== VERSION ===" && docker exec sub2api /app/main --version

# 5b. 健康检查
echo "=== HEALTH ===" && curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8080/health

# 5c. 迁移落地检查（8 项）
echo "=== 24x MIGRATIONS ===" && docker exec sub2api-postgres psql -U sub2api -d sub2api -t -c "SELECT filename FROM schema_migrations WHERE filename LIKE '\''24%'\'' ORDER BY filename"
# 期望：241 / 242 / 243（改名跳过）+ 244 / 245（新增已执行）
# 同时检查 238b / 239_channel / 240_affiliate
echo "=== 23x NEW ===" && docker exec sub2api-postgres psql -U sub2api -d sub2api -t -c "SELECT filename FROM schema_migrations WHERE filename ~ '\''^23[89]b|^240_affiliate'\'' ORDER BY filename"
# 期望：238b_content_moderation_engine_meta / 239_channel_reasoning_effort_multipliers / 240_affiliate_ledger_operation_id

# 5d. 迁移日志确认（无错误）
echo "=== MIGRATION LOG ===" && docker logs sub2api 2>&1 | grep -iE "migration|lottery|affiliate|moderation|tier" | tail -10

# 5e. 关键路由注册
echo "=== ROUTES ===" && docker exec sub2api /app/main --help 2>/dev/null | grep -iE "lottery|affiliate|moderation" | head -5
'
```

> ⚠️ **不要用 `grep -iE "ERROR|FATAL|panic"` 判断真实错误。** 本项目 `security_audit` 日志含 `"error_code": ""` 字段，会被 `-i error` 匹配，但级别是 `INFO`，属误报。判真实错误须匹配**独立日志级别字段**：
>
> ```bash
> # 正确的真实错误计数（本次部署结果为 0）
> ssh us-server 'docker logs sub2api 2>&1 | grep -cE "	(ERROR|FATAL|PANIC)	|level=(ERROR|FATAL|PANIC)" || echo 0'
> ```
>
> 本次部署核实：真实 ERROR/FATAL/PANIC = **0**；先前显示的 10 条均为 `security_audit` 的 `error_code` 空值误匹配。

---

## 4. 前端部署

前端与后端独立，源码在 `frontend/`，构建产物装配到 `ui/`。

### Step 6 — 本地构建前端

```bash
cd /f/中转站运营/sshzy
bash scripts/rebuild.sh
# 此脚本依次执行：install → build → fw-cachebust → 装配到 ui/current/
# 构建产物验证：
ls -la ui/current/index.html ui/current/assets/
```

### Step 7 — 上传新 release 并切换

> ⚠️ **前端不是「全部内联到 current」。** nginx 的实际规则（`/etc/nginx/sites-enabled/sub2api`）是：
> - `location = /assets/fw-cachebust.js` → 从 **current** 读取
> - `location /assets/` → 从 **shared** 读取
>
> `switch-release.sh` 用 `cp -rn`（**不覆盖**）把新 release 的 assets 补入 `shared/assets`。因此**必须做同名冲突检查**：若本地资产与线上 `shared` 存在「同名但内容不同」，`cp -rn` 会保留旧内容，而 `fw-cachebust.js` 又 `import('/assets/index-<hash>.js')`，可能加载到旧入口导致白屏。
>
> 哈希文件名保证正常情况下同名即同内容；唯一例外是固定文件名的 `fw-cachebust.js`——但它从 current 读取，由新 release 提供，不构成风险。

```bash
cd /f/中转站运营/sshzy

# 1. 打包（打包 current 内容：index.html + logo.svg + assets）
rm -f /f/deploy-tmp/ui-0.2.11-r6.tar.gz
cd ui/current && tar czf /f/deploy-tmp/ui-0.2.11-r6.tar.gz index.html logo.svg assets && cd /f/中转站运营/sshzy

# 2. 服务器建 release 目录并上传解压
ssh us-server 'mkdir -p /opt/sshzyu-ui/releases/0.2.11-r6'
scp /f/deploy-tmp/ui-0.2.11-r6.tar.gz us-server:/opt/sshzyu-ui/releases/0.2.11-r6.tar.gz
ssh us-server 'cd /opt/sshzyu-ui/releases/0.2.11-r6 && tar xzf ../0.2.11-r6.tar.gz && rm -f ../0.2.11-r6.tar.gz && ls index.html assets | head'

# 3. 校验上传完整性（本地与线上 index.html md5 必须一致）
md5sum ui/current/index.html
ssh us-server 'md5sum /opt/sshzyu-ui/releases/0.2.11-r6/index.html'

# 4. 【关键】同名冲突检查：本地资产中与线上 shared 同名但内容不同的文件
#    本次结果：191 个同名文件内容全一致；唯一"同名不同内容"的是 fw-cachebust.js（从 current 读取，无风险）
ssh us-server 'ls /opt/sshzyu-ui/shared/assets/ | wc -l'

# 5. 切换 current 软链（用脚本、不手动 ln；脚本自带校验与失败回滚）
ssh us-server 'bash /opt/sshzyu-ui/switch-release.sh 0.2.11-r6'
# 期望输出：current -> 0.2.11-r6 / VERIFY_OK
```

### Step 8 — 前端验证

```bash
# 1) 首页与 SPA 路由
curl -sk --max-time 15 -o /dev/null -w "GET /            -> %{http_code}\n" https://sshzyu.com/
curl -sk --max-time 15 -o /dev/null -w "GET /manage/lottery -> %{http_code}\n" https://sshzyu.com/manage/lottery

# 2) 首页入口标识（应为新构建的 fw-cachebust.js?v=...）
curl -sk --max-time 15 https://sshzyu.com/ | grep -o 'fw-cachebust.js?v=[^"]*' | head -1

# 3) 【关键】入口 chunk 必须能从 shared 取到 200 —— 这是白屏的直接判据
ENTRY=$(curl -sk --max-time 15 https://sshzyu.com/assets/fw-cachebust.js | grep -o 'index-[A-Za-z0-9_-]*\.js' | head -1)
echo "entry=$ENTRY"
curl -sk --max-time 15 -o /dev/null -w "$ENTRY -> %{http_code} (%{size_download} bytes)\n" "https://sshzyu.com/assets/$ENTRY"

# 4) 浏览器访问确认：页面加载正常，lottery 管理页可用，无白屏/JS 报错
```

> 本次部署实测：首页 200；`fw-cachebust.js` 从 current 提供新版 `v=BTNOU4q8`；入口 `index-BTNOU4q8.js` 从 shared 返回 200（188064 bytes）；`switch-release.sh` 输出 `VERIFY_OK`。

---

## 5. 回滚方案

### 后端回滚（代码层）

```bash
# 切回旧镜像 tag
ssh us-server '
sed -i "s|local/sub2api-batch:[^ ]*|local/sub2api-batch:0.2.10-r5|g" /opt/sub2api-deploy/docker-compose.yml
cd /opt/sub2api-deploy && docker compose up -d --no-build --force-recreate sub2api'
```

> 新增的 5 个迁移（238b/239_channel/240_affiliate/244/245）全是 ADD COLUMN 或 CREATE INDEX，旧代码不认识这些列和索引，不会主动使用，**无副作用**。

> **迁移记录恢复**：若因改名对齐出问题需要恢复，用 Step 0 的备份（位于非易失目录）：
> ```bash
> ssh us-server 'docker exec -i sub2api-postgres psql -U sub2api -d sub2api < /opt/sub2api-deploy/backups/schema_migrations_pre-0.2.11-r6.sql'
> ```
>
> ⚠️ 该备份是**整表 dump**，直接恢复前先确认不会与后续新增的 238b/239_channel/240_affiliate/244/245 记录冲突（必要时先 `TRUNCATE schema_migrations` 或改为逐行 UPDATE 回退）。

### 前端回滚

```bash
ssh us-server 'bash /opt/sshzyu-ui/switch-release.sh 20260929-ui-r1'
```

---

## 6. 风险与注意事项

1. **改名对齐必须在部署前、旧镜像仍在跑时做**——只影响启动扫描，不影响运行中的容器。若跳过后直接启动新镜像，3 个改名迁移会因文件名不同而作为新迁移重执行（内容全部幂等，安全通过），但 `schema_migrations` 会保留旧编号记录成为死数据。

2. **checksum 一致性是本方案的前提。** 本文档已用执行器真实算法（`TrimSpace+SHA256`）逐文件复核。若后续改动过这三个迁移文件的内容，必须重算。

3. **前端与后端建议一并发布，但可分别回滚**。后端只动 `backend/`，前端只动 `frontend/`。

4. **线上 `8080` = sub2api 容器，本地 `8080` = 站点 nginx**—端口语义不同，勿混用。

5. **`234_channel_max_reasoning_effort_multiplier.sql` 未改名**（线上和本地同名）。执行器会自动检查该迁移的 checksum 匹配。若线上旧记录与该文件的本地 checksum 不一致，启动会被拒绝。部署前可在 Step 1 可选验证：
   ```bash
   ssh us-server 'docker exec sub2api-postgres psql -U sub2api -d sub2api -t -c "SELECT checksum FROM schema_migrations WHERE filename = '\''234_channel_max_reasoning_effort_multiplier.sql'\''"'
   ```
   然后本地计算：
   ```bash
   cd /f/中转站运营/sshzy && python -c "import hashlib; print(hashlib.sha256(open('backend/migrations/234_channel_max_reasoning_effort_multiplier.sql','rb').read().decode('utf-8').strip().encode('utf-8')).hexdigest())"
   ```
   若不一致，需先评估是否需要兼容规则或确认该迁移是否曾被修改。

6. **本机 githooks (`pre-push`) 门禁**不会影响本次部署（`docker build` 不触发 `git push`）。但若计划在部署后推送仓库，需先提交变更并通过门禁检查。

7. **前端 current 与 shared 的映射边界：** nginx 对 `/assets/fw-cachebust.js` 走 **current**，其余 `/assets/*` 走 **shared**；`switch-release.sh` 用 `cp -rn` 把新 release 的 assets **补入（不覆盖）** `shared`。因此「只上传 current」是不够的——**切换脚本必须执行**，且切换前要做同名冲突检查。本次部署中 581 个本地资产有 191 个与线上 shared 同名且内容一致，唯一同名不同内容的是 `fw-cachebust.js`（从 current 读，无风险）。

---

## 6.1 本次部署实际结果（2026-10-01 已执行）

| 环节 | 结果 |
|---|---|
| Step 0 备份 | `schema_migrations_pre-0.2.11-r6.sql`（39K）+ `sub2api_dump_pre-0.2.11-r6.sql.gz`（141M） |
| Step 1 改名对齐 | `234→241` / `239→242` / `240→243`，checksum 逐字节一致，旧编号已消失 |
| Step 2 构建 | `local/sub2api-batch:0.2.11-r6`，COMMIT `1198128dfeae5a85f7526c4f457bad063d76bb25` |
| Step 3 上传加载 | 线上镜像 ID `2e8e7ae5422a`，`--version` 正确 |
| Step 4 切换重建 | compose tag → `0.2.11-r6`，容器 healthy |
| Step 5 后端验证 | 8 项迁移全部落地；真实 ERROR/FATAL/PANIC = **0** |
| Step 6-8 前端 | `current → 0.2.11-r6`，`v=BTNOU4q8`，外网首页与入口 chunk 均 200 |

**已知遗留（未完成，需人工处理）：**

1. **本机 `docker_data.vhdx` 仍占 70G，未压缩归还宿主。** WSL2 虚拟磁盘不自动收缩；Windows 11 Home 无 `Optimize-VHD`，`wsl --manage --set-sparse` 对 Docker 自定义 VHD 不生效。需**管理员权限**执行：
   ```
   diskpart
   select vdisk file="C:\Users\ASUS\AppData\Local\Docker\wsl\disk\docker_data.vhdx"
   attach vdisk readonly
   compact vdisk
   detach vdisk
   ```
2. **线上 tar 包** `/opt/sub2api-deploy/sub2api-0.2.11-r6.tar`（737M）为传输媒介，镜像已 load，可删。
3. **本机 C 盘仍偏紧**（本次清理后约 9–14G）。根因是上述 vhdx。

---

## 7. 附录：迁移增量完整检视清单

部署前在线上执行以下只读查询，确认所有前提：

```bash
# 改名对齐前提确认（3 个旧记录存在）
ssh us-server 'docker exec sub2api-postgres psql -U sub2api -d sub2api -c "SELECT filename, substring(checksum::text,1,16) FROM schema_migrations WHERE filename IN ('\''234_batch_image_idempotency_unique.sql'\'','\''239_lottery.sql'\'','\''240_lottery_tier.sql'\'','\''234_channel_max_reasoning_effort_multiplier.sql'\'') ORDER BY filename"'

# 确认新增迁移对应的表存在（可选）
ssh us-server 'docker exec sub2api-postgres psql -U sub2api -d sub2api -c "SELECT to_regclass('\''public.content_moderation_logs'\'') AS tbl, to_regclass('\''public.channel_model_pricing'\'') AS ch, to_regclass('\''public.user_affiliate_ledger'\'') AS aff, to_regclass('\''public.lottery_activities'\'') AS lott"'
```

---

*本文档由本地合并验证结果整理，供 us-server 部署参考。执行前请以实际线上状态为准复核。*