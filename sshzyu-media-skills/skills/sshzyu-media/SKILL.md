---
name: sshzyu-media
description: "跨媒体创作总入口：识别图片、视频、短剧或组合任务，检查输入和阶段，维护付费确认与交付状态，并转交独立专业技能。"
---

# SSHZYU 媒体统筹

本技能负责跨媒体需求的入口判断、阶段交接及安全边界，不复制图片/视频参数表或短剧创作知识。具体职责见 [路由契约](ROUTING.md)。

- 单次图片生成/编辑直接使用 `$sshzyu-image`，其 CLI、凭据和图片参考资料由它维护。
- 单次视频提交/查询/下载直接使用 `$sshzyu-video`，其 CLI 与视频参考资料由它维护。
- 短剧项目启动、继续、Dashboard 和五份创作文档交 `$short-drama`；明确的单阶段任务直接交相应 `$short-drama-*`。
- 短剧素材批量生产交 `$short-drama-produce`；它负责项目任务单、预览、确认、恢复与归档。当前没有已接通的 SSHZYU 生产 adapter，不能将路由说明视为可投产。

启动项目不等于授权生成。普通媒体调用需先核实模型、尺寸或时长、画幅、参考素材、任务数及费用风险，执行不读取密钥、不联网的 `--dry-run`，取得用户对本次计划的明确确认后才可加 CLI `--confirm`。短剧任务须另按项目任务单的 `prepare → confirm → run/collect` 契约确认；不能用历史确认、CLI 参数或用户泛泛的“继续”替代。异步任务失败先查询原任务，绝不盲目重复付费。

兼容入口 `scripts/sshzyu_media.py` 保留旧版 `image`、`video`、`drama`、`doctor`、`models` 命令；其运行需要并列安装 `sshzyu-image` 与 `sshzyu-video`，并非本技能目录下的子技能。旧 `drama run` 是镜头 manifest 兼容命令，**不是**短剧生产任务单适配器。运行产物写入用户指定输出目录，不写在技能安装目录；密钥只通过可信本地配置或环境提供，禁止打印。直接调用对应独立 Skill 时无需绕过本入口。

```bash
python3 -I {技能目录}/scripts/sshzyu_media.py doctor
```

该命令只检查本地依赖与凭据来源，不调用付费接口；缺少独立技能时须明确报告缺失。不要将 `doctor` 成功、ZIP 存在或项目初始化等同于生产链路已验证。
