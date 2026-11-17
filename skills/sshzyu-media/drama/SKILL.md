---
name: sshzyu-drama
description: "SSHZYU 短剧与漫剧创作工作流：从原著分析、分集剧本、视觉设定、分镜和图片/视频提示词，到确认后生产、剪辑成片与独立审查。"
---

# SSHZYU 短剧创作工作流

这是 `sshzyu-media` 内置的短剧技能套件。原有阶段技能保留在 `skills/` 下，调用名称和项目文档格式保持不变：

- `$short-drama`：项目初始化、阶段路由与 Dashboard
- `$short-drama-novel-analyze`：原著抽样与改编评估
- `$short-drama-breakdown`：对标剧拆解与视频反推剧本
- `$short-drama-develop`：故事开发与分集规划
- `$short-drama-write`：分集剧本
- `$short-drama-assets`：人物、场景、道具和连戏设定
- `$short-drama-image-prompts`：图片提示词与参考图计划
- `$short-drama-storyboard`：分镜、首帧和尾帧
- `$short-drama-video-prompts`：视频动作与运镜提示词
- `$short-drama-produce`：确认后投产、任务恢复与结果归档
- `$short-drama-edit`：剪辑单与成片
- `$short-drama-review`：独立审查

## 媒体能力边界

短剧工作流不改变源 `drama skills` 的阶段顺序，但所有付费媒体任务统一回到 SSHZYU 路由：

```text
$short-drama-image-prompts / $short-drama-storyboard
  → 图片需求
  → $sshzyu-image
  → https://sshzyu.com/v1/images/*

$short-drama-video-prompts
  → 视频需求
  → sshzyu-media video
  → https://sshzyu.com/v1/videos
```

图片生成统一由独立技能 `$sshzyu-image` 负责，入口为：

```text
${SSHZYU_SKILLS_ROOT}/sshzyu-image/scripts/sshzyu_image.py
```

短剧技能只产生提示词、参考图绑定和任务单；不得在短剧或视频技能中复制图片 API、模型能力、Key 读取或图片校验逻辑。短剧生产中的旧供应商直连 adapter 不是 SSHZYU 路由，不能用于新任务。

生产时仍遵守 `sshzyu-media` 的 preflight：先预览、再取得本次明确确认，最后才提交付费任务。图片结果可作为视频的公网参考素材，但本地文件、内网地址和需要登录的 URL 不得自动上传。

## 项目事实源

短剧项目继续使用原有 Markdown 文档和稳定 ID：`剧本.md`、`视觉设定.md`、`分镜.md`、`图片提示词.md`、`视频提示词.md`、`剪辑单.md`。Dashboard 只是视图，不创建第二份事实源。

## 目录说明

```text
drama/
├── SKILL.md
└── skills/
    ├── short-drama/
    ├── short-drama-assets/
    ├── short-drama-breakdown/
    ├── short-drama-develop/
    ├── short-drama-edit/
    ├── short-drama-image-prompts/
    ├── short-drama-novel-analyze/
    ├── short-drama-produce/
    ├── short-drama-review/
    ├── short-drama-storyboard/
    ├── short-drama-video-prompts/
    └── short-drama-write/
```
