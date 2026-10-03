# 定制更新执行器接入手册

本目录仅包含样例；没有安装服务、创建凭证或更新线上容器。只适用于 `us-server`。执行下面的接入与重启步骤之前，必须获得逐项授权，并切换当前 AI 对话的上游。

## 目录与职责

| 仓库目录 | 内容 |
| --- | --- |
| `backend/pkg/release/` | 共享发布契约、数值版本比较、Ed25519 验签、migration checksum |
| `backend/pkg/deployment/` | 维护屏障与在途请求/批量任务计数 |
| `tools/sshzy-updater/` | 独立 Go module，宿主机 daemon 和受控发布/恢复 CLI |
| `scripts/release/` | 发布准入与 migration 风险审阅记录 |
| `.github/workflows/` | 检查和签名发布，不持有生产 SSH 私钥 |
| `deploy/updater/` | 初次安装、socket 挂载和 systemd 样例 |

## GitHub 一次性设置

1. 保持自己的发布仓库 `ravicc02/sshzyu` 和官方 `Wei-Shaw/sub2api` 独立。
2. 设置 `custom-release` 发布 environment，按仓库套餐可用能力配置保护、分支限制及维护者权限。
3. 单独生成专用于发布的 Ed25519 PKCS8 PEM 私钥；值仅放 environment secret `SSHZY_RELEASE_SIGNING_KEY`，不可放源码、聊天、日志或镜像。不要复用 SSH 私钥。
4. 设置 variable `SSHZY_RELEASE_SIGNING_KEY_ID`，例如公开标识 `release-1`；在服务器通过可信渠道固定对应公钥及指纹。
5. 配置 GHCR 包私有可见性及仓库发布权限；Actions 只用自己的 `GITHUB_TOKEN` 发布包与 Release。
6. 全量门禁不绕过失败。当前遗留测试问题须单独治理或经过审批后制定精确例外；不得把工作流结果改为无条件成功。

推送到 main 后执行验证；完整版本号高于已发布定制版才构建成品。不递增版本时仅验证，不覆盖现有发布。手动 Run workflow 也只允许 main 和自己的仓库。

## 生产接入前置条件

- 明确批准：独立 updater 服务安装/启停、控制目录及文件权限、socket 挂载、首个新后端/UI、待执行的生产 migration 与短暂中断。
- 固定 `config.example.json` 中的服务器身份和目录；不能更换成其它服务器，不能把控制接口暴露公网。
- 为宿主机配置只读 Release token；为 Docker 专用配置目录配置只读 GHCR 访问凭证。私有 GHCR 需要 PAT classic 的 `read:packages` 等实际必要权限，不能假定 fine-grained PAT 可替代它。
- 控制 token 是独立服务凭证，只在服务器生成、保存并挂载单个文件；不返回给网页。权限建议 root 所有、部署组可读、不可组写/世界可读。公钥不是秘密，但只能由可信操作员更新。
- 样例网站进程 UID/GID 为 1000。实际 UID/GID、socket 组和 token 文件组必须一致；不是通过放宽 SSH 私钥权限来解决。
- 新镜像以非 root 运行；先检查现有 `/app/data` 映射的所有权和可读写性，权限变更必须明确批准，不能直接批量 chmod/chown 线上数据。
- `activation_enabled` 与 `payment_callbacks_reviewed` 默认 false。确认支付回调具备重试和本地幂等能力，或确认该站点没有相关业务后，才可逐项开启；网页不提供更改这些部署策略的入口。

## 首次安装

1. 先通过既有人工流程备份并安装第一份 `0.2.13-r3` 或更高的配套后端/UI。不能复用 r1/r2 标签指代新的代码。
2. 执行器不能给旧后端补上它不存在的排空协议。首轮的停机、任务排空和 migration 需要人工检查与单独授权。
3. 构建或下载 Linux updater。首次信任建议从已审核提交本地构建；如下载 CI 产物，先使用独立可信公钥验证 manifest，再核对 manifest 内 updater SHA256/大小，不能让未验证的下载程序验证自己。
4. 配置 `/etc/sshzy-updater/config.json`、公钥和凭证文件；样例不会携带真实值。将 daemon 安装到 `/usr/local/sbin/sshzy-updater`，经批准启用本目录的 unit。
5. 按样例将控制目录和单个 control token 只读挂给后端。**不要挂 Docker socket，不要把部署目录或 GitHub token 挂进网站。**将最终环境和卷合入正式 Compose 后，日常执行器仍只允许修改 image。
6. 服务初次启动可以列举发布，但未完成 bootstrap 时网站保持禁用更新。配置中的文件路径只作引用，不能把 `.env` 内容拷入仓库。
7. 已手工安装并验证对应版本后，在服务器运行：

   ```bash
   sshzy-updater bootstrap \
     --config /etc/sshzy-updater/config.json \
     --manifest /path/to/verified/release-manifest.json \
     --signature /path/to/verified/release-manifest.sig \
     --confirm
   ```

   它核对真实镜像引用/labels、UI build-info、current、固定 bootstrap 和 ledger 后才建立安装记录；不会替你部署镜像或执行 migration。
