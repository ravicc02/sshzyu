# 定制发布与网站更新改动清单

目标构建号：`0.2.13-r3`；官方基线保持 `v0.2.13`。

实现期间工作区新增了已有 `0.2.13-r2` 的可空 batch 字段修复提交，本次保留该修复并顺延版本，不复用 r1/r2。

## 1. 本地开发与发布契约

- `customizations.json`：新增自有仓库、官方源、镜像名称、tag 前缀、客户端契约与定制保留模块清单。
- `backend/pkg/release/manifest.go`、`manifest_test.go`：完整发布契约、数值 `rN` 比较、版本/仓库/镜像/架构绑定、Ed25519 验签、runner-compatible migration checksum 和历史 ledger 对照。
- `backend/cmd/server/VERSION`、`upstream-baseline.json`：本地版本递增 r3，保留官方 tag/SHA 与既有集成来源。
- `scripts/upstream_release_guard.py`：干净构建检查覆盖工作流、执行器、发布脚本及定制清单；官方新版本/未知状态仍阻断，不自动整合。
- `scripts/release/migration-reviews.json`：三项既有新增迁移的 checksum 与风险说明。审阅记录不是生产 migration 授权；其它新迁移默认 manual。
- 历史 SQL 文件未改名、重编号或修改；未增加新的数据库 migration。

## 2. GitHub 构建与签名发布

- `.github/workflows/verify.yml`：后端/执行器测试、合成迁移验证、前端类型/全量测试及编译；PR 只读，不取得签名或生产凭证。
- `.github/workflows/publish-custom.yml`：main/手动发布准入、版本递增、官方守卫、配套 UI、纯后端镜像、签名清单和完整 draft Release；成功后才发布为可见成品。Actions 固定已核验 commit SHA。
- `scripts/release/check_publish.py`、`test_check_publish.py`：最高数值定制版比较、不覆盖旧版本、私有资产 API、跨 CDN 去认证头及上一签名清单读取。
- `backend/Dockerfile`：多阶段精简运行镜像、非 root、固定 `/app/main`、完整 VERSION/COMMIT/DATE/release 与 OCI labels，包含所需 PostgreSQL 客户端。
- `backend/.dockerignore`：保留原缓存排除，并排除 env、凭证文件、本地配置及非发布产物。
- `scripts/build-online-equivalent.mjs`：保留 cache-bust 后处理，新增 UI `build-info.json`（版本、commit、契约、source/release 标识）。
- 产物包括固定 digest 镜像、UI 包、签名 manifest、checksums、完整迁移报告和 updater。updater 字节大小/hash 同样纳入签名清单。

没有创建 GitHub environment/secrets、修改仓库/包可见性、推送代码、上传镜像或发布 Release。工作流写入源码不等于已在 GitHub 运行成功。

## 3. 宿主机独立执行器

- `tools/sshzy-updater/go.mod`、`go.sum`：独立 Go module，通过本地 replace 复用后端公共发布契约。
- `cmd/sshzy-updater/main.go`：serve、bundle、verify、bootstrap、recover 受控 CLI。
- `internal/updater/manager.go`：原子持久化、单操作互斥、actor-scoped 幂等、明确阶段、精确迁移/停机确认、恢复时不自动重放。
- `source.go`：固定自己的 GitHub/GHCR 源、私有 asset ID 下载、签名/manifest hash/大小校验、严格 HTTPS 跳转与 token 隔离。
- `driver.go`：固定 us-server 身份/目录、只改 image 的 Compose 校验、历史 ledger 核验、镜像/资源校验、维护屏障、优雅退出、离线备份、后端与 UI 配套切换。
- `server.go`、`listen_linux.go`、`listen_other.go`：只监听 Unix socket；控制 token、Linux peer UID/组权限与宿主机单实例锁；不向网站开放 Docker/SSH。
- `recovery.go`：首轮真实身份登记、显式结束维护或兼容应用回退；不自动恢复数据库、不删除新列或 ledger。
- `bundle.go`：从干净提交和成功验证记录构建 UI 包、完整 migration 清单及签名发布契约；不打包累积 ui/shared。
- `space_linux.go`、`space_other.go`：空间保护与非 Linux fail-closed。
- `*_test.go`：认证、文件路径、特殊归档、版本/目标漂移、幂等、确认、重启恢复，以及 Linux 临时文件系统/模拟命令的 prepare→activate 流程。

激活与支付回调安全审阅默认关闭；没有安装执行器、生成生产凭证或操作实际数据库。

## 4. 后端网站更新与维护保护

