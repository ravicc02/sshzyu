# SSHZYU 媒体路由契约

本文件是 `sshzyu-media` 与内置短剧套件、独立生图技能之间的唯一媒体路由约定。

## 用户意图到技能的闭环

```text
用户想生图
  → $sshzyu-media（总入口）
  → $sshzyu-co-image（独立生图子技能）
  → https://sshzyu.com/v1/images/generations 或 /images/edits

用户想直接生视频
  → $sshzyu-media（总入口）
  → scripts/sshzyu_video.py
  → https://sshzyu.com/v1/videos

用户想做漫剧/短剧
  → $sshzyu-media/drama（源 drama skills 完整工作流）
  → $short-drama-* 阶段技能
  → $short-drama-produce
  → 图片需求：$sshzyu-co-image → https://sshzyu.com/v1/images/*
  → 视频需求：sshzyu-media 视频路由 → https://sshzyu.com/v1/videos
  → short-drama-edit → 成片
```

## 不允许的旁路

- 短剧阶段技能不得直接调用图片供应商 API。
- 视频技能不得复制生图实现、Key 读取、图片模型表或图片结果校验。
- 短剧生产不得把 `OPENAI_API_KEY`、`ARK_API_KEY`、`MINIMAX_API_KEY` 等供应商凭据作为 SSHZYU 图片或视频的运行凭据。
- `drama/skills/short-drama-produce/scripts/provider_adapters.py` 中的原项目直连供应商适配器仅作为历史兼容代码保留；不得作为 SSHZYU 生产路由使用。新任务必须通过本契约指定的 `$sshzyu-co-image` 或 `sshzyu-media` 入口提交。
- 任何真实请求仍须遵守总入口和子技能的 preflight、dry-run、明确确认、单次提交与失败恢复规则。

## 路由判定

| 需求 | 唯一入口 | 唯一服务端点 |
| --- | --- | --- |
| 文生图、改图、多图、mask、markup | `$sshzyu-co-image` | `https://sshzyu.com/v1/images/*` |
| 文生视频、图生视频、首尾帧、视频参考 | `sshzyu-media` 的 `video` | `https://sshzyu.com/v1/videos` |
| 短剧中的角色/场景/道具/首帧/尾帧 | `$sshzyu-co-image` | `https://sshzyu.com/v1/images/*` |
| 短剧中的镜头视频 | `sshzyu-media` 的 `video` | `https://sshzyu.com/v1/videos` |
| 短剧剪辑和拼接 | `short-drama-edit` | 本地 `ffmpeg` / `ffprobe` |

兼容入口 `sshzyu-media/scripts/sshzyu_image.py` 只负责把旧调用转发到 `$sshzyu-co-image`，不得新增图片逻辑。
