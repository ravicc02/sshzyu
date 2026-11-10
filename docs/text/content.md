SSHZYU 的普通文本与多模态接口兼容 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages。客户端可用同一个 SSHZYU API Key 调用当前分组允许的模型。

> 模型是否可用由 API Key 所属分组决定。调用前先读取 `GET /v1/models`，不要只根据本文示例推断权限。

## 1. 接入信息

| 项目 | 值 |
| --- | --- |
| Base URL | `https://sshzyu.com/v1` |
| Chat Completions | `POST https://sshzyu.com/v1/chat/completions` |
| Responses | `POST https://sshzyu.com/v1/responses` |
| Anthropic Messages | `POST https://sshzyu.com/v1/messages` |
| 模型列表 | `GET https://sshzyu.com/v1/models` |
| 鉴权方式 | `Authorization: Bearer <你的 API Key>` |
| 密钥入口 | [API 密钥](/keys) |
| 模型价格 | [模型广场](/model-plaza) |

客户端会自动追加 `/v1` 时，只填写 `https://sshzyu.com`。不要把地址写成 `/v1/v1`。

## 2. 接口总览

| 方法 | 路径 | 适用协议 | 用途 |
| --- | --- | --- | --- |
| `GET` | `/v1/models` | 通用 | 查询当前 Key 可见的模型 |
| `POST` | `/v1/chat/completions` | Chat Completions | 文本、多模态与流式对话 |
| `POST` | `/v1/responses` | Responses | Responses 原生或兼容调用 |
| `POST` | `/v1/messages` | Anthropic Messages | Anthropic 原生消息格式 |

模型列表接口不计费。对话、图片输入和工具调用按实际请求记录计费。

## 3. 创建 API Key

1. 登录 SSHZYU，打开 [API 密钥](/keys)。
2. 点击创建密钥，选择需要使用模型的分组。
3. 设置名称、额度、IP 限制和有效期。
4. 创建后立即保存密钥，并把密钥放在服务端环境变量中。

测试阶段建议使用独立密钥和小额度。不要把长期密钥写进浏览器 JavaScript、移动应用安装包或公开仓库。

## 4. 模型清单与协议

下表列出当前文档覆盖的公开模型。模型的最终可见性、成功率和价格以当前 Key、分组与模型广场为准。

| 模型 | Chat Completions | Responses | Anthropic Messages | 备注 |
| --- | --- | --- | --- | --- |
| `gpt-6-astra` | 支持 | - | - | 按当前渠道映射调用 |
| `gpt-6.1-sol` | 支持 | - | - | 按当前渠道映射调用 |
| `gpt-6-luna` | 支持 | - | - | 按当前渠道映射调用 |
| `gemini-4-argon` | 支持 | - | - | 按当前渠道映射调用 |
| `gemini-3.8-flash` | 支持 | - | - | 按当前渠道映射调用 |
| `claude-haiku-5-5` | 支持 | - | - | 按当前渠道映射调用 |
| `claude-opus-5-5-low` | 支持 | - | - | 不要套用完整 Opus 的协议能力 |
| `claude-opus-5-5` | 支持 | 支持 | 支持 | 三种协议均可按当前路由调用 |

不要只根据模型名称相似度选择协议。例如 `claude-opus-5-5-low` 不能因为名称接近 `claude-opus-5-5`，就假定支持 Responses 或 Anthropic Messages。

## 5. Chat Completions

### 5.1 请求示例

```json
{
  "model": "gpt-6.1-sol",
  "messages": [
    {
      "role": "user",
      "content": "请用一句话说明 SSHZYU API 的用途。"
    }
  ],
  "temperature": 0.3
}
```

### 5.2 PowerShell

```powershell
$base = 'https://sshzyu.com/v1'
$apiKey = $env:SSHZYU_API_KEY
if (-not $apiKey) { throw '请先设置 SSHZYU_API_KEY' }

$body = @{
    model = 'gpt-6.1-sol'
    messages = @(
        @{
            role = 'user'
            content = '请用一句话说明 SSHZYU API 的用途。'
        }
    )
    temperature = 0.3
} | ConvertTo-Json -Depth 10

$response = Invoke-RestMethod -Method Post -Uri "$base/chat/completions" `
    -Headers @{ Authorization = "Bearer $apiKey" } `
    -ContentType 'application/json; charset=utf-8' `
    -Body ([Text.Encoding]::UTF8.GetBytes($body))

$response.choices[0].message.content
```

### 5.3 curl

```bash
curl --fail-with-body https://sshzyu.com/v1/chat/completions \
  -H "Authorization: Bearer $SSHZYU_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-6.1-sol",
    "messages": [
      {"role": "user", "content": "请用一句话说明 SSHZYU API 的用途。"}
    ]
  }'
