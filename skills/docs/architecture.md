# 套件架构

```text
skills/                         # 套件根
├── skills/                     # 独立 Skill（安装入口）
│   ├── sshzyu-media/           # 统一入口：初始化、约束、状态、路由
│   ├── sshzyu-image/           # 图片能力（唯一权威实现）
│   ├── sshzyu-video/           # 视频能力（唯一权威实现）
│   ├── short-drama/            # 短剧项目入口
│   └── short-drama-*/          # 短剧阶段入口
├── docs/                       # 套件文档
├── tests/                      # 套件级检查
└── tools/                      # 打包工具
```

## 路由关系

1. 用户进入 `sshzyu-media` 后，入口先识别任务类型与已有项目状态：
   - 单次图片或视频任务转交 `sshzyu-image` / `sshzyu-video`；
   - 短剧项目转交 `short-drama`，再由其协调阶段 Skill。
2. 用户明确点名专业 Skill 时可直接调用，不强制经过总入口。
3. `sshzyu-media` 不维护第二套图片/视频 API 实现，不代替阶段 Skill 写剧本、设定、分镜或提示词。

## 事实源

- 图片 API、模型与参数：`skills/skills/sshzyu-image/references/`。
- 视频 API、模型与参数：`skills/skills/sshzyu-video/references/`。
- 短剧阶段规则：各 `short-drama*` Skill 自身。
- 跨能力入口规则与状态约定：`skills/skills/sshzyu-media/`。

## 分发

- 发布包由 `tools/package-suite.py` 生成，顶层为 `skills/<名称>/` 与套件说明文件。
- 安装后每个 Skill 位于 `<安装根>/<名称>/SKILL.md`；同名冲突必须由用户决定，不得静默覆盖。
- 生产调用需要用户本次明确确认；打包与静态检查不得触发付费接口。
