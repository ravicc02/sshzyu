<template>
  <TransitionGroup tag="div" class="agent-chat" aria-label="对话消息" :css="false" @enter="enterMessage" @enter-cancelled="motion.cancel">
    <template v-for="message in conversationItems" :key="message.role === 'assistant' && message.run_id ? `reply-${message.run_id}` : message.id">
      <details v-if="message.kind === 'question_answer'" class="answer-confirmation" @toggle="motion.disclose">
        <summary><Icon name="check" size="xs" /><span>已确认{{ message.answers?.length ? ` ${message.answers.length} 项` : '回答' }}</span><Icon name="chevronDown" size="xs" class="answer-chevron" /></summary>
        <dl v-if="message.answers?.length"><div v-for="answer in message.answers" :key="answer.label"><dt>{{ answer.label }}</dt><dd>{{ answer.value }}</dd></div></dl>
        <p v-else class="legacy-answer">{{ message.text }}</p>
        <slot name="assets" :message="message" />
      </details>
      <article v-else class="agent-message" :class="message.role === 'user' ? 'from-user' : 'from-assistant'">
        <div class="message-role"><span class="message-author">{{ message.role === 'user' ? '你' : 'Agent' }}</span><template v-if="message.role !== 'user' && modelName(message)"><span class="message-role-separator" aria-hidden="true">·</span><span class="message-model" :title="modelName(message)">{{ modelName(message) }}</span></template></div>
        <p v-if="message.pending" class="message-pending" role="status">正在重新生成…</p>
        <AgentExecution v-if="message.role !== 'user' && message.trace?.length" :steps="message.trace" :status="message.run_status" :answers="answersFor(message)"><template #answer-assets="{ message: answer }"><slot name="assets" :message="answer" /></template></AgentExecution>
        <p v-if="message.role === 'user'" class="user-content">{{ message.text }}</p>
        <div v-else-if="messageText(message)" class="message-markdown" v-html="renderAgentMarkdown(messageText(message))" />
        <slot name="assets" :message="message" />
        <slot name="work" :message="message" />
        <footer v-if="!message.pending && message.role !== 'user' && (message.text || message.run_id)" class="reply-toolbar">
          <div class="reply-actions">
            <button type="button" class="copy-message reply-action" :class="{ 'copy-error': copyError === message.id }" :disabled="!messageText(message)" :aria-label="copyLabel(message.id)" :title="copyLabel(message.id)" @click="copy(message)"><Icon :name="copied === message.id ? 'check' : 'copy'" size="sm" /></button>
            <button v-if="message.run_id && (!hideLatestRetry || message.run_id !== retryRunId)" type="button" class="retry-message reply-action" :disabled="retryDisabled || message.run_id !== retryRunId" aria-label="重试回复" :title="message.run_id !== retryRunId ? '仅可重试最新一轮回复' : '重试回复'" @click="emit('retry', message)"><Icon name="refresh" size="sm" /></button>
          <button v-if="message.can_recover_image && message.run_id" type="button" class="recover-message reply-action" :disabled="recoverDisabled" title="从已保存的上游响应恢复，不重新生成" aria-label="恢复已有图片，不重新生成" @click="emit('recover', message)"><Icon name="photograph" size="sm" /><span>恢复已有图片</span></button>
          </div>
          <div v-if="replyData(message).length" class="reply-metrics" :title="metricsTitle(message)">
            <span v-for="(value, index) in replyData(message)" :key="index">{{ value }}</span>
          </div>
          <time v-if="replyTime(message)" class="reply-time" :datetime="message.created_at">{{ replyTime(message) }}</time>
          <span v-if="copied === message.id || copyError === message.id" class="sr-only" role="status">{{ copyLabel(message.id) }}</span>
        </footer>
        <p v-if="retryError && message.run_id === retryRunId" class="reply-error" role="alert">{{ retryError }}</p>
      </article>
    </template>
    <slot />
  </TransitionGroup>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAgentMotion } from '@/composables/useAgentMotion'
import AgentExecution from './AgentExecution.vue'
import type { AgentMessage } from '@/api/agent'
import { renderAgentMarkdown } from '@/utils/agentPresentation'
const props = defineProps<{ messages: AgentMessage[]; retryRunId?: string; retryDisabled?: boolean; recoverDisabled?: boolean; retryError?: string; hideLatestRetry?: boolean }>()
const emit = defineEmits<{ retry: [message: AgentMessage]; recover: [message: AgentMessage] }>()
const emptyAnswers: AgentMessage[] = []
const answerGroups = computed(() => {
  const groups = new Map<string, AgentMessage[]>()
  for (const message of props.messages) {
    if (message.kind !== 'question_answer' || !message.run_id) continue
    const group = groups.get(message.run_id) || []; group.push(message); groups.set(message.run_id, group)
  }
  return groups
})
const inlineAnswers = computed(() => {
  const ids = new Set<string>()
  for (const message of props.messages) {
    if (message.role !== 'assistant' || !message.run_id) continue
    for (const step of message.trace || []) if (step.name === 'ask_user' && step.status === 'completed') ids.add(`${message.run_id}:${step.id}`)
  }
  return ids
})
const conversationItems = computed(() => props.messages.filter(message => message.kind !== 'question_answer' || !message.question_id || !inlineAnswers.value.has(`${message.run_id}:${message.question_id}`)))
function answersFor(message: AgentMessage) { return answerGroups.value.get(message.run_id || '') || emptyAnswers }

