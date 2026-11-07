# GPT Image 2.6

2026-11-06 实测 sshzyu 模型列表提供通用 ID `gpt-image-2.6`。不要擅自替换成 `gpt-image-2`；以实时模型列表为准。

上游 OpenAI 还将 2.6 分为 Sunburst（精细视觉）和 Flare（快速迭代）。当前 sshzyu 列表未暴露这两个 2.6 子 ID，因此脚本路由使用 `gpt-image-2.6`，执行前仍应核对 `--models`。

## 生图要点

- 适合细节丰富、文字较多、复杂布局和多轮编辑。
- `complexity=low|medium|high` 控制生成前的规划量，与 `quality` 独立。
- 简单局部替换优先 `low`；复杂布局、多物体关系、密集文字可试 `medium/high`。
- 更高 complexity 可能增加延迟并引入额外变化，不能假定一定更好。
- `quality` 是最终渲染质量，可选 `auto/low/medium/high/xhigh/max`。
- 默认按服务端 `auto`，真实调用前必须让用户明确质量；需要确认他说的“几 K”是指尺寸还是画质。

## 输出

- 生成：PNG、JPEG、WebP、GIF、SVG。
- 编辑：PNG、JPEG、WebP。
- GIF：`frame_count=2..16`，默认 9；`frame_duration_ms=20..2000`，默认 200。
- SVG：`n=1`；解码 `b64_json` 后按 UTF-8 XML 保存。
- GIF/SVG 不支持编辑；SVG 不能作为输入图。

## Markup

- 编辑第一张参考图时，可额外上传透明 PNG `markup`。
- markup 与原图宽高必须一致。
- `markup` 与 `mask` 不能同时使用。
- sshzyu 当前接入文档尚未明确列出 markup；如果代理返回 `422`，停止并询问是否改用提示词描述或 2.5。

## 兼容风险

sshzyu 当前基础接入文档仍主要列出 PNG/JPEG/WebP。2.6 的 complexity、GIF、SVG、markup 是脚本按新模型能力透传，真实代理是否完全支持以响应为准。发生 `422` 时不自动换模型、不自动降参数。
