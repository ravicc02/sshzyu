# GPT Image 2

当前 sshzyu 可用 ID：`gpt-image-2`。

## 参数

- 生成和编辑格式：PNG、JPEG、WebP。
- `quality`：`auto/low/medium/high`。
- 支持 `n=1..10`、参考图、mask。
- 不支持 complexity、GIF、SVG、markup。

## 路由

- 仅在用户明确要求旧模型、兼容已有工作流，或上层脚本依赖该 ID 时使用。
- 用户没有明确模型选择时，脚本默认 `gpt-image-2.6`，但真实调用前仍必须询问并确认模型。
- 不因 2.6 返回 422 自动降级到 2；必须重新确认模型和计划。
