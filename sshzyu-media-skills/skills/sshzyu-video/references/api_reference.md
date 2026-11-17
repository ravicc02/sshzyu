# SSHZYU 视频 API

来源：`https://sshzyu.com/docs/vedio/content.md`，2026-11-15 核对。

## 接入

| 项目 | 值 |
| --- | --- |
| Base URL | `https://sshzyu.com/v1` |
| 鉴权 | `Authorization: Bearer <API Key>` |
| 创建 | `POST /videos` |
| 兼容创建别名 | `POST /videos/generations` |
| 查询 | `GET /videos/{id}` |
| 下载 | `GET /videos/{id}/content` |
| 模型列表 | `GET /models` |

任务 ID 只使用 SSHZYU 响应中的 `id`。不要使用后台行号或其他平台的内部任务号。

## 创建请求

最小请求：

```json
{
  "model": "minimax_h3",
  "prompt": "海边空镜，缓慢推镜，自然光",
  "seconds": 4,
  "resolution": "480p",
  "aspect_ratio": "16:9"
}
```

通用字段：

| 字段 | 说明 |
| --- | --- |
| `model` | 当前 Key 可用的精确模型名，必填 |
| `prompt` | 视频提示词，必填 |
| `seconds` | 正整数生成秒数 |
| `duration` | `seconds` 的同义字段，同时填写时必须一致 |
| `resolution` | 输出分辨率档位 |
| `aspect_ratio` | 输出画幅 |
| `image_urls` | 公网 HTTPS 参考图片数组 |
| `video_urls` | 公网 HTTPS 参考视频数组 |
| `audio_urls` | 公网 HTTPS 参考音频数组 |
| `references` | 带 `type`、`role`、`url` 的参考素材数组 |

`image_urls`、`video_urls`、`audio_urls` 与 `references` 是两种写法，不得在同一个请求中混用。

参考素材必须是可直接读取的公网 HTTPS 地址。以下地址不可用：

- 本地路径。
- `localhost`、回环地址、内网 IP。
- 需要登录或 Cookie 的地址。
- 网页播放页，而不是媒体文件直链。
- 带用户名和密码的 URL。

## 参考素材对象

```json
{
  "references": [
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
}
```

`type` 只能是 `image`、`video` 或 `audio`。`role` 的具体可用值由目标模型及上游决定；常见值为 `reference`、`first_frame` 和 `last_frame`。

## 创建响应

HTTP 200 只表示任务受理：

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

立即保存：

- 任务 `id`。
- 原请求 JSON。
- 提交时间、时区和请求 ID。
- 原始响应。

## 查询状态

建议每 3 秒至 5 秒查询一次。

| 状态 | 处理 |
| --- | --- |
| `queued` | 继续查询 |
| `in_progress` | 继续查询 |
| `completed` | 下载原任务成片 |
| `failed` | 保存错误和任务 ID，不自动重试 |
| 未知状态 | 停止自动处理，交管理员核查 |

`progress` 是进度值，不是剩余秒数，也不能用于计算费用。

## 下载

客户端请求：

```text
GET https://sshzyu.com/v1/videos/{id}/content
Authorization: Bearer <API Key>
```

响应可能是：

- `200`：完整文件。
- `206`：分段内容，需要检查 `Content-Range`。
- `307`：跳转到临时签名地址。
- `4xx` / `5xx`：检查任务归属、状态和签名有效期。

安全要求：

1. API Key 只发送给 `sshzyu.com`。
2. 跨源跳转后移除 `Authorization`、Cookie 和其他敏感请求头。
3. 不把完整签名下载 URL 写入日志。
4. 先写临时文件，校验视频魔数和大小后再原子改名。
5. 不覆盖已有文件。

## 失败处理

创建视频没有可靠的客户端幂等去重。以下情况不得自动重新创建：

- `429`
- `502`、`503`、`504`
- 网络超时
- 客户端连接断开
- 已取得任务 ID 但查询失败
- 上游返回任务提交结果不明

先查询原任务：

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" video wait "TASK_ID"
```

只有用户明确授权新任务后，才能重新创建。

## 常用命令

```powershell
# 只查询
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" video get "TASK_ID"

# 等待并下载
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" video wait "TASK_ID" --download

# 直接下载已完成任务
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_media.py" video download "TASK_ID" --out "OUTPUT.mp4"
```
