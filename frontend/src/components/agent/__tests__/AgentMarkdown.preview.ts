import { createApp, h, ref } from 'vue'
import AgentChat from '../AgentChat.vue'
import AgentBlockCopy from '../AgentBlockCopy.vue'
import '@/style.css'
import '@/styles/workspace-shell.css'
import './AgentMarkdown.preview.css'
const reply = [
  '## 先看这段实现',
  '下面的代码、提示词和每个独立内容块，都可以分别复制。**正文排版**、列表、表格和链接会保留。',
  '```typescript\ninterface Task { title: string; done: boolean }\nconst task: Task = { title: "生成演示文稿", done: false }\nconsole.log(task.title)\n```',
  '```prompt\n设计一组统一风格的演讲幻灯片：暖白底色、深色正文与克制的蓝色重点。\n每页只表达一个观点，保留参考图片的主体特征。\n```',
  '> 先确认目标，再组织内容。素材不足时继续沟通，已明确的内容直接执行。',
  '| 能力 | 状态 |\n| --- | --- |\n| 代码高亮与块复制 | 已接入 |\n| 图表和数学公式 | 按需渲染 |',
  '## 从需求到结果',
  '```mermaid\nflowchart LR\n  A[用户需求] --> B{信息齐全？}\n  B -->|是| C[执行任务]\n  B -->|否| D[继续沟通]\n  D --> B\n  C --> E[返回结果]\n```',
  '行内公式 $E=mc^2$ 也能正常排版；价格 $20 and $30 则仍是普通文本。',
  '$$\n\\int_0^1 x^2\\,dx = \\frac{1}{3}\n$$',
  '## 长代码与嵌套内容',
  '- 配置示例\n\n  ```json\n  {"title":"统一风格的演讲幻灯片", "aspect_ratio":"16:9", "description":"这是一行较长的内容，用来验证横向滚动不会挤宽整个消息区域，手机端同样保持在容器内。"}\n  ```',
  '- [x] 支持键盘访问\n- [x] 支持浅色与暗色\n- [ ] 继续补充内容',
  '[查看示例文档](https://example.com)',
].join('\n\n')
createApp({ setup() {
  const dark = ref(false); const streaming = ref(false)
  const toggle = () => { dark.value = !dark.value; document.documentElement.classList.toggle('dark', dark.value) }
  return () => h('div', { class:'workspace-shell preview-shell' }, [
    h('header', { class:'preview-toolbar' }, [h('div', [h('strong','Agent'), h('span','内容展示预览')]), h('div', [h('button',{onClick:toggle},dark.value?'切换浅色':'切换暗色'),h('button',{onClick:()=>streaming.value=!streaming.value}, streaming.value?'补全流式内容':'测试未完成内容')])]),
    h('main', { class:'preview-body' }, [h(AgentChat,{ messages:[{id:'q',role:'user',text:'给我一个代码示例、一段可复制的提示词，以及图表和公式。'},{id:'a',role:'assistant',text:streaming.value?'这是一段正在输出的内容。\n\n```typescript\nconst text = "正在接收"\n```\n\n```mermaid\nflowchart LR\nA -->':reply}]}),
      h('textarea', { 'aria-label':'检查复制结果', placeholder:'复制后可在这里粘贴确认', style:'width:100%;min-height:100px;margin-top:24px;padding:12px;border:1px solid var(--workspace-divider);border-radius:8px;background:var(--workspace-soft);color:var(--workspace-ink)' }),
      h('section',{class:'control-states'},[h('h3','复制控件状态'),...['default','hover','focus','active','disabled','loading','error','success'].map(state=>h('div',{class:'state-row'},[h('span',state),h(AgentBlockCopy,{text:'只复制这一块',class:`is-${state}`,disabled:state==='disabled'||state==='loading', 'data-state':state})]))])])])
} }).mount('#preview')
