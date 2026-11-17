# SSHZYU Media Skills

`sshzyu-media-skills/` 是**可整体获取、构建和分发的 Skill 套件仓库根**，本身不是可调用的 Skill。仓库根下的 `skills/` 存放并列的独立 Skill，每个目录都有各自的 `SKILL.md`，可被安装器分别安装到全局技能目录。

## 入口

| 入口 | 职责 |
| --- | --- |
| `sshzyu-media` | 统一入口：短剧项目初始化与配置、创作台、导出打包、跨能力约束与状态、阶段分发 |
| `sshzyu-image` | 图片生成、编辑、模型查询与本地检查 |
| `sshzyu-video` | 视频生成、状态查询、等待与下载 |
| `short-drama-*` | 短剧各阶段专业入口（开发、设定、分镜、提示词、生产、剪辑、审查等） |

`sshzyu-media` 持有短剧项目生命周期与跨能力规则，不复制图片、视频或短剧阶段的业务实现。用户明确指定某个专业 Skill 时，可以直接调用对应入口。

## 目录约定

```text
sshzyu-media-skills/       # 套件仓库根（本目录）
├── skills/                 # 并列的独立 Skill，安装器从这里发现入口
│   ├── sshzyu-media/
│   ├── sshzyu-image/
│   ├── sshzyu-video/
│   └── short-drama-*/
├── docs/                   # 套件架构与使用说明
├── tests/                  # 套件级检查
├── tools/                  # 打包与发布工具
├── AGENTS.md               # 套件内开发规范
├── CHANGELOG.md
├── VERSION
├── LICENSE
└── README.md
```

每个 `skills/<名称>/` 必须包含与目录同名的 `SKILL.md`（frontmatter `name` 一致），并自带运行所需的 `scripts/`、`references/`、`assets/` 等资源。**不要把其它 Skill 嵌套到 `sshzyu-media/` 内**；`references` 子目录不是独立入口。

## 构建发布包

在仓库根执行：

```bash
python sshzyu-media-skills/tools/package-suite.py          # 生成 docs/install/sshzyu-media.zip
python sshzyu-media-skills/tools/package-suite.py --check  # 校验现有 ZIP 是否与套件源码一致
```

发布 ZIP 的顶层包含套件说明文件和 `skills/<名称>/` 并列技能目录。打包器会校验入口名单、frontmatter、必需资源、相对链接，并排除密钥、缓存和测试运行产物。

## 安装

- NewMax 使用应用内正式 Skill 安装器安装套件源目录或发布 ZIP，以安装器报告的全部入口为准。
- 安装前检查目标安装根是否已有同名 Skill；出现冲突时停止，由用户选择保留原版或替换，不得静默覆盖。
- 安装后预期每个 Skill 分别位于 `<安装根>/<名称>/SKILL.md`。
- **必须整体同根安装**：`sshzyu-video` 在导入阶段就要求同一安装根下存在 `sshzyu-image`（复用其 HTTP 请求实现），缺失时会直接退出；`sshzyu-media` 的兼容命令还要求 `sshzyu-video`。只安装其中一部分会导致该技能不可用。`short-drama-*` 阶段技能可独立安装。
- **配置自己的 Key**：安装后把 `skills/sshzyu-image/.env.example` 与 `skills/sshzyu-video/.env.example` 分别复制为同目录的 `.env`，填入自己的 Key。Key 到 [sshzyu.com](https://sshzyu.com) 申请。图片与视频必须使用**不同平台分组**的 Key，不能共用同一把（原因见两个技能各自的 `SKILL.md`）。`.env` 已被忽略，不要提交。
- 不提供 API Key 也可先做离线检查（在仓库根执行）：`python sshzyu-media-skills/skills/sshzyu-media/scripts/sshzyu_media.py doctor`。

## 版本

见 `VERSION`。
