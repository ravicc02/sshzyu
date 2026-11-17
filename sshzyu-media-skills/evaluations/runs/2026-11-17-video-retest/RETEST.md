# 视频链路重测（2026-11-17）

## 背景

上一轮 15 秒短剧试跑的 3 条视频全部失败，错误为：

```
HTTP 404: Videos API is not supported for this platform
```

## 根因

套件当时通过进程环境变量 `SSHZYU_API_KEY` 使用 key 280（`124k`，组 16「Web 生图」，platform=`openai`）。

sub2api 网关的 `/v1/videos` 只接受 platform ∈ {`grok`, `composite`, `video`}（`backend/internal/server/routes/gateway.go:92-110`），openai 平台分组必然返回 404，请求未进入调度（该条日志无 `account_id` / `platform` 字段，latency 0ms）。

线上视频能力本身正常：组 104「Seedance」与 108「Test Seedance」（platform=`video`）配置完整，账号 337（上游 `https://newapi.foxfoxiiix.xyz/v1`）active 且可调度，组价格含 `minimax_h3`；当日 15:36 已有客户端用 104 组 key 成功跑通创建、轮询与下载。

## 修复

`sshzyu-video` 改为读取**视频专用**凭据，按顺序：

1. `skills/sshzyu-video/.env` 的 `SSHZYU_VIDEO_API_KEY`
2. 进程环境变量 `SSHZYU_VIDEO_API_KEY`
3. 回退通用 `SSHZYU_API_KEY`，并打印平台不匹配警告

原因：图片（`openai`/`grok`）与视频（`grok`/`composite`/`video`）的平台分组不同，同一把 Key 无法同时满足两者。

## 实测

| 项 | 值 |
|---|---|
| 凭据 | key 879 `sd`，组 104 Seedance，platform=`video`（本地 md5 与线上一致） |
| 任务 ID | `task_sHjUx40oXMCOlsRhIzAB7b2K4fKdNnM1` |
| 模型 / 参数 | `minimax_h3`，5s，480p，9:16 |
| 结果 | `success` |
| 成片 | 480×864，H.264 + AAC，5.17s，768,837 B |
| SHA256 | `6dd657039402fbdb016658d7f9dbd64ff43669c23123d374a50cde7db10e1142` |
| 线上日志 | 全部请求 `account_id=337`、`platform=video`、HTTP 200，含 `/content` 下载 |

产物：`video-20261117T152654830117Z-video-01-5c1de90c/video.mp4`

## 观察

- 首次尝试因本机 `HTTPS_PROXY=127.0.0.1:7890` 未运行而连接被拒，请求未发出；清除代理后成功。CLI 未自动重试，符合设计。
- 104/108 组未关联任何渠道（`channel_groups` 为空），但 `/v1/models` 仍返回 14 个视频模型，说明视频模型展示不依赖 `channel_groups`。
- 104/108 组白名单仍是 14 个 Claude 模型的模板残留（`enabled=false`，不拦截，但无实际意义）。

## 遗留

- `short-drama-produce` 尚未内置 SSHZYU 视频适配器；本次验证的是 `sshzyu-video` CLI 直连，不等于短剧生产任务单已自动接通。
- 宿主侧 Seedance 403（`Seedance requires an OpenAI or composite group`）与套件无关，是宿主自身配置的 key 分组问题。
