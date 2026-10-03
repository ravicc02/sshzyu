# sshzy 定制版发布与网站更新改造方案

> 日期：2026-10-03
>
> 状态：已据此实施工作区源码改造，目标 `0.2.13-r3`。GitHub secrets、GHCR 实际发布、生产执行器安装和真实数据库迁移尚未执行，仍需单独授权。实现与首次接入边界见 `deploy/updater/README.md`。
>
> 目标：本地开发并推送代码后，GitHub 提前构建定制成品；管理员在网站选择版本、确认风险后更新，无须日常从电脑上传大镜像。
>
> 适用：`ravicc02/sshzyu` 统一仓库，以及 `us-server` 的 Docker 后端 + nginx 独立静态 UI。

## 1. 三个必须打通的环节

| 环节 | 改造目标 | 不允许的行为 |
| --- | --- | --- |
| 本地开发 | 同时管理官方基线和定制功能，可靠追溯每次发布 | 用官方源码覆盖定制、重写历史 migration |
| GitHub 构建 | 从确定的提交生成配套后端镜像、UI 和签名发布清单 | 上传本地脏产物、产物不完整就标记可更新 |
| 网站更新 | 检查和部署自己的发布源，展示官方新版本但不直接安装 | 网站按钮下载官方二进制、只更新后端遗漏 UI |

**最终使用流程：**

```text
本地开发、测试、处理官方更新
  → 确定本地版本与基线
  → 提交并推送到 GitHub
  → CI 检查，通过后构建并发布定制成品
  → 网站提示“发现 sshzy 定制版更新”
  → 管理员点击“准备更新”，不影响当前站点
  → 管理员确认备份、迁移、短暂中断及目标版本
  → 独立部署执行器更新后端和 UI
  → 健康、版本、资源及关键功能验收
```

推送代码不等于上线；“官方出了新版本”也不等于“我们的定制版已经适配并发布”。

## 2. 当前实现与缺口

以下为本次读取工作区源码确认的现状，不代表已核验 GitHub Actions 的可用额度、仓库可见性或服务器的未来配置。

| 位置 | 现状 | 需要改变 |
| --- | --- | --- |
| `upstream-baseline.json`、`backend/cmd/server/VERSION` | 官方基线与 `X.Y.Z-rN` 已落地；当前记录为 `v0.2.13` / `0.2.13-r1` | 保留并成为 CI 与发布清单的共同输入 |
| `scripts/upstream_release_guard.py`、`.githooks/` | 校验官方 Release/tag SHA、版本与干净构建；本机有 pre-push 守卫 | GitHub 必须再次执行，不能只依赖本机钩子 |
| `.github/` | 有 `SECURITY.md`，尚无构建发布工作流 | 新增验证与发布工作流 |
| `backend/Dockerfile` | 单阶段构建，运行镜像保留编译环境和源码 | 多阶段构建，生成线上纯后端精简镜像 |
| `scripts/rebuild.sh` | 前端构建、cache-bust、装配本地 `ui/` | 保留本地用途；CI 直接打包本次干净 dist |
| `scripts/build-online-equivalent.mjs` | 生成固定 bootstrap 和版本化入口 | CI 必须执行，不能只运行 `pnpm build` |
| `backend/internal/service/update_service.go` | 固定官方仓库 `Wei-Shaw/sub2api`；下载并替换运行中二进制；版本比较忽略 `-rN` | 定制源与官方提示分离；Docker 更新不原地换二进制 |
| `backend/internal/repository/github_release_service.go` | API 请求支持 `UPDATE_GITHUB_TOKEN`；普通下载方法未携带私有资产 API 认证 | 保留官方只读查询；为独立执行器新增按仓库/资产 ID 读取私有发布文件的受控客户端 |
| `backend/internal/handler/admin/system_handler.go` | 更新、回滚为长请求；已有系统操作锁与幂等支持 | 返回持久化操作 ID，以异步任务查询进度 |
| `frontend/src/components/common/VersionBadge.vue` | 固定官方仓库与 `weishaw/sub2api`，包含官方安装/回滚命令 | 保留 UI 风格，改为自己的发布源、进度与回滚记录 |
| `frontend/src/api/admin/system.ts`、`frontend/src/stores/app.ts` | 单一版本来源，更新请求等待至 15 分钟 | 分开运行版本、定制更新、官方提醒和部署操作状态 |

线上采用纯后端容器和独立 UI，容器里替换二进制会脱离镜像身份，并且容器重建后可能回到原镜像内容。因此，**只把现有 `githubRepo` 改成自己的仓库并不构成完整适配**。

## 3. 总体架构与明确选型

### 3.1 发布源

| 用途 | 固定来源 |
| --- | --- |
| 官方基线核验、官方新版本提示 | `Wei-Shaw/sub2api` |
| 定制 GitHub Release、UI 包、发布清单 | `ravicc02/sshzyu` |
| 定制后端镜像 | 拟采用 `ghcr.io/ravicc02/sshzyu-backend` |
| 生产目标 | 仅 `us-server`，不接受网页传入任意目标主机 |

GitHub Actions 可将 Docker 镜像发布到 GHCR；正式部署使用清单中的 digest，而不是可变化的 `latest` 标签，依据官方资料 [S1][S2][S5]。

