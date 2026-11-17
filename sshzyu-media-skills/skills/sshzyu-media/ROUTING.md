# SSHZYU 媒体路由契约

本文件仅定义跨媒体边界，不替代各技能的参数、计费和项目契约。

| 用户意图 | 负责入口 | 事实源 |
| --- | --- | --- |
| 单次生图、改图 | `$sshzyu-image` | `sshzyu-media-skills/skills/sshzyu-image/` |
| 单次生视频、查询及下载 | `$sshzyu-video` | `sshzyu-media-skills/skills/sshzyu-video/` |
| 短剧项目创建、阶段路由和 Dashboard | `$sshzyu-media` | 原五份 Markdown 文档和项目配置 |
| 短剧单阶段创作 | 对应 `$short-drama-*` | 当前项目文档 |
| 短剧生产任务单和恢复 | `$short-drama-produce` | 项目内已有稳定任务 ID 和运行记录 |

`$sshzyu-media` 只负责跨能力任务的识别、交接和共同安全边界。图片/视频 CLI 只负责媒体调用；短剧创作与任务单由原短剧阶段技能处理。套件源码为 `sshzyu-media-skills/skills/<name>/SKILL.md` 并列结构，发布包与安装后也须并列；原 `sshzyu-drama` 包装入口不再发布。

兼容命令 `sshzyu-media/scripts/sshzyu_media.py image|video|drama ...` 保留，但依赖相邻独立 `sshzyu-image`、`sshzyu-video` 安装；`drama run` 是旧镜头 manifest 流程，不是 `short-drama-produce` 任务单 adapter。生产工具没有已实现的 SSHZYU adapter，不能用兼容命令绕过 `prepare → confirm → run/collect`。

**付费边界**：每次先展示任务数量、模型、关键参数、费用风险和 dry-run；仅用户明确确认本次计划后才可提交。`--confirm` 是 CLI 技术门，不是授权证据；短剧任务单另需本次预览确认。异步请求拿到 ID 后先持久化，再查询原任务；失败不盲目重试。禁止把密钥、签名 URL 或本地私有素材自动上传/打印。历史直连供应商 adapter 只保留旧任务兼容，不作为新任务入口。
