---
name: sshzyu-media
description: "SSHZYU 媒体统一入口：初始化并继续短剧/漫剧项目，维护五份创作文档、项目配置、创作台与交付打包，识别图片、视频、短剧或组合任务并分发到对应专业能力；同时维护付费确认与跨能力安全边界。用户提到 SSHZYU 生图、生视频、AI 短剧、AI 漫剧、创建或继续短剧项目、看进度、制作形态选型、打开创作台、打包导出时使用。"
license: MIT
---

# SSHZYU 媒体统筹

本技能是套件的唯一用户入口：负责短剧项目生命周期、跨能力判断与分发、以及共同安全边界。它不复制图片/视频的参数表和 API 实现，也不替代各阶段技能的专业产出。

## 职责边界


| 我负责                | 我不负责                                                  |
| ------------------ | ----------------------------------------------------- |
| 初始化、继续、修改短剧项目配置    | 写剧本、拆视觉设定、做分镜（交对应 `$short-drama-*`）                   |
| 维护五份创作文档的格式约定与校验   | 图片/视频的 API 参数与调用实现（交 `$sshzyu-image`、`$sshzyu-video`） |
| 打开本地创作台（Dashboard） | 代替用户确认付费生成                                            |
| 判断媒体意图并分发到专业能力     | 把项目初始化当成生成授权                                          |
| 导出与交付打包            | 跳过阶段直接产出成片                                            |


跨能力路由细则见 [路由契约](ROUTING.md)。

## 媒体分发


| 用户需求                       | 去向                                   |
| -------------------------- | ------------------------------------ |
| 单次生图、改图、多图批处理              | `$sshzyu-image`                      |
| 文生视频、图生视频、首尾帧/参考素材视频、查询与下载 | `$sshzyu-video`                      |
| 短剧开发、写作、设定、提示词、分镜、剪辑、审稿    | 对应 `$short-drama-*`                  |
| 短剧素材批量生产与任务恢复              | `$short-drama-produce`（先预览，本次确认后才执行） |
| 项目初始化、配置、创作台、导出打包          | 本技能                                  |


用户明确点名某个阶段技能时可直接调用它，不必先绕回本入口。

## 快速开始

每集按需直接维护五份核心 Markdown 文档（`剧本.md`、`视觉设定.md`、`分镜.md`、`图片提示词.md`、`视频提示词.md`），不另存重复的结构化副本；具体格式、阶段归属与输出语言规则见 五份创作文档。安装或排障时运行自检（需 Python 3.9+）：

```bash
python3 {技能目录}/scripts/selftest.py
```

## 环节分工与路由


| 用户要做什么                   | 负责技能与产出                                                          |
| ------------------------ | ---------------------------------------------------------------- |
| 开发创意、核心看点、改编方案与分集大纲      | `$short-drama-develop`                                           |
| 拆解长篇原著、抽样快评与分集候选         | `$short-drama-novel-analyze` → `项目开发/source-analysis/`           |
| 拆解对标剧、同尺比对差距、视频反推剧本与成片还原 | `$short-drama-breakdown` → `项目开发/对标拆解.md` 或 `输入/对标/<REF-ID>/还原/` |
| 写或改单集剧本                  | `$short-drama-write` → `剧集/<EP>/剧本.md`                           |
| 整理人物、造型、地点、道具与声线设定       | `$short-drama-assets` → `剧集/<EP>/视觉设定.md`                        |
| 写静态设定图提示词                | `$short-drama-image-prompts` → `剧集/<EP>/图片提示词.md`                |
| 做分镜与首尾帧提示词               | `$short-drama-storyboard` → `剧集/<EP>/分镜.md`                      |
| 写视频动态提示词或配乐提示词           | `$short-drama-video-prompts` → `剧集/<EP>/视频提示词.md`                |
| 调用外部模型生成图、视频、配音或配乐       | `$short-drama-produce`（先看预览，确认后执行）                               |
| 编排剪辑单并渲染成片               | `$short-drama-edit` → `剧集/<EP>/剪辑单.md` 与 `制作成果/成片/`              |
| 独立审稿或出片问题归因              | `$short-drama-review`（按需启用）                                      |
| 初始化与修改全局配置、打开创作台、导出或打包   | 本技能                                                              |


## 执行与写入约定

1. **按需切入与连续推进**：优先使用用户指定的目录，只读取当前任务直接用到的输入文件，不预建空白前序文档。用户交代做到哪一步就完整做到哪一步；分批处理只用于控制单次篇幅，跑通一批直接继续。
2. **合并补问（最多一次）**：立项或配置缺省时先用合理默认值建项推进；仅在缺少关键信息且不同选法会明显改变剧情走向或画面风格时，在单轮内合并提出不超过 1 次精简补问。下一步要调用外部模型生成素材时，必须先展示预览并等创作者明确确认。
3. **安全保存与跨文档检查**：直接读写 Markdown 时采用原子保存，若发现文件刚被外部编辑器或创作台修改，先重读合并再写入；`.short-drama/`、`输入/` 中的内部记录与密钥绝不写入创作正文。每完成一份文档运行 `creator_markdown_check.py <剧集目录> --partial`，五份齐备后跑全量检查。
4. **参考图挂接分流**：项目内已有匹配图片时挂 `REF-...`；尚无现成图片且创作者未要求暂停等图时，默认按 `PLAN-...` 写出外部挂图计划，或按创作者意图直接写 `- 输入参考图：无` 走文生视频；仅在创作者明确要求等图进项目时写 `无（待补参考图：……）` 并暂停。

