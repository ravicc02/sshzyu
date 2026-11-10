SSHZYU 生图接口兼容 OpenAI Images API。客户端可以提交文本生成图片，也可以用参考图完成改图、风格迁移和局部重绘。

> 生图请求会产生费用。提交前先确认当前 Key 所属分组、模型权限和模型广场价卡。

## 1. 接入信息

| 项目 | 值 |
| --- | --- |
| Base URL | `https://sshzyu.com/v1` |
| 图片生成 | `POST /v1/images/generations` |
| 图片编辑 | `POST /v1/images/edits` |
| 异步生成 | `POST /v1/images/generations/async` |
| 异步编辑 | `POST /v1/images/edits/async` |
| 查询任务 | `GET /v1/images/tasks/{task_id}` |
| 鉴权方式 | `Authorization: Bearer <你的 API Key>` |
| 密钥入口 | [API 密钥](/keys) |
| 模型价格 | [模型广场](/model-plaza) |

异步任务入口需要当前部署开启对象存储。若接口返回 `404` 和 `async image tasks are not enabled`，改用同步接口或联系管理员。

## 2. 生图模型

当前文档覆盖以下生图模型：

| 模型 | 主要用途 | 当前参考价 |
| --- | --- | ---: |
| `gemini-3-pro-image-preview` | 文生图、参考图编辑、高分辨率输出 | `$0.20/次` |
| `nano-banana-2.1` | 文生图、参考图编辑、快速出图 | `$0.20/次` |

参考价按上游基础价加 `$0.1` 设置，单位为 USD。分组倍率、活动价、失败退款规则和最终结算以提交时的 SSHZYU 配置及使用记录为准。

模型是否可见以 `GET /v1/models` 为准。不要因为模型名称相似，就把其他平台的参数直接套用到 SSHZYU。

## 3. 图片生成

### 3.1 请求示例

```json
{
  "model": "gemini-3-pro-image-preview",
  "prompt": "一只白色纸船放在蓝色桌面上，柔和侧光，极简摄影",
  "resolution": "1K",
  "size": "1:1",
  "quality": "high",
  "n": 1
}
```

### 3.2 PowerShell

```powershell
$base = 'https://sshzyu.com/v1'
$apiKey = $env:SSHZYU_API_KEY
if (-not $apiKey) { throw '请先设置 SSHZYU_API_KEY' }

$idempotencyKey = [guid]::NewGuid().ToString()
$body = @{
    model = 'gemini-3-pro-image-preview'
    prompt = '一只白色纸船放在蓝色桌面上，柔和侧光，极简摄影'
    resolution = '1K'
    size = '1:1'
    quality = 'high'
    n = 1
} | ConvertTo-Json -Depth 10

$result = Invoke-RestMethod -Method Post -Uri "$base/images/generations" `
    -Headers @{
        Authorization = "Bearer $apiKey"
        'Idempotency-Key' = $idempotencyKey
    } `
    -ContentType 'application/json; charset=utf-8' `
    -Body ([Text.Encoding]::UTF8.GetBytes($body)) `
    -TimeoutSec 600

$result
```

### 3.3 curl

```bash
curl --fail-with-body https://sshzyu.com/v1/images/generations \
  -H "Authorization: Bearer $SSHZYU_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: image-$(date +%s)-001" \
  -d '{
    "model": "gemini-3-pro-image-preview",
    "prompt": "一只白色纸船放在蓝色桌面上，柔和侧光，极简摄影",
    "resolution": "1K",
    "size": "1:1",
    "quality": "high",
    "n": 1
  }'
```

每次新任务使用新的 `Idempotency-Key`。网络超时后恢复同一任务时，重放完全相同的请求体，并使用原幂等键。

## 4. 图片编辑

图片编辑使用 `POST /v1/images/edits`，请求类型通常为 `multipart/form-data`。

```bash
curl --fail-with-body https://sshzyu.com/v1/images/edits \
  -H "Authorization: Bearer $SSHZYU_API_KEY" \
  -H "Idempotency-Key: edit-$(date +%s)-001" \
  -F "model=gemini-3-pro-image-preview" \
  -F "prompt=保留主体，把背景改为浅灰色摄影棚" \
  -F "resolution=2K" \
  -F "size=1:1" \
  -F "quality=high" \
  -F "image=@./input.png"
```

参考图必须清晰、无损坏，并符合当前模型的格式与尺寸限制。不要把本地路径、内网地址或需要登录的链接作为远程图片 URL。

## 5. 参数说明

| 参数 | 可选值 | 说明 |
| --- | --- | --- |
| `model` | 当前 Key 可见的生图模型 | 使用精确模型 ID |
| `prompt` | 文本 | 描述主体、构图、光线和风格 |
| `resolution` | `1K`、`2K`、`4K` | 默认 `1K`，实际支持档位以模型为准 |
| `size` | `1:1`、`3:2`、`2:3`、`4:3`、`3:4`、`16:9`、`9:16` | 也可使用像素尺寸 `WxH` |
| `quality` | `low`、`medium`、`high`、`auto` | 质量越高，耗时和失败概率通常越高 |
| `n` | `1` | 当前每次请求生成一张 |

