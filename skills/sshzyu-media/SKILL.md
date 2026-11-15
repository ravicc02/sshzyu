---
name: sshzyu-media
description: "通过 sshzyu.com 完成 AI 生图、图片编辑、异步视频生成、首尾帧/参考素材、任务轮询下载，以及漫剧或短剧的镜头清单执行与 FFmpeg 成片装配。用户提到 SSHZYU 生图、生视频、AI 漫剧、AI 短剧、媒体批处理时使用。"
---

# SSHZYU 通用媒体

使用 `scripts/sshzyu_media.py` 统一处理图片、视频和短剧。脚本只依赖 Python 标准库；成片拼接可选依赖 `ffmpeg` 和 `ffprobe`。

## 🔴 CHECKPOINT · 强制 Preflight

任何真实图片或视频请求都会产生费用。提交前必须明确用户意图，并复述最终计划。

图片计划至少包含：

- 用途、画面内容、模型。
- 宽高比、具体尺寸、quality、输出格式。
- prompt 数、每项张数、并发数、参考图、mask 或 markup。

视频计划至少包含：

- 用途、剧情动作、模型、生成秒数。
- 分辨率、画幅、参考图片/视频/音频 URL。
- 任务数量、是否会产生多笔费用。
- 首尾帧、角色参考或视频参考分别对应哪些素材。

短剧计划还必须包含：

- 镜头清单、顺序和总视频数。
- 每镜头时长、模型、画幅和参考素材。
- 是否顺序执行、是否继续执行失败后的镜头。
- 最终画幅、帧率和外部配音文件。

### 🛑 STOP · 参数不全时禁止提交

以下任一情况必须停止，不提交真实任务：

- 缺少模型、时长、分辨率、画幅或关键动作。
- 参考素材仍是本地路径、内网地址或需要登录的 URL。
- 用户未确认费用，或没有明确确认本次测试计划。
- API Key 缺失。

参数不全时，只向用户追问缺失项，不自行补默认模型、尺寸或时长。用户一次性给出完整参数并明确授权执行时，视为 preflight 已确认。执行前仍先运行 `--dry-run` 展示计划；`--dry-run` 不读取 Key、不联网、不计费。

### 🔴 CHECKPOINT · 真实提交前

真实任务只允许按以下顺序开始：

1. 复述最终计划。
2. 执行 `--dry-run` 并展示输出。
3. 用户明确确认费用和任务数量。
4. 真实命令加入 `--confirm`。

用户没有明确确认本次计划时，命令停在 dry-run。

## 路由

| 用户需求 | 命令 |
| --- | --- |
| 单提示词生图、多图、改图 | `image` |
| 文生视频、图生视频、参考素材视频 | `video run` |
| 查询已有视频任务 | `video get`、`video wait` |
| 下载已有视频 | `video download` |
| 漫剧/短剧按镜头批量生成 | `drama run` |
| 将已生成镜头拼成成片 | `drama assemble` |
| 查看当前 Key 可用模型 | `models` |
| 检查 Python、Key 来源和 FFmpeg | `doctor` |

旧版生图参数可以不带 `image` 子命令，脚本会自动兼容。

## 快速检查

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" doctor
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" models --kind video
```

示例使用 PowerShell。macOS、Linux 或使用 POSIX shell 的 runtime 将 `py -3.12` 替换为 `python3`，并将行尾反引号替换为反斜杠续行。

模型可见性由 Key 所属分组决定。图像与视频可能属于不同分组；商品图可用不代表视频可用。

`/models` 可能随分组和上游账号切换而变化。模型不在某一次列表中时，将其视为警告，不自动换模型；最终以创建接口返回的 `400/403/422` 为准。

## 图片

单提示词多图：

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" image `
  "一只戴墨镜的橘猫，暖色调，无文字" `
  --model gpt-image-2.6-sunburst --size 1024x1024 --quality high `
  --n 2 --out "TASK_OUTPUT_DIR" --dry-run
```

确认后加入 `--confirm`。多 prompt、编辑、mask、markup 和 JSON manifest 的完整参数见：

- [图片 API](references/image/api_reference.md)
- [图片参数矩阵](references/image/parameter-matrix.md)
- [图片模型路由](references/image/models/README.md)

图片输出仍使用 `run-时间戳-短ID/` 目录，每任务保存图片、`job-result.json` 和 `batch-summary.json`。`partial` 退出码为 `2`。

## 视频

文生视频或图生视频：

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" video run `
  --prompt "海边空镜，缓慢推镜，自然光" `
  --id ocean-shot `
  --model minimax_h3 --seconds 4 --resolution 480p --aspect-ratio 16:9 `
  --out "TASK_OUTPUT_DIR" --dry-run
```

参考图片、视频或音频必须是可以匿名下载的公网 HTTPS 直链：

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" video run `
  --prompt "保持角色外观，缓慢转身看向镜头" `
  --model sd25-满血-新 --seconds 5 --resolution 720p --aspect-ratio 9:16 `
  --image-url "https://assets.example.com/character.png" `
  --out "TASK_OUTPUT_DIR" --dry-run
```

首尾帧使用 `references`：

```json
[
  {
    "type": "image",
    "role": "first_frame",
    "url": "https://assets.example.com/first.png"
  },
  {
    "type": "image",
    "role": "last_frame",
    "url": "https://assets.example.com/last.png"
  }
]
```

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" video run `
  --prompt "人物从窗边转身走向门口，动作连续" `
  --model doubao-seedance-2-5 --seconds 5 --resolution 720p --aspect-ratio 9:16 `
  --references-file "references.json" --out "TASK_OUTPUT_DIR" --dry-run
