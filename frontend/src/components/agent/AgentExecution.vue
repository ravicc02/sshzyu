<template>
  <component :is="finished && phases.length ? 'details' : 'div'" :key="finished ? 'finished' : 'live'" class="agent-execution" :class="{ 'execution-finished': finished }" aria-label="执行过程" @toggle="toggleProcess">
    <p v-if="finished && !phases.length" class="execution-duration-only">执行用时<span v-if="duration"> · {{ duration }}</span></p>
    <summary v-if="finished && phases.length" class="execution-heading" title="模型与工具执行耗时，不包含等待用户回答"><span class="process-label">执行过程<span v-if="duration"> · {{ duration }}</span></span><Icon name="chevronDown" size="xs" /><span v-if="failedCount" class="execution-failure">{{ failedCount }} 项未完成</span></summary>
    <Transition :css="false" @enter="motion.enter" @enter-cancelled="motion.cancel"><div v-if="!finished || processOpen" class="execution-timeline">
      <section v-for="phase in phases" :key="phase.id" class="execution-phase" :data-phase-id="phase.id">
        <details v-if="phase.model?.summary?.trim()" class="thought-disclosure" :open="finished || active(phase.model) ? true : undefined" @toggle="motion.disclose">
          <summary><Icon name="sparkles" size="xs" /><span>{{ active(phase.model) ? '正在思考' : '深度思考' }}</span><time v-if="elapsed(phase.model)" title="本次模型阶段耗时，包含摘要和回复生成">{{ elapsed(phase.model) }}</time><Icon name="chevronDown" size="xs" /></summary>
          <AgentMarkdown class="reasoning-summary" :text="phase.model.summary" :copyable="false" />
          <span class="thought-source">模型提供的公开摘要</span>
        </details>
        <p v-else-if="phase.model && active(phase.model) && !phase.model.commentary && !finished" class="thinking-pending" role="status"><span class="activity-dot" />正在思考<time v-if="elapsed(phase.model)">{{ elapsed(phase.model) }}</time></p>
        <AgentMarkdown v-if="phase.model?.commentary && (!finished || phase.tools.length)" class="stage-commentary" :text="phase.model.commentary" :copyable="false" />
        <component :is="finished ? 'div' : 'details'" v-if="phase.tools.length" class="tool-stage" :open="!finished && phase.running ? true : undefined" @toggle="motion.disclose">
          <summary v-if="!finished" class="stage-heading"><span class="stage-names">{{ toolNames(phase.tools) }}</span><span v-if="phase.running" class="stage-counts">已完成 {{ completed(phase.tools) }}/{{ phase.tools.length }}，{{ activityCounts(phase.tools) }}</span><span v-else-if="phase.failed" class="stage-counts">{{ phase.failed }} 项未完成</span><Icon name="chevronDown" size="xs" /></summary>
          <AgentTrace :steps="phase.tools" inline />
        </component>
        <template v-for="answer in phase.answers" :key="answer.id">
          <section class="stage-answer" aria-label="用户输入"><header><Icon name="chat" size="sm" /><span>用户输入</span><span class="answer-confirmed"><Icon name="check" size="xs" />已确认</span></header><dl v-if="answer.answers?.length"><div v-for="value in answer.answers" :key="value.label"><dt>{{ value.label }}</dt><dd>{{ value.value }}</dd></div></dl><p v-else>{{ answer.text }}</p><slot name="answer-assets" :message="answer" /></section>
        </template>
      </section>
      <div v-if="currentTools.length && !finished" class="current-tool" role="status" aria-live="polite"><span class="activity-spinner" aria-hidden="true" /><div><span>{{ activityLabel(currentTools[0]!) }} · {{ toolLabel(currentTools[0]!) }}</span><small>{{ currentTools[0]?.arguments?.prompt || currentTools[0]?.summary || currentTools[0]?.name }}<time v-if="elapsed(currentTools[0]!)"> · {{ elapsed(currentTools[0]!) }}</time></small></div></div>
    </div></Transition>
  </component>