默认建议私有镜像；源码仓库与包的实际可见性、账号权限及计费额度在实施前检查，不在本次擅自更改。后端镜像与 UI 必须来自同一提交。

### 3.2 为什么需要独立部署执行器

网站后端将在更新过程中被替换，不能依靠其自身 goroutine 持续执行部署、回滚和写入最终状态。

建议新增**宿主机独立更新服务 `sshzy-updater`**：

```text
管理员浏览器
  → 网站 admin API（鉴权、风险确认）
  → 本机 Unix socket（受限协议，不暴露公网）
  → 宿主机 sshzy-updater
      ├─ 获取、验签发布清单
      ├─ 拉取固定 digest 镜像、下载 UI
      ├─ 备份、校验、更新指定容器
      ├─ 验证并切换 UI
      └─ 持久化进度、审计与回滚记录
```

约束：

- 不给网站容器挂载 Docker socket，不以 `privileged` 方式运行网站。
- 执行器独立受 systemd 管理；首次安装和相关 Compose/socket 挂载配置需要单独授权。
- 只监听 Unix socket，例如 `/run/sshzy-updater/control.sock`，目录预先由宿主机创建，网站只挂载控制目录，不挂载部署目录或 Docker socket。
- 按固定 UID/GID、socket 权限及对端身份校验限制客户端；如增加控制凭证，独立存放于服务器，不返回前端或写入仓库。
- socket 只提供固定来源的 `check_releases` 等只读查询，以及结构化的 `prepare`、`activate`、`status`、`cancel-before-activation`、`rollback`；不接受 shell、任意路径、任意镜像、任意 Compose 参数。
- 网站容器只做鉴权代理，Docker 权限仅由独立执行器持有。该能力仍属于高权限更新入口，必须审计和做限流，不能视为普通 API。
- 执行器自身更新不纳入第一版网站一键更新；使用单独、已批准的维护流程。
- 第一阶段保留 SSH 手动执行同一协议的受控 CLI，作为网站失联时的恢复通道；CI 不持有生产 SSH 私钥。

状态保存在 `/opt/sub2api-deploy/updater/state/`，不依赖即将重建的容器文件系统；凭证与原始敏感输出另存 root 私有路径。管理员只能获得脱敏状态。

## 4. 本地开发：定制与官方源适配

### 4.1 保留现有版本治理

- 官方版本固定为已核验的稳定 Release/tag/commit，不以官方浮动 `main` 作为正式基线。
- 本地发布继续使用 `X.Y.Z-rN`；同一官方基线下，`rN` 递增；新基线从 `r1` 开始。
- 定制 Git tag 使用 `sshzy-vX.Y.Z-rN`，避免占用官方 `vX.Y.Z` 标签命名。
- 一次定制发布包括后端、UI、migration 集合和部署契约；即使只改 UI，也发布新的整体修订号，第一版不引入分离的 UI 版本序列。
- `VERSION`、基线记录、Git tag、镜像身份、二进制版本、UI build-info 与发布清单必须一致。
- 已发布版本不重新打包替换；同版本不同 SHA 直接拒绝。需要修改时递增 `rN`。

版本比较使用数值元组 `(X, Y, Z, N)`，不能沿用当前剥离 `-rN` 的比较，也不能把 `r10` 与 `r9` 作为字符串比较。官方提醒单独按 `(X, Y, Z)` 比较。

### 4.2 本地开发步骤

1. 开发前读取 `AGENTS.md` 与当前基线，检查工作区归属；不清理他人未提交文件。
2. 在功能分支开发、验证，并记录受影响的定制功能。
3. 上游守卫检查官方稳定 Release；发现新版本时停止，先由用户决定是否整合。
4. 整合时采用原官方基线、本地定制、目标官方版本的三方增量比较，不整仓覆盖。
5. 更新 Ent/schema/API、前端类型、依赖锁文件及定制回归测试。
6. 更新 `VERSION` 和基线记录，合并到 `main`，推送前执行现有守卫。
7. GitHub 对该提交独立验证并发布；本地 `ui/` 的累积产物不能充当 CI 发布输入。

新增建议：

- `customizations.json`：版本化的定制保留清单，列出模块、相关源码、回归入口和上游重叠范围，不保存运营数据。
- 至少覆盖品牌布局、模型规则、额度/计费、抽奖、OpenAI/Gemini 批量生图、总任务与 ZIP 下载。
- 实施时更新 `plan_docs/official-upstream-versioning-workflow.md` 的历史状态说明，保留现有治理原则。

### 4.3 上游更新门禁

继续坚持“有新官方 Release 或状态未知，不自动合并、不自动忽略、不自动发版”。

CI 拉取源码后显式配置并核验 `upstream`，完整获取所需引用，执行基线、tag SHA 与干净源码检查；不能假定 GitHub checkout 已有本机 remote 或 hooks。

如果未来需要“暂不整合官方新版本，但发布紧急定制修复”，须先补充经过用户批准的、绑定目标官方 tag/SHA、发布提交、理由和期限的一次性例外机制，并在发布清单展示。第一版不设置全局跳过守卫开关。

官方版本在构建期间变化：重新核验发布准入；无法确认时不发布为 ready，而不是悄悄换官方基线。

### 4.4 Migration 契约

