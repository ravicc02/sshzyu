<template>
  <section class="agent-diagram" aria-label="Mermaid 图表">
    <header><span>MERMAID</span><div><button type="button" class="source-toggle" :aria-pressed="showSource" @click="showSource = !showSource">{{ showSource ? '查看图表' : '查看源码' }}</button><AgentBlockCopy :text="source" label="复制图表源码" /></div></header>
    <div v-if="!showSource && svg" class="diagram-canvas" tabindex="0" title="宽图可横向滚动查看" role="img" aria-label="已渲染的 Mermaid 图表" v-html="svg" />
    <p v-else-if="!showSource && loading" class="diagram-note" role="status">正在绘制图表…</p>
    <p v-else-if="!showSource && failed" class="diagram-note">图表暂无法显示，可查看或复制源码。</p>
    <pre v-if="showSource || failed" tabindex="0" aria-label="图表源码"><code>{{ source }}</code></pre>
  </section>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AgentBlockCopy from './AgentBlockCopy.vue'
const props = defineProps<{ source: string }>()
const svg = ref(''); const loading = ref(true); const failed = ref(false); const showSource = ref(false)
const dark = ref(false)
let revision = 0; let timer: ReturnType<typeof setTimeout> | undefined; let observer: MutationObserver | undefined
watch(() => [props.source, dark.value] as const, () => {
  const current = ++revision
  clearTimeout(timer); loading.value = true; failed.value = false; svg.value = ''
  timer = setTimeout(async () => {
    try {
      const { renderAgentDiagram } = await import('@/utils/agentDiagram')
      const result = await renderAgentDiagram(props.source, dark.value)
      if (revision === current) svg.value = result
    } catch { if (revision === current) failed.value = true }
    finally { if (revision === current) loading.value = false }
  }, 180)
}, { immediate: true })
onMounted(() => {
  const sync = () => { dark.value = document.documentElement.classList.contains('dark') }
  sync(); observer = new MutationObserver(sync); observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})
onBeforeUnmount(() => { revision++; clearTimeout(timer); observer?.disconnect() })
</script>
<style scoped>
.agent-diagram { min-width:0; margin:14px 0; border:1px solid var(--workspace-divider); border-radius:10px; overflow:hidden; background:var(--workspace-surface); }
header { display:flex; align-items:center; justify-content:space-between; gap:10px; min-height:40px; padding:3px 9px 3px 15px; border-bottom:1px solid var(--workspace-divider); color:var(--workspace-muted); font-size:11px; font-weight:600; letter-spacing:.04em; }
header > div { display:flex; align-items:center; gap:4px; }
.source-toggle { min-height:32px; padding:5px 8px; border-radius:6px; color:var(--workspace-muted); font-size:11px; font-weight:400; }
@media(hover:hover) { .source-toggle:hover { background:var(--workspace-soft); color:var(--workspace-ink); } }
.source-toggle:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:2px; }.source-toggle:active { transform:translateY(1px); }
.diagram-canvas { display:block; min-width:0; overflow:auto; padding:22px 18px; }
.diagram-canvas :deep(svg) { display:block; max-width:100%; height:auto; margin-inline:auto; }
.diagram-canvas:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:-3px; }
.diagram-note { padding:18px; margin:0; color:var(--workspace-muted); font-size:12px; }
pre { margin:0; padding:15px 17px; overflow:auto; white-space:pre; font-size:12px; line-height:1.75; background:var(--workspace-soft); }
pre:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:-3px; }
@media(pointer:coarse) { .source-toggle { min-height:44px; } }
</style>
