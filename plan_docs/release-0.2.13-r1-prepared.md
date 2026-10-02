# 0.2.13-r1 集成与切换边界

日期：2026-10-03

## 范围

- 官方基线从 `v0.2.11` 增量整合至 `v0.2.13`，目标官方提交为 `3040209f205472038c1ba745a1bedd2edd9053b1`。
- 功能提交为 `2f21ad604679ba044289ebd1cb4268f1d49e36fb`，上游增量整合提交为 `351827563530519f4f3b83b37268240220e7f08b`。
- 保留 sshzy 品牌界面、模型定制、抽奖和 OpenAI/Gemini 批量生图配置、总任务及统一 ZIP 下载。
- 采用受控三方源码移植，没有用官方 monorepo 覆盖整个统一仓库；Ent 由合并后的 schema 重新生成。
- 修正平台枚举和测试桩的兼容；额度编辑和默认额度序列化保留全部后端支持的平台。
- 不实施此前暂定的 Codex/WebMCP 自动化方案。

## 已完成验证

- 线上 294 项历史迁移与原工作区及 Git 内容的 SHA256 校验值均一致。
- 历史 migration 不改内容、不改名、不重编号；SQL 和 Go 源码采用 LF，避免 Windows 构建改变 migration checksum 或影响源码审计测试。
- 在用户授权的本地隔离库 `sub2api_preflight_v0213_20261003` 中复制本地数据库结构和 migration 记录，没有复制用户或账号数据。
- 三项新增迁移已通过本地隔离验证，记录数为 297，用户和账号表记录数均为 0：
  - `241_add_payment_order_bonus_amount.sql`
  - `241_add_typesafe_platform.sql`
  - `246_batch_image_collection_id.sql`
- Migration 使用完整文件名作为记录键，保留官方的两个 `241_*` 文件名，不覆盖本地已有的 `241_batch_image_idempotency_unique.sql`。
- 前端类型检查、生产构建通过；全量测试合并前为 2573 项、235 项失败，合并后为 2611 项、230 项失败，按文件和测试名称对比没有新增失败。
- Windows 后端全量单元测试的四项失败已在合并前版本复现；其中三个依赖 Unix `sh`，一个为既有 Ollama 回调测试问题。Linux 发布环境结果需以内部测试报告为准，不宣称全量测试全部通过。
- TypeSafe 平台、额度、充值赠送、Key 排序和批量生图的新增或受影响测试已逐项检查。

## 发布前必须保持的边界

- 当前对话连接站点上游，因此只准备、构建、上传产物；不执行线上 `docker compose up`、容器重建或前端切换。
- 线上新增迁移尚未授权，本地隔离验证授权不等于线上迁移授权。最终切换前需要确认上述三项线上迁移。
- 新后端启动会自动执行内嵌 migration，不能通过直接重建容器绕过迁移授权。
- 发布前备份数据库及部署配置，保留旧镜像、旧 UI release、所有日志和备份。
- 发布镜像使用完整本地版本、实际源提交 SHA、UTC 构建时间和 `release` 构建身份；向 origin 推送前执行上游门禁。

## UI 切换特别检查

- 上传阶段只写 `/opt/sshzyu-ui/releases/0.2.13-r1/`，不覆盖当前入口或 shared 中的固定文件。
- 当前 `switch-release.sh` 对 assets 使用只增不覆盖的同步；固定文件名 `fw-cachebust.js` 不能依赖该同步更新。
- 最终切换时先备份当前 `shared/assets/fw-cachebust.js`，在新后端健康后更新它并调用现有 `switch-release.sh`。
- 切换失败时同时恢复旧 UI release 和旧 `fw-cachebust.js`，避免入口指向错误版本。
- 不手改 `current` 软链，不重启 nginx、fail2ban 或 cron。

## 内部发布记录

构建 SHA、镜像 ID、产物 SHA256、服务器预备目录、备份位置及最终执行提示词保存在内部忽略目录 `records/release-v0.2.13-r1-20261003/`。以实际准备完成后的清单为准；本文件不代表线上已发布。