- 历史文件名、内容与已应用 checksum 不改、不重编号，不把当前 `241_*` 共存问题通过重命名掩盖。
- 对比上一已发布定制版的完整 migration 集合；修改或移除历史 migration 默认阻断发布。
- 清单记录每个文件的执行顺序、runner checksum、新增迁移的风险、是否非事务、旧应用回退兼容条件。
- 当前 runner 使用 `SHA256(strings.TrimSpace(SQL内容))`；清单生成器必须与 Go runner 的 Unicode trim/编码语义一致。原始文件 SHA256 和 runner checksum 如都保存，要分别命名，不能混用。
- 保持 LF；CI 通过隔离测试库验证“旧 schema + 旧 ledger → 新版本”，不只验证空库。
- TypeSafe 等约束变化需要含旧平台值的合成数据；批量任务迁移需要含历史任务的合成数据。禁止把真实用户/账号数据库上传 GitHub。
- 每个待执行迁移均需人工审阅风险说明；SQL 关键字扫描只能辅助分类，不能证明“无损”。
- 后端启动会执行 migration，所以生产授权必须发生在重建新后端之前。

## 5. GitHub：检查、构建和发布

### 5.1 两条工作流

拟新增：

| 工作流 | 触发与职责 | 权限 |
| --- | --- | --- |
| `.github/workflows/verify.yml` | PR / push 检查版本契约、定制回归、类型、编译、测试、迁移 | 默认 `contents: read`，不取得发布签名密钥或生产凭证 |
| `.github/workflows/publish-custom.yml` | `main` 上版本递增且检查通过后自动发布；另提供 `workflow_dispatch` 重试入口 | 按 job 最小化 `packages: write`、`contents: write`，签名 job 单独访问签名 secret |

自动发布成品不执行生产部署。普通功能分支和外部 PR 不得发布 stable；手动输入的 ref 必须解析为明确 SHA，并验证属于允许的 `main` 历史及通过所需检查。

首次实施时可以先只启用手动发布，验证完整链路后再启用 `main` 自动发布。这是过渡步骤；最终用户日常只需推送符合发版条件的提交。

### 5.2 构建流水线

```text
固定 source SHA
  → 官方基线和本地版本门禁
  → 定制/更新链路回归、编译与迁移验证
  → 多阶段纯后端镜像构建、--version 与启动 smoke
  → 前端干净构建、cache-bust、资源完整性检查
  → 发布风险说明、migration 清单与兼容性检查
  → 推送候选镜像并取得 registry digest
  → 生成 manifest、校验文件及签名
  → 上传 draft Release 的完整资产
  → 重新核对资产和镜像可读取
  → 发布 Release，网站才可发现更新
```

要求：

- 固定同一 SHA 构建，不在不同 job 使用变化的 `main`；测试与构建使用独立干净 checkout。
- Go 版本读取 `backend/go.mod`，Node/pnpm 从约定构建配置获取；依赖使用已有 lock 文件。
- 正式注入 `VERSION`、`COMMIT`、UTC `DATE`、`BuildType=release`，检查二进制与 OCI labels。
- 运行 `pnpm --dir frontend build` 和 `node scripts/build-online-equivalent.mjs`；打包本次 `backend/internal/web/dist/`，不打包累积 `ui/shared/`。
- 增加 UI `build-info.json`，保存定制版号、源码 SHA、manifest schema 与 API 兼容范围；入口、bootstrap 和资源必须一致。
- 缓存 Go modules、构建缓存和 pnpm store；缓存键纳入锁文件/工具链，不缓存密钥或用户数据。
- 第三方 Actions 固定审核过的 commit SHA；不在方案里写未经确认的“最新 action 版本”。
- 增加发布互斥；不能让较旧构建取消较新版本发布，也不能两个任务争抢同一版号。
- Release 的所有产物就绪后才公开发布；失败不能生成可升级的半成品。
- 已发布同版同 SHA 只返回原产物，不重新覆盖 digest、包或签名；失败候选的重试另有明确清理/恢复策略，不自动删除历史产物。

### 5.3 精简后端镜像

将 `backend/Dockerfile` 改成多阶段，编译阶段保留工具链，运行阶段只包含程序、必要资源、CA/时区及实际需要的数据库备份工具和运行库 [S4]。

必须验证：

- 保持线上纯后端模式，二进制入口继续兼容 `/app/main`，不把品牌 UI 偷换为后端内嵌页面。
- 验证 `CGO_ENABLED`、运行库和架构适配；不能只凭镜像能 build 就认为能启动。
- 优先非 root 网站进程；卷权限调整纳入首次部署审批，不直接递归改线上数据权限。
- 后端构建上下文使用 `backend/` 时补充 `backend/.dockerignore`；根 `.dockerignore` 不能被误认为自动保护另一个构建上下文。
- 两条构建链均排除 `.env`、本地配置、数据库卷、`records/`、测试产物和任何私钥；线上 runtime 配置通过现有卷/环境注入。
- 第一版目标架构 `linux/amd64`；后续增加其它架构时，清单明确每个架构的 digest。
- 保留根 `Dockerfile` 的本地内嵌前端用途与版本契约，并继续按相关改动做验证。
- 记录镜像及实际传输体积，与当前镜像作比较；不承诺固定压缩比例或整条升级链路“几秒完成”。

