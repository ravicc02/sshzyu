# 账号管理 OpenCode / MiniMax 适配补齐——实施记录

日期：2026-10-06
对照方案：[account-provider-adaptation-repair-plan.md](account-provider-adaptation-repair-plan.md)
对照基线：官方 `Wei-Shaw/sub2api` v0.2.13，commit `3040209f205472038c1ba745a1bedd2edd9053b1`
状态：代码与测试已落地；本记录不代表发布或部署授权。

## 1. 结论

筛选目录（`constants/platforms.ts`）与凭证辅助库（`credentialsBuilder.ts`）此前**已包含** MiniMax 与 OpenCode，缺口集中在两个弹窗组件：本地是从官方基线裁掉 MiniMax / OpenCode 分支后的版本，属于整合期的前端链路不完整，而非缺少单个按钮。

本次按官方基线逐段补齐，未整文件覆盖，保留 TypeSafe / Jev 定制平台与其余本地逻辑。

## 2. 改动清单

### 2.1 前端组件

| 文件 | 改动 |
| --- | --- |
| `frontend/src/components/account/CreateAccountModal.vue` | 补 MiniMax、OpenCode 平台入口；补 OpenCode Zen/Go 模式块；协议区改为 `isMultiProtocolPlatform`；接入 `OpenCodeGoProtocolRulesEditor`；补齐模式/协议/地址联动 watcher、平台切换重置、提交 payload（`account_mode`、`api_protocol`、`api_base_urls`、`protocol_rules`）。 |
| `frontend/src/components/account/EditAccountModal.vue` | 补 `isCNApiKeyAccount` 的平台判定到 MiniMax / OpenCode；补 OpenCode Zen/Go 模式回填与保存；接入协议规则编辑器；回填补齐旧账号默认值（无 `account_mode` 视为 GO）；保存时写入 `account_mode` 与 `protocol_rules`。 |
| `frontend/src/components/account/CnBaseUrlPresets.vue` | `platform` 属性类型补 `'minimax'`，与 `CN_BASE_URL_PRESETS` 对齐。 |
| `frontend/src/components/common/PlatformIcon.vue` | 补 MiniMax、OpenCode 图标（此前回落到通用图标）。 |
| `frontend/src/i18n/locales/{zh,en}/admin/accounts.ts` | 补 `admin.accounts.opencodeGo.accountMode.*` 文案（Zen / GO 及其说明）。 |

### 2.2 测试

| 文件 | 新增覆盖 |
| --- | --- |
| `CreateAccountModal.spec.ts` | 目录与添加入口一致性断言；OpenCode Zen payload；OpenCode GO 切换后 payload；MiniMax 自适应 payload。 |
| `EditAccountModal.spec.ts` | OpenCode Zen 回填与保存往返；旧 OpenCode 账号（无 `account_mode`）按 GO 处理；MiniMax 自适应回填与保存。 |
| `credentialsBuilder.spec.ts` | OpenCode 模式解析、Zen/GO 默认端点与默认规则、规则解析与写入、用量单元格可见性；请求头覆写资格扩展。 |
| `credentialsBuilder.cnAdaptive.spec.ts` | MiniMax 原生 Responses 支持与 CN/Intl 默认端点。 |

### 2.3 规范

- `frontend/src/constants/platforms.ts`、`credentialsBuilder.ts`、`constants/__tests__/platforms.spec.ts`：与官方基线一致，无需改动。
- `plan_docs/official-upstream-versioning-workflow.md`：补充“新增上游平台的前端贯通检查”与验证要求。

## 3. 平台验收矩阵

| 平台 | 添加 → payload | 编辑回填 → 保存 |
| --- | --- | --- |
| MiniMax | 测试 `submits adaptive MiniMax protocol endpoints`：`account_mode=payg`、`api_protocol=adaptive`、CN 三协议默认地址 | 测试 `preserves adaptive MiniMax endpoints on submit` |
| OpenCode (Zen) | 测试 `submits OpenCode Zen default protocol rules with adaptive endpoints`：`account_mode=zen`、Zen 端点、Zen 默认规则 | 测试 `preserves OpenCode Zen account type and endpoints on submit` |
| OpenCode (GO) | 测试 `submits OpenCode GO endpoints after switching account type`：模式切换后 `account_mode=go`、Go 端点与规则 | 测试 `treats a legacy OpenCode account without account_mode as GO` |
| 既有平台 | 回归用例 `submits adaptive Kimi …` 等未修改，全部通过 | 现有 Kimi / OpenAI 等编辑用例未修改，全部通过 |
| TypeSafe / Jev | `selectTypeSafePlatform` 与入口保留，目录断言覆盖 | 未改动 |

## 4. 后端契约核对

- `backend/internal/domain/constants.go`：`PlatformMiniMax`、`PlatformOpenCodeGo` 已定义。
- `backend/internal/service/account_service.go`：创建校验白名单已包含 MiniMax / OpenCode。
- `backend/internal/service/account.go`：`account_mode` / `api_base_urls` / 分协议端点解析已有实现。
- `backend/internal/repository/account_repo*.go`：OpenCode `account_mode=zen` 的用量身份分支已实现。
- 结论：前端 payload 字段与后端 DTO、校验、路由解释一致，无需改后端。

## 5. 检查结果

- 定向测试：`vitest run`（CreateAccountModal / EditAccountModal / credentialsBuilder / credentialsBuilder.cnAdaptive / platforms），169 项通过。
- 相关目录全量测试：`vitest run src/components/account src/constants`，30 个文件 / 411 项全部通过。
- `vue-tsc --noEmit`：通过。
- 改动文件 `eslint`：通过。
- 生产构建：通过。
- 关键文案键：`opencodeGo.accountMode.{zen,zenDesc,go,goDesc}` 在 zh / en 均存在。

### 构建

`npm run build`（`vue-tsc -b && vite build`）成功，耗时 48.35s，退出码 0。产物按既有配置写入 `backend/internal/web/dist/`（已被 `.gitignore` 忽略），未产生新的 Git 跟踪改动。

> 说明：`ui/` 目录下约 274 个构建产物存在未提交改动，其文件修改时间为 22:37，早于本次改动与构建，属于先前遗留状态，本次未触碰。

## 5.1 本地浏览器验收

- 2026-10-06：按用户要求执行本地切版 `bash scripts/rebuild.sh` 并重启 `sshzyu-ui-local` 容器，站点 `http://127.0.0.1:8080/` 已提供本次构建产物（入口 `fw-cachebust.js?v=CWJ7oGiP`，`AccountsView` chunk 内含 MiniMax / OpenCode 入口），交由用户人工验收。
- 切版校验输出：`fw-cachebust.js: current/shared 一致`、`MISSING targets in shared/assets: 0`、`index.html asset refs: 8 missing: 0`。
- 弹窗级交互由用户在本站点人工确认；组件级 mock 测试同时覆盖“添加 → payload → 编辑回填 → 保存”。
- 本次未向本地数据库写入任何测试账号。

## 6. 未覆盖 / 需另行授权

- 未升级官方基线，未拉入 main 新功能。
- 未执行真实上游账号创建与调用，未消耗额度。
- 未向本地数据库写入测试账号（仅组件级 mock 测试）。
- OpenCode Go 用量面板（`opencode_go_usage`）与编辑弹窗的用量区块不在本次范围。