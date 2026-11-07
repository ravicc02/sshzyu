---
name: sshzyu-image
description: "用 sshzyu.com 的 gpt-image 系列接口生成或编辑图片。用户要求画图、生图、出图、封面、插画时使用；支持单提示词多图、多提示词并发生图、多提示词并发改图、参考图、蒙版、markup、尺寸、质量、PNG/JPEG/WebP/GIF/SVG，并保存实际用量。"
---

# SSHZYU 生图

使用 `scripts/sshzyu_image.py`。脚本只依赖 Python 标准库，默认模型为 `gpt-image-2.6`。先完成 preflight，再读取模型页并执行。示例使用 Windows 的 `py -3.12`；macOS/Linux 将同一位置替换为 `python3`。

## 🔴 CHECKPOINT · 强制 Preflight

任何真实生成或编辑前必须暂停，向用户问清以下项目并复述最终计划：

- 用途与画面内容。
- 模型，例如 `gpt-image-2.6`、`gpt-image-2.5-flare`。
- 宽高比和具体尺寸档位，例如 `1:1 / 1024x1024 / 1K`。
- 质量，例如 `high`，以及需要几 K 的成品尺寸。
- PNG、JPEG、WebP、GIF 或 SVG。
- prompt 数量、每个 prompt 的张数和预计总张数。
- 是否上传参考图；上传哪些文件。
- 多任务是否并发，以及并发数。
- 是否可能产生多笔计费。

🔴 CHECKPOINT · 🛑 STOP：缺少任一项或用户尚未明确确认时，禁止调用 `/images/generations` 或 `/images/edits`。

用户可以一次性提供全部信息，但仍需在真正调用前复述计划并取得明确确认。缺少模型、比例、尺寸或质量时停止询问，不按默认值擅自开跑。确认后先执行 `--dry-run` 核对，再在真实命令加入 `--confirm`。脚本缺少 `--confirm` 时会拒绝真实请求。

## 路由

| 用户需求 | 执行方式 |
| --- | --- |
| 一个 prompt 生成多张 | 一次请求传 `--n` |
| 多个 prompt 各生成一张或多张 | 多个 `--prompt-file` 或 JSON manifest，脚本并发请求 |
| 多个 prompt 分别修改多张图 | manifest 内每个任务绑定自己的 `images`，脚本并发请求 |
| 一个 prompt 使用同一组参考图编辑 | 单个 prompt 加重复 `--image` |
| GIF / SVG | 仅 `gpt-image-2.6` 生成模式；先读模型页 |

详细模型能力见 [模型路由](references/models/README.md)，参数矩阵与 manifest 格式见 [标准参数矩阵](references/parameter-matrix.md)，通用 API 约束见 [API 参考](references/api_reference.md)。

## 执行

1. 完成强制 preflight。不得自行扩大任务数、图片数、并发数或参考图范围。
2. 需要模型参数时读取对应模型页。当前 sshzyu 列表包含 `gpt-image-2.6`、`gpt-image-2.5`、`gpt-image-2.5-flare`、`gpt-image-2.5-sunburst`、`gpt-image-2`；执行前可用 `--models` 重新核对。
3. 单提示词多图只发一次请求：

```bash
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_image.py" \
  "一只戴墨镜的橘猫，暖色调，无文字" \
  --model gpt-image-2.6 --size 1024x1024 --quality high \
  --n 4 --out "TASK_OUTPUT_DIR" --dry-run
```

确认计划后，移除 `--dry-run` 并加入 `--confirm` 执行。

4. 多提示词生成时，每个提示词文件对应一个并发任务：

```bash
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_image.py" \
  --prompt-file "cover-a.txt" \
  --prompt-file "cover-b.txt" \
  --model gpt-image-2.6 --size 1536x1024 --quality high \
  --n 2 --workers 2 --out "TASK_OUTPUT_DIR" --dry-run
```

5. 多提示词改多张图必须使用 manifest，显式绑定每个 prompt 与参考图：

```json
{
  "defaults": {
    "model": "gpt-image-2.6",
    "size": "1024x1024",
    "quality": "high",
    "output_format": "png"
  },
  "jobs": [
    {
      "id": "product-a",
      "prompt": "把背景改成暖灰色摄影棚，保留产品标签与包装不变",
      "images": ["inputs/product-a.png"]
    },
    {
      "id": "product-b",
      "prompt": "把背景改成暖灰色摄影棚，保留产品标签与包装不变",
      "images": ["inputs/product-b.png"],
      "n": 2
    }
  ]
}
```

```bash
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_image.py" \
  --manifest "edit-jobs.json" --workers 2 --out "TASK_OUTPUT_DIR" --dry-run
```

manifest 内相对路径以 manifest 所在目录为基准。dry-run 不读 Key、不联网、不计费。

6. 默认 4 个并发请求，上限 8，单次最多 64 个任务。不同 prompt 通过 `ThreadPoolExecutor` 并发；任务失败不会取消其他任务，失败结果写入汇总。
7. 输出目录每次自动创建 `run-时间戳-短ID/`，每任务一个子目录，内含图片、`job-result.json` 和 batch 级 `batch-summary.json`。
8. 打开图片检查主体、构图、文字、实际尺寸和格式。失败时如实说明，不自动重试或追加计费。