### 5.4 Release 资产与签名清单

每个 `sshzy-vX.Y.Z-rN` 发布：

| 资产 | 用途 |
| --- | --- |
| `release-manifest.json` | 机器读取的唯一发布契约 |
| `release-manifest.sig` | 对 manifest 原始字节的独立签名 |
| `sshzy-ui-X.Y.Z-rN.tar.gz` | 本次 UI release，包含 bootstrap 与 build-info |
| `checksums.txt` | UI 等文件的传输校验，不能代替来源签名 |
| `migration-report.json` | 新增迁移、checksum、风险与兼容性结果 |
| 发布说明 | 定制变更、官方基线、测试摘要与更新风险 |

manifest v1 至少包含：

| 分组 | 必需字段 |
| --- | --- |
| 身份 | `schema_version`、`distribution=sshzy`、仓库、完整版本、Git tag、source SHA、发布时间 |
| 官方来源 | 官方 tag/SHA、集成提交、门禁结果、明确批准的例外（若有） |
| 后端 | GHCR 固定名称、版本标签、registry digest、架构、二进制身份、构建类型 |
| UI | 发布资产名/大小/SHA256、源码 SHA、入口、bootstrap SHA256、build-info |
| 迁移 | 完整有序文件名/runner checksum、新增项目、风险和非事务标记、报告 SHA256 |
| 兼容性 | 最低 updater 协议、最低可直接升级版、后端/UI 契约、支持的旧 UI/API 契约范围、应用回退条件 |
| 验证 | 定向/全量测试结果、隔离迁移结果、受审阅的已知失败、可发布状态 |

最低可信链：

- 使用独立、专用于发布的 Ed25519 签名密钥；私钥只放发布 job 的受保护 secret，不使用 SSH 私钥签名。
- 宿主机执行器预置固定公钥及 key ID；签名验证对下载的 manifest 原始字节进行，失败即拒绝。
- 网站/执行器不能从同一个下载包里取得公钥然后直接相信它；公钥轮换必须走单独批准的信任更新。
- GitHub 成功状态、TLS、SHA256、镜像标签不能单独代替发布身份验证。
- 来源、签名、digest、UI SHA 和嵌入版本/SHA 不一致时 fail closed；不回退安装官方版。

CI 不能悄悄忽略当前已有的全量测试失败。实施时须先复现并治理；若用户接受暂存例外，例外要精确到测试名、原因、范围、期限和审批记录，定制/更新链路及新增失败不得豁免。未经批准的失败不发布 ready 成品。

## 6. 网站：自己的源、自己的更新入口

### 6.1 更新源与安装模式分离

拟新增 `UpdateSource`/定制发布 provider 和 `DeploymentAgentClient`，通过依赖注入接入，避免把部署命令写进现有 handler。

- 定制更新 provider：只读取 `ravicc02/sshzyu` 的合法签名 Release。
- 官方 provider：只提供新基线信息与发布说明，不提供本站安装按钮。
- 定制 provider 通过 `DeploymentAgentClient` 请求执行器的只读查询；私有发布 API 访问和下载均由执行器持有凭证执行，网站进程不取得 GHCR/私有 Release token。现有 GitHub 客户端保留官方只读职责；新的私有资产客户端在执行器侧实现。
- 安装模式独立标识为 `custom-ghcr`、`source` 或 `unknown`；不能只根据 `build_type=release` 决定可更新。
- 允许的来源、GHCR 名称、通道和公钥由部署配置固定；第一版后台不开放任意 URL、任意仓库或“切回官方安装”选项。
- source/unknown 构建、执行器未安装、身份异常或信任链缺失时展示原因，禁用更新与回滚，不降级为旧下载逻辑。
- `GetVersion` 读取本机运行身份，不依赖 GitHub 可用性；提供完整定制版、官方基线、SHA 和部署模式。

### 6.2 检查、比较与私有资产读取

- 定制 Release 列表按数值版号筛选，只接受对应前缀的稳定发布、有效 manifest 与配套资产；跳过 draft、prerelease 和不完整发布。
- 不把 GitHub `/releases/latest` 指针等同于最高定制版本，不能只比较发布日期。
- 分别返回 `custom_update` 和 `official_upstream_notice`；只给定制更新计数和更新按钮。
- 缓存键纳入来源、通道、架构、当前安装身份及清单 schema；官方与定制缓存不能共用现有单一键。
- 404、限流、凭证过期或超时标记为“无法检查”；旧缓存明确标注 stale，不显示“已经最新”。
- 正式激活前重新获取目标的固定 manifest 并验签，校验其 hash 未变，不能在用户确认后偷偷改为最新版本。

私有 Release 资产需要使用受认证的 GitHub asset API，按资产 ID 下载，支持返回文件内容或跳转 [S3]。不能仅在查版本 API 带 token，就假定 `browser_download_url` 也能读取。

认证请求只发到精确允许的 API host；跳转到允许的资产 CDN 时移除 Authorization，限制 HTTPS、主机、跳转次数、文件大小与超时，并防止 SSRF。恢复临时网络失败须重新校验文件，不能跳过签名/hash。

### 6.3 拟定 admin API

