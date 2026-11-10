SSHZYU Video API 提供 OpenAI 兼容的异步视频接口。客户端先创建任务，再查询状态，完成后通过内容接口下载。

> 视频任务创建会产生费用。提交超时或返回 5xx 时，先保留原任务 ID 并查询原任务，不要自动再次创建。

## 1. 接入信息

| 项目 | 值 |
| --- | --- |
| Base URL | `https://sshzyu.com/v1` |
| 鉴权方式 | `Authorization: Bearer <你的 API Key>` |
| 创建接口 | `POST /v1/videos` |
| 查询接口 | `GET /v1/videos/{id}` |
| 下载接口 | `GET /v1/videos/{id}/content` |
| 管理入口 | [API 密钥](/keys) |
| 模型价格 | [模型广场](/model-plaza) |

客户端会自动补 `/v1` 时，只填写 `https://sshzyu.com`。不要拼接成 `/v1/v1`。

所有请求使用 UTF-8 JSON。任务 ID 必须使用 SSHZYU 返回的 `id`，不要使用管理后台行号或其他平台的内部任务号。

## 2. 接口总览

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/v1/models` | 查询当前 Key 可见的模型 |
| `POST` | `/v1/videos` | 创建视频任务 |
| `POST` | `/v1/videos/generations` | 创建视频任务的兼容别名 |
| `GET` | `/v1/videos/{id}` | 查询原任务状态 |
| `GET` | `/v1/videos/{id}/content` | 下载已完成视频 |

`GET /v1/models` 不计费。创建视频、查询结果和下载内容需要使用同一个 SSHZYU 账号下的有效 Key。

## 3. 首次连接

先在当前终端设置 Key。示例不会把密钥写入文件。

### 3.1 PowerShell

```powershell
$base = 'https://sshzyu.com/v1'
$apiKey = $env:SSHZYU_API_KEY
if (-not $apiKey) { throw '请先设置当前进程的 SSHZYU_API_KEY' }

$headers = @{ Authorization = "Bearer $apiKey" }
$models = Invoke-RestMethod -Method Get -Uri "$base/models" -Headers $headers
$models.data | Select-Object id
```

### 3.2 curl

```bash
curl --fail-with-body https://sshzyu.com/v1/models \
  -H "Authorization: Bearer $SSHZYU_API_KEY"
```

## 4. 创建视频任务

### 4.1 请求示例

```json
{
  "model": "minimax_h3",
  "prompt": "一只小猫沿着海边慢慢走动，镜头稳定，自然光",
  "seconds": 5,
  "resolution": "768p",
  "aspect_ratio": "16:9"
}
```

```powershell
$videoBody = @{
    model = 'minimax_h3'
    prompt = '一只小猫沿着海边慢慢走动，镜头稳定，自然光'
    seconds = 5
    resolution = '768p'
    aspect_ratio = '16:9'
} | ConvertTo-Json -Depth 10

# 创建请求只执行一次，不要为超时或 5xx 自动重发。
$video = Invoke-RestMethod -Method Post -Uri "$base/videos" `
    -Headers $headers -ContentType 'application/json; charset=utf-8' `
    -Body ([Text.Encoding]::UTF8.GetBytes($videoBody)) -TimeoutSec 600

$taskId = $video.id
if (-not $taskId) { throw '没有取得任务 id，请保留原请求和响应' }
$video
```

创建成功通常返回 HTTP 200。该响应只表示任务已受理，不表示视频已经生成完成。

```json
{
  "id": "task_example",
  "object": "video",
  "model": "minimax_h3",
  "status": "queued",
  "progress": 0,
  "created_at": 1792458000
}
```

请立即保存：

- SSHZYU 任务 `id`
- 原请求 JSON
- 提交时间和时区
- 响应中的请求 ID

### 4.2 通用字段

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `model` | 是 | 当前 Key 可用的精确模型名 |
| `prompt` | 是 | 视频描述 |
| `seconds` | 视模型 | 正整数生成秒数 |
| `duration` | 视模型 | `seconds` 的同义字段，同时填写时必须一致 |
| `resolution` | 视模型 | 输出分辨率档位 |
| `aspect_ratio` | 否 | 输出画幅 |
| `image_urls` | 否 | 参考图片公网 HTTPS 直链 |
| `video_urls` | 否 | 参考视频公网 HTTPS 直链 |
| `audio_urls` | 否 | 参考音频公网 HTTPS 直链 |
| `references` | 否 | 带 `type`、`role`、`url` 的参考素材结构 |

参考素材必须可直接下载。网页播放页、内网地址、本地路径和需要登录的地址均不能作为素材 URL。

## 5. 查询任务

```powershell
$escapedTaskId = [Uri]::EscapeDataString([string]$taskId)
$status = Invoke-RestMethod -Method Get -Uri "$base/videos/$escapedTaskId" `
    -Headers $headers -TimeoutSec 120
$status
```

建议间隔 3 至 5 秒查询。`progress` 是进度数字，不是剩余秒数。