</template>
<script setup lang="ts">
import AgentMarkdown from './AgentMarkdown.vue'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import AgentTrace from './AgentTrace.vue'
import { useAgentMotion } from '@/composables/useAgentMotion'
import { agentToolLabel } from '@/utils/agentPresentation'
import type { AgentMessage, AgentTraceStep } from '@/api/agent'
const props = defineProps<{ steps: AgentTraceStep[]; status?: string; answers?: AgentMessage[] }>()
const motion = useAgentMotion()
const processOpen = ref(false)
function toggleProcess(event: Event) { const element = event.currentTarget as HTMLElement; if (element.tagName !== 'DETAILS') return; const open = (element as HTMLDetailsElement).open; if (processOpen.value === open) return; processOpen.value = open; if (!open) motion.cancel(element) }
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined
const active = (step: AgentTraceStep) => ['queued', 'running', 'reconciling', 'waiting_user'].includes(step.status)
const finished = computed(() => props.status ? ['completed', 'partial_success', 'failed', 'expired', 'cancelled'].includes(props.status) : !props.steps.some(active))
watch(finished, () => { processOpen.value = false })
const currentTools = computed(() => props.steps.filter(step => step.kind === 'tool' && active(step)))
const failedCount = computed(() => props.steps.filter(step => ['failed', 'expired', 'cancelled'].includes(step.status)).length)
interface Phase { id: string; model?: AgentTraceStep; tools: AgentTraceStep[]; answers: AgentMessage[]; running: boolean; failed: number }
const phases = computed(() => {
  const result: Phase[] = []
  for (const step of props.steps) {
    if (step.kind === 'reasoning' || !result.length) result.push({ id: step.id, model: step.kind === 'reasoning' ? step : undefined, tools: [], answers: [], running: false, failed: 0 })
    if (step.kind !== 'tool') continue
    const phase = result[result.length - 1]!
    phase.tools.push(step); phase.running ||= active(step)
    if (['failed', 'expired', 'cancelled'].includes(step.status)) phase.failed++
    if (step.name === 'ask_user' && step.status === 'completed') phase.answers.push(...(props.answers || []).filter(answer => answer.question_id === step.id))
  }
  return result.filter(phase => phase.tools.length || phase.model?.summary?.trim() || (phase.model?.commentary && !finished.value) || (phase.model && active(phase.model)))
})
function toolLabel(step: AgentTraceStep) { return agentToolLabel(step.name) }
function toolNames(steps: AgentTraceStep[]) {
  const counts = new Map<string, number>()
  for (const step of steps) { const name = toolLabel(step); counts.set(name, (counts.get(name) || 0) + 1) }
  return [...counts].map(([name, count]) => count > 1 ? `${name} × ${count}` : name).join('，')
}
function completed(steps: AgentTraceStep[]) { return steps.filter(step => step.status === 'completed').length }
function activityCounts(steps: AgentTraceStep[]) { const names: Record<string, string> = { queued: '等待', running: '执行中', reconciling: '核对中', waiting_user: '待回答' }; const counts = new Map<string, number>(); for (const step of steps.filter(active)) counts.set(step.status, (counts.get(step.status) || 0) + 1); return [...counts].map(([status, count]) => `${names[status] || '处理中'} ${count} 项`).join('，') }
function activityLabel(step: AgentTraceStep) { return ({ queued: '等待执行', reconciling: '正在核对', waiting_user: '等待用户回答', running: '正在调用' } as Record<string, string>)[step.status] || '正在处理' }
function formatDuration(ms: number) { const seconds = Math.max(0, Math.floor(ms / 1000)); return seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒` }
function elapsed(step: AgentTraceStep) { const start = Date.parse(step.started_at), end = step.ended_at ? Date.parse(step.ended_at) : active(step) ? now.value : NaN; return Number.isFinite(start) && Number.isFinite(end) && step.status !== 'queued' ? formatDuration(end - start) : '' }
const duration = computed(() => {
  // Human decision time is not model/tool execution time.
  const intervals = props.steps.filter(step => step.name !== 'ask_user').map(step => [Date.parse(step.started_at), step.ended_at ? Date.parse(step.ended_at) : !finished.value && active(step) ? now.value : NaN]).filter(([a, b]) => Number.isFinite(a) && Number.isFinite(b) && b! >= a!).sort((a, b) => a[0]! - b[0]!)
  if (!intervals.length) return ''
  let sum = 0, start = intervals[0]![0]!, end = intervals[0]![1]!
  for (const interval of intervals.slice(1)) { if (interval[0]! <= end) end = Math.max(end, interval[1]!); else { sum += end - start; start = interval[0]!; end = interval[1]! } }
  return formatDuration(sum + end - start)
})
watch(() => !finished.value && props.steps.some(active), running => { clearInterval(clock); now.value = Date.now(); if (running) clock = setInterval(() => { now.value = Date.now() }, 1000) }, { immediate: true })
onBeforeUnmount(() => clearInterval(clock))
</script>
<style scoped>
/* finesse · component: AgentExecution · register=product · tokens: inherited */
.execution-duration-only { color:var(--workspace-muted); font-size:11px; margin:5px 0; }.agent-execution { margin:5px 0 16px; color:var(--workspace-ink); min-width:0; }
.execution-heading,.stage-heading,.thought-disclosure > summary { display:flex; align-items:center; gap:8px; list-style:none; cursor:pointer; color:var(--workspace-muted); font-size:12px; line-height:1.6; border-radius:6px; padding:5px 0; }
summary::-webkit-details-marker { display:none; } summary:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:3px; } summary > :deep(svg:last-child) { flex-shrink:0; transition:transform 160ms ease-out; } details[open] > summary > :deep(svg:last-child) { transform:rotate(180deg); }
.execution-heading { width:fit-content; }.execution-failure { font-size:10px; color:var(--workspace-muted); }.execution-finished[open] > .execution-timeline { margin-top:12px; padding-left:13px; border-left:1px solid var(--workspace-divider); }
.execution-phase + .execution-phase { margin-top:16px; }.stage-commentary { font-size:14px; line-height:1.85; overflow-wrap:anywhere; margin:8px 0 10px; }.stage-commentary :deep(p) { margin-bottom:8px; }.stage-commentary :deep(ul),.stage-commentary :deep(ol) { padding-left:20px; }.stage-commentary :deep(ul),.reasoning-summary :deep(ul) { list-style:disc; }.stage-commentary :deep(ol),.reasoning-summary :deep(ol) { list-style:decimal; }
.stage-heading { gap:7px; flex-wrap:wrap; }.stage-names { min-width:0; overflow-wrap:anywhere; }.stage-counts { font-size:11px; }.tool-stage > :deep(.agent-process) { margin-top:5px; }.thought-disclosure > summary { width:fit-content; font-size:11px; }.thought-disclosure time { font-variant-numeric:tabular-nums; }.reasoning-summary { font-size:13px; line-height:1.85; overflow-wrap:anywhere; color:var(--workspace-muted); margin:8px 0; }.reasoning-summary :deep(p) { margin:8px 0; }.reasoning-summary :deep(ul),.reasoning-summary :deep(ol) { padding-left:20px; }.thought-source { color:var(--workspace-muted); font-size:10px; }
.stage-answer { margin:14px 0; padding:14px 16px; border:1px solid var(--workspace-divider); border-radius:12px; background:var(--workspace-soft); }.stage-answer header { display:flex; align-items:center; gap:8px; margin-bottom:10px; font-size:12px; }.answer-confirmed { display:inline-flex; align-items:center; gap:4px; margin-left:auto; color:var(--workspace-muted); font-size:10px; }.stage-answer dl { display:grid; gap:10px; border-top:1px solid var(--workspace-divider); padding-top:12px; }.stage-answer dt { font-size:11px; color:var(--workspace-muted); margin-bottom:3px; }.stage-answer dd,.stage-answer p { font-size:13px; line-height:1.7; white-space:pre-wrap; overflow-wrap:anywhere; }
.current-tool { display:flex; align-items:center; gap:10px; margin:14px 0 5px; color:var(--workspace-muted); font-size:12px; }.current-tool > div { min-width:0; }.current-tool small { display:block; max-width:100%; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:11px; margin-top:3px; }.current-tool time { font-variant-numeric:tabular-nums; }.activity-spinner { width:14px; height:14px; flex:0 0 14px; border:1.5px solid var(--workspace-divider); border-top-color:var(--workspace-accent); border-radius:50%; animation:tool-spin 1s linear infinite; }.thinking-pending { display:flex; align-items:center; gap:8px; color:var(--workspace-muted); font-size:12px; margin:8px 0; }.thinking-pending time { font-size:11px; }.activity-dot { width:6px; height:6px; border-radius:50%; background:var(--workspace-accent); animation:thought-pulse 1.8s ease-in-out infinite; }
@media(hover:hover) { .execution-heading:hover,.stage-heading:hover,.thought-disclosure > summary:hover { color:var(--workspace-ink); } }
@keyframes tool-spin { to { transform:rotate(360deg); } }@keyframes thought-pulse { 50% { opacity:.35; } }
@media(prefers-reduced-motion:reduce) { .activity-spinner,.activity-dot { animation:none; }summary > :deep(svg:last-child) { transition:none; } }
@media(max-width:640px) { .stage-answer { padding:12px; }.stage-commentary { font-size:13px; }.execution-finished[open] > .execution-timeline { padding-left:8px; }.stage-counts { font-size:10px; } }
</style>