保留 `/api/v1/admin/system` 分组，复用现有管理员鉴权、限流、操作 ID 和幂等设施；以下为拟新增/改造契约：

| 接口 | 职责 |
| --- | --- |
| `GET /version` | 本机实际运行的完整身份，不触发下载 |
| `GET /check-updates` | 定制更新、官方提醒、缓存状态、执行器能力与阻断原因 |
| `GET /releases` | 可选择的定制版本、manifest hash 与发布风险 |
| `POST /updates/prepare` | 传目标 Release ID + manifest hash；返回 `202`、持久化 operation ID |
| `GET /updates/operations/:id` | 经管理员鉴权读取脱敏进度、风险、验收与错误 |
| `POST /updates/operations/:id/activate` | 提交对准备快照、迁移列表、停机提示的明确确认 |
| `POST /updates/operations/:id/cancel` | 仅允许激活前取消，保留记录和已有备份 |
| `GET /rollback-versions` | 读取本站已部署、仍可验证且兼容的定制历史，而非官方版本列表 |
| `POST /updates/rollback` | 目标部署记录 + 独立风险确认，交由同一执行器执行 |

安全与兼容规则：

- 重复点击返回同一操作或 `409`；同时最多一个执行器变更站点，锁不能仅存在于会重启的网站 Redis TTL 中。
- 关键激活需要近期管理员再认证；项目已有 2FA 时复用，不在此次顺便重写整套登录方案。
- Cookie 鉴权时落实 CSRF/Origin 校验；浏览器不能直连执行器。
- 启动激活的 `202` 响应之前，执行器已持久化接受操作；后续浏览器断连不取消已批准的部署。
- 旧 `/update`、`/rollback`、`/restart` 写接口在 `custom-ghcr` 模式明确禁用旧二进制路径；缺少目标/确认返回结构化错误，不能忽略请求体直接更新 latest。
- 容器仅重启不等于换镜像；网站不能继续使用现有 `sysutil.RestartServiceAsync()` 充当定制部署。
- 新旧网站版本与执行器协议兼容范围写入 manifest。执行器升级需求不满足时先阻断，不在运行中自我替换。

### 6.4 界面交互

改造现有 `VersionBadge.vue`，必要时将长流程独立到更新详情弹窗，保留原布局、暗色模式与中英文文案。

展示：

- 当前：`sshzy X.Y.Z-rN`、官方基线、运行 SHA、UI 版本。
- 可更新：定制版号、变更说明、发布时间、构建/测试状态。
- 官方提醒：“官方有新版本，尚未整合到本站定制版”，无安装官方按钮。
- 来源：“本站定制源”，不要继续显示官方 Docker Hub 镜像或官方 install.sh。

操作：

1. “检查更新”：只查元数据。
2. “准备更新”：下载、验签、拉镜像和预检，不修改 active Compose、UI current 或固定 bootstrap。
3. “确认更新”：展示准确目标、完整 migration 文件列表与影响、可回退范围；默认未勾选授权，确认绑定本次快照。
4. 对话提示：“更新会短暂中断本站；如正在用本站作为 AI 上游，请先切换上游。”
5. 进度显示实际阶段和字节数；不假造总百分比或固定 8 秒成功倒计时。
6. 网站短暂不可达时显示“服务正在切换，尝试恢复连接”；恢复后用 operation ID 继续查询，不能把网络错误当作更新成功。
7. 成功后刷新至对应 UI，并展示真正运行版本；失败展示是否回退成功和需要的人工动作。
8. 页面关闭/刷新后可恢复操作；未登录时重新登录再查询，不公开部署状态或日志。

## 7. 执行器：分阶段、保数据、能恢复

### 7.1 状态机

```text
queued → fetching → verifying → preflight → ready
  → awaiting_confirmation → draining → backing_up → revalidating
  → activating_backend → verifying_backend
  → activating_ui → verifying_site → completed

激活前失败 → failed（旧站点不变）
激活前取消 → cancelled（不删除备份/日志）
激活后失败 → rollback_pending → rolled_back / manual_intervention
```

每个阶段持久化时间、actor、目标版本、manifest hash、旧/新镜像身份、旧/新 UI、备份引用与脱敏错误。执行器重启后根据真实容器/文件状态恢复，不能盲目重新激活；设置每阶段超时及整个操作的可恢复检查点。

### 7.2 维护屏障与新旧版本兼容

**首版采用短暂禁止业务写入的更新窗口，不承诺零中断。**执行器在管理员授权激活后设置持久化维护状态，网站通过只读控制目录或受限 socket 读取；新容器启动后也必须遵守该状态，不能启动即恢复写入。

排空规则：