```

视频任务流程：

1. `POST /v1/videos` 只提交一次。
2. 保存任务 `id`、请求体、提交时间和响应。
3. 默认每 5 秒查询原任务。
4. `completed` 后从内容接口下载到临时文件。
5. 校验视频魔数、大小、时长和画幅后原子改名。
6. 失败、超时或未知状态不自动重新创建任务。

完整字段和模型限制见 [视频 API](references/video/api_reference.md) 与 [视频参数矩阵](references/video/parameter-matrix.md)。

## 漫剧与短剧

先创建镜头 manifest。所有 prompt 文件、音频路径相对于 manifest 所在目录解析：

```json
{
  "project": "demo-drama",
  "defaults": {
    "model": "minimax_h3",
    "seconds": 4,
    "resolution": "480p",
    "aspect_ratio": "9:16",
    "poll_interval": 5,
    "poll_timeout": 1800
  },
  "shots": [
    {
      "id": "EP01_SH001",
      "prompt": "雨夜街头，女主回头，镜头缓慢推近"
    },
    {
      "id": "EP01_SH002",
      "prompt_file": "prompts/EP01_SH002.txt",
      "image_urls": ["https://assets.example.com/ep01-location.png"]
    }
  ],
  "assembly": {
    "resolution": "1080x1920",
    "fps": 25,
    "audio": "audio/voice.wav"
  }
}
```

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" drama run `
  "drama.json" --out "TASK_OUTPUT_DIR" --dry-run
```

### 🔴 CHECKPOINT · 成片拼接前

真实执行时默认逐镜头提交。先检查 `drama-summary.json`：

```powershell
Get-Content "RUN_DIR/drama-summary.json" -Raw
```

只有全部镜头状态为 `success`，或 `partial` 已经人工验收，才允许拼接。镜头未完成时先续跑：

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" drama run `
  "drama.json" --out "OUTPUT_DIR" --resume-run "RUN_DIR" --confirm
```

续跑默认跳过状态为 `success` 或 `partial` 且视频文件仍存在的镜头，不重建已完成任务。全部镜头可用后再拼接：

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" drama assemble `
  "TASK_OUTPUT_DIR/drama-..." `
  --out "TASK_OUTPUT_DIR/final.mp4"
```

`drama run` 会保存：

- `manifest.snapshot.json`
- `drama-state.json`
- `drama-summary.json`
- 每镜头目录与 `job-result.json`

再次运行同一 `--resume-run` 目录时，只跳过已经成功且文件仍存在的镜头。

漫剧设计与镜头拆分见 [短剧工作流](references/drama/workflow.md)。

## 凭据与网络边界

- Key 唯一推荐位置：技能目录 `.env` 中的 `SSHZYU_API_KEY=...`。
- 读取顺序：`.env`、进程环境变量、旧 Windows 凭据、旧 `key.txt`。
- 不使用命令行明文 Key，不打印 Key、Authorization 或签名下载 URL。
- 请求只发往 `https://sshzyu.com/v1`。
- JSON 接口不允许任意 URL。
- 视频下载允许 307/308，但跨源跳转时丢弃 Authorization、Cookie 等敏感头。
- 参考素材只接受公网 HTTPS 直链，文件会由 SSHZYU 或其上游读取。
- 本地文件、内网地址、浏览器 Cookie、凭据和私人素材不得自动上传。

## 失败恢复

| 情况 | 处理 |
| --- | --- |
| `401` / `403` | 检查 Key、分组和模型权限，不绕过 |
| `402` | 停止，报告余额不足，不自动充值 |
| `422` | 核对模型参数、时长、分辨率和素材数量 |
| `429` / `502` / `503` / `504` | 不重新创建，先查询原任务 |
| 创建响应超时但可能已受理 | 保存请求和请求 ID，先查用量或原任务 |
| 已取得任务 id 后查询失败 | 保留 task id，使用 `video wait` 继续查询 |
| 下载内容不是视频 | 拒绝保存并删除临时文件 |
| `partial` | 保留有效文件，报告差异，不自动补发 |

## 输出契约

dry-run 必须报告：

1. 路由：`image`、`video` 或 `drama`。
2. 模型、关键参数、任务数和预计输出数量。
3. 完整命令。
4. 明确“未读取 Key、未联网、未计费、未执行”。

真实执行必须报告：

1. 实际运行的命令或等价的脱敏参数摘要。
2. `run_dir`、视频任务 `id` 和任务状态。
3. 已保存文件、有效张数或秒数。
4. warnings、`partial` 原因和退出码。
5. 失败时的 HTTP 状态、下一步查询命令，以及“不自动重试”声明。

不要在汇报中打印 Key、Authorization、签名下载 URL 或完整私密素材地址。

- `success`：任务完成，文件校验通过，退出码 `0`。
- `partial`：保留有效结果，但存在素材短缺、时长/画幅差异或 FFprobe 警告，退出码 `2`。
- `error`：请求、查询、下载或校验失败，退出码 `1`。
- 不把“任务已受理”当作“视频已生成”。
- 不把进程退出码当作唯一依据，同时读取 `job-result.json`。

## 红灯反例

- 未做 preflight 就提交图片或视频。
- 视频创建超时后换模型或重复创建任务。
- 自动重试，自动换 Key，自动追加镜头。
- 把本地文件伪装成本地 HTTP 服务上传到公网。
- 把 API Key 写入 prompt、URL、输出 JSON、日志或 manifest。
- 将未审核的人脸、声音、版权角色或音乐直接用于 AI 短剧。
- 在缺少授权链时发布漫剧或短剧。
