# sshzyu.com 生图能力接入规范（Agent 执行手册）

> 本文档写给 **AI Agent**。当用户对你说「请根据本文档的规范做好生图 skills」时，你（Agent）需要**独立完成**以下全流程，不需要再向用户询问技术细节：
>
> 1. 编写一个调用 sshzyu.com 生图接口的命令行脚本（零第三方依赖）
> 2. 自测脚本（真实出一张图）
> 3. 把脚本封装成 Skill，安装到全局技能目录
> 4. **用装好的 Skill 重新触发一次生图**，验证全链路可用
> 5. 向用户报告：Skill 名称与位置、生成的图片路径

---

## 1. 接入信息

| 参数 | 值 |
| --- | --- |
| Base URL | `https://sshzyu.com/v1` |
| 认证头 | `Authorization: Bearer YOUR_API_KEY` |
| 文生图 | `POST https://sshzyu.com/v1/images/generations`（JSON 请求体） |
| 图片编辑 / 参考图 | `POST https://sshzyu.com/v1/images/edits`（multipart/form-data） |
| 模型列表 | `GET https://sshzyu.com/v1/models` |

用户提示词第一行的 `key:xxx` 即 `YOUR_API_KEY`。**安全要求：API Key 只能存放在本机（技能目录内的 `key.txt`，或环境变量 `SSHZYU_API_KEY`），禁止写进脚本源码、公共仓库或对话输出。**

---

## 2. 生图请求规范（OpenAI 格式）

`/v1/images/generations` 请求体为 JSON，字段全部按 OpenAI Images API 风格透传上游：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 否 | `gpt-image-` 前缀家族；缺省按 `gpt-image-2` 处理。以 `GET /v1/models` 返回为准 |
| `prompt` | string | **是** | 画面描述，建议越具体越好 |
| `size` | string | 否 | `宽x高`（如 `1024x1024`）或 `auto`。完整矩阵见第 3 节 |
| `quality` | string | 否 | `auto`（默认）/ `low` / `medium` / `high`；gpt-image-2.5 家族另支持 `xhigh` |
| `output_format` | string | 否 | `png`（默认）/ `jpeg` / `webp` |
| `output_compression` | int | 否 | `0-100`，仅 `jpeg` / `webp` 有效 |
| `background` | string | 否 | `auto`（默认）/ `transparent` / `opaque` |
| `n` | int | 否 | 生成张数 `1-10`，缺省 `1`。**计费按张数叠加** |
| `moderation` | string | 否 | `auto`（默认）/ `low` |
| `response_format` | string | 否 | 缺省返回 `b64_json`；脚本按 b64 解码保存即可，无需传此参数 |

`/v1/images/edits`（可选扩展）用 multipart 上传：`image[]` 参考图最多 **16 张**、可选 `mask`，其余字段同上。

**响应结构**（HTTP 200）：

```json
{
  "created": 1759000000,
  "data": [ { "b64_json": "<base64 图片数据>", "revised_prompt": "..." } ],
  "usage": { "input_tokens": 12, "output_tokens": 4160, "total_tokens": 4172 }
}
```

脚本必须解码 `data[i].b64_json` 并写入本地文件；`usage` 需打印给用户核对。

---

## 3. 参数矩阵（size）——1K/2K/4K 三档全开，可调宽高比

站内尺寸按「档位 × 宽高比」组织，三档全部开放。`size` 使用矩阵预设值即可，也接受任意 `宽x高` 自定义值（站内不设白名单，按计费规则分档）。脚本默认从 1K 档起步，用户明确要高分辨率时按矩阵直接请求对应档位：

| 宽高比 | 1:1 | 3:2 | 2:3 | 16:9 | 9:16 | 4:3 | 3:4 | 21:9 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **1K** | 1024x1024 | 1536x1024 | 1024x1536 | 1280x720 | 720x1280 | 1024x768 | 768x1024 | 1280x544 |
| **2K** | 2048x2048 | 2160x1440 | 1440x2160 | 2560x1440 | 1440x2560 | 2048x1536 | 1536x2048 | 2560x1088 |
| **4K** | 2880x2880 | 3456x2304 | 2304x3456 | 3840x2160 | 2160x3840 | 3200x2400 | 2400x3200 | 3840x1600 |

> 计费按请求尺寸**最长边**判定：≤ 1024 → 1K 档、≤ 2048 → 2K 档、> 2048 → 4K 档（`auto` 按 2K 档）。矩阵里的规律：**正方形与 4:3 / 3:4 按本档结算，其余宽比例（3:2 / 2:3 / 16:9 / 9:16 / 21:9）升一档**（1K→2K、2K→4K；4K 档一律按 4K）。向用户报价时按此换算。

