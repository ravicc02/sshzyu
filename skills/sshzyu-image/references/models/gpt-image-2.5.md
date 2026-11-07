# GPT Image 2.5

当前 sshzyu 可用 ID：

- `gpt-image-2.5`
- `gpt-image-2.5-flare`
- `gpt-image-2.5-sunburst`

## 路由

- 需要快速反馈、草稿、大量候选：`gpt-image-2.5-flare`。
- 需要精细编辑、复杂构图：`gpt-image-2.5-sunburst`。
- 上游或代理只接受通用名：`gpt-image-2.5`。
- 用户明确指定某个 ID 时保留原 ID，不擅自切换。

## 参数

- 生成和编辑格式：PNG、JPEG、WebP。
- `quality`：`auto/low/medium/high/xhigh/max`。
- 支持 `n=1..10`、参考图、mask。
- 不启用 2.6 的 complexity、GIF、SVG。
- 代理是否支持 markup 以真实响应为准；脚本允许尝试，但 `422` 后停止。

## 生图要点

- Sunburst 优先保证编辑一致性和细节。
- Flare 适合快速迭代，但仍需在 preflight 中确认尺寸、质量、格式和张数。
- 不用 2.5 冒充 2.6；用户要求新能力但代理不兼容时，先说明冲突并询问选择。
