# SSHZYU 图片接口

来源：`https://sshzyu.com/docs/`，结合 2026-11-06 `GET /v1/models` 实测。服务端是 OpenAI Images API 风格的代理；最终支持范围以真实模型列表和响应为准。

## 连接

- Base URL：`https://sshzyu.com/v1`
- 认证：`Authorization: Bearer` 加运行时读取的 API Key。
- User-Agent：`sshzyu-image-skill/1.1`
- 模型列表：`GET /models`
- 文生图：`POST /images/generations`，JSON。
- 编辑：`POST /images/edits`，multipart，`image[]` 最多 16 张。
- 请求禁止重定向，只允许上述三个路径。

## 生成字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `model` | string | `gpt-image-` 家族，默认 `gpt-image-2.6` |
| `prompt` | string | 必填，最多 32000 字符 |
| `size` | string | `auto` 或 `宽x高` |
| `quality` | string | `auto/low/medium/high`；2.5、2.6 还支持 `xhigh/max` |
| `output_format` | string | 2.6 支持 `png/jpeg/webp/gif/svg`；其他模型通常为前三者 |
| `output_compression` | int | `0-100`，仅 JPEG/WebP |
| `background` | string | `auto/transparent/opaque` |
| `n` | int | 1-10，同一 prompt 的张数 |
| `moderation` | string | `auto/low` |
| `complexity` | string | 2.6 支持 `low/medium/high`，省略时用服务端默认值 |
| `frame_count` | int | GIF 2-16，默认 9 |
| `frame_duration_ms` | int | GIF 20-2000，默认 200 |
| `safety_identifier` | string | 可选，最多 128 字符 |

## 编辑字段

编辑使用同一组通用字段，并通过 multipart 上传：

- `image[]`：1-16 张参考图。
- `mask`：可选 PNG，必须与第一张图同宽高。
- `markup`：可选透明 PNG，必须与第一张图同宽高；2.5/2.6，取决于代理是否透传。
- `mask` 与 `markup` 不能同时使用。
- 编辑输出只允许 PNG/JPEG/WebP；不使用 GIF 或 SVG。

## 响应

```json
{
  "created": 1759000000,
  "data": [
    {
      "b64_json": "<base64>",
      "revised_prompt": "..."
    }
  ],
  "usage": {
    "input_tokens": 12,
    "output_tokens": 4160,
    "total_tokens": 4172
  }
}
```

只接受 `data[i].b64_json`，不自动下载响应中的任意 URL。保存前校验 PNG/JPEG/WebP/GIF 魔数与尺寸，或解析 SVG XML。

## 批处理与并发

- 单 prompt 多图：一次请求使用 `n`，不按张数循环。
- 多 prompt：每个 prompt 一个任务，由 `ThreadPoolExecutor` 并发。
- 多 prompt 改多图：manifest 每个任务绑定自己的 `images`。
- 每任务请求数固定为 1，不自动重试。
- 并发上限 8，单批任务上限 64。

## 凭据

唯一推荐来源是技能根目录 `.env`：

```dotenv
SSHZYU_API_KEY=...
```

读取顺序：

1. 技能根目录 `.env`
2. 进程环境变量 `SSHZYU_API_KEY`
3. 旧 Windows 凭据管理器，仅兼容
4. 旧 `key.txt`，仅兼容

`--store-key` 写入 `.env`，不打印 Key；`--migrate-credential-to-env` 可迁移旧 Windows 凭据。不得打包 `.env`。

## 错误

- `401`：认证失败。
- `402`：余额不足。
- `403`：分组没有图片权限。
- `413`：上传体积过大。
- `422`：模型或字段不兼容。
- `429`：限流。
- 超时/5xx：可能已经生成或计费，先核对服务商记录，不自动重发。
- 返回张数或实际尺寸不同：保留有效结果，报告差异，不自动补发。

本地限制：响应上限 128 MiB、单张上传 32 MiB、总上传 128 MiB。该限制不是服务商官方承诺。