1. 拒绝新的生图/批量提交、LLM 整理、充值/余额/账号等业务写入，以及会产生用量/计费的 `/v1`、`/v1beta` 等网关请求，返回可识别的维护错误和 `Retry-After`；保留 health、版本与已鉴权更新状态查询。
2. 停止调度新的后台写任务，已有写请求和任务到达安全提交点后才报告排空完成。维护屏障必须覆盖异步 worker，不能只增加 HTTP 中间件。
3. 已提交给上游的批量任务不取消、不重复提交；确认上游任务 ID、幂等键及本地状态已持久化，暂停轮询，升级后恢复。尚未落库的上游提交结果必须先处理，不能直接杀进程。
4. 支付回调等外部写入必须验证提供方重试与本地幂等能力；没有保障的路径阻断普通一键更新，进入单独维护方案，不能假定拒绝回调就不会漏单。
5. 执行器等待业务在途计数归零和 worker 安全点确认，默认最多 5 分钟、可按批准的部署策略调整；超时则取消尚未切换的部署并安全恢复旧站点接单，不强杀忙碌任务。
6. 屏障保持期间才刷新数据库 dump、核验 ledger、重建和验收；新后端可以执行已获授权的启动 migration，但不允许恢复普通业务写入。
7. 完成新后端、新 UI 与配套契约验收后解除屏障并恢复 worker；安全回退时由旧后端确认屏障及 schema 相容后恢复。无法确认则维持维护状态，提示人工介入。

当前已打开的网页可能仍运行旧 UI，所以不能把“新后端和新 UI 来自同一 SHA”当作完整兼容证明：

- 新后端必须支持当前已部署 UI 的 API 契约以及新 UI 契约，CI 用上一已部署 UI 的合成客户端样例覆盖关键调用。
- Prepare 对照已安装 UI build-info 与目标 manifest 的兼容范围；不满足时阻断普通一键更新，不能让旧页面调用不兼容 API。
- 维护屏障内的旧页面收到明确维护提示；切换后旧客户端契约在承诺范围内继续可用，并提示刷新。未知/过旧客户端的业务写入返回 `CLIENT_UPGRADE_REQUIRED`，不得错误解析为新请求。
- 前端 API client 增加随构建固定的客户端契约标识，后端仍严格校验请求结构，该标识不作为鉴权凭证。对尚无此标识的首个旧版本，使用经审核的 legacy 契约兼容规则；不能未经验证把所有缺少标识的旧页面当成新客户端。
- 移除旧契约前至少经过一轮已审核的兼容过渡版本；破坏性 API 升级须单独维护，不靠强刷所有浏览器来代替契约验证。
- 首次人工接入面对无维护屏障/build-info 的旧版，需单独批准的排空和停机方案。不能声称新版屏障保护了尚不支持它的旧程序。

### 7.3 Prepare：不能影响当前站点

1. 下载并验签目标 manifest，确认来源、签名、公钥、架构、协议和兼容范围。
2. 拉取指定 GHCR digest，下载 UI/报告并校验大小/hash；拉镜像不启动新应用实例。
3. 安全解包到全新 release 目录，拒绝绝对路径、越界路径、符号链接、特殊文件与解压炸弹。
4. 核验 UI 入口、所有资源和 build-info；同名 shared 资源内容不同则阻断，固定 bootstrap 由激活步骤专门处理。
5. 只读核验数据库 ledger 与清单：所有已执行项必须相容；计算真正待执行列表，不只对比记录总数。
6. 检查磁盘空间，覆盖镜像拉取、UI、数据库备份和回退空间，不删除历史文件腾空间。
7. 记录当前运行镜像、Compose hash、UI 指向、bootstrap hash、ledger 快照及准备目标。
8. 验证候选 Compose 解析结果；只允许改变 `services.sub2api.image`，不改变端口、卷、网络、环境或其它容器。
9. 校验当前 UI → 目标后端和目标 UI → 目标后端两种契约组合，确认旧版具备可用的维护屏障及 worker 安全点；不具备时进入人工接入/维护流程。

### 7.4 Activate：需要显式授权

1. 执行器取得宿主机排他锁，校验目标 hash、准备状态、管理员确认与 migration 授权。
2. 发现 Compose、部署身份、目标发布或待执行迁移列表有漂移，停止并要求重新准备/确认。
3. 按 §7.2 设置维护屏障并等待业务/worker 排空确认；超时或上游结果未持久化则停止，尚不重建后端。
4. 排空后建立新的时间戳备份，不覆盖已有备份；保存数据库 dump、校验和、目录清单、配置、真实旧镜像 ID、UI 指向和旧 bootstrap。保持屏障并再次核验升级条件；敏感配置仅留服务器。
5. 首版仅支持经审核的向后兼容迁移；非兼容/破坏性或无法明确回退的迁移进入单独维护流程，不提供普通一键激活。
6. 原子更新已验证的 Compose 镜像字段，以 manifest digest 固定目标；只执行 `docker compose up -d --no-build --no-deps --force-recreate sub2api`。
7. 等待健康，验证实际二进制版本、SHA、构建类型、数据库迁移名/checksum及关键 schema；不启动第二个会同时 migration 的后端实例。
8. 后端正常后，将新 hashed assets 只增不覆盖同步到 shared，排除 `fw-cachebust.js`；随后原子替换已备份的固定 bootstrap，通过现有 `switch-release.sh` 切 UI。
9. 验证首页、bootstrap 入口、动态资源、UI build-info、HTTPS health 和不产生费用的关键接口；成功后写入完整部署记录，再解除维护屏障、恢复后台任务。
10. 保留旧镜像、release、日志、备份。全过程不重启 nginx/postgres/redis，不改 SSH，不公开 8080。

后端启动可能逐文件提交 migration，非事务迁移还可能部分执行。不能假设“更新失败就整批 SQL 自动回滚”；恢复决策依据实际 ledger/schema。

