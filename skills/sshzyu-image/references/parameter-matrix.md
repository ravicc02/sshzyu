# 标准参数矩阵

## Preflight

真实请求前确认：模型、宽高比、具体尺寸、K 档、quality、输出格式、prompt 数、每 prompt 张数、总张数、参考图、并发数。

`K` 是站内尺寸/计费档位，不是 `quality` 字段。向用户确认时同时写清比例和像素尺寸，例如：

```text
1:1，2K，2048x2048
16:9，2K，2560x1440（按 4K 计费档）
```

## 尺寸矩阵

| 档位 | 1:1 | 3:2 | 2:3 | 16:9 | 9:16 | 4:3 | 3:4 | 21:9 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1K | 1024x1024 | 1536x1024 | 1024x1536 | 1280x720 | 720x1280 | 1024x768 | 768x1024 | 1280x544 |
| 2K | 2048x2048 | 2160x1440 | 1440x2160 | 2560x1440 | 1440x2560 | 2048x1536 | 1536x2048 | 2560x1088 |
| 4K | 2880x2880 | 3456x2304 | 2304x3456 | 3840x2160 | 2160x3840 | 3200x2400 | 2400x3200 | 3840x1600 |

计费档按请求尺寸最长边估算：`<=1024` 为 1K，`<=2048` 为 2K，`>2048` 为 4K；`auto` 按 2K。该结果只用于风险提示，不是货币金额。最终以服务商记录和响应 `usage` 为准。

站内也接受其他正整数宽高，但脚本不会保证服务端按请求尺寸返回。实际尺寸必须写回结果。

## 模型能力

| 模型 | 定位 | quality | 生成格式 | 编辑格式 | 2.6 专属 |
| --- | --- | --- | --- | --- | --- |
| `gpt-image-2.6` | 新模型，精细制作 | auto/low/medium/high/xhigh/max | PNG/JPEG/WebP/GIF/SVG | PNG/JPEG/WebP | complexity、GIF、SVG、markup |
| `gpt-image-2.5-sunburst` | 精细构图与编辑 | auto/low/medium/high/xhigh/max | PNG/JPEG/WebP | PNG/JPEG/WebP | 无 |
| `gpt-image-2.5-flare` | 快速迭代 | auto/low/medium/high/xhigh/max | PNG/JPEG/WebP | PNG/JPEG/WebP | 无 |
| `gpt-image-2.5` | 2.5 通用别名 | auto/low/medium/high/xhigh/max | PNG/JPEG/WebP | PNG/JPEG/WebP | 无 |
| `gpt-image-2` | 基础模型 | auto/low/medium/high | PNG/JPEG/WebP | PNG/JPEG/WebP | 无 |

代理可能比上游文档滞后。出现 `422` 时停止并询问用户是否降级模型或移除新参数，不自动换模型。

## 输出格式

| 格式 | 生成 | 编辑 | 约束 |
| --- | --- | --- | --- |
| PNG | 是 | 是 | 默认；透明背景可用 |
| JPEG | 是 | 是 | 可用 compression |
| WebP | 是 | 是 | 可用 compression |
| GIF | 2.6 是 | 否 | `frame_count`、`frame_duration_ms` |
| SVG | 2.6 是 | 否 | `n=1`，解码后为 UTF-8 XML |

## Manifest

```json
{
  "defaults": {
    "model": "gpt-image-2.6",
    "size": "1024x1024",
    "quality": "high",
    "output_format": "png",
    "complexity": "medium"
  },
  "jobs": [
    {
      "id": "generate-a",
      "prompt": "第一张提示词",
      "n": 2
    },
    {
      "id": "generate-b",
      "prompt_file": "prompts/b.txt",
      "size": "1536x1024",
      "n": 1
    },
    {
      "id": "edit-a",
      "mode": "edit",
      "prompt": "只替换背景，保留主体",
      "images": ["inputs/a.png"]
    },
    {
      "id": "edit-b",
      "mode": "edit",
      "prompt": "只替换背景，保留主体",
      "images": ["inputs/b-1.png", "inputs/b-2.png"],
      "mask": "inputs/b-mask.png"
    }
  ]
}
```

字段优先级：任务自身 > CLI 显式参数 > manifest `defaults` > 脚本内置默认值。

任务字段：

- `id`、`prompt` 或 `prompt_file`。
- `model`、`size`、`quality`、`output_format`、`n`。
- `images`、`mask`、`markup`。
- `complexity`、`frame_count`、`frame_duration_ms`。
- `background`、`moderation`、`output_compression`。
- `safety_identifier`、`timeout`、`mode`。

所有相对路径以 manifest 所在目录为基准。

## CLI

```text
--model                 模型
--size                  具体宽高或 auto
--quality               auto/low/medium/high/xhigh/max
--format                png/jpeg/webp/gif/svg
--n                     每个任务的图片数 1-10
--workers               并发数 1-8
--prompt-file           可重复，每个文件一个任务
--manifest              JSON 批处理
--image                 参考图，可重复
--mask                  PNG 蒙版
--markup                透明 PNG 标记层
--complexity            2.6 的 low/medium/high
--frame-count           GIF 帧数
--frame-duration-ms     GIF 每帧时长
--dry-run               只展开任务，不计费
--confirm               用户确认计划后才允许真实请求
```

单 prompt 多图使用一个请求和 `n`。多 prompt 使用并发任务；不能用同一错误的结果反复重试。