- `internal/service/custom_update.go`、`custom_update_test.go`：Unix IPC 客户端、自有更新来源、数值修订比较、实际运行身份、未知状态和官方只读提醒；GitHub/registry token 留宿主机。
- `internal/service/update_service.go`：定制版阻断原地二进制更新和官方回退，不再只剥离 rN 后比较官方版。
- `internal/handler/admin/custom_update_handler.go`、测试：查看定制发布、准备、操作进度、激活、取消及准备回滚；真人管理员、同 origin、TOTP、严格 JSON 和参数约束。
- `internal/handler/admin/system_handler.go`：实际版本读本地身份；定制版禁止旧的 exit/restart 更新路径。
- `pkg/deployment/gate.go`、测试：维护状态读取失败时拒绝业务，新请求/批量执行计数、后台服务延迟启动。
- `internal/server/middleware/deployment.go`、测试，`router.go`、`routes/admin.go`：维护屏障、客户端契约、受控制凭证保护的 drain 状态与 admin 操作路由。
- `middleware/cors.go`：新增客户端契约与 Idempotency-Key 的预检允许头。
- `service/wire.go`、`batch_image_worker*.go`、`batch_image_cleanup.go`：后台启动和批量安全点接入维护保护；旧 worker 完全退出后才刷新 dump。
- handler/service BuildInfo、main、Wire provider 与重新生成的 `cmd/server/wire_gen.go`：贯通真实 commit/date 和 TOTP 依赖；补齐原已有抽奖仓储/UserSource 的 Wire 绑定，保留业务实现。

## 5. 前端更新交互

- `components/common/CustomUpdateBadge.vue`、测试：自有源、完整版号/commit/官方基线、版本选择、准备/确认分离、逐项迁移说明、停机确认、TOTP、取消、进度、失联重连与完成后刷新。
- `components/common/VersionBadge.vue`：定制版进入新的交互，不展示官方安装/回退命令；非定制模式保持原逻辑。
- `api/admin/customUpdate.ts`、测试：独立更新 API、确认策略、持久化且验证 operation ID，不保存访问凭证。
- `api/admin/system.ts`、`stores/app.ts`：完整定制检查信息；未知源不伪装成已经最新。
- `api/client.ts`：发送固定客户端契约标识，不把它当作身份凭证。
- `i18n/locales/en/customUpdate.ts`、`zh/customUpdate.ts`、对应 index：双语界面、阶段、维护和错误说明。

## 6. 目录与操作文档

- `AGENTS.md`：新增目录角色与 GitHub→GHCR→人工激活主链；保留首次/应急 rsync + save/load，明确代码落地不等于生产接入。
- `deploy/updater/config.example.json`：固定目录、源信任路径、UID/GID及默认关闭的激活/回调审阅开关，无凭证值。
- `deploy/updater/sshzy-updater.service`：独立 systemd 样例、运行目录、权限隔离及资源约束。
- `deploy/updater/docker-compose.updater.example.yml`：只读 control 目录/单文件 token 与固定 origin；无 Docker socket、GitHub token 或部署目录挂载。
- `deploy/updater/README.md`、`tools/sshzy-updater/README.md`：首次接入、信任建立、命令、维护窗口、失败恢复、回滚和本地验证。
- `plan_docs/custom-release-and-site-update-plan.md`、`official-upstream-versioning-workflow.md`：更新源码实施状态和与现有上游治理的关系。
- `.gitignore`：忽略独立执行器的本地二进制目录。

## 7. 验证与尚未接入项

- 发布契约、维护 gate、定制更新服务、handler、路由保护、既有 update/Wire/batch 定向测试通过，后端源码编译通过。
- 前端更新策略/组件用户流程与 app store 定向测试 32 项通过；类型检查、定向 ESLint、生产构建与 bootstrap 后处理通过。
- Python 发布策略 3 项与原上游守卫 14 项通过。
- Linux 执行器 prepare→activate、显式 complete/rollback、签名打包和备份测试通过；使用合成数据/模拟 Docker 与数据库，不能代替真实生产升级验收。共享发布契约覆盖率 90.1%，维护 gate 100%，执行器包 63.0%，CLI 尚未计入覆盖测试；执行器尚未达到方案的 80% 覆盖目标，生产接入前需继续补充测试。
- 全局 locale 完整性测试仍有既有 admin.groups、admin.accounts 等缺失/空文案失败，本次没有修改这些业务文案。GitHub 全量门禁保留，因此未治理/未经批准的现有失败仍会阻断正式发布。
- 未提交/推送本次改动、未创建生产凭证、未安装/重启服务、未执行实际 production migration、未切线上 UI。完整首次接入、真实迁移与停止运行前确认仍是单独步骤。