### 7.5 回滚边界

- 回退来源只允许自己的签名发布和本站已部署记录，保留已验证的旧 digest/镜像 ID；不得回退成官方原版。
- 回退也有准备、确认和互斥；复用维护屏障、业务/worker 排空与新备份步骤，校验目标应用与当前 schema 是否兼容。
- 兼容时恢复旧 Compose 镜像、重建同一后端，恢复旧 UI release **和旧固定 bootstrap**。
- `switch-release.sh` 失败会恢复软链，但不能据此认定固定 bootstrap 也恢复了；执行器须明确补偿。
- 不兼容或 schema 状态不确定时停止自动回退，进入人工维护，不盲目启动旧应用写数据库。
- 不自动删新列、修改 migration ledger 或恢复数据库 dump。恢复数据库需单独授权，并说明会丢失哪些备份之后的写入。
- `/docs/`、`/image/` 第一版不纳入自动发布，沿用单独审核与 rsync；后续如加入，必须写进 manifest 和对应回退契约。

## 8. 凭证与首次接入

| 凭证/配置 | 存放与权限 | 说明 |
| --- | --- | --- |
| Actions 发布包权限 | job 的 `GITHUB_TOKEN` | 只用于 GHCR/Release 发布，不是生产登录凭证 |
| 发布签名私钥 | GitHub 专用发布 secret | 不发给 PR，不打印，不进入构建层 |
| GHCR 拉取凭证 | 宿主机执行器专用凭证存储 | 私有 GHCR 当前使用 PAT classic，至少 `read:packages`；无写包/删包权限 [S1] |
| 私有 Release 读取凭证 | 宿主机专用存储 | 可选择仅授权该仓库 Contents 读取的 fine-grained token；与 GHCR credential 区分 [S3] |
| 可信公钥、源身份、路径白名单 | 执行器只读配置 | 不是秘密，但必须防止普通网页任意更换 |
| 原 `.env`、JWT、数据库配置 | 原服务器路径 | 不上传 GitHub，不重建数据库，不更换现有登录身份 |

私有 GHCR 的 PAT classic 不能误写成“直接用 fine-grained PAT”；实施时验证实际包授权、token scope、账户访问范围及失效续期。Docker 登录使用 stdin/凭证助手，禁止将 token 拼进命令参数、URL、日志或发布包 [S1]。

首次接入顺序：

1. 本地实现、回归后，先在隔离环境跑通 signed release、执行器与更新页面。
2. 用户确认包可见性、签名 secret 和只读凭证的设置；检查 GitHub 额度/存储/流量及服务器出站连通性。
3. GitHub 发布第一份有更新能力的配套成品，不能复用已发布 `0.2.13-r1` 表示另一份代码。
4. 用户单独批准宿主机执行器安装、socket 配置、首次迁移和短暂中断。
5. 使用现有人工发布流程安装执行器与首版后端/UI；旧网站按钮不能引导完成它尚不支持的升级。
6. 固化首次安装的真实版本、SHA、镜像 ID、UI、ledger 和备份为 bootstrap 部署记录，处理当前标签与实际旧二进制不一致的历史现象。
7. 从下一份新定制发布开始，网站更新按钮承担日常 prepare/activate。
8. 没有授权时仍允许本地/GitHub准备，停在重建生产服务之前。

## 9. 文件改造清单与实施顺序

以下目录和接入代码已在工作区实现；生产接入仍是独立步骤，改动清单见 `custom-release-implementation-changes.md`。

| 阶段 | 主要文件/模块 | 完成标准 |
| --- | --- | --- |
| P1 本地与契约 | `upstream-baseline.json`、版本文件、守卫及测试、`customizations.json`、发布 schema/manifest 生成器 | 定制与官方身份、版号、历史迁移、签名协议明确且可测试 |
| P2 构建与发布 | `backend/Dockerfile`、`backend/.dockerignore`、`.github/workflows/verify.yml`、`.github/workflows/publish-custom.yml`、`scripts/build-online-equivalent.mjs`、资源校验/打包脚本 | 同 SHA 精简镜像和 UI，经门禁后发布签名清单；网站未受影响 |
| P3 执行器 | 拟新增 `tools/sshzy-updater/`（独立 Go module）、`deploy/updater/` 的 unit/配置样例、受控 CLI 与协议测试 | 隔离环境可 prepare/activate/recover/rollback，跨容器重建持久化 |
| P4 后端更新源 | `update_service.go`、`github_release_service.go`、`system_handler.go`、admin routes、config/依赖注入；新增定制 provider、agent client，以及业务写路径/worker 的维护屏障接入 | 官方只读、私有资产代理、异步 API、旧危险写接口阻断、排空确认 |
| P5 网站交互 | `VersionBadge.vue`、`api/admin/system.ts`、`api/client.ts`、`stores/app.ts`、必要的更新详情组件及 i18n | 完成客户端契约、准备、迁移确认、中断重连、真实结果及配套回滚 |
| P6 首次上线 | 更新 `AGENTS.md`、上游治理文档、更新/回滚手册；经批准接入服务器 | 改为 GHCR 主路径，保留 rsync 应急与离线 save/load，首次安装验收 |

