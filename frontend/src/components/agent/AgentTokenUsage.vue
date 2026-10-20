<template>
  <span ref="trigger" class="token-usage" tabindex="0" :aria-label="`${label}：${value}`" :aria-describedby="show ? tooltipId : undefined" @mouseenter="open" @mouseleave="leave" @focus="open" @blur="close" @click="open" @keydown.esc.stop.prevent="close">
    {{ value }}
  </span>
  <Teleport v-if="show" :to="host">
    <div :id="tooltipId" ref="tooltip" class="token-usage-tooltip" role="tooltip" :style="position" @mouseleave="leaveTooltip" @keydown.esc.stop.prevent="close">
      <strong>{{ label }}</strong>
      <dl v-if="recorded"><dt>输入</dt><dd>{{ count(usage.input_tokens) }}</dd><dt>输出</dt><dd>{{ count(usage.output_tokens) }}</dd><dt>缓存读取</dt><dd>{{ count(usage.cache_read_tokens) }}</dd><dt>缓存写入</dt><dd>{{ count(usage.cache_creation_tokens) }}</dd><dt>合计</dt><dd>{{ count(usage.total_tokens) }}</dd></dl>
      <p v-if="usage.missing_usage_records">{{ count(usage.missing_usage_records) }} 次调用缺少 Token 记录{{ recorded ? '，仅统计已记录用量。' : '。' }}</p>
    </div>
  </Teleport>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useId } from 'vue'
import type { AgentUsageGroup } from '@/api/agent'
const props = withDefaults(defineProps<{ usage: AgentUsageGroup; label?: string }>(), { label: 'Token 用量' })
const count = (value: number) => new Intl.NumberFormat('zh-CN').format(value)
const recorded = computed(() => !props.usage.requests || props.usage.missing_usage_records < props.usage.requests)
const value = computed(() => recorded.value ? count(props.usage.total_tokens) : '未记录')
const trigger = ref<HTMLElement>(), tooltip = ref<HTMLElement>(), show = ref(false), host = ref<HTMLElement | string>('body')
const tooltipId = `agent-token-${useId()}`
const position = ref({ top: '0px', left: '0px' })
function place() {
  if (!show.value || !trigger.value || !tooltip.value) return
  const rect = trigger.value.getBoundingClientRect(), box = tooltip.value.getBoundingClientRect(), gap = 7
  const top = rect.top >= box.height + gap + 8 ? rect.top - box.height - gap : rect.bottom + gap
  position.value = { top: `${Math.max(8, Math.min(top, window.innerHeight - box.height - 8))}px`, left: `${Math.max(8, Math.min(rect.right - box.width, window.innerWidth - box.width - 8))}px` }
}
async function open() {
  // Keep the tooltip in the settings dialog's top layer and inherit its theme.
  host.value = trigger.value?.closest('dialog') || 'body'
  show.value = true
  await nextTick(); place()
}
function close() { show.value = false }
function inside(el: HTMLElement | undefined, target: EventTarget | null) { return target instanceof Node && !!el?.contains(target) }
function leave(event: MouseEvent) { if (!inside(tooltip.value, event.relatedTarget)) close() }
function leaveTooltip(event: MouseEvent) { if (!inside(trigger.value, event.relatedTarget)) close() }
function onEscape(event: KeyboardEvent) { if (event.key === 'Escape' && show.value) { event.preventDefault(); event.stopPropagation(); close() } }
window.addEventListener('resize', close)
window.addEventListener('scroll', close, true)
document.addEventListener('keydown', onEscape, true)
onBeforeUnmount(() => { window.removeEventListener('resize', close); window.removeEventListener('scroll', close, true); document.removeEventListener('keydown', onEscape, true) })
</script>
<style scoped>
/* finesse · component: AgentTokenUsage · register=product · tokens: inherited */
.token-usage { display:inline-block; cursor:help; font:inherit; color:inherit; font-variant-numeric:tabular-nums; border-radius:3px; }.token-usage:focus-visible { outline:2px solid var(--settings-accent,var(--workspace-accent)); outline-offset:4px; }
.token-usage-tooltip { position:fixed; z-index:1000; width:min(228px,calc(100vw - 16px)); padding:13px 15px; box-sizing:border-box; border:1px solid var(--settings-line,var(--workspace-divider)); border-radius:9px; background:var(--settings-bg,var(--workspace-surface)); color:var(--settings-ink,var(--workspace-ink)); box-shadow:0 8px 28px color-mix(in srgb,var(--settings-ink,var(--workspace-ink)) 14%,transparent); font-size:11px; font-weight:400; line-height:1.7; text-align:left; }
.token-usage-tooltip::after { content:''; position:absolute; inset:-8px 0; z-index:-1; }.token-usage-tooltip strong { display:block; font-size:11px; font-weight:550; margin-bottom:8px; }.token-usage-tooltip dl { display:grid; grid-template-columns:1fr auto; gap:5px 16px; }.token-usage-tooltip dt { color:var(--settings-muted,var(--workspace-muted)); }.token-usage-tooltip dd { margin:0; font-variant-numeric:tabular-nums; text-align:right; }.token-usage-tooltip p { margin-top:8px; color:var(--settings-muted,var(--workspace-muted)); font-size:10px; }
</style>
