# 文档站发布流程

## 发布范围

`docs/` 是公开文档站的源文件。nginx 将它直接映射到 `/docs/`，无需编译。
主站 UI 和后端使用独立的发布包。更新主站不会更新 `/opt/sshzyu-docs/`。

只在用户授权发布或部署后执行线上同步。同一次站点部署若包含文档改动，须一并发布文档。
发布完成时，分别报告主站、文档站和生图工作台的状态。
不能只凭主站版本号或 `/health` 返回 200，就判定文档已更新。

| 路径 | 内容 |
|---|---|
| `docs/index.html`、`docs/content.md` | API 主文档 |
| `docs/image/` | 生图模型文档 |
| `docs/vedio/` | 视频模型文档，保留已有 URL 拼写 |
| `docs/docs-category.css` | 分类导航样式 |
| `docs/iamge/` | 文档图片，保留已有路径拼写 |
| `docs/install/` | 下载包 |
| `docs/legal/` | 合规文档 |
| `deploy/docs/` | 运维流程，保留在仓库中 |

发布脚本按 Git 跟踪清单完整暂存 `docs/`，包括新增子目录。
先检查引用资源是否已纳入 Git，再执行同步。不要只上传首页和正文。
本地记录、凭证和临时文件放在 `records/` 等非公开目录。
新文档及图片若被 `.gitignore` 忽略，先核对内容，再用 `git add -f -- <具体路径>` 纳入版本管理。

## 本地修改和预览

1. 修改 `docs/`，同步对应正文、导航、图片和下载链接。
2. 检查暂存内容，仅提交本次变更。提交信息使用 `docs(scope): ...`。
3. 本地 Compose 将 `docs/` 直接挂载到 nginx。重新启动受影响的本地服务后，即可查看页面。
4. 本地地址为 `http://127.0.0.1:8080/docs/`，并核对 `/image/` 和 `/vedio/` 分类页。

文档没有前端编译步骤。仅修改文档时，无需重建后端镜像或主站 UI。
若同时修改了 `frontend/`、`image-playground/` 或服务端代码，另按对应构建链处理。
`docs/legal/` 也被主站前端在构建时读取。修改它时，须同时重建并装配主站 UI。
未获发布授权的开发阶段，不运行测试套件。

## 发布前检查

在仓库根目录执行：

```bash
python -m unittest discover -s scripts/tests -p test_docs_release.py -v
python scripts/docs-release.py check
```

检查范围包括三个分类页、动态载入的 Markdown、样式及引用图片。
新增文件漏跟踪、缺失子页面或资源引用越界时，检查失败。
GitHub 的 `verify.yml` 也执行这两项检查。CI 只验证文件，不连接生产服务器。

发布前须完成本地提交。`deploy` 会拒绝尚未提交的 `docs/` 修改。
若修改了文档发布脚本，也应先提交脚本，再执行发布。

## 发布命令

```bash
python scripts/docs-release.py deploy
```

脚本只连接本机 SSH 配置中的 `us-server`。它依次执行：

1. 按 Git 跟踪清单创建完整暂存目录，检查链接并生成 SHA256 清单。
2. 用 `rsync -a --partial --info=progress2` 上传独立暂存目录。
3. 在服务器校验暂存文件，取得文档发布锁。
4. 完整备份当前文档目录。备份成功后，先同步资源和子页面，再原子替换首页。
5. 校验线上目录中的全部文件，再逐个请求公网 URL，比较状态码和 SHA256。
6. 检查三个目录入口，保存验收结果，并更新服务器的文档部署记录。

脚本会输出本地暂存路径和远端备份路径。
本地 `deployment.json` 记录提交 SHA、`docs/` 的 Git tree 和文件清单。
`verification.json` 记录公网验收结果。
发布目录与备份均保留，不自动清理。

| 服务器路径 | 用途 |
|---|---|
| `/opt/sshzyu-docs/` | nginx 当前读取的文档 |
| `/opt/sshzyu-docs-releases/<发布号>/` | 本次暂存、清单及验收记录 |
| `/opt/sshzyu-docs-backups/<发布号>/` | 同步前的完整备份 |
| `/opt/sshzyu-docs-deployed.json` | 最近一次完成公网验收的部署清单 |

Windows 缺少原生 rsync 时，脚本使用 WSL Ubuntu 的 rsync。
SSH 调用复用 Windows 本机客户端及其配置，不复制私钥。
WSL Ubuntu 必须已有 rsync；本机必须可以直接执行 `ssh us-server`。
整个流程不使用 `scp` 或 `--delete`，也不重启生产 nginx 和后端。

需要保留指定的本地暂存目录时，使用一个空目录：

```bash
python scripts/docs-release.py deploy --output records/docs-publish/<本次发布号>
```

只准备文件、暂不部署时，执行 `stage`：

```bash
python scripts/docs-release.py stage --output records/docs-publish/<本次发布号>
```

## 发布验收和主站联动

完成脚本校验后，在浏览器中查看：

- `https://sshzyu.com/docs/`
- `https://sshzyu.com/docs/image/`
- `https://sshzyu.com/docs/vedio/`

确认分类导航、正文、目录及图片正常。不要创建会扣费的生图或视频任务。
需要重新验收时，使用本次暂存目录中的清单：

```bash
python scripts/docs-release.py verify --bundle records/docs-publish/<本次发布号>
```

同一份清单也可用于检查本地挂载内容：

```bash
python scripts/docs-release.py verify --bundle records/docs-publish/<本次发布号> --base-url http://127.0.0.1:8080/docs/
```

主站上线前，读取服务器 `/opt/sshzyu-docs-deployed.json` 中的 `docs_tree`，
与 `git rev-parse HEAD:docs` 对比。初次接入没有清单，也按“待发布”处理。
若两者不同且本次包含文档部署授权，执行完整文档发布。
若文档部署不在授权范围内，须明确报告“文档待发布”。
`publish-custom.yml` 的签名发布物仍只覆盖主站 UI、后端和更新器；不将 CI 全绿当成文档已上线。

## 失败和回退

检查或备份失败时，脚本在覆盖线上文件前退出。
同步或公网验收失败时，保留暂存目录、备份和错误输出，并报告实际状态。
发布脚本不会自动回退，也不会删除旧资源。
删除或改名的页面需要另行确认重定向或下线方案。

需要回退时，先确认本次发布号和目标备份，再执行下面的命令。
命令会先保留当前状态，然后用备份覆盖同名文件。新增文件继续保留。

```bash
ssh us-server 'bash -s' <<'REMOTE'
set -euo pipefail
backup=/opt/sshzyu-docs-backups/<需要恢复的发布号>
save=/opt/sshzyu-docs-backups/before-restore-$(date -u +%Y%m%dT%H%M%SZ)
exec 9>/opt/.sshzyu-docs-publish.lock
flock -n 9
test -f "$backup/index.html"
test ! -e "$save"
cp -a /opt/sshzyu-docs "$save"
rsync -a --partial --info=progress2 --exclude=/index.html "$backup/" /opt/sshzyu-docs/
install -m 644 "$backup/index.html" /opt/sshzyu-docs/.index.html.restore
mv -f /opt/sshzyu-docs/.index.html.restore /opt/sshzyu-docs/index.html
if test -f /opt/sshzyu-docs-deployed.json; then
  mv /opt/sshzyu-docs-deployed.json "$save/deployment-before-restore.json"
fi
REMOTE
```

回退后重新检查公网内容。若有旧版清单，执行 `verify` 比较文件 SHA256。
回退示例会移走最新部署标记，避免它继续声称线上是回退前的版本。
业务接口和数据库不在文档回退范围内。