`resolution` 与 `size` 控制不同维度。`resolution` 表示清晰度档位，`size` 表示画幅或像素尺寸。两者不要混用。

## 6. 参考图限制

| 项目 | 限制 |
| --- | --- |
| JSON 请求中的 `images` | 最多 4 个 Data URL |
| 单张参考图解码后 | 不超过 `10 MiB` |
| 全部参考图解码后 | 不超过 `20 MiB` |
| 完整请求体 | 不超过 `28 MiB` |
| `Idempotency-Key` | 最长 255 个字符 |

大图会先增加 Base64 体积，再进入请求体。28 MiB 请求上限不等于 28 MiB 原图上限，实际可用尺寸要按 Base64 膨胀后的体积计算。

## 7. 幂等与异步恢复

收费图片请求应携带稳定的 `Idempotency-Key`。推荐格式：

```text
<业务名>-<任务唯一值>-<序号>
```

例如：

```text
product-cover-20261110-001
```

同一个业务任务重试时继续使用原键。创建新任务时不要复用旧键，否则可能得到原任务结果，而不是新图片。

需要异步执行时使用 `/async` 入口：

```json
{
  "model": "nano-banana-2.1",
  "prompt": "蓝色背景上的白色陶瓷杯，电商主图",
  "resolution": "2K",
  "size": "1:1",
  "quality": "high",
  "n": 1
}
```

```powershell
$task = Invoke-RestMethod -Method Post -Uri "$base/images/generations/async" `
    -Headers @{
        Authorization = "Bearer $apiKey"
        'Idempotency-Key' = $idempotencyKey
    } `
    -ContentType 'application/json; charset=utf-8' `
    -Body ([Text.Encoding]::UTF8.GetBytes($body))

$task.id
$task.poll_url
```

异步提交成功通常返回 `202 Accepted`。按响应中的 `poll_url` 查询，没有 `poll_url` 时使用：

```text
GET /v1/images/tasks/{task_id}
```

任务进入 `processing` 时，按 `Retry-After` 延后查询。进入 `completed` 后读取结果；进入 `failed` 后保存错误并结束，不要自动创建同内容的新付费任务。

部分兼容客户端会发送 `Prefer: respond-async`。SSHZYU 当前异步任务使用独立 `/async` 入口；客户端应以实际响应状态和 `poll_url` 为准。

## 8. 下载与结果格式

同步响应通常是 OpenAI 兼容 JSON。客户端应从当前响应中读取图片 URL、Base64 内容或 Data URL。

```powershell
$json = $result | ConvertTo-Json -Depth 30
$json
```

如果返回 Data URL，客户端按下面的结构拆分：

```text
data:image/png;base64,<图片数据>
```

下载远程图片时，只对图片托管域名发送下载请求。不要附带 SSHZYU API Key、Cookie 或 `Idempotency-Key`。

图片文件名应使用任务 ID 或业务 ID，不要把临时签名 URL 直接当长期资源地址。

## 9. 计费说明

当前文档参考价为 `$0.20/次`：

```text
最终费用 = 单次基础价 × 分组倍率 × 实际生成次数
```

当前 `n` 固定为 `1`。一次新请求生成一个新任务。使用相同 `Idempotency-Key` 恢复原任务时，不应重复创建新计费任务。

以下情况以使用记录为准：

- 分组倍率与活动折扣
- 失败任务的退款或结算结果
- 不同分辨率和质量档位的实际价格
- 上游变更后调整的新价格

不要用其他生图模型的价格反推历史订单。

## 10. 错误处理

| HTTP 状态 | 常见含义 | 客户端处理 |
| --- | --- | --- |
| `400` | 参数、图片格式或尺寸不合法 | 修改请求，不自动重试 |
| `401` | Key 缺失或无效 | 检查 Bearer 鉴权 |
| `403` | 分组未开启生图或权限不足 | 检查 Key、分组和生图开关 |
| `404` | 模型或异步任务不存在 | 检查模型 ID 和任务归属 |
| `409` | 幂等键冲突或任务状态冲突 | 查询原任务，不换键重发 |
| `413` | 请求体或参考图超过限制 | 压缩图片并重试原业务任务 |
| `429` | 频率或图片并发受限 | 延后重试，保留原幂等键 |
| `502` / `503` / `504` | 上游或任务服务暂时异常 | 保留任务 ID 与请求 ID |

收到超时、`502` 或 `504` 时，先查询是否已经取得任务 ID。没有可靠结果时再联系管理员核对，不要连续创建多个付费任务。

## 11. 安全建议

- API Key 只保存在服务端环境变量或密钥管理系统中。
- 参考图上传前移除身份证号、住址和其他敏感信息。
- 不在公开日志中记录完整 Data URL 和签名下载地址。
- 不向图片 CDN 或对象存储域名发送 SSHZYU API Key。
- 定期检查使用记录中的图片数量、分辨率档位和实际扣费。