## 项目初始化与全局配置

创建项目时将已知参数一次传入，未定项直接留默认（不反复追问）：

```bash
python3 {技能目录}/scripts/project_tool.py init ./my-drama --title "示例短剧" --aspect-ratio 9:16
```

后续在对话中确定或修改画幅、集数、单集时长、语言、语速、视觉方向、目标视频模型配置（`production_profile`）或节奏目标（`rhythm_profile`）时，直接用 `configure` 增量写入 `short-drama.json` 并标记为已确认（无需手写中间台账文件）：

```bash
python3 {技能目录}/scripts/project_tool.py configure <project> \
  --aspect-ratio 16:9 --episode-count 24 --target-seconds 90 \
  --pacing-json '{"spoken_characters_per_second":4.2,"seconds_per_action_paragraph":2.5}' \
  --production-profile-json '{"target_video_model":"seedance-2.5","video_prompt_language":"zh-CN","native_duration_seconds":{"min":4,"max":15},"supported_generation_modes":["text","first_frame","reference"],"audio_generation":"same_pass"}' \
  --rhythm-profile-json '{"form":"国漫二次元","first_hook_seconds":5}'
```

- **模型配置（**`--production-profile-json`**）**：用户在对话中点名目标视频模型（如 Seedance 2.5、MiniMax H3）时，先用 `configure` 写入模型参数、提示词语言、是否启用多镜合生（`multi_shot_containers`）及接力路线（`continuation_route`），再开展下游提示词编写。
- **节奏目标（**`--rhythm-profile-json`**）**：支持实拍、二维动态漫、风格化三维、水墨笔触、Q 版表达、国漫二次元等全部制作形态名称，按需填写已敲定的部分指标即可，不必凑齐全部字段。可用 `status` 查看当前生效配置。

## 短剧创作台（Dashboard）

用户要求打开创作台时运行：

```bash
python3 {技能目录}/scripts/dashboard_server.py --workspace <workspace> --port 0 --detach --open
python3 {技能目录}/scripts/dashboard_server.py --workspace <workspace> --status
python3 {技能目录}/scripts/dashboard_server.py --workspace <workspace> --stop
```

`--detach` 在后台运行并把会话信息记入 `<workspace>/.short-drama/dashboard.json`（更换端口或令牌加 `--restart`）。创作台以只读卡片展示五份文档、素材与成片，可在「编辑原文」中直接保存 Markdown。

## 导出与交付打包

```bash
python3 {技能目录}/scripts/project_tool.py export <project> --out <项目外目录>
python3 {技能目录}/scripts/project_tool.py package <project> --episode EP001
python3 {技能目录}/scripts/project_tool.py verify <project> --episode EP001
```

- `export`：将各集现有创作文档、`剪辑单.md` 与 `制作成果/` 备份到项目外目录（可选 `--episode EP001`、`--no-media`、`--overwrite`）。
- `package` / `verify`：直接按集打包项目当前的创作者文档与有效交付文件（也可用 `--include` 指定子集），生成 `交付/<EP>/manifest.json` 与 `checksums.sha256`（不含 `输入/` 与 `.short-drama/`，自动拦截符号链接与越界路径；旧式台账项目需强核审稿状态时可加 `--strict-ledger`）。

## 🔴 付费边界

启动项目、写好提示词、跑通 dry-run 都不等于授权生成。任何真实图片或视频请求都会产生费用：

- 普通媒体调用先核实模型、尺寸或时长、画幅、参考素材、任务数及费用风险，执行不读密钥、不联网的 `--dry-run`，取得用户对**本次**计划的明确确认后才可加 `--confirm`。
- 短剧任务须按项目任务单的 `prepare → confirm → run/collect` 契约单独确认；不能用历史确认、CLI 参数或用户泛泛的「继续」替代。
- 参考素材只接受公网 HTTPS 直链；本地文件、内网地址和私有素材不得自动上传。
- 异步任务失败先查询原任务，绝不盲目重复付费。

## 兼容入口

`scripts/sshzyu_media.py` 保留旧版 `image`、`video`、`drama`、`doctor`、`models` 命令；其运行需要并列安装 `sshzyu-image` 与 `sshzyu-video`，并非本技能目录下的子技能。旧 `drama run` 是镜头 manifest 兼容命令，**不是**短剧生产任务单适配器。

```bash
python3 -I {技能目录}/scripts/sshzyu_media.py doctor
```

该命令只检查本地依赖与凭据来源，不调用付费接口；缺少独立技能时须明确报告缺失。不要将 `doctor` 成功、ZIP 存在或项目初始化等同于生产链路已验证。

## 按问题查阅参考文档

- 阶段问题路由与四级规则说明：规则与路由索引
- 五份文档格式、文档归属与输出语言：五份创作文档
- 六种制作形态差异与视觉风格定调（Look Development）试片：制作形态
- 参考图十种用途（含原生多模态 `主体` 参考）与控制边界：参考图用途
- 按悬念类型交代遮挡与揭晓动作：信息揭晓时机
- 母版、局部补拍与替代版的返工归属：母版、补拍与替代版

&nbsp;