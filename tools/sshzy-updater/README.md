# sshzy-updater

独立 Go module，使用 `../../backend/pkg/release` 的共享契约，不运行在网站进程中。

```text
cmd/sshzy-updater/       CLI 入口：serve / bundle / verify / bootstrap / recover
internal/updater/
  manager.go            持久化状态机、互斥、幂等与精确授权
  source.go             自有 GitHub 源、私有资产 API、验签
  driver.go             白名单 Docker/Compose、备份及 UI 切换
  recovery.go           首次身份登记与显式恢复
  bundle.go             CI 发布包和完整 migration 清单
  server.go             仅 Unix socket 的受控 HTTP 协议
  listen_linux.go       单实例锁、socket 权限及 peer UID 校验
  *_test.go             合成数据、故障/授权/文件系统测试
```

构建：`go build -o <outside-workspace-output> ./cmd/sshzy-updater`。正式 Linux daemon 只能操作 `us-server` 的固定部署目录。Windows 可运行 bundle/verify 及单元测试，不启动生产 daemon。

完整接入、审批与恢复步骤见 `deploy/updater/README.md`。签名私钥、GHCR/Release token、control token 不属于源码或本目录内容。
