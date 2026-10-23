<template><div class="agent-markdown"><MarkdownBody /></div></template>
<script setup lang="ts">
import { computed, defineComponent, Fragment, h, type VNodeChild } from 'vue'
import { agentMarkdownLink, parseAgentMarkdown, renderAgentInlineToken, sanitizeAgentHtml, type AgentMarkdownToken } from '@/utils/agentMarkdown'
import AgentBlockCopy from './AgentBlockCopy.vue'
import AgentCodeBlock from './AgentCodeBlock.vue'
import AgentDiagram from './AgentDiagram.vue'
import AgentMath from './AgentMath.vue'
const props = withDefaults(defineProps<{ text: string; copyable?: boolean }>(), { copyable: true })
const tokens = computed(() => parseAgentMarkdown(props.text))
function inline(items: AgentMarkdownToken[] = []): VNodeChild[] {
  return items.map((token, index) => {
    const key = `${index}-${token.type}`
    if (token.type === 'agent_math_inline') return h(AgentMath, { key, source: token.text || '' })
    if (['strong', 'em', 'del'].includes(token.type)) return h(token.type, { key }, inline(token.tokens))
    if (token.type === 'link') return h('a', { key, href: agentMarkdownLink(token.href), title: token.title || undefined, rel: 'noopener noreferrer' }, inline(token.tokens))
    if (token.type === 'image') return null
    if (token.type === 'br') return h('br', { key })
    if (token.type === 'html') return h('span', { key, innerHTML: sanitizeAgentHtml(token.text || token.raw) })
    if (token.type === 'text' && token.tokens) return h(Fragment, { key }, inline(token.tokens))
    return h('span', { key, innerHTML: renderAgentInlineToken(token) })
  })
}
function content(token: AgentMarkdownToken, index: number): VNodeChild {
  const children = () => blocks(token.tokens || [], false)
  switch (token.type) {
    case 'space': return null
    case 'code': {
      const language = token.lang?.trim().split(/\s+/)[0]?.toLowerCase() || ''
      if (language === 'mermaid') return h(AgentDiagram, { source: token.text || '' })
      if (['math', 'latex', 'tex'].includes(language)) return h(AgentMath, { source: token.text || '', display: true })
      return h(AgentCodeBlock, { source: token.text || '', language: token.lang || '' })
    }
    case 'agent_math_block': return h(AgentMath, { source: token.text || '', display: true })
    case 'heading': return h(`h${Math.min(6, Math.max(1, token.depth || 2))}`, inline(token.tokens))
    case 'paragraph': return h('p', inline(token.tokens))
    case 'text': return h('p', token.tokens ? inline(token.tokens) : token.text || '')
    case 'blockquote': return h('blockquote', children())
    case 'list': return h(token.ordered ? 'ol' : 'ul', { start: token.ordered ? token.start || undefined : undefined }, (token.items || []).map((item, i) => h('li', { key: i, class: { 'markdown-task': item.task } }, [item.task ? h('span', { class: 'task-mark', role: 'img', 'aria-label': item.checked ? '已完成' : '未完成' }, item.checked ? '☑' : '☐') : null, ...blocks(item.tokens || [], false)])))
    case 'table': {
      const header = (token.header || []).map((cell, i) => h('th', { key: i, style: { textAlign: token.align?.[i] || undefined } }, inline(cell.tokens)))
      const rows = (token.rows || []).map((row, i) => h('tr', { key: i }, row.map((cell, j) => h('td', { key: j, style: { textAlign: token.align?.[j] || undefined } }, inline(cell.tokens)))))
      return h('div', { class: 'markdown-table-scroll', tabindex: 0, 'aria-label': '表格，可横向滚动' }, [h('table', [h('thead', [h('tr', header)]), h('tbody', rows)])])
    }
    case 'hr': return h('hr')
    case 'html': return h('div', { innerHTML: sanitizeAgentHtml(token.text || token.raw) })
    default: return h('p', { key: index }, token.text || token.raw)
  }
}
function blocks(items: AgentMarkdownToken[], copyable: boolean): VNodeChild[] {
  return items.map((token, index) => {
    const node = content(token, index)
    const canCopy = copyable && ['paragraph', 'blockquote', 'list', 'table'].includes(token.type) && token.raw.trim().length > 0
    return h('div', { key: `${index}-${token.type}`, class: ['markdown-block', { 'copyable-block': canCopy }] }, [node, canCopy ? h(AgentBlockCopy, { text: token.raw.trimEnd(), iconOnly: true, label: token.type === 'table' ? '复制表格内容' : token.type === 'blockquote' ? '复制引用内容' : token.type === 'list' ? '复制列表内容' : '复制此段内容', class: 'prose-copy' }) : null])
  })
}
const MarkdownBody = defineComponent({ setup: () => () => h(Fragment, blocks(tokens.value, props.copyable)) })
</script>
<style scoped>
.agent-markdown { min-width:0; color:inherit; overflow-wrap:anywhere; font-size:inherit; line-height:inherit; }
.agent-markdown :deep(.markdown-block) { min-width:0; }
.agent-markdown :deep(.copyable-block) { position:relative; padding-right:36px; }
.agent-markdown :deep(.prose-copy) { position:absolute; top:-3px; right:0; opacity:0; transition:opacity .12s; }
@media(hover:hover) { .agent-markdown :deep(.copyable-block:hover > .prose-copy) { opacity:1; } }
.agent-markdown :deep(.copyable-block:focus-within > .prose-copy) { opacity:1; }
.agent-markdown :deep(.prose-copy[data-state=success]),.agent-markdown :deep(.prose-copy[data-state=error]) { opacity:1; }
.agent-markdown :deep(p) { margin:0 0 12px; }
.agent-markdown :deep(h1),.agent-markdown :deep(h2),.agent-markdown :deep(h3),.agent-markdown :deep(h4),.agent-markdown :deep(h5),.agent-markdown :deep(h6) { font-weight:600; line-height:1.55; margin:22px 0 10px; }
.agent-markdown :deep(h1) { font-size:20px; }.agent-markdown :deep(h2) { font-size:18px; }.agent-markdown :deep(h3),.agent-markdown :deep(h4) { font-size:16px; }.agent-markdown :deep(h5),.agent-markdown :deep(h6) { font-size:14px; }
.agent-markdown :deep(ul),.agent-markdown :deep(ol) { padding-left:24px; margin:10px 0 16px; }
.agent-markdown :deep(ul) { list-style:disc; }.agent-markdown :deep(ol) { list-style:decimal; }.agent-markdown :deep(li) { margin:5px 0; }.agent-markdown :deep(li p) { margin-bottom:5px; }
.agent-markdown :deep(.markdown-task) { list-style:none; position:relative; }.agent-markdown :deep(.task-mark) { position:absolute; left:-22px; color:var(--workspace-muted); }
.agent-markdown :deep(p code),.agent-markdown :deep(li code),.agent-markdown :deep(td code) { font-size:.9em; background:var(--workspace-soft); padding:2px 5px; border-radius:4px; }
.agent-markdown :deep(.agent-code-block code) { padding:0; border-radius:0; background:transparent; font-size:inherit; }
.agent-markdown :deep(a) { color:var(--workspace-accent); text-decoration:underline; text-underline-offset:3px; }
.agent-markdown :deep(a:focus-visible) { outline:2px solid var(--workspace-accent); outline-offset:3px; }
.agent-markdown :deep(blockquote) { padding:10px 16px; margin:12px 0; border-left:2px solid var(--workspace-divider); color:var(--workspace-muted); }
.agent-markdown :deep(blockquote > .markdown-block:last-child p) { margin-bottom:0; }
.agent-markdown :deep(hr) { margin:22px 0; border:0; border-top:1px solid var(--workspace-divider); }
.agent-markdown :deep(.markdown-table-scroll) { max-width:100%; overflow-x:auto; margin:16px 0; }
.agent-markdown :deep(table) { width:max-content; min-width:100%; border-collapse:collapse; font-size:13px; }
.agent-markdown :deep(th),.agent-markdown :deep(td) { max-width:420px; padding:9px 13px; border:1px solid var(--workspace-divider); white-space:normal; }
.agent-markdown :deep(th) { font-weight:600; background:var(--workspace-soft); text-align:left; }
.agent-markdown :deep(.markdown-table-scroll:focus-visible) { outline:2px solid var(--workspace-accent); outline-offset:2px; }
@media(hover:none),(pointer:coarse),(max-width:640px) { .agent-markdown :deep(.prose-copy) { opacity:1; }.agent-markdown :deep(.copyable-block) { padding-right:46px; } }
@media(prefers-reduced-motion:reduce) { .agent-markdown :deep(.prose-copy) { transition:none; } }
</style>
