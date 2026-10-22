<template>
  <component :is="inline ? 'div' : 'details'" class="agent-process" :class="{ 'trace-inline': inline }" aria-label="思考与工具调用过程" @toggle="motion.disclose">
    <summary v-if="!inline" class="process-heading"><Icon name="chevronRight" size="xs" class="process-chevron" /><span class="process-label">{{ processLabel }}</span><span class="process-counts">{{ processCounts }}</span><span v-if="totalDuration" class="process-duration">{{ totalDuration }}</span></summary>
    <div v-for="step in steps" :key="step.id" class="process-step" :data-step-id="step.id" :data-step-status="step.status">
      <button type="button" class="step-toggle" :aria-expanded="isOpen(step)" :aria-controls="panelId(step)" @click="toggle(step)">
        <span class="step-icon" :class="{ working: active(step), failed: step.status === 'failed' }"><Icon :name="step.kind !== 'tool' ? 'sparkles' : step.name === 'generate_image' ? 'photograph' : step.name === 'ask_user' ? 'chat' : 'document'" size="xs" /></span>
        <span class="step-title" :title="step.arguments?.prompt || step.name">{{ step.name === 'ask_user' && step.status === 'completed' ? '已确认回答' : step.kind === 'tool' ? `${agentToolLabel(step.name)}${step.name === 'generate_image' && inline && step.arguments?.prompt ? ' · ' + step.arguments.prompt : ''}` : active(step) ? '正在思考' : step.summary ? '思考摘要' : '模型思考' }}</span>
        <span class="step-state" :class="[`state-${step.status}`, { 'sr-only': inline }]">{{ statusLabel(step) }}</span>
        <Icon v-if="inline && step.status === 'completed'" name="check" size="sm" class="tool-check" /><span v-else-if="inline && active(step)" class="tool-spinner" aria-hidden="true" /><Icon v-else-if="inline && step.status === 'failed'" name="x" size="sm" class="tool-failed" />
        <time v-if="elapsed(step)" class="step-duration" :title="'本阶段耗时'">{{ elapsed(step) }}</time>
        <Icon name="chevronRight" size="xs" class="step-chevron" :class="{ expanded: isOpen(step) }" />
      </button>
      <Transition :css="false" @enter="motion.enter" @enter-cancelled="motion.cancel"><div v-if="isOpen(step)" :id="panelId(step)" class="step-content">
        <template v-if="step.kind === 'reasoning'">
          <div v-if="step.summary" class="reasoning-summary" v-html="renderAgentMarkdown(step.summary)" />
          <p v-else class="step-note">{{ active(step) ? '正在接收模型公开的思考摘要…' : '该模型未返回可展示的思考摘要。' }}</p>
          <div class="step-metadata"><span>模型 {{ step.name }}</span><span v-if="step.summary">模型提供的公开摘要</span></div>
        </template>
        <template v-else-if="step.name === 'save_work_plan'"><p class="step-note">已保存配置卡，批准后才会执行生图。</p><code>save_work_plan</code></template>
        <template v-else-if="step.name === 'ask_user'"><p class="step-note question-result">{{ step.summary }}</p><div class="step-metadata"><code>ask_user</code></div></template>
        <template v-else-if="step.name !== 'generate_image'"><p class="step-note">{{ step.summary || (step.status === 'running' ? '工具正在执行。' : step.status === 'queued' ? '已创建工具调用，等待执行。' : '工具结果已返回。') }}</p><div class="step-metadata"><code>{{ step.name }}</code></div></template>
        <template v-else>
          <div v-if="step.arguments" class="tool-input"><h4>工具输入 <code>{{ step.name }}</code></h4><p class="tool-prompt">{{ step.arguments.prompt }}</p><dl class="tool-specs"><div><dt>模型</dt><dd>{{ step.arguments.model }}</dd></div><div><dt>规格</dt><dd>{{ step.arguments.image_size }} · {{ step.arguments.aspect_ratio }}</dd></div><div><dt>参考图</dt><dd>{{ step.arguments.reference_count }} 张</dd></div></dl></div>
          <p v-if="step.asset_ids?.length" class="tool-output">已生成 {{ step.asset_ids.length }} 张图片，结果已附在本条回复中。</p>
          <p v-else-if="step.status === 'running'" class="step-note">工具正在生成图片，结果返回后会显示在对话中。</p>
          <p v-else-if="step.status === 'queued'" class="step-note">已创建工具调用，等待执行。</p>
        </template>
        <p v-if="step.error_code" class="step-error" role="status">{{ agentError(step.error_code, step.failure) }}</p>
        <p v-if="step.cost_usd" class="step-cost">已确认费用 ${{ step.cost_usd }}</p>
        <p v-else-if="step.status === 'reconciling'" class="step-note">正在读取已保存的响应。</p>
      </div></Transition>
    </div>
  </component>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAgentMotion } from '@/composables/useAgentMotion'
