# 测试报告：15 秒短剧全流程

## 结论

**创作、剪辑、字幕、验收、导出这条链跑通了；真实视频生成没跑通。**

本次交付的 15.1 秒成片是「3 张静帧 + 推近运镜 + 硬字幕 + 本地合成音效」的动态漫，不是 AI 视频生成。原因是三条视频通道同时不可用：

1. `short-drama-produce` 自述尚无已实现的 SSHZYU 任务单 adapter；
2. SSHZYU `/v1/videos` 返回 `HTTP 404: Videos API is not supported for this platform`；
3. 宿主视频供应商返回 `403 Seedance requires an OpenAI or composite group`。

把这次成片说成「短剧视频生成已打通」是不准确的；准确说法是「短剧文本到成片的本地剪辑链已打通，视频素材生产未接通」。

## 实跑结果

| 项 | 结果 |
| --- | --- |
| 项目初始化 | 成功，`SD-B06E57451CAF` |
| 五份创作文档 | 完成，`creator_markdown_check.py` 全量 `OK` |
| 图片 | 3/3 成功；1 张 `partial`（948×1659） |
| 视频 | 0/3；两处 404/403 |
| 剪辑检查 | 通过，1 条运镜速度提醒 |
| 成片 | 1080×1920 · 24fps · 15.1s · H.264/AAC |
| 响度 | -16.33 LUFS，真峰 -2.64 dBTP |
| 交付 | `package` 6 个文本/JSON；`export` 26 个文件含成片 |
| 成片 SHA256 | `f3278c8f48ce16c0642dfce3d431f0c6a636e75f2f6042fbd39ecd3cfe6402d7` |

## 卡点优先级

### P0：不修就无法做真实视频短剧

| 卡点 | 事实 | 修法 |
| --- | --- | --- |
| K1 视频生产链断裂 | adapter 未实现；SSHZYU 404；宿主 403 | 实现 SSHZYU 任务单 adapter，或在 `short-drama-produce` 与 README 中把视频生产标为未接通，并给出静帧降级路径 |
| K3 视频模型无预检 | `/models` 无 `kind=video`，CLI 仅警告后提交，最终 404 | 提交前把「无视频模型」当硬失败，直接说明账号未开通 |

### P1：会造成误解或返工

| 卡点 | 事实 | 修法 |
| --- | --- | --- |
| K5 首帧提示词双写 | `分镜.md` 与 `图片提示词.md` 各存一份首帧提示词 | 明确唯一归属，另一处只放编号引用 |
| K6 `package` 语义 | 只含文本/JSON，成片在 `export` | SKILL 明确两者分工 |
| K7 视觉设定字段名 | 「状态变化」vs「本集状态」 | 统一字段名，示例与说明同源 |
| K2 代理错误误导 | 默认走失效 127.0.0.1:7890，表现为平台不可达 | `doctor` 增加只读连通性探测与代理提示 |

### P2/P3：体验与质量

- K4 尺寸对齐只标 `partial`，未解释原因。
- K8 VO 全集秒数无字段，被塞进分镜「声音」行。
- K9 `pacing` 未配置但目标时长已设置，写剧本阶段无换算依据。
- K10 相邻段运镜速度差 33%，`check` 只提醒不自动统一。
- K11 字幕标点被替换、长句拆行，与「逐字」表述有偏差。
- P2 三张图角色造型不一致：独立文生图无法保证跨图一致性，需要参考图复用或一致性检查。

## 下一轮优化建议（按投入产出排序）

1. **先修 K1 + K3**：这两项决定下一轮实跑能否从静帧升级为真实视频。若 adapter 短期做不完，就把「未接通」写进 `short-drama-produce` 的第一段，避免用户按文档走到最后才发现无法提交。
2. **再修 K5 + K6 + K7**：都是文档级、低风险、高收益，能直接减少子代理的猜测。建议在 `references/creator-documents.md` 增加一张「首帧提示词唯一归属」表，并在 `package` / `export` 两处写明边界。
3. **补 K2**：`doctor` 增加只读 `/health` 探测与代理提示；不自动改系统配置，只报告「直连可用/代理不可用」。
4. **补 K9**：`init` 后根据语言与目标时长给 `pacing` 安全默认值，或在 `status` 中把该提示升级为「下一步必须配置」。
5. **质量项最后做**：角色一致性、字幕标点、运镜同速属于成片质量，不影响流程可执行性。

## 未验证项

- 没有成功的真实视频生成，因此视频提示词对真实模型的效果、音画同步、镜头连续性均未验证。
- 没有做人工看片打分；抽帧只核对了画面与字幕是否正常，不代表叙事质量。
- 没有跑仓库规定的发布前测试套件；本轮只做构建、静态检查、检查器与真实图片调用。
- 宿主视频供应商 403 是配置/分组问题，未尝试修改 NewMax 配置。

## 复现命令

```bash
cd sshzyu-media-skills

# 项目初始化
python -X utf8 skills/sshzyu-media/scripts/project_tool.py init <project> \
  --title "最后一班电梯" --aspect-ratio 9:16 --episode-count 1 --target-seconds 15

# 文档检查
python -X utf8 skills/sshzyu-media/scripts/creator_markdown_check.py <project>/剧集/EP001 \
  --project-root <project>

# 图片（需要 SSHZYU_API_KEY，且绕开失效代理）
NO_PROXY="sshzyu.com,127.0.0.1,localhost" HTTPS_PROXY= HTTP_PROXY= \
python -X utf8 skills/sshzyu-image/scripts/sshzyu_image.py "<prompt>" \
  --model gpt-image-2.6-flare --size 1024x1792 --quality high --n 1 \
  --out <project>/剧集/EP001/制作成果/images --confirm

# 剪辑
python -X utf8 skills/short-drama-edit/scripts/edit_tool.py check  <project>/剧集/EP001 --project-root <project>
python -X utf8 skills/short-drama-edit/scripts/edit_tool.py render <project>/剧集/EP001 --project-root <project>
python -X utf8 skills/short-drama-edit/scripts/edit_tool.py verify <project>/剧集/EP001 --project-root <project>

# 交付
python -X utf8 skills/sshzyu-media/scripts/project_tool.py package <project> --episode EP001
python -X utf8 skills/sshzyu-media/scripts/project_tool.py export  <project> --out <export-dir>
```