function messageText(message: AgentMessage) {
  const failedImage = message.role === 'assistant' && !!message.error_code && !message.asset_ids?.length && message.trace?.some(step => step.kind === 'tool' && step.name === 'generate_image')
  const streamingStage = message.run_status && !['completed', 'partial_success', 'failed', 'expired', 'cancelled'].includes(message.run_status) && message.trace?.some(step => step.commentary === message.text && !!step.commentary)
  return failedImage || streamingStage ? '' : message.text
}
const motion = useAgentMotion()
function enterMessage(element: Element, done: () => void) {
  if (element.classList.contains('from-user') || element.querySelector('.message-pending')) { done(); return }
  motion.enter(element, done)
}
const copied = ref('')
const copyError = ref('')
let copyTimer: ReturnType<typeof setTimeout> | undefined
function elapsed(message: AgentMessage): number | undefined {
  if (message.metrics?.elapsed_ms != null) return message.metrics.elapsed_ms
  const steps = (message.trace || []).filter(step => step.ended_at && step.name !== 'ask_user')
  if (!steps.length) return undefined
  const durations = steps.map(step => Date.parse(step.ended_at!) - Date.parse(step.started_at))
  return durations.every(value => Number.isFinite(value) && value >= 0) ? durations.reduce((sum, value) => sum + value, 0) : undefined
}
function compactTokens(value: number) { return value >= 1_000_000 ? `${(value / 1_000_000).toFixed(1)}M` : value >= 1000 ? `${(value / 1000).toFixed(1)}k` : String(value) }
function replyData(message: AgentMessage): string[] {
  const values: string[] = [], duration = elapsed(message), metrics = message.metrics
  if (duration != null) { const seconds = duration / 1000; values.push(`${seconds < 10 ? seconds.toFixed(1) : Math.round(seconds)}s`) }
  if (metrics && metrics.total_tokens > 0) values.push(`${compactTokens(metrics.total_tokens)} tokens`)
  if (metrics && metrics.generation_ms != null && metrics.generation_ms > 0 && metrics.output_tokens > 0) values.push(`${(metrics.output_tokens * 1000 / metrics.generation_ms).toFixed(1)} tok/s`)
  return values
}
function metricsTitle(message: AgentMessage) {
  const m = message.metrics
  return m ? `输入 ${m.input_tokens} tokens · 输出 ${m.output_tokens} tokens · 缓存 ${m.cache_tokens} tokens；耗时为调用累计时间，速度为输出 tokens / 生成时间（扣除首字等待）` : '调用耗时（不含等待用户回答）'
}
function replyTime(message: AgentMessage) { const value = message.created_at && new Date(message.created_at); return value && Number.isFinite(value.getTime()) ? value.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }) : '' }
function modelName(message: AgentMessage) { return message.trace?.find(step => step.kind === 'reasoning')?.name }
function copyLabel(id: string) { return copied.value === id ? '已复制' : copyError.value === id ? '复制失败，点击重试' : '复制回复' }
async function copy(message: AgentMessage) {
  try { await navigator.clipboard.writeText(messageText(message)); copied.value = message.id; copyError.value = ''; clearTimeout(copyTimer); copyTimer = setTimeout(() => { copied.value = '' }, 2000) }
  catch { copyError.value = message.id }
}
onBeforeUnmount(() => clearTimeout(copyTimer))
</script>
<style scoped>
/* finesse · component: AgentChat · register=product · tokens: inherited */
.agent-chat { width: 100%; }
.agent-message { margin: 0 0 28px; min-width: 0; }
.message-role { display:flex; align-items:center; gap:8px; min-width:0; margin-bottom: 8px; font-size: 12px; font-weight: 600; color: var(--workspace-muted); }
 .message-author, .message-role-separator { flex-shrink:0; }