```

## 6. Responses

Responses 接口使用 `input` 和 `instructions` 等字段。它适合需要 Responses 原生语义的客户端。

```json
{
  "model": "claude-opus-5-5",
  "instructions": "回答保持简短。",
  "input": "列出两个适合首次接入的测试步骤。"
}
```

```powershell
$responsesBody = @{
    model = 'claude-opus-5-5'
    instructions = '回答保持简短。'
    input = '列出两个适合首次接入的测试步骤。'
} | ConvertTo-Json -Depth 10

$result = Invoke-RestMethod -Method Post -Uri "$base/responses" `
    -Headers @{ Authorization = "Bearer $apiKey" } `
    -ContentType 'application/json; charset=utf-8' `
    -Body ([Text.Encoding]::UTF8.GetBytes($responsesBody))

$result.output_text
```

`output_text` 是常见聚合字段。实际客户端也可以按 `output` 数组读取结构化内容。

## 7. Anthropic Messages

`claude-opus-5-5` 可使用 Anthropic Messages 格式。请求仍使用 SSHZYU API Key 的 Bearer 鉴权。

```json
{
  "model": "claude-opus-5-5",
  "max_tokens": 512,
  "messages": [
    {
      "role": "user",
      "content": "用三点说明如何验证 API 接入。"
    }
  ]
}
```

```bash
curl --fail-with-body https://sshzyu.com/v1/messages \
  -H "Authorization: Bearer $SSHZYU_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-opus-5-5",
    "max_tokens": 512,
    "messages": [
      {"role": "user", "content": "用三点说明如何验证 API 接入。"}
    ]
  }'
```

如果客户端要求填写 Anthropic 原生地址，仍以 SSHZYU 当前密钥页面生成的 Base URL 为准。不要把官方 Anthropic 地址和 SSHZYU 密钥混用。

## 8. 流式响应

Chat Completions 和 Responses 都支持流式输出；模型与分组是否允许流式调用，以当前路由为准。

Chat Completions 请求增加：

```json
{
  "model": "gpt-6.1-sol",
  "stream": true,
  "messages": [
    {
      "role": "user",
      "content": "逐步输出三条接入检查项。"
    }
  ]
}
```

流式响应使用 `text/event-stream`。客户端逐行读取 `data:` 事件，直到收到 `[DONE]`。不要按普通 JSON 一次性解析整个响应。

```text
data: {"choices":[{"delta":{"content":"第一条"}}]}

data: {"choices":[{"delta":{"content":"第二条"}}]}

data: [DONE]
```

## 9. 多模态输入

Chat Completions 可把文本和图片放进同一个用户消息。图片 URL 必须能被上游正常访问；需要登录、带防盗链或只能在内网访问的地址不能使用。

```json
{
  "model": "gpt-6.1-sol",
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "描述这张图片的主题和主要颜色。"
        },
        {
          "type": "image_url",
          "image_url": {
            "url": "https://assets.example.com/reference.jpg"
          }
        }
      ]
    }
  ]
}
```

图片大小、格式、数量和支持字段由模型与上游决定。大图会同时增加上传时间、上下文 Token 和失败概率。正式接入前先用一张小图完成链路测试。

## 10. 失败与排障

| HTTP 状态 | 常见含义 | 客户端处理 |
| --- | --- | --- |
| `400` | 请求字段或模型参数不合法 | 检查 JSON、协议字段和模型参数 |
| `401` | 缺少 Key 或 Key 无效 | 检查 Bearer 前缀与密钥状态 |
| `403` | Key、分组或模型权限不足 | 检查分组、模型白名单和余额 |
| `404` | 路径或模型不存在 | 检查 `/v1`、模型 ID 和当前 Key 权限 |
| `429` | 频率或并发受限 | 停止重试，延后发送 |
| `500` / `502` / `503` / `504` | 服务或上游暂时异常 | 保留请求 ID，稍后重试 |

流式请求已经输出部分内容后，不要盲目自动重试。先保存请求 ID、已接收内容和错误事件，确认是否会重复计费。

排障时提供以下信息：

- Base URL 和请求路径
- 精确模型 ID
- HTTP 状态码
- 请求 ID
- 发生时间与时区
- 已脱敏的请求字段

不要发送完整 API Key、登录密码、Cookie 或包含隐私的原始图片地址。

## 11. 完成检查

- `GET /v1/models` 能看到准备调用的模型。
- 密钥属于正确的分组，并已设置合理额度。
- Chat、Responses 或 Messages 使用了匹配的请求格式。
- 非流式短请求已经成功返回。
- 流式请求能正确处理 `data:` 事件和结束标记。
- 使用记录中的模型、Token 和费用符合预期。
- API Key 没有写入公开客户端或代码仓库。