---

## 4. 脚本要求

写一个**单文件、零第三方依赖**的命令行脚本（建议 Python 3 标准库 `urllib.request` + `json` + `base64`），命名 `sshzyu_image.py`：

```text
用法: python sshzyu_image.py "提示词" [选项]
  --size SIZE        尺寸，默认 1024x1024（见参数矩阵）
  --quality Q        质量，默认 auto
  --format FMT       png / jpeg / webp，默认 png
  --n N              张数 1-10，默认 1
  --model NAME       模型，默认 gpt-image-2
  --out DIR          输出目录，默认 ./sshzyu-images
  --key KEY          显式传入 API Key（否则依次读 key.txt / 环境变量 SSHZYU_API_KEY）
```

行为要求：

1. Key 读取顺序：`--key` → 环境变量 `SSHZYU_API_KEY` → 脚本同目录 `key.txt`；都没有时报错并提示如何配置。
2. 请求失败时打印 HTTP 状态码与 `error.message`，并给出针对性提示：`401` Key 无效、`402` 余额不足、`403` 分组未开通图片生成权限、`429` 限流稍后重试。
3. 成功时把每张图按 `时间戳-序号.png/jpeg/webp` 保存到输出目录，打印**绝对路径**与 `usage` 计费信息。
4. 校验保存文件以 PNG（`89 50 4E 47`）/ JPEG（`FF D8`）/ WebP（`RIFF`）魔数开头，防止把错误响应当图片保存。

先用脚本真实生成一张图完成自测（如「a cute orange cat wearing sunglasses」），确认图片可打开后进入下一步。

---

## 5. 封装为 Skill 并安装到全局

把脚本包装成技能目录并装入 **全局**技能目录（不要装在某个项目里）：

```text
~/.claude/skills/sshzyu-image/          ← Claude Code 全局技能
~/.newmax/skills/sshzyu-image/          ← NewMax 全局技能
  ├─ SKILL.md                            ← 技能说明（frontmatter + 用法）
  ├─ sshzyu_image.py                     ← 生图脚本
  └─ key.txt                             ← API Key（ chmod 600，禁止提交/分享）
```

若当前 Agent 使用其它扩展机制（如 Codex 的自定义 prompts），装入其等价的全局位置即可，结构不变。

`SKILL.md` 模板（必须含 frontmatter）：

```markdown
---
name: sshzyu-image
description: 用 sshzyu.com 中转站的 gpt-image 系列模型生成图片。当用户要求画图、生成图片、出图时使用。
---

# sshzyu-image

生成图片：python <本技能目录>/sshzyu_image.py "画面描述" [--size 1024x1024] [--quality auto] [--format png] [--n 1]

- API Key 已存放在本技能目录 key.txt，脚本自动读取，无需向用户索要。
- 尺寸矩阵与计费规则见 https://sshzyu.com/docs/install/iamge_gen.md ；默认 1K 档（1024x1024），用户明确要高分辨率时按矩阵直接请求 2K/4K，无需额外确认。
- 生成后把图片绝对路径告诉用户。
```

---

## 6. 安装后验证（必须执行）

1. **以 Skill 方式**（不是手动跑脚本）触发一次生图，例如让 Agent 调用 `sshzyu-image` 技能生成「一只戴墨镜的橘猫，坐在打字机前，暖色调」。
2. 确认：调用走的是刚装好的全局技能、图片文件存在且魔数正确、`usage` 计费已打印。
3. 向用户报告：Skill 安装位置、验证图片的绝对路径。验证失败时按第 4 节错误提示排查，不要跳过本步骤宣称完成。

---

## 7. 常见错误对照

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| `403 Permission Denied` | Key 所在分组未开通「图片生成」权限 | 到控制台换用已开通生图权限的分组/Key |
| `401 Authentication Fails` | Key 错误或 `Bearer` 前缀缺失 | 检查 key.txt 内容与请求头 |
| `402 Insufficient Balance` | 余额不足 | 控制台充值 |
| `422 Invalid Parameters` | size/quality/format 不在支持范围 | 按第 2、3 节参数矩阵修改 |
| 请求 2K/4K 但返回分辨率更低 | 输出分辨率由服务端实际能力决定，`size` 是期望值非保证值 | 如实向用户报告实际分辨率与 `usage`，不要反复重试 |