.message-model { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:11px; font-weight:400; }
.message-pending { color:var(--workspace-muted); font-size:13px; line-height:1.8; }
.from-user { margin-left: auto; max-width: 88%; width: fit-content; }
.from-user .message-role { justify-content:flex-end; text-align:right; }
.user-content { white-space: pre-wrap; overflow-wrap: anywhere; padding: 12px 18px; background: var(--workspace-soft); border-radius: 16px 16px 4px 16px; font-size: 14px; line-height: 1.8; }
.message-markdown { overflow-wrap: anywhere; font-size: 14px; line-height: 1.85; color: var(--workspace-ink); }
.message-markdown :deep(p) { margin: 0 0 12px; }
.message-markdown :deep(h1), .message-markdown :deep(h2), .message-markdown :deep(h3), .message-markdown :deep(h4) { font-size: 17px; font-weight: 600; margin: 22px 0 10px; }
.message-markdown :deep(ul), .message-markdown :deep(ol) { padding-left: 24px; margin: 10px 0 16px; }
.message-markdown :deep(ul) { list-style: disc; }
.message-markdown :deep(ol) { list-style: decimal; }
.message-markdown :deep(li) { margin: 5px 0; }
.message-markdown :deep(pre) { max-width: 100%; overflow-x: auto; padding: 16px; background: var(--workspace-soft); border: 1px solid var(--workspace-divider); border-radius: 8px; margin: 14px 0; white-space: pre; }
.message-markdown :deep(code) { font-size: 12px; background: var(--workspace-soft); padding: 2px 4px; border-radius: 3px; }
.message-markdown :deep(pre code) { padding: 0; }
.message-markdown :deep(a) { color: var(--workspace-accent); text-decoration: underline; text-underline-offset: 3px; }
.message-markdown :deep(blockquote) { padding: 10px 16px; background: var(--workspace-soft); border-radius: 6px; margin: 12px 0; }
.message-markdown :deep(table) { display: block; max-width: 100%; overflow-x: auto; border-collapse: collapse; margin: 16px 0; }
.message-markdown :deep(th), .message-markdown :deep(td) { padding: 8px 12px; border: 1px solid var(--workspace-divider); white-space: normal; }
.answer-confirmation { width:fit-content; max-width:88%; margin:12px 0 24px auto; color:var(--workspace-muted); font-size:12px; }
.answer-confirmation > summary { display:flex; align-items:center; gap:7px; width:fit-content; max-width:100%; min-height:34px; margin-left:auto; padding:6px 10px; border:1px solid var(--workspace-divider); border-radius:8px; background:var(--workspace-soft); font-size:12px; font-weight:500; line-height:1.5; cursor:pointer; list-style:none; }
.answer-confirmation > summary::-webkit-details-marker { display:none; }
.answer-confirmation > summary::marker { content:''; }
.answer-confirmation > summary:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:3px; }
.answer-confirmation > summary svg { flex-shrink:0; }
.answer-confirmation > summary > span { white-space:nowrap; }
.answer-confirmation[open] .answer-chevron { transform:rotate(180deg); }
.answer-confirmation dl, .legacy-answer { margin-top:10px; padding:10px 14px; border:1px solid var(--workspace-divider); border-radius:8px; background:var(--workspace-surface); text-align:left; }
.answer-confirmation dl > div { display:grid; grid-template-columns:minmax(70px,120px) minmax(0,1fr); gap:12px; padding:5px 0; line-height:1.7; }
.answer-confirmation dt { color:var(--workspace-muted); overflow-wrap:anywhere; }
.answer-confirmation dd, .legacy-answer { color:var(--workspace-ink); white-space:pre-wrap; overflow-wrap:anywhere; }
.legacy-answer { line-height:1.7; }
.reply-toolbar { display:flex; align-items:center; flex-wrap:wrap; gap:14px; margin-top:14px; color:var(--workspace-muted); font-size:11px; line-height:1.5; font-variant-numeric:tabular-nums; }
.reply-actions { display:flex; align-items:center; gap:5px; }
.reply-action { display:inline-flex; align-items:center; justify-content:center; width:28px; height:28px; padding:5px; border-radius:5px; color:var(--workspace-muted); }
@media (hover: hover) {
.reply-action:hover:not(:disabled) { color:var(--workspace-ink); background:var(--workspace-soft); } }
.reply-action:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:2px; }
.reply-action:disabled { opacity:.35; cursor:not-allowed; }
.reply-metrics { display:flex; align-items:center; flex-wrap:wrap; gap:0; }
.reply-metrics span + span::before { content:'·'; display:inline-block; margin:0 7px; }
.reply-time { white-space:nowrap; }
.reply-error { margin-top:6px; color:var(--workspace-ink); font-size:12px; }
.copy-error { color:var(--workspace-ink); }
@media (max-width: 640px) { .from-user { max-width:95%; } .answer-confirmation { max-width:100%; } .answer-confirmation dl > div { grid-template-columns:1fr; gap:2px; } }

.message-role { gap:8px; }
.message-role-separator { opacity:.6; }
.message-model { font-variant-numeric:tabular-nums; }
.reply-toolbar { gap:12px; }
.reply-action { border-radius:7px; }
.reply-action:active:not(:disabled) { transform:scale(.94); }
.copy-message[aria-label="已复制"] { color:var(--workspace-accent); background:var(--workspace-selected); }
.answer-chevron { transition:transform 160ms ease-out; }
@media (hover: hover) {
.answer-confirmation > summary:hover { color:var(--workspace-ink); border-color:var(--workspace-accent); } }
@media(prefers-reduced-motion:reduce) { .answer-chevron { transition:none; } }

.recover-message { gap:5px; width:auto; padding-inline:7px; font-size:11px; }
</style>
