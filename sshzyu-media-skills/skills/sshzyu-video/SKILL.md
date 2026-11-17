---
name: sshzyu-video
description: "通过 SSHZYU 提交异步视频任务，查询、等待和下载已有任务；用于文生视频、图生视频、首尾帧和参考素材视频。"
---

# SSHZYU 视频

本技能负责视频请求、查询、等待、下载及结果校验，唯一实现为 `scripts/sshzyu_video.py`。运行时依赖同一安装根下的 `sshzyu-image`（HTTP 请求模块）；如需旧版统一 CLI，再并列安装 `sshzyu-media`。缺少图片依赖时停止，不自行加载未知脚本。

- [视频 API](references/api_reference.md)
- [视频参数矩阵](references/parameter-matrix.md)

## 凭据

视频与图片属于**不同的平台分组**：`/v1/videos` 只接受 `grok`、`composite`、`video` 组，`/v1/images/generations` 只接受 `openai`、`grok` 组。同一把 Key 无法同时满足两者，因此本技能只读取**视频专用**凭据，按顺序：

1. 本技能目录 `.env` 中的 `SSHZYU_VIDEO_API_KEY`
2. 进程环境变量 `SSHZYU_VIDEO_API_KEY`

**两者都没有时直接报错并给出换 Key 指引，不会回退到 `sshzyu-image` 的 `SSHZYU_API_KEY`。** 生图组的 Key 调 `/v1/videos` 必然返回 404，回退只会把「Key 用错了」延后成一条难以定位的报错。

模型同样不静默降级：`--model` 指定的模型不在当前 Key 的 `/models` 列表中时，提交前就停止并列出该 Key 可见的视频模型，提示换模型或换 Key；确认模型确实可用时可加 `--no-verify-model` 跳过该检查。若网关仍返回 `404 not supported for this platform`，CLI 会明确提示这是 Key 分组平台不匹配，而不是模型名写错。

Key 到 [sshzyu.com](https://sshzyu.com) 申请，选择 platform 为 `grok`、`composite` 或 `video` 的分组。把 Key 写入本目录 `.env`（先从 `.env.example` 复制，`.env` 不入版本库）：

```bash
cp .env.example .env
# 编辑 .env，填入自己的视频组 Key
SSHZYU_VIDEO_API_KEY=<视频组 Key>
```

## 付费提交门槛

先明确模型、时长、分辨率、画幅、任务数量、参考素材的匿名公网 HTTPS 直链和费用；先执行同参数 `--dry-run`，得到用户对本次计划的明确授权后才加 `--confirm`。命令行开关不是用户授权。dry-run 不读取 Key、不联网。任务创建只提交一次；超时、断连、未知状态不重复创建，先查询原任务。不要打印 Key、签名 URL 或敏感响应。

```bash
python3 -I {技能目录}/scripts/sshzyu_video.py run --prompt "海边空镜，缓慢推镜" --id ocean-shot --model minimax_h3 --seconds 4 --resolution 480p --aspect-ratio 16:9 --out TASK_OUTPUT_DIR --dry-run
```

下载已有任务使用 `get`、`wait`、`download` 子命令；输出需检查任务 ID、`job-result.json` 与实际文件，不把受理当成完成。图片素材由 `$sshzyu-image` 生成，本地图片不可自动充当公网 URL。短剧生产任务必须另经 `$short-drama-produce` 的本次任务单确认和适配器门槛；本技能的 `--confirm` 不能代替它。
