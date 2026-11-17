# 套件架构

```text
sshzyu-media-skills/           # 套件仓库根
├── skills/                     # 独立 Skill（安装入口）
│   ├── sshzyu-media/           # 统一入口：短剧项目生命周期 + 约束 + 状态 + 路由
│   ├── sshzyu-image/           # 图片能力（唯一权威实现）
│   ├── sshzyu-video/           # 视频能力（唯一权威实现）
│   └── short-drama-*/          # 短剧阶段入口
├── docs/                       # 套件文档
├── tests/                      # 套件级检查
└── tools/                      # 打包工具
```

## 路由关系

1. 用户进入 `sshzyu-media` 后，入口先识别任务类型与已有项目状态：
   - 单次图片或视频任务转交 `sshzyu-image` / `sshzyu-video`；
   - 短剧项目由 `sshzyu-media` 自身初始化、配置并协调阶段 Skill，不再有独立短剧总入口。
2. 用户明确点名专业 Skill 时可直接调用，不强制经过总入口。
3. `sshzyu-media` 不维护第二套图片/视频 API 实现，不代替阶段 Skill 写剧本、设定、分镜或提示词。

## 事实源

- 图片 API、模型与参数：`sshzyu-media-skills/skills/sshzyu-image/references/`。
- 视频 API、模型与参数：`sshzyu-media-skills/skills/sshzyu-video/references/`。
- 短剧阶段规则：各 `short-drama*` Skill 自身。
- 跨能力入口规则与状态约定：`sshzyu-media-skills/skills/sshzyu-media/`。

## 分发

- 发布包由 `tools/package-suite.py` 生成，顶层为 `skills/<名称>/` 与套件说明文件。
- 安装后每个 Skill 位于 `<安装根>/<名称>/SKILL.md`；同名冲突必须由用户决定，不得静默覆盖。
- **同根约束**：`sshzyu-video` 在导入阶段要求同根存在 `sshzyu-image`；`sshzyu-media` 的兼容命令还要求 `sshzyu-video`。三者必须整体安装，`short-drama-*` 可独立安装。
- 凭据按能力分离：图片读 `SSHZYU_API_KEY`（`sshzyu-image/.env`、进程环境变量或系统凭据库）；视频只读 `SSHZYU_VIDEO_API_KEY`（`sshzyu-video/.env` 或进程环境变量），缺失时直接失败并提示换 Key，**不回退**到通用 Key。两者平台分组不同，同一把 Key 无法同时满足图片与视频。
- 视频模型不做静默降级：模型不在当前 Key 的 `/models` 列表中时，提交前停止并列出可用模型，提示换模型或换 Key；网关返回平台不匹配时明确指向 Key 分组问题。
- 生产调用需要用户本次明确确认；打包与静态检查不得触发付费接口。