### 状态契约

- `success`：请求图片全部返回，格式和尺寸与请求一致。
- `partial`：保留了有效图片，但存在短缺、无效图片或尺寸不一致；退出码为 `2`。
- `error`：请求失败或没有可保存图片；退出码为 `1`。
- 全部完整成功时退出码为 `0`。查看 `batch-summary.json` 的 `jobs_succeeded`、`jobs_partial`、`jobs_failed`，不要只看进程退出码。

## 模型参数

- `complexity=low|medium|high`、GIF、SVG 只对 `gpt-image-2.6` 生效。
- `gpt-image-2.5-flare` 用于快速迭代；`gpt-image-2.5-sunburst` 用于精细视觉与编辑。
- 支持 `quality=auto|low|medium|high|xhigh|max` 的模型已在脚本中分档校验。
- 生成格式：2.6 为 PNG/JPEG/WebP/GIF/SVG；2.5 与 2 为 PNG/JPEG/WebP。
- 编辑格式：PNG/JPEG/WebP。GIF 和 SVG 不做编辑输出。
- GIF 参数：`frame_count=2..16`，`frame_duration_ms=20..2000`。
- SVG 必须 `n=1`，不用于编辑，也不接受 SVG 作为输入图。

## 凭据与边界

- Key 唯一推荐位置：本技能目录 `.env` 中的 `SSHZYU_API_KEY=...`。
- 不把 Key 写入 `SKILL.md`、脚本、输出目录、job-result、日志、对话或 `.skill` 包。
- `.env` 已加入 `.gitignore`。打包分发前必须排除 `.env`；不要把当前包含真实 Key 的目录直接打包。
- 读取顺序：`.env` → 进程环境变量 → 旧 Windows 凭据管理器 → 旧 `key.txt`。后三项仅作兼容。
- 首次或更换 Key：运行 `--store-key`，输入不回显；只有明确要求覆盖时加 `--replace-env`。
- 旧 Windows 凭据可运行 `--migrate-credential-to-env` 迁移到 `.env`，命令不会打印 Key。
- `--doctor` 只检查来源；`--models` 只读取模型列表。
- 请求只发往 `https://sshzyu.com/v1`，阻止重定向并验证 TLS。
- 不自动重试。超时可能已经计费，先查服务商记录。
- 参考图会上传 sshzyu.com。只上传用户明确授权用于编辑的材料，不读取凭据、私人素材或浏览器 Cookies。
- 本技能不自动发布、不充值、不修改账户。

## 失败恢复

| 触发条件 | 一线修复 | 仍失败兜底 |
| --- | --- | --- |
| `--dry-run`、manifest 或输入文件校验失败 | 按报错修正一次模型、尺寸、格式或路径，再执行同一 dry-run | 停止，不带 `--confirm`，不产生请求 |
| `images_saved < images_requested` | 脚本判为 `partial` 并返回退出码 `2`；保留图片、usage 和 warnings，报告缺失张数 | 未获得新的明确授权不重发；不得把 job `success` 或退出码 `0` 当成完整交付 |
| 实际格式或尺寸与请求不同 | 报告请求值与实际值，保留有效文件 | 未获得新的明确授权不补发或替换模型 |
| `401` / `403` | 分别修正 `.env` Key / 分组图片权限，再跑一次 `--doctor` 或 `--models` | 仍失败则停止，不尝试默认 Key、其他分组或绕过权限 |
| `402` | 停止，报告余额不足 | 不自动充值，不换账户，不重试 |
| `422` | 读取对应模型页并运行 `--models`，提出一份修正后的参数计划 | 请求用户重新确认；仍返回 422 则停止，不自动降模型或删参数重发 |
| `429` / `5xx` / 超时 | 停止，报告可能已计费，并让用户检查服务商用量 | 只有用户明确授权新任务后才可再次请求 |
| 参考图、mask 或 markup 无效 | 停止并说明格式、尺寸或透明度要求，由用户决定是否替换文件 | 不上传替代文件，不将私密素材发给第三方试错 |

## 红灯反例

以下动作禁止执行：

- 未完成 preflight、未取得用户明确确认就带 `--confirm` 发起请求。
- 用户没有指定模型、比例、尺寸或质量时，用脚本默认值代替用户决策。
- 失败后自动重试、自动换模型、自动降 quality、自动拆单或追加图片。
- 把脚本 job `success` 当成最终成功，而忽略 shortage、尺寸差异、warnings 或 batch summary。
- 在 `images_saved < images_requested` 时声称全部完成。
- 用命令行参数传 Key、打印 `.env`、把 `.env` 写进日志或打包进 `.skill`。
- 上传用户未明确授权的参考图、私人素材、凭据或浏览器 Cookies。
- 自动下载响应 URL、调用非 `https://sshzyu.com/v1` 地址或绕过 401/402/403。
- 自动发布图片、充值、修改账户或替代用户确认新增费用。