8. 确认维护文件存在且配置/身份一致后，启用网站更新入口。每次激活必须由真人管理员作迁移和停机确认，并通过近期 TOTP step-up。

`SSHZY_UPDATE_SITE_ORIGIN` 固定浏览器站点 origin；生产使用 `https://sshzyu.com`，本地隔离测试使用实际本地 origin（例如 `http://127.0.0.1:8080`）。不要信任用户可伪造的转发头来放宽更新源检查。

## 日常更新

1. 本地处理官方基线和定制修改，递增 `VERSION` 及基线中的本地构建号；历史 migration 不改。
2. 正常提交并推送；本地 pre-push 与 GitHub 守卫均需通过。官方有新版本或状态未知时停止，不自动安装官方原版。
3. GitHub 发布固定 digest 的纯后端镜像、同提交 UI、完整 ledger 与签名清单；发布成功不等于上线。
4. 网站选择版本并准备。执行器拉取、验签、校验配置/ledger/资源；不改当前 Compose、current 或固定 bootstrap。
5. 查看真正待执行的迁移。破坏性、未审阅或非事务迁移不支持普通一键激活。
6. 切换 AI 上游后确认维护和 migration，完成 TOTP。执行器设置屏障，等待 HTTP/批量任务安全点；使用 `docker stop --signal SIGTERM --timeout -1` 等待优雅退出，不让 Docker 超时后发 SIGKILL。执行器自己的等待超时会转入人工恢复，不强杀应用。该参数语义以 Docker 官方 `container stop` 文档为依据。
7. **首版采用离线备份窗口**：旧后端完全退出后才刷新 dump，备份期间网站 API 不可用。这个选择保证备份之后旧 worker 不继续写入；停机时间包括备份，不能宣称几秒完成。
8. 新后端启动时 maintenance 保持 active，后台服务延迟启动，业务请求关闭；仅获授权的启动 migration 可执行。后端验证后同步 hashed assets、更新固定 bootstrap，并调用现有 switch-release。
9. 验收后解除屏障并记录版本。短暂失联不代表成功；网页使用持久化 operation ID 查询，用户可在恢复后刷新到新 UI。

## 失败与回滚

- 准备失败不切换站点；进程在 preparing 阶段退出会标记准备中断，可重新发起。
- 激活中异常/执行器重启进入 `manual_intervention`，不会自动重放命令或恢复数据库。先只读检查实际容器、Compose、UI、ledger 和本操作备份。
- 如果后台显示 manual_intervention，需要经批准的运维人员使用受控 CLI：

   ```bash
   sshzy-updater recover --config /etc/sshzy-updater/config.json \
     --operation <validated-operation-id> --decision complete --confirm
   ```

   或在确认旧应用兼容当前 schema 后选择 `--decision rollback`。complete 只对真实新后端和新 UI 已就绪的状态结束维护；rollback 只恢复配套旧应用、旧 UI 与旧 bootstrap，不还原 dump。
- 若优雅退出超时、备份尚未产生、信任源不可读、配置漂移或 schema 不确定，受控恢复可能拒绝。不要强行绕过；保持维护，由操作员单独制定恢复步骤。首次手动恢复同样需要授权。
- 网站“准备回滚”只允许该机部署过且兼容的签名定制版本；从可见发布版本中选择目标后，执行器核对 history，不允许下载官方版回退。
- 不删除新列、ledger、备份、镜像或历史 release；数据库还原是另一项高风险操作，必须说明备份后数据损失并单独批准。

## 本地验证

```bash
go -C backend test ./pkg/release ./pkg/deployment
go -C tools/sshzy-updater test ./...
python -m unittest discover -s scripts/release -p 'test_*.py'
pnpm --dir frontend test:run src/api/admin/__tests__/customUpdate.spec.ts src/components/common/__tests__/CustomUpdateBadge.spec.ts
pnpm --dir frontend typecheck
```

Linux lifecycle 测试使用临时文件系统、合成数据和模拟 Docker/数据库命令，不接入实际站点或付费上游；它不等于生产数据库升级验收。最终真实安装验收仍需单独进行。