import type { AgentTraceStep } from '@/api/agent'
import { agentError, agentToolLabel, renderAgentMarkdown } from '@/utils/agentPresentation'
const props = defineProps<{ steps: AgentTraceStep[]; inline?: boolean }>()
const motion = useAgentMotion()
const expanded = ref<Record<string, boolean>>({})
const now = ref(Date.now())
const instanceId = useId()
let clock: ReturnType<typeof setInterval> | undefined
const active = (step: AgentTraceStep) => ['running', 'reconciling', 'waiting_user'].includes(step.status)
const isOpen = (step: AgentTraceStep) => expanded.value[step.id] ?? (!props.inline && active(step))
const toggle = (step: AgentTraceStep) => { expanded.value[step.id] = !isOpen(step) }
const panelId = (step: AgentTraceStep) => `${instanceId}-${step.id}`
function statusLabel(step: AgentTraceStep) {
  return ({ waiting_user: '等待回答', queued: '待执行', running: step.kind === 'tool' ? '执行中' : '思考中', reconciling: '恢复中', completed: '已完成', failed: '失败', cancelled: '已停止', expired: '已超时', skipped: '未执行' } as Record<string, string>)[step.status] || '处理中'
}
const processLabel = computed(() => {
  const step = props.steps.find(active)
  return step?.kind === 'reasoning' ? '正在思考' : step?.status === 'waiting_user' ? '等待回答' : step ? '正在处理' : '思考与工具'
})
const processCounts = computed(() => {
  const complete = props.steps.filter(step => step.status === 'completed').length
  const failed = props.steps.filter(step => step.status === 'failed').length
  return [`已完成 ${complete}/${props.steps.length}`, failed ? `失败 ${failed} 项` : ''].filter(Boolean).join(' · ')
})
const totalDuration = computed(() => {
  const starts = props.steps.map(step => Date.parse(step.started_at)).filter(Number.isFinite)
  const ends = props.steps.map(step => step.ended_at ? Date.parse(step.ended_at) : Number.NaN).filter(Number.isFinite)
  if (!starts.length || (!ends.length && !props.steps.some(active))) return ''
  const end = props.steps.some(active) ? now.value : Math.max(...ends)
  const seconds = Math.max(0, Math.floor((end - Math.min(...starts)) / 1000))
  return seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`
})
function elapsed(step: AgentTraceStep) {
  if (step.status === 'queued') return ''
  const start = Date.parse(step.started_at), end = step.ended_at ? Date.parse(step.ended_at) : active(step) ? now.value : Number.NaN
  if (!Number.isFinite(start) || !Number.isFinite(end)) return ''
  const seconds = Math.max(0, Math.floor((end - start) / 1000))
  return seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`
}
watch(() => props.steps.some(active), running => {
  clearInterval(clock)
  now.value = Date.now()
  if (running) clock = setInterval(() => { now.value = Date.now() }, 1000)
}, { immediate: true })
onBeforeUnmount(() => clearInterval(clock))
</script>
<style scoped>
/* finesse · component: AgentTrace · register=product · tokens: inherited */
.agent-process { --trace-muted:var(--workspace-muted); margin:0 0 18px; color:var(--workspace-ink); }
.process-heading { display:flex; align-items:center; flex-wrap:wrap; gap:5px 8px; min-height:30px; width:fit-content; max-width:100%; cursor:pointer; list-style:none; font-size:12px; color:var(--trace-muted); }
.process-heading::-webkit-details-marker { display:none; }.process-heading:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:4px; }.process-chevron { flex-shrink:0; opacity:.7; }.agent-process[open] > .process-heading .process-chevron { transform:rotate(90deg); }.process-label { font-weight:500; }.process-counts { font-size:11px; }.process-duration { font-size:11px; opacity:.8; font-variant-numeric:tabular-nums; }.process-duration::before { content:'·'; padding-right:8px; }
.process-step { margin-left:5px; padding-left:14px; border-left:1px solid var(--workspace-divider); }.process-step:first-of-type { margin-top:8px; }.step-toggle { display:flex; align-items:center; gap:8px; width:100%; min-height:34px; padding:6px 4px; text-align:left; font-size:12px; color:var(--trace-muted); border-radius:5px; }@media (hover: hover) {.step-toggle:hover { background:var(--workspace-soft); color:var(--workspace-ink); } }.step-toggle:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:2px; }.step-icon { display:flex; align-items:center; flex:0 0 16px; }.step-title { flex:1; min-width:0; font-weight:400; }.step-state,.step-duration { white-space:nowrap; font-size:10px; font-variant-numeric:tabular-nums; }.state-failed,.failed { color:#b13730; }.state-reconciling { color:#936400; }:global(.dark .agent-process .state-failed),:global(.dark .agent-process .failed) { color:#ffaaa3; }:global(.dark .agent-process .state-reconciling) { color:#f2ce7b; }.step-chevron { opacity:.6; flex-shrink:0; }.step-chevron.expanded { transform:rotate(90deg); }
.step-content { margin:3px 4px 12px 24px; padding:5px 0; overflow-wrap:anywhere; font-size:12px; line-height:1.85; }.step-note,.step-cost,.step-metadata,.tool-specs dt { color:var(--trace-muted); }.question-result { white-space:pre-wrap; }.step-metadata { display:flex; flex-wrap:wrap; gap:6px 16px; margin-top:10px; font-size:10px; }.step-error { margin-top:8px; color:#b13730; }:global(.dark .agent-process .step-error) { color:#ffaaa3; }.step-cost { margin-top:8px; font-size:10px; }.tool-input h4 { display:flex; gap:12px; margin-bottom:6px; font-size:10px; font-weight:400; color:var(--trace-muted); }.tool-input h4 code { font:inherit; }.tool-prompt { white-space:pre-wrap; max-height:180px; overflow-y:auto; padding:10px 12px; border-radius:6px; background:var(--workspace-soft); }.tool-specs { display:flex; flex-wrap:wrap; gap:8px 20px; margin-top:10px; }.tool-specs dt { font-size:10px; }.tool-specs dd { font-size:11px; overflow-wrap:anywhere; }.tool-output { margin-top:10px; }
.reasoning-summary :deep(p) { margin-bottom:10px; }.reasoning-summary :deep(ul),.reasoning-summary :deep(ol) { padding-left:20px; margin:8px 0; }.reasoning-summary :deep(ul) { list-style:disc; }.reasoning-summary :deep(ol) { list-style:decimal; }.reasoning-summary :deep(pre) { white-space:pre-wrap; padding:10px; background:var(--workspace-soft); border-radius:6px; }.reasoning-summary :deep(a) { color:var(--workspace-accent); text-decoration:underline; }.reasoning-summary :deep(h1),.reasoning-summary :deep(h2),.reasoning-summary :deep(h3),.reasoning-summary :deep(h4) { font-size:13px; font-weight:600; margin:12px 0 8px; }.reasoning-summary :deep(table) { display:block; max-width:100%; overflow-x:auto; }.reasoning-summary :deep(th),.reasoning-summary :deep(td) { border:1px solid var(--workspace-divider); padding:5px 8px; }
.working { animation:trace-pulse 1.8s ease-in-out infinite; }@keyframes trace-pulse { 50% { opacity:.4; } }@media(prefers-reduced-motion:reduce) { .working { animation:none; } }@media(max-width:640px) { .process-step { padding-left:8px; }.step-content { margin-left:8px; }.step-state,.step-duration { font-size:10px; } }

.process-heading { padding:5px 7px; margin-left:-7px; border-radius:8px; }
@media (hover: hover) {
.process-heading:hover { background:var(--workspace-soft); color:var(--workspace-ink); } }
.process-counts, .process-duration { font-variant-numeric:tabular-nums; }
.process-chevron, .step-chevron { transition:transform 160ms ease-out; }
.step-toggle { min-height:38px; padding:7px 6px; border-radius:8px; }
.step-icon { flex:0 0 24px; width:24px; height:24px; justify-content:center; border-radius:7px; background:var(--workspace-soft); }
.step-icon.working { color:var(--workspace-accent); background:var(--workspace-selected); }
.step-state { padding:2px 6px; border-radius:5px; background:var(--workspace-soft); }
.step-content { margin-left:38px; }
.step-metadata { gap:6px 12px; }
.step-toggle:active { transform:translateY(1px); }
@media(max-width:640px) { .step-content { margin-left:12px; } }
@media(prefers-reduced-motion:reduce) { .process-chevron, .step-chevron { transition:none; } }

.tool-check { color:#39856c; }.tool-failed { color:#b13730; }:global(.dark .tool-check) { color:#90d8ba; }:global(.dark .tool-failed) { color:#ffaaa3; }.tool-spinner { width:13px; height:13px; flex-shrink:0; border:1.5px solid var(--workspace-divider); border-top-color:var(--workspace-accent); border-radius:50%; animation:inline-tool-spin 1s linear infinite; }@keyframes inline-tool-spin { to { transform:rotate(360deg); } }@media(prefers-reduced-motion:reduce) { .tool-spinner { animation:none; } }
.trace-inline { margin:0; }.trace-inline .process-step { border-left:0; margin-left:0; padding-left:0; }.trace-inline .step-toggle { min-height:42px; border:1px solid var(--workspace-divider); border-radius:22px; padding:9px 13px; margin:8px 0; }.trace-inline .step-title { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }.trace-inline .step-icon { background:transparent; }.trace-inline .step-state { background:transparent; }.trace-inline .step-content { margin:5px 12px 14px; }
</style>
