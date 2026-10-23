<template>
  <section v-if="display" class="agent-math-block" aria-label="数学公式"><header><span>公式</span><AgentBlockCopy :text="source" label="复制公式源码" /></header><div class="math-body"><span v-if="html" v-html="html" /><code v-else :title="failed ? '公式暂无法排版，已保留原文' : undefined">{{ source }}</code></div></section>
  <span v-else class="agent-math-inline"><span v-if="html" v-html="html" /><code v-else :title="failed ? '公式暂无法排版，已保留原文' : undefined">{{ source }}</code></span>
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import DOMPurify from 'dompurify'
import AgentBlockCopy from './AgentBlockCopy.vue'
const props = withDefaults(defineProps<{ source: string; display?: boolean }>(), { display: false })
const html = ref(''); const failed = ref(false); let revision = 0
watch(() => [props.source, props.display] as const, async () => {
  const current = ++revision; html.value = ''; failed.value = false
  if (props.source.length > 10000) { failed.value = true; return }
  try {
    const [{ default: katex }] = await Promise.all([import('katex'), import('katex/dist/katex.min.css')])
    const result = katex.renderToString(props.source, { displayMode: props.display, throwOnError: true, trust: false, strict: 'ignore', maxExpand: 1000, maxSize: 10 })
    if (current === revision) html.value = DOMPurify.sanitize(result, { USE_PROFILES: { html: true, mathMl: true, svg: true }, FORBID_TAGS: ['a', 'img', 'image', 'script', 'iframe', 'foreignObject'], ALLOW_DATA_ATTR: false })
  } catch { if (current === revision) failed.value = true }
}, { immediate: true })
onBeforeUnmount(() => revision++)
</script>
<style scoped>
.agent-math-block { min-width:0; margin:14px 0; border:1px solid var(--workspace-divider); border-radius:10px; background:var(--workspace-surface); }
header { display:flex; align-items:center; justify-content:space-between; gap:12px; padding:3px 9px 3px 15px; min-height:40px; border-bottom:1px solid var(--workspace-divider); color:var(--workspace-muted); font-size:11px; }
.math-body { max-width:100%; overflow-x:auto; padding:15px 18px; text-align:center; }
.math-body :deep(.katex-display) { margin:8px 0; }
.agent-math-inline { display:inline; color:inherit; }
code { color:var(--workspace-ink); overflow-wrap:anywhere; font-size:.9em; }
</style>
