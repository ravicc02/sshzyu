<template>
  <div class="model-picker-wrap">
    <button ref="trigger" type="button" class="model-summary" aria-haspopup="dialog" :aria-expanded="open" :aria-controls="popupId" :disabled="disabled" @click="toggle"><span>{{ loading ? '正在切换…' : (config.model || '选择模型') + ' · ' + effortLabel }}</span><Icon name="chevronDown" size="xs" /></button>
    <Teleport to="body"><div v-if="open" :id="popupId" ref="panel" class="agent-model-popover" :style="position" role="dialog" :aria-labelledby="`${popupId}-title`" @keydown="onKeys">
      <div class="picker-heading"><h3 :id="`${popupId}-title`">切换模型</h3><button type="button" aria-label="关闭模型菜单" @click="close(true)">×</button></div>
      <p v-if="busy || loading" class="picker-note" role="status">{{ loading ? '正在应用模型设置…' : '任务结束后可切换模型，当前任务会使用原配置。' }}</p>
      <div class="picker-lists"><section v-for="purpose in purposes" :key="purpose" class="picker-group" role="radiogroup" :aria-label="purpose === 'text' ? '对话模型' : '生图模型'">
        <h4>{{ purpose === 'text' ? '对话模型' : '生图模型' }}<span v-if="purpose === 'image' && !config.image_api_key_id">未启用</span></h4>
        <p v-if="!models(purpose).length" class="picker-empty">在设置中添加常用模型</p>
        <button v-for="model in models(purpose)" :key="model" type="button" role="radio" class="model-option" :data-model-purpose="purpose" :aria-checked="selected(purpose) === model" :disabled="busy || loading" @click="choose(purpose, model)"><Icon :name="purpose === 'text' ? 'chat' : 'photograph'" size="sm" /><span>{{ model }}</span><Icon v-if="selected(purpose) === model" name="check" size="sm" /></button>
      </section></div>
      <section class="effort-picker" aria-label="思考强度">
        <div class="effort-heading"><span>思考强度</span><strong>{{ levels.length ? agentReasoningLabels[levels[effortIndex]!] : '—' }}</strong></div>
        <p v-if="capabilitiesLoading" class="picker-empty" role="status">正在读取模型档位…</p>
        <div v-else-if="capabilitiesError" class="effort-error"><span>{{ capabilitiesError }}</span><button type="button" @click="emit('reload-capabilities')">重试</button></div>
        <template v-else-if="levels.length > 1">
          <input v-model.number="effortIndex" type="range" min="0" :max="levels.length - 1" step="1" aria-label="模型思考强度" :aria-valuetext="agentReasoningLabels[levels[effortIndex]!]" :disabled="busy || loading" :style="{ '--effort-fill': `${effortIndex / (levels.length - 1) * 100}%` }" @change="changeEffort" />
          <div class="effort-ticks" aria-hidden="true"><span v-for="level in levels" :key="level">{{ agentReasoningLabels[level] }}</span></div>
        </template>
        <p v-else class="picker-empty">{{ levels.length === 1 ? '该模型使用固定思考档位' : '该模型未声明可调节的思考档位' }}</p>
      </section>
      <button type="button" class="manage-models" @click="manage"><Icon name="cog" size="sm" />管理常用模型 <span>→</span></button>
    </div></Teleport>
  </div>
