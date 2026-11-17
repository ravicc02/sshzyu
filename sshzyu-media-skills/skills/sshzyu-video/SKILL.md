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

## 参数必须与模型能力对齐

不同模型支持的分辨率档位、时长范围和参考素材能力不同，**不能把一个模型的参数套到另一个模型上**。能力表在 `references/video_model_capabilities.json`，CLI 在提交前用它校验：

```bash
# 查看全部模型能力（不联网、不读 Key）
python3 {技能目录}/scripts/sshzyu_video.py models

# 只看某个模型，并对比当前 Key 的 /models 可见性
python3 {技能目录}/scripts/sshzyu_video.py models --model minimax_h3 --check-live
```

校验规则：分辨率不在该模型档位、时长超出范围、或模型不支持所传参考素材时，**提交前直接失败并列出可用值**，不会先花一次钱再让上游拒绝。`--dry-run` 也会执行这套校验。

实测要点：

- `minimax_h3` 只接受 `480p`、`768p`、`1080p`、`2k`、`4k`，**没有 `720p`**；画幅由分辨率档决定，建议省略 `--aspect-ratio`。1080p 实际输出是 **1088×1920**，不是 1080×1920。
- `sd25官转`、`sd2官转`、`doubao-seedance-2-5` 只有 **720p** 可用，传 1080p 会被上游以「该模型不支持所选分辨率」拒绝。
- 输出尺寸以 `ffprobe` 实测为准；与能力表记录不一致时 CLI 会给警告，提示更新能力表。

## 参考素材

参考图既支持公网 HTTPS 直链，也支持本地文件：

```bash
# 公网直链
--image-url https://example.com/frame.png
# 本地文件，自动转 Base64 data URI（PNG/JPEG/WebP，单张上限 8MB）
--image-file ./frames/SHOT-001.png
```

上游参考图字段接受 URL 或 Base64，**「必须公网」是旧实现的限制，不是上游要求**。代价是本地图会让请求体变大（8MB 图约 10.7MB Base64），线上 nginx 上限 256MB，通常足够。

## 付费提交门槛

先明确模型、时长、分辨率、画幅、任务数量、参考素材和费用；先执行同参数 `--dry-run`，得到用户对本次计划的明确授权后才加 `--confirm`。命令行开关不是用户授权。dry-run 不读取 Key、不联网。任务创建只提交一次；超时、断连、未知状态不重复创建，先查询原任务。不要打印 Key、签名 URL 或敏感响应。

```bash
python3 {技能目录}/scripts/sshzyu_video.py run --prompt "海边空镜，缓慢推镜" --id ocean-shot --model minimax_h3 --seconds 4 --resolution 480p --out TASK_OUTPUT_DIR --dry-run
```

下载已有任务使用 `get`、`wait`、`download` 子命令；输出需检查任务 ID、`job-result.json` 与实际文件，不把受理当成完成。图片素材由 `$sshzyu-image` 生成，生成结果可直接用 `--image-file` 作为参考图。短剧生产任务必须另经 `$short-drama-produce` 的本次任务单确认和适配器门槛；本技能的 `--confirm` 不能代替它。
