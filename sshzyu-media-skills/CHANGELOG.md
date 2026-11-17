# 变更记录

## 0.1.1

- 修正骨架一致性：`sshzyu-image`、`sshzyu-video` 的测试移入各自 `tests/`，修复 `parents[1]` 误指向 `skills/` 的路径错误；CI 的 discover 路径同步。
- 清理重复入口：删除 `sshzyu-media/` 根目录下的 `sshzyu_media.py`、`sshzyu_image.py`（与 `scripts/` 实现重复且文档未引用），以及不会被读取的 `sshzyu-media/.env.example`。
- 修正路由：`sshzyu-video` 参考命令改指 `scripts/sshzyu_video.py`；CLI `prog` 名与真实入口一致；`sshzyu-media` 评测提示词不再指向旧 `sshzyu-media video`。
- 修复 `sshzyu-media/SKILL.md` 被误转为纯文本的 6 条参考链接，并接入旧版镜头清单说明。
- 标注 `short-drama-produce` 内置供应商仅用于历史兼容，消除与「SSHZYU adapter 未接通」的表述矛盾。
- 补充同根安装约束与凭据单一来源说明（README、`docs/architecture.md`、`SKILL.md`）。

## 0.1.0

- 将 `sshzyu-media`、`sshzyu-image`、`sshzyu-video` 与 12 个 `short-drama*` 技能整理为套件内的并列入口（`sshzyu-media-skills/skills/`）。
- 套件根新增说明、版本、许可、开发规范、架构文档、打包工具与包检查。
- 发布包改为顶层包含 `skills/<名称>/` 的套件结构。