P1 → P2/P3 → P4 → P5 → P6；可并行建设构建和隔离执行器，但不得在 API/信任协议未定时先开放生产更新按钮。

发布生成器/校验脚本可放 `scripts/release/`；执行器是单独程序，不混入网站主进程。不为第一版引入 Kubernetes、Watchtower、生产自托管 GitHub runner 或自动整合官方代码。

## 10. 测试与验收清单

### 10.1 本地/官方治理

- 定制 `r1 → r2`、`r9 → r10`、官方 `X.Y.Z → X.Y.(Z+1)` 比较正确；旧/同/非法版本不能误判。
- 官方新版本只提示；源码/CI 因上游未知而阻断，不擅自 merge。
- 三方集成后定制功能回归通过，历史 migration 不变。
- 同版不同 SHA、标签/二进制/UI身份不一致均阻断。

### 10.2 CI 与发布供应链

- PR 无发布写权限或签名 secret；不可信手动 ref 不发版。
- 构建失败、UI 缺失、签名失败或测试新增失败不产生 ready Release。
- 镜像可启动，身份正确，大小改善有实测；运行层不含源码、编译链或敏感文件。
- 私有镜像与私有资产实际读取通过；凭证过期显示可解释失败，token 不随 CDN 跳转泄露。
- 发布并发/重试不覆盖已发布同版本；候选失败不影响线上。

### 10.3 网站与执行器

- 非管理员、过期再认证、伪造确认、任意 URL/路径/命令、跨来源镜像全部拒绝。
- 准备前后旧容器 ID、Compose、UI 指向与 bootstrap 不变。
- 明确“有几条迁移、改什么、允许什么”后才激活；未授权不会通过启动后端执行迁移。
- 两个浏览器重复操作只产生一次部署；确认后目标/配置漂移必须重新准备。
- 维护屏障覆盖业务写接口、支付回调和异步 worker；排空超时不强制切换，已提交批量任务升级后继续轮询且不重复提交。
- 当前旧 UI → 新后端的过渡组合通过回归；未知/不兼容客户端被明确阻断，而不是写入错误数据。
- 断网、低磁盘、坏归档、路径穿越、digest/hash/签名不符均在切换前失败。
- 关闭浏览器、重建后端、重启执行器后仍能找到操作和正确状态；不重复 migration 或重新启动已完成的部署。
- 后端不健康、UI 验证失败、部分迁移完成、自动回退不兼容均有明确处理；绝不自动还原数据库。
- 回退恢复 backend + UI + bootstrap，不只恢复软链；不出现官方镜像/install.sh 回退选项。

### 10.4 端到端验收

在本地隔离 Compose 环境完成：发布 A → 发布 B → 浏览器检查 → 准备 → 人工确认 → 后端/迁移/UI 更新 → 版本验收 → 兼容回退 A。

使用合成用户、旧任务和平台约束样本验证数据保留；记录关键表计数与样本摘要，不把“空库升级成功”当作真实数据安全证明。不得未经确认调用付费生图上游。

生产首轮仅在批准窗口验收：记录各阶段耗时、数据/配置保留、健康和登录、定制批量生图交互、回滚能力。已有非全绿测试必须公开说明并完成治理或明确批准，不能用此方案替代准入。

## 11. 实施前需确认的事项

本方案按以下默认值设计，确认后才开展实现/接入：

1. 自有发布仓库继续为 `ravicc02/sshzyu`，镜像名采用 `ghcr.io/ravicc02/sshzyu-backend`；默认不公开包。
2. 最终 `main` 合法版本递增后自动发布成品，线上仍由网站管理员人工确认；初期手动发布过渡。
3. 接受一次性安装独立宿主机执行器，而不是向网站授予 Docker/SSH 全权。
4. 官方源只用于整合基线和提示，本站更新/回退只走定制源。
5. 首版保持后端/UI 配套发布，`docs/`、`playground/` 不自动变更。
6. 更新涉及 migration 时逐次确认；破坏性/不兼容迁移走单独维护流程。
7. 凭证设置、生产服务安装、迁移、停机均是独立操作授权；本次写方案不代表批准上述操作。

## 12. 参考与来源

### 仓库依据

- `AGENTS.md`
- `plan_docs/official-upstream-versioning-workflow.md`
- `plan_docs/release-0.2.13-r1-prepared.md`
- `upstream-baseline.json`
- `scripts/upstream_release_guard.py`
- `backend/internal/service/update_service.go`
- `backend/internal/repository/github_release_service.go`
- `backend/internal/handler/admin/system_handler.go`
- `backend/internal/repository/migrations_runner.go`
- `frontend/src/components/common/VersionBadge.vue`

### 官方技术资料

以下链接用于核对平台能力，不代表本仓库已经启用相应功能；实施时重新核验权限和平台限制。

- [S1 GitHub Container registry：包权限、认证与 digest 拉取](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
- [S2 GitHub Actions：构建并发布 Docker 镜像](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images)
- [S3 GitHub REST：Release assets 的认证、下载与权限](https://docs.github.com/en/rest/releases/assets)
- [S4 Docker：多阶段构建](https://docs.docker.com/build/building/multi-stage/)
- [S5 Docker：镜像层复用与按 digest 拉取](https://docs.docker.com/reference/cli/docker/image/pull/)
