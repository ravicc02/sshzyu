# 模型路由

先确认用户要求和模型，再读取对应模型页：

| 模型 | 何时读取 |
| --- | --- |
| `gpt-image-2.6-sunburst` | 默认；需要精细视觉、complexity、GIF、SVG、markup |
| `gpt-image-2.6-flare` | 快速迭代；保留 2.6 参数能力 |
| `gpt-image-2.5-*` | 需要稳定的 2.5 能力或 2.6 参数在代理侧返回 422 |
| `gpt-image-2` | 兼容旧调用或明确指定基础模型 |

执行前用 `models --kind image` 核对当前进程环境或 `.env` 中的 Key。模型列表可能随分组切换，不在单次列表中出现时不要自动换模型，最终以实际图片接口响应为准。

模型页只描述差异；通用端点、凭据、尺寸矩阵和批处理规则见：

- [API 参考](../api_reference.md)
- [标准参数矩阵](../parameter-matrix.md)
