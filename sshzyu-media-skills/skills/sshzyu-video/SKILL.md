---
name: sshzyu-video
description: "通过 SSHZYU 提交异步视频任务，查询、等待和下载已有任务；用于文生视频、图生视频、首尾帧和参考素材视频。"
---

# SSHZYU 视频

本技能负责视频请求、查询、等待、下载及结果校验，唯一实现为 `scripts/sshzyu_video.py`。运行时依赖同一安装根下的 `sshzyu-image`（HTTP 请求模块）；如需旧版统一 CLI，再并列安装 `sshzyu-media`。缺少图片依赖时停止，不自行加载未知脚本。

- [视频 API](references/api_reference.md)
- [视频参数矩阵](references/parameter-matrix.md)

## 凭据

视频与图片属于**不同的平台分组**：`/v1/videos` 只接受 `grok`、`composite`、`video` 组，`/v1/images/generations` 只接受 `openai`、`grok` 组。同一把 Key 无法同时满足两者，因此本技能读取**视频专用**凭据，按顺序：

1. 本技能目录 `.env` 中的 `SSHZYU_VIDEO_API_KEY`
2. 进程环境变量 `SSHZYU_VIDEO_API_KEY`
3. 回退到 `sshzyu-image` 的通用 `SSHZYU_API_KEY`（运行时打印警告；该 Key 通常属于 openai 生图组，调 `/v1/videos` 会返回 404）

把视频组 Key 写入本目录 `.env`（不入版本库）：

```bash
SSHZYU_VIDEO_API_KEY=<视频组 Key>
```

## 付费提交门槛

先明确模型、时长、分辨率、画幅、任务数量、参考素材的匿名公网 HTTPS 直链和费用；先执行同参数 `--dry-run`，得到用户对本次计划的明确授权后才加 `--confirm`。命令行开关不是用户授权。dry-run 不读取 Key、不联网。任务创建只提交一次；超时、断连、未知状态不重复创建，先查询原任务。不要打印 Key、签名 URL 或敏感响应。

```bash
python3 -I {技能目录}/scripts/sshzyu_video.py run --prompt "海边空镜，缓慢推镜" --id ocean-shot --model minimax_h3 --seconds 4 --resolution 480p --aspect-ratio 16:9 --out TASK_OUTPUT_DIR --dry-run
```

下载已有任务使用 `get`、`wait`、`download` 子命令；输出需检查任务 ID、`job-result.json` 与实际文件，不把受理当成完成。图片素材由 `$sshzyu-image` 生成，本地图片不可自动充当公网 URL。短剧生产任务必须另经 `$short-drama-produce` 的本次任务单确认和适配器门槛；本技能的 `--confirm` 不能代替它。