</template>
<script setup lang="ts">
import { computed, watch, nextTick, onBeforeUnmount, onMounted, ref, useId, type CSSProperties } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAgentMotion } from '@/composables/useAgentMotion'
import type { AgentModelCapability, AgentReasoningEffort, AgentConfig } from '@/api/agent'
import { agentReasoningEfforts, agentReasoningLabels } from '@/utils/agentReasoning'
import type { AgentModelPurpose } from '@/utils/agentPreferences'
const props = defineProps<{ config: AgentConfig; textModels: string[]; imageModels: string[]; capabilities?: AgentModelCapability[]; capabilitiesLoading?: boolean; capabilitiesError?: string; disabled?: boolean; busy?: boolean; loading?: boolean }>()
const emit = defineEmits<{ select: [purpose: AgentModelPurpose, model: string]; manage: []; effort: [value: AgentReasoningEffort]; 'reload-capabilities': [] }>()
const motion = useAgentMotion()
const purposes = ['text', 'image'] as const, popupId = `agent-model-picker-${useId()}`
const open = ref(false), trigger = ref<HTMLButtonElement>(), panel = ref<HTMLElement>(), position = ref<CSSProperties>({})
const capability = computed(() => props.capabilities?.find(item => item.id === props.config.model))
const levels = computed(() => agentReasoningEfforts.filter(level => capability.value?.reasoning_levels?.includes(level)))
const currentEffort = computed(() => {
  const value = props.config.reasoning_effort || 'medium'
  return levels.value.includes(value) ? value : capability.value?.default_reasoning_effort || levels.value[0]
})
const effortIndex = ref(0), effortLabel = computed(() => currentEffort.value ? agentReasoningLabels[currentEffort.value] : agentReasoningLabels[props.config.reasoning_effort || 'medium'])
watch(() => [currentEffort.value, props.config.model, levels.value.join(), props.loading], () => {
  if (!props.loading) effortIndex.value = Math.max(0, levels.value.indexOf(currentEffort.value!))
}, { immediate: true })
function changeEffort() {
  const value = levels.value[effortIndex.value]
  if (!props.busy && !props.loading && value && value !== currentEffort.value) emit('effort', value)
}
function selected(purpose: AgentModelPurpose) { return purpose === 'text' ? props.config.model : props.config.image_model }
function models(purpose: AgentModelPurpose) { return purpose === 'text' ? props.textModels : props.imageModels }
function place() {
  const bounds = trigger.value?.getBoundingClientRect()
  if (!bounds) return
  const width = Math.min(350, window.innerWidth - 24), left = Math.max(12, Math.min(bounds.left, window.innerWidth - width - 12)), above = bounds.top - 12, below = window.innerHeight - bounds.bottom - 12
  position.value = above >= below ? { width: `${width}px`, left: `${left}px`, bottom: `${window.innerHeight - bounds.top + 8}px`, maxHeight: `${Math.max(120, Math.min(580, above - 8))}px` } : { width: `${width}px`, left: `${left}px`, top: `${bounds.bottom + 8}px`, maxHeight: `${Math.max(120, Math.min(580, below - 8))}px` }
}
async function toggle() { if (props.disabled) return; if (open.value) return; place(); open.value = true; await nextTick(); motion.reveal(panel.value); (panel.value?.querySelector<HTMLButtonElement>('.model-option[aria-checked=true]:not(:disabled)') || panel.value?.querySelector<HTMLButtonElement>('.model-option:not(:disabled), [aria-label="关闭模型菜单"]'))?.focus({ preventScroll: true }) }
function close(restore = false) { motion.cancel(panel.value); open.value = false; if (restore) trigger.value?.focus() }
function choose(purpose: AgentModelPurpose, model: string) { if (props.busy || props.loading) return; if (selected(purpose) !== model) emit('select', purpose, model) }
function manage() { close(true); emit('manage') }
function outside(event: Event) { const target = event.target as Node | null; if (open.value && target && !panel.value?.contains(target) && !trigger.value?.contains(target)) close() }
function scrolled() { if (open.value) place() }
function resized() { if (open.value) place() }
function onKeys(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true); return }
  if ((event.target as HTMLElement)?.tagName === 'INPUT' && (event.target as HTMLInputElement).type === 'range') return
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  const buttons = Array.from(panel.value?.querySelectorAll<HTMLButtonElement>('.model-option:not(:disabled)') || [])
  if (!buttons.length) return
  event.preventDefault()
  const current = buttons.indexOf(document.activeElement as HTMLButtonElement)
  const index = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : event.key === 'ArrowDown' ? (current + 1) % buttons.length : (current <= 0 ? buttons.length - 1 : current - 1)
  buttons[index]?.focus()
}
onMounted(() => { document.addEventListener('pointerdown', outside); window.addEventListener('resize', resized); document.addEventListener('scroll', scrolled, true) })
onBeforeUnmount(() => { document.removeEventListener('pointerdown', outside); window.removeEventListener('resize', resized); document.removeEventListener('scroll', scrolled, true) })
</script>
<style scoped>
/* finesse · component: AgentModelPicker · register=product · tokens: inherited */
.model-picker-wrap { min-width:0; }.model-summary { display:inline-flex; align-items:center; gap:6px; min-width:0; max-width:240px; min-height:32px; padding:6px; font-size:11px; color:var(--workspace-muted); border-radius:6px; }.model-summary span { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }@media (hover: hover) {.model-summary:hover { background:var(--workspace-soft); color:var(--workspace-ink); } }.model-summary:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:2px; }.model-summary:disabled { cursor:not-allowed; opacity:.5; }
.agent-model-popover { --picker-bg:#fff; --picker-ink:#202124; --picker-muted:#64646d; --picker-line:#e5e5e9; --picker-soft:#f6f6f7; --picker-accent:#0064c9; position:fixed; display:flex; flex-direction:column; padding:14px 0 0; z-index:80; margin:0; background:var(--picker-bg); color:var(--picker-ink); border:1px solid var(--picker-line); border-radius:12px; box-shadow:0 12px 36px #0002; overflow:hidden; }
:global(.dark .agent-model-popover) { --picker-bg:#222225; --picker-ink:#f5f5f7; --picker-muted:#b0b0b9; --picker-line:#3d3d44; --picker-soft:#2d2d31; --picker-accent:#69aeff; color-scheme:dark; }.picker-heading { display:flex; justify-content:space-between; align-items:center; padding:0 14px; }.picker-heading h3 { font-size:13px; font-weight:600; }.picker-heading button { width:28px; height:28px; font-size:20px; color:var(--picker-muted); }.picker-lists { overflow-y:auto; min-height:0; padding:0 8px 8px; }.picker-group h4 { display:flex; justify-content:space-between; padding:12px 8px 7px; color:var(--picker-muted); font-size:10px; font-weight:500; }.picker-group h4 span { font-weight:400; }.model-option { display:flex; width:100%; align-items:center; gap:8px; padding:10px 8px; font-size:12px; text-align:left; border-radius:6px; }.model-option span { flex:1; min-width:0; overflow-wrap:anywhere; }.model-option[aria-checked=true] { background:var(--picker-soft); }@media (hover: hover) {.model-option:hover { background:var(--picker-soft); } }.model-option[aria-checked=true] { color:var(--picker-accent); }.model-option:disabled { opacity:.5; cursor:not-allowed; }.picker-empty,.picker-note { font-size:11px; line-height:1.8; color:var(--picker-muted); padding:6px 8px; }.picker-note { margin:0 14px 8px; padding:8px; background:var(--picker-soft); border-radius:6px; }.manage-models { display:flex; align-items:center; gap:8px; border-top:1px solid var(--picker-line); padding:12px 16px; font-size:11px; text-align:left; color:var(--picker-muted); }.manage-models span { margin-left:auto; }@media (hover: hover) {.manage-models:hover { background:var(--picker-soft); color:var(--picker-ink); } }.agent-model-popover :where(button,input):focus-visible { outline:2px solid var(--picker-accent); outline-offset:-2px; }@media(max-width:640px) { .model-summary { max-width:155px; } }
.effort-picker { flex-shrink:0; border-top:1px solid var(--picker-line); padding:14px 18px 12px; }.effort-heading { display:flex; align-items:center; justify-content:space-between; font-size:12px; color:var(--picker-muted); }.effort-heading strong { font-weight:500; color:var(--picker-accent); }.effort-picker input { appearance:none; display:block; width:100%; height:24px; margin:12px 0 8px; border-radius:20px; background:linear-gradient(to right,var(--picker-accent) var(--effort-fill),var(--picker-line) var(--effort-fill)); cursor:pointer; }.effort-picker input::-webkit-slider-thumb { appearance:none; width:30px; height:30px; border-radius:50%; background:var(--picker-ink); border:2px solid var(--picker-bg); }.effort-picker input::-moz-range-thumb { width:26px; height:26px; border-radius:50%; background:var(--picker-ink); border:2px solid var(--picker-bg); }.effort-picker input:disabled { cursor:not-allowed; opacity:.5; }.effort-ticks { display:flex; justify-content:space-between; color:var(--picker-muted); font-size:10px; }.manage-models { flex-shrink:0; }
.effort-error { display:flex; align-items:center; justify-content:space-between; margin-top:10px; font-size:11px; color:var(--picker-muted); }.effort-error button { padding:5px 8px; color:var(--picker-accent); }

.agent-model-popover { border-radius:14px; box-shadow:0 12px 32px color-mix(in srgb,var(--picker-ink) 12%,transparent); }
.picker-heading { padding-bottom:4px; }
.picker-heading button { border-radius:7px; }
@media (hover: hover) {
.picker-heading button:hover { background:var(--picker-soft); } }
.model-option { min-height:38px; border:1px solid transparent; border-radius:8px; }
.model-option[aria-checked=true] { border-color:var(--picker-line); font-weight:500; }
.model-option:active:not(:disabled), .manage-models:active { transform:translateY(1px); }
.model-option:focus-visible, .manage-models:focus-visible { outline:2px solid var(--picker-accent); outline-offset:-2px; }
.model-summary:active:not(:disabled) { transform:translateY(1px); }

</style>
