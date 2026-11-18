# AGENTS.md — SSHZYU Media Skills 套件开发规范

本文件约束 `sshzyu-media-skills/` 套件仓库内的修改。仓库级规则仍以 `../AGENTS.md` 为准；两者冲突时，仓库级规则优先。

## 结构约束

- 套件仓库根是 `sshzyu-media-skills/`，可安装的 Skill 位于 `sshzyu-media-skills/skills/<名称>/`，每个目录必须有 `SKILL.md`。
- `SKILL.md` frontmatter 的 `name` 必须与目录名一致。
- 不得把其它 Skill 嵌套进 `sshzyu-media/`；不得用 `references/` 子目录伪装独立入口。
- 每个 Skill 自带所需 `scripts/`、`references/`、`assets/`；跨 Skill 只引用，不复制业务实现。

## 职责边界

- `sshzyu-media` 是唯一总入口，负责短剧项目初始化与配置、创作台、交付打包、跨能力约束、状态与路由，不复制图片/视频/短剧阶段实现。
- `sshzyu-image`、`sshzyu-video` 分别是图片、视频能力的唯一权威实现与参考位置。
- `short-drama-*` 阶段 Skill 负责短剧专业工作；生产确认与任务归档由 `short-drama-produce` 承担。不再有独立的 `short-drama` 总入口。

## 打包与安全

- 打包入口：`tools/package-suite.py`；发布前运行 `--check`。
- 不得把 `.env`、密钥、凭据、缓存、测试运行产物打入发布包。
- 修改入口名单、目录结构或脚本路径时，同步更新 `tools/package-suite.py`、`tests/`、`README.md`、`.github/workflows/verify.yml` 和公开安装说明。
- 未获发布授权前，不 push、不部署线上、不调用付费生成接口。
