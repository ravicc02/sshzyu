<template>
  <aside v-show="show" ref="panel" class="agent-artifact-panel" :class="{ persistent }" role="region" :aria-label="title" tabindex="-1" @keydown.esc.stop.prevent="emit('close')">
    <header class="artifact-heading"><button v-if="canBack" type="button" class="artifact-back" :aria-label="backLabel || '返回配置卡'" @click="emit('back')"><Icon name="chevronLeft" size="sm" /></button><div><h3>{{ title }}</h3><p v-if="subtitle" :title="subtitle">{{ subtitle }}</p></div><div class="artifact-actions"><slot name="actions" /><button type="button" class="artifact-close" aria-label="关闭右侧面板" @click="emit('close')"><Icon name="x" size="sm" /></button></div></header>
    <div class="artifact-body"><slot /></div>
  </aside>
</template>
<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAgentMotion } from '@/composables/useAgentMotion'
const props = defineProps<{ show: boolean; title: string; subtitle?: string; canBack?: boolean; backLabel?: string; persistent?: boolean }>()
const emit = defineEmits<{ close: []; back: []; overlay: [value: boolean] }>()
const panel = ref<HTMLElement>(), motion = useAgentMotion()
let media: MediaQueryList | undefined
function syncOverlay() { emit('overlay', !!media?.matches && !props.persistent) }
onMounted(() => { if (typeof window.matchMedia === 'function') { media = window.matchMedia('(max-width:1100px)'); media.addEventListener('change', syncOverlay); syncOverlay() } })
watch(() => props.persistent, syncOverlay)
onBeforeUnmount(() => media?.removeEventListener('change', syncOverlay))
watch(() => props.show, async show => { await nextTick(); if (show) { panel.value?.focus({ preventScroll: true }); motion.reveal(panel.value) } else motion.cancel(panel.value) })
</script>
<style scoped>
/* finesse · component: AgentArtifactPanel · register=product · tokens: inherited */
.agent-artifact-panel { display:flex; flex-direction:column; min-width:0; min-height:0; overflow:hidden; border-left:1px solid var(--workspace-divider); background:var(--workspace-surface); color:var(--workspace-ink); outline:none; }
.artifact-heading { display:flex; align-items:center; gap:10px; padding:17px 18px; border-bottom:1px solid var(--workspace-divider); flex-shrink:0; }.artifact-heading > div:first-of-type { min-width:0; }.artifact-heading h3 { font-size:14px; font-weight:600; }.artifact-heading p { margin-top:5px; color:var(--workspace-muted); font-size:11px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }.artifact-actions { margin-left:auto; display:flex; align-items:center; gap:8px; flex-shrink:0; }.artifact-close,.artifact-back { display:flex; align-items:center; justify-content:center; width:32px; height:32px; border-radius:7px; color:var(--workspace-muted); }.artifact-close:active,.artifact-back:active { transform:translateY(1px); }.artifact-body { flex:1; min-height:0; overflow:auto; overscroll-behavior:contain; padding:18px; }
:where(button,a):focus-visible { outline:2px solid var(--workspace-accent); outline-offset:3px; }
@media(hover:hover) { .artifact-close:hover,.artifact-back:hover { background:var(--workspace-soft); color:var(--workspace-ink); } }
@media(max-width:1100px) { .agent-artifact-panel { position:absolute; inset:0 0 0 auto; z-index:25; width:min(620px,100%); box-shadow:-14px 0 40px color-mix(in srgb,var(--workspace-ink) 12%,transparent); }.artifact-body { padding:16px; } }

.agent-artifact-panel.persistent { position:static; inset:auto; width:auto; box-shadow:none; }
@media(max-width:760px) { .agent-artifact-panel.persistent { height:65dvh; min-height:380px; border-left:0; border-top:1px solid var(--workspace-divider); } }
</style>
