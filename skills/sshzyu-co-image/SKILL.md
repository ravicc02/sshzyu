---
name: sshzyu-co-image
description: "通过 sshzyu.com 完成 AI 生图、图片编辑、多图批处理、mask、markup 与结果校验。短剧与视频流程需要图片时统一引用本技能，不在视频技能中重复实现生图逻辑。"
---

# SSHZYU 生图

本技能是 SSHZYU 媒体体系中的独立图片能力包，负责文生图、图片编辑、多图输出、参考图、mask、markup、任务结果落盘和输出校验。

## 路径与调用

- CLI：`scripts/sshzyu_image.py`
- API 说明：`references/image/api_reference.md`
- 参数矩阵：`references/image/parameter-matrix.md`
- 模型说明：`references/image/models/README.md`
- 服务端点：`https://sshzyu.com/v1`

短剧、视频和媒体编排技能只应调用本技能的 CLI 或通过 `$sshzyu-co-image` 路由图片任务，不得复制图片请求、模型能力表、凭据读取或输出校验逻辑。

## 强制 Preflight

任何真实图片请求都会产生费用。提交前必须明确：用途、画面内容、模型、尺寸、质量、输出格式、prompt 数、每项张数、并发数、参考图、mask 或 markup。

参数不全、Key 缺失、参考素材不是匿名公网 HTTPS 直链，或用户未确认费用时，禁止真实提交。先运行 `--dry-run`；dry-run 不读取 Key、不联网、不计费。用户明确确认任务数量和费用后，真实命令才加入 `--confirm`。

## 示例

```powershell
py -3.12 -I -X utf8 "${SKILL_DIR}/scripts/sshzyu_image.py" `
  "一只戴墨镜的橘猫，暖色调，无文字" `
  --model gpt-image-2.6-sunburst --size 1024x1024 --quality high `
  --n 2 --out "TASK_OUTPUT_DIR" --dry-run
```

真实执行必须使用已确认的完全相同参数并加入 `--confirm`。不得自动换模型、自动追加数量、自动重试或打印 Key、Authorization、签名下载 URL。

## 凭据与输出

- 推荐在本技能目录的 `.env` 中设置 `SSHZYU_API_KEY`，也可使用进程环境变量或已配置的 Windows Credential Manager。
- 不把 Key 写入 prompt、manifest、日志或输出 JSON。
- 每次运行保存到 `run-时间戳-短ID/`，包含图片、`job-result.json` 和 `batch-summary.json`。
- `success` 为文件校验通过；`partial` 保留有效结果但需人工核对；`error` 不自动补发。

## 与短剧/视频的边界

- `short-drama-image-prompts` 只负责提示词和参考图计划。
- `short-drama-produce` 负责确认后生产，但图片任务必须路由到本技能。
- `sshzyu-media` 保留兼容入口，但其图片实现由本技能提供。
- `sshzyu-video` 只处理视频任务；视频前置图片由本技能生成或引用其输出，不在视频脚本中实现图片 API。
