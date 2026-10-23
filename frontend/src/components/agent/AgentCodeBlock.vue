<template>
  <section class="agent-code-block" :aria-label="label">
    <header class="content-block-toolbar"><span class="content-block-label">{{ label }}</span><AgentBlockCopy :text="source" :label="plain ? '复制文本块' : '复制代码块'" /></header>
    <pre tabindex="0" :aria-label="plain ? '文本内容' : `${label} 代码`"><code v-if="highlighted" class="hljs" v-html="highlighted" /><code v-else>{{ source }}</code></pre>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import AgentBlockCopy from './AgentBlockCopy.vue'
const props = withDefaults(defineProps<{ source: string; language?: string }>(), { language: '' })
const language = computed(() => props.language.trim().split(/\s+/)[0]?.toLowerCase() || '')
const plain = computed(() => ['', 'text', 'txt', 'plaintext', 'prompt', '提示词'].includes(language.value))
const label = computed(() => ['prompt', '提示词'].includes(language.value) ? '提示词' : plain.value ? '文本' : language.value.toUpperCase())
const highlighted = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let revision = 0
watch(() => [props.source, language.value] as const, () => {
  const current = ++revision
  clearTimeout(timer); highlighted.value = ''
  if (plain.value || props.source.length > 50000) return
  timer = setTimeout(async () => {
    try {
      const { highlightAgentCode } = await import('@/utils/agentCodeHighlight')
      if (revision === current) highlighted.value = highlightAgentCode(props.source, language.value)
    } catch { /* The original code stays readable when highlighting is unavailable. */ }
  }, 80)
}, { immediate: true })
onBeforeUnmount(() => { revision++; clearTimeout(timer) })
</script>
<style scoped>
.agent-code-block { min-width:0; margin:14px 0; border:1px solid var(--workspace-divider); border-radius:10px; overflow:hidden; background:var(--workspace-soft); }
.content-block-toolbar { display:flex; align-items:center; justify-content:space-between; gap:12px; min-height:40px; padding:3px 9px 3px 15px; border-bottom:1px solid var(--workspace-divider); }
.content-block-label { min-width:0; overflow:hidden; text-overflow:ellipsis; color:var(--workspace-ink); opacity:.7; font-size:11px; font-weight:600; letter-spacing:.04em; }
pre { max-width:100%; overflow-x:auto; margin:0; padding:15px 17px 18px; white-space:pre; tab-size:2; line-height:1.75; font-size:12px; }
pre:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:-3px; }
code { font-family:ui-monospace,SFMono-Regular,Consolas,'Liberation Mono',monospace; color:var(--workspace-ink); }
.hljs :deep(.hljs-keyword),.hljs :deep(.hljs-selector-tag),.hljs :deep(.hljs-literal) { color:var(--agent-syntax-keyword); }
.hljs :deep(.hljs-string),.hljs :deep(.hljs-attr),.hljs :deep(.hljs-addition) { color:var(--agent-syntax-string); }
.hljs :deep(.hljs-number),.hljs :deep(.hljs-built_in),.hljs :deep(.hljs-type) { color:var(--agent-syntax-number); }
.hljs :deep(.hljs-title),.hljs :deep(.hljs-function),.hljs :deep(.hljs-section) { color:var(--agent-syntax-title); }
.hljs :deep(.hljs-comment),.hljs :deep(.hljs-quote),.hljs :deep(.hljs-meta) { color:var(--workspace-muted); }
.hljs :deep(.hljs-deletion) { color:var(--agent-syntax-keyword); }
</style>

<style>
/* Scoped Vue selectors cannot carry the theme ancestor through deep tokens.
   Keep only namespaced syntax variables global; all component geometry is scoped. */
.agent-code-block { --agent-syntax-keyword:#a12f50; --agent-syntax-string:#146242; --agent-syntax-number:#825125; --agent-syntax-title:#5b42a4; }
.dark .agent-code-block { --agent-syntax-keyword:#ff9aae; --agent-syntax-string:#a4d5af; --agent-syntax-number:#f2c088; --agent-syntax-title:#cbb5fa; }
</style>