| 状态 | 含义 | 客户端处理 |
| --- | --- | --- |
| `queued` | 已排队 | 继续查询原任务 |
| `in_progress` | 生成中 | 继续查询原任务 |
| `completed` | 已完成 | 下载原任务成片 |
| `failed` | 失败终态 | 保留错误和任务 ID |
| 未知状态 | 无法识别 | 停止自动处理并交管理员核查 |

## 6. 下载视频

```powershell
if ($status.status -eq 'completed') {
    Invoke-WebRequest -Method Get -Uri "$base/videos/$escapedTaskId/content" `
        -Headers $headers -OutFile './generated-video.mp4' -TimeoutSec 900
}
```

```bash
curl --fail-with-body --location \
  -H "Authorization: Bearer $SSHZYU_API_KEY" \
  "https://sshzyu.com/v1/videos/$TASK_ID/content" \
  --output generated-video.mp4
```

下载接口可能返回：

| 响应 | 处理 |
| --- | --- |
| `307` | 跟随 `Location` 到临时签名地址 |
| `200` | 直接读取完整文件 |
| `206` | 按 `Content-Range` 读取分段 |
| `4xx` / `5xx` | 核对任务归属、状态和签名有效期 |

SSHZYU API Key 只发送给 `sshzyu.com`。跨域跳转后不要携带平台 Key、Cookie 或 `Authorization`。

分段下载时保留客户端 `Range`。返回 `206` 才能拼接对应分段，不要重复拼接多个完整的 `200` 响应。

如果查询响应返回 `download_url`，该字段已经指向 SSHZYU 内容入口。可直接请求该地址。

## 7. 视频模型与参数

模型可见性和调用权限由当前 Key 所属分组决定。若模型没有出现在 `GET /v1/models`，先核对 Key、分组和账号权限。

### 7.1 按秒计费模型

价格单位为 USD，按“所选分辨率每秒单价 × 提交生成秒数”计算。参考素材不追加生成秒数。

| 模型 | 时长 | 480p | 720p | 768p | 1080p | 2k | 4k |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `sd-2.0-满血` | 4～15 秒 | `$0.28/s` | `$0.35/s` | - | `$0.90/s` | - | `$3.60/s` |
| `sd-2.5-满血` | 4～30 秒 | `$0.38/s` | `$0.43/s` | - | `$1.20/s` | - | - |
| `sd2-满血-新` | 4～15 秒 | `$0.27/s` | `$0.35/s` | - | - | - | - |
| `sd25-满血-新` | 4～30 秒 | `$0.30/s` | `$0.45/s` | - | - | - | - |
| `minimax_h3` | 4～15 秒 | `$0.156/s` | - | `$0.2008/s` | `$0.2568/s` | `$0.2568/s` | `$0.2904/s` |

`minimax_h3` 必须显式选择 `480p`、`768p`、`1080p`、`2k` 或 `4k`。不要用 `720p` 替代 `768p`。

示例：`minimax_h3`、`768p`、4 秒的费用为：

```text
0.2008 × 4 = $0.8032
```

### 7.2 按次计费模型

每个成功任务收取一次固定费用。任务提交的生成秒数和输出时长通常不改变价格，具体以模型参数为准。

| 模型 | 时长 | 分辨率 | 单次价格 |
| --- | --- | --- | ---: |
| `sd2.5-y` | 4～30 秒 | 720p | `$6.00` |
| `sd2.5720按次稳定不烧心` | 4～30 秒 | 720p | `$8.10` |
| `sd2-v` | 4～15 秒 | 720p | `$3.10` |
| `sd-903-特稳` | 5～15 秒 | 720p | `$1.90` |
| `seedance2.5反直链` | 5～30 秒 | 720p | `$7.10` |
| `videopipe-dola-2.0` | 固定 15 秒 | 服务端决定 | `$1.20` |
| `videopipe-doubao-2.0` | 5、10 或 15 秒 | 服务端决定 | `$1.20` |

### 7.3 按 Token 计费模型

最终费用使用任务完成响应中的 `usage.total_tokens`。`completion_tokens` 不重复相加。

| 模型 | 分辨率 | 无参考视频 | 有参考视频 |
| --- | --- | ---: | ---: |
| `sd2官转` | 720p | `$27.70/百万 Token` | `$16.90/百万 Token` |
| `sd25官转` | 720p | `$42.10/百万 Token` | `$25.30/百万 Token` |
| `doubao-seedance-2-5` | 720p | `$52.60/百万 Token` | `$52.60/百万 Token` |

计算方式：

```text
最终费用 = usage.total_tokens × 每百万 Token 单价 ÷ 1,000,000
```

只有提交参考视频时才使用“有参考视频”档。纯文本、图片参考和仅参考音频使用“无参考视频”档。

### 7.4 画幅与素材限制

| 模型 | 参考图 / 视频 / 音频 | 常用画幅 |
| --- | --- | --- |
| `doubao-seedance-2-5` | 30 / 10 / 10 | `adaptive`、`16:9`、`9:16`、`1:1`、`21:9`、`4:3`、`3:4` |
| `minimax_h3` | 待确认 | 待确认 |
| `sd-2.0-满血` | 9 / 3 / 3 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9`、`9:21` |
| `sd-2.5-满血` | 30 / 10 / 10 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9`、`9:21` |
| `sd-903-特稳` | 9 / 0 / 3 | `16:9`、`9:16`、`1:1` |
| `sd2-v` | 9 / 3 / 3 | `9:16`、`16:9`、`4:3`、`3:4`、`1:1` |
| `sd2-满血-新` | 9 / 3 / 3 | `21:9`、`16:9`、`4:3`、`1:1`、`3:4`、`9:16` |
| `sd2.5-y` | 30 / 10 / 10，总计不超过 30 项 | `auto`、`16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9` |
| `sd2.5720按次稳定不烧心` | 素材传输总量不超过 128 项 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9` |
| `sd25-满血-新` | 30 / 10 / 10 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9`、`9:21` |
| `sd25官转` | 30 / 10 / 10 | `16:9`、`9:16`、`1:1`、`21:9`、`4:3`、`3:4` |
| `sd2官转` | 9 / 3 / 3 | `16:9`、`9:16`、`1:1`、`21:9`、`4:3`、`3:4` |
| `seedance2.5反直链` | 30 / 10 / 10 | `16:9`、`9:16`、`1:1`、`3:4`、`4:3`、`21:9` |
| `videopipe-dola-2.0` | 10 / 0 / 0 | `21:9`、`16:9`、`4:3`、`1:1`、`3:4`、`9:16` |
| `videopipe-doubao-2.0` | 10 / 0 / 0 | `21:9`、`16:9`、`4:3`、`1:1`、`3:4`、`9:16` |

`videopipe-dola-2.0` 和 `videopipe-doubao-2.0` 只接受参考图片，不接受参考视频或参考音频。建议省略 `resolution`，实际输出规格由服务端决定。

### 7.5 MiniMax H3 示例

```json
{
  "model": "minimax_h3",
  "prompt": "一只小猫沿着海边慢慢走动，镜头稳定，自然光",
  "seconds": 5,
  "resolution": "768p",
  "aspect_ratio": "16:9"
}
```

参考素材可使用 `image_urls`、`video_urls`、`audio_urls`，也可使用 `references`：

```json
{
  "model": "minimax_h3",
  "prompt": "参考图片中的角色在海边缓慢行走",
  "seconds": 5,
  "resolution": "1080p",
  "aspect_ratio": "16:9",
  "references": [
    {
      "type": "image",
      "role": "reference",
      "url": "https://assets.example.com/reference.png"
    }
  ]
}
```

两种参考素材写法不要在同一个请求中混用。

## 8. 失败与重试

创建视频没有可靠的客户端幂等去重。以下情况不得自动重新创建：

- HTTP 429
- HTTP 502 或 504
- 网络超时
- 客户端连接断开
- 已取得任务 ID 但查询失败
- 上游返回任务提交结果不明

先保存原请求、请求时间、请求 ID 和任务 ID，再查询原任务或联系管理员。

| HTTP 状态 | 常见含义 | 客户端处理 |
| --- | --- | --- |
| `400` | 模型、参数或素材不支持 | 修改参数后再提交 |
| `401` | Key 无效或缺少鉴权 | 检查 Authorization |
| `403` | Key 权限、模型范围或余额不足 | 检查 Key、分组和余额 |
| `404` | 任务不存在或不属于当前 Key | 使用原 Key 和正确任务 ID |
| `429` | 限流 | 延后查询，不自动重建任务 |
| `502` / `503` / `504` | 上游或调度暂不可用 | 保留原请求并核查任务 |

## 9. 安全与排障

- 不要把长期 API Key 写入前端 JavaScript 或公开代码仓库。
- 不要把完整签名下载地址写入公开日志。
- 不要向对象存储域名发送 SSHZYU API Key。
- 不要用任务 `progress` 推算费用。
- 不要用其他模型的价格重新计算已提交任务。

排障时提供以下信息：

- 入口域名 `sshzyu.com`
- 精确模型名
- 提交时间和时区
- SSHZYU 任务 `id`
- HTTP 状态码
- 请求 ID
- 已脱敏的请求参数
- 错误原文

不要发送完整 Key、密码、登录 Cookie 或未脱敏的参考素材地址。

## 10. 计费说明

当前文档价 = 上游参考基础价 + `$0.1`：

- 按秒模型：每秒单价增加 `$0.1`
- 按次模型：每次价格增加 `$0.1`
- 按 Token 模型：每百万 Token 增加 `$0.1`

以上价格为 SSHZYU 当前基础价，单位为 USD。分组倍率、活动价、余额预扣和最终结算以任务提交时的 SSHZYU 配置及使用记录为准。

已提交任务使用提交时冻结的价格。后续调整价卡不追溯重算历史订单。
