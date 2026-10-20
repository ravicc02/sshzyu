<template>
  <form ref="questionRoot" class="agent-question" :aria-busy="!!sending" aria-label="回答 Agent 的问题" @submit.prevent="submit">
    <div class="question-heading"><strong><Icon name="chatBubble" size="sm" aria-hidden="true" />需要你确认</strong><div class="question-heading-actions"><button type="button" class="question-exit" :disabled="sending" aria-label="退出本次询问" @click="emit('cancel')">退出</button></div></div>
    <p>{{ question.question }}</p>
    <fieldset :disabled="sending"><div v-for="field in question.fields" :key="field.id" class="question-field">
      <label :for="`${question.id}-${field.id}`">{{ field.label }}<span v-if="field.required" aria-hidden="true"> *</span></label>
      <div v-if="field.options?.length" class="question-options"><button v-for="option in field.options" :key="option" type="button" :aria-pressed="selectedOptions[field.id] === option" @click="selectOption(field.id, option)"><span class="option-indicator" aria-hidden="true"><Icon v-if="selectedOptions[field.id] === option" name="check" size="xs" /></span>{{ option }}</button></div>
      <input v-if="!field.options?.length || customOption(field.id)" :id="`${question.id}-${field.id}`" v-model="answers[field.id]" :required="field.required" maxlength="2000" :placeholder="field.options?.length ? '请填写你的具体内容' : '填写你的想法'" />
      <p v-if="field.options?.length && customOption(field.id)" class="field-hint">已选择“其他”，请补充具体内容。</p>
    </div></fieldset>
    <p v-if="error" role="alert" class="question-error">{{ error }}</p>
    <footer><span>{{ valid ? '确认后继续处理本次请求' : '请先完成带 * 的必填项' }} · {{ deadline }} 前可继续</span><button type="submit" class="question-submit" :disabled="sending || !valid">{{ sending ? '正在提交…' : '确认并继续' }}</button></footer>
  </form>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAgentMotion } from '@/composables/useAgentMotion'
import type { AgentQuestion } from '@/api/agent'
const props = defineProps<{ question: AgentQuestion; sending?: boolean; error?: string }>()
const emit = defineEmits<{ answer: [answers: Record<string, string>]; cancel: [] }>()
const motion = useAgentMotion(), questionRoot = ref<HTMLElement>()
onMounted(() => motion.reveal(questionRoot.value))
const answers = reactive<Record<string, string>>({})
const selectedOptions = reactive<Record<string, string>>({})
watch(() => props.question.id, () => { Object.keys(answers).forEach(key => delete answers[key]); Object.keys(selectedOptions).forEach(key => delete selectedOptions[key]) }, { immediate: true })
const valid = computed(() => props.question.fields.every(field => !field.required || answers[field.id]?.trim()))
const deadline = computed(() => new Date(props.question.deadline).toLocaleTimeString('zh-CN', { hour12: false }))
function customOption(fieldId: string) { const option = selectedOptions[fieldId] || ''; return option.includes('其他') || option.trim().toLowerCase() === 'other' }
function selectOption(fieldId: string, option: string) { selectedOptions[fieldId] = option; if (!customOption(fieldId)) answers[fieldId] = option; else answers[fieldId] = '' }
function submit() { if (valid.value && !props.sending) emit('answer', Object.fromEntries(props.question.fields.map(f => [f.id, answers[f.id]?.trim() || '']))) }
</script>
<style scoped>
/* finesse · component: AgentQuestion · register=product · tokens: inherited */
.agent-question { margin: 20px 0; padding: 20px; border: 1px solid var(--workspace-divider); border-radius: 12px; background: var(--workspace-soft); color: var(--workspace-ink); }
.question-heading, footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.question-heading-actions { display: inline-flex; align-items: center; gap: 12px; }
.question-exit { padding: 4px 8px; border: 1px solid var(--workspace-divider); border-radius: 6px; color: var(--workspace-muted); background: transparent; font-size: 12px; }
@media (hover: hover) {
.question-exit:hover { color: var(--workspace-ink); background: var(--workspace-surface); } }
.question-heading code, footer span { color: var(--workspace-muted); font-size: 11px; }
.agent-question > p { white-space: pre-wrap; margin: 12px 0 18px; font-size: 14px; line-height: 1.7; }
.question-field { margin: 16px 0; }.question-field label { display: block; font-size: 13px; margin-bottom: 8px; }
.question-options { display: flex; gap: 8px; flex-wrap: wrap; margin: 8px 0; }.question-options button { border: 1px solid var(--workspace-divider); padding: 6px 12px; border-radius: 6px; font-size: 12px; }
.field-hint { margin: 7px 0 0; color: var(--workspace-muted); font-size: 11px; }
.question-options button[aria-pressed=true] { border-color: var(--workspace-accent); color: var(--workspace-accent); background: var(--workspace-surface); }
input { width: 100%; border: 1px solid var(--workspace-divider); background: var(--workspace-surface); border-radius: 6px; padding: 10px; font-size: 13px; color: var(--workspace-ink); }
footer { margin-top:18px; flex-wrap:wrap; }
footer > span { flex:1 1 230px; min-width:0; line-height:1.7; overflow-wrap:anywhere; }
.question-submit { display:inline-flex; align-items:center; justify-content:center; flex-shrink:0; min-width:108px; min-height:38px; margin-left:auto; padding:9px 14px; border:1px solid var(--workspace-ink); border-radius:7px; background:var(--workspace-ink); color:var(--workspace-surface); font-size:13px; font-weight:500; line-height:1.5; white-space:nowrap; }
@media (hover: hover) {
.question-submit:hover:not(:disabled) { box-shadow:0 1px 3px #0002; } }
button:disabled { opacity:.5; cursor:not-allowed; }
.question-submit:disabled { opacity:1; background:var(--workspace-soft); color:var(--workspace-ink); border-color:var(--workspace-divider); }
.question-error { color: #dc2626; } button:focus-visible, input:focus-visible { outline: 2px solid var(--workspace-accent); outline-offset: 3px; }

.agent-question { background:var(--workspace-surface); border-radius:14px; box-shadow:0 4px 20px color-mix(in srgb,var(--workspace-ink) 4%,transparent); }
.question-heading strong { display:inline-flex; align-items:center; gap:9px; font-size:14px; font-weight:600; }
.question-heading strong svg { color:var(--workspace-accent); }
.question-field { margin:20px 0; }
.question-field > label { color:var(--workspace-ink); font-weight:500; }
.question-options { gap:8px; }
.question-options button { display:inline-flex; align-items:center; gap:7px; min-height:36px; padding:7px 11px; background:var(--workspace-surface); color:var(--workspace-ink); border-radius:8px; white-space:nowrap; }
.option-indicator { display:inline-flex; align-items:center; justify-content:center; flex:0 0 14px; width:14px; height:14px; border:1px solid var(--workspace-divider); border-radius:50%; color:var(--workspace-surface); }
.question-options button[aria-pressed=true] { color:var(--workspace-ink); border-color:var(--workspace-accent); background:var(--workspace-selected); }
.question-options button[aria-pressed=true] .option-indicator { background:var(--workspace-accent); border-color:var(--workspace-accent); }
.question-options button.is-hover { border-color:var(--workspace-accent); }
@media (hover: hover) {
.question-options button:hover:not(:disabled) { border-color:var(--workspace-accent); } }
.question-options button:active:not(:disabled), .question-submit:active:not(:disabled), .question-submit.is-active { transform:translateY(1px); }
input { min-height:40px; border-radius:8px; }
input:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:2px; }
.question-submit { min-width:110px; border-radius:8px; }
.question-submit.is-hover { box-shadow:0 3px 10px color-mix(in srgb,var(--workspace-ink) 12%,transparent); }
@media (hover: hover) {
.question-submit:hover:not(:disabled) { box-shadow:0 3px 10px color-mix(in srgb,var(--workspace-ink) 12%,transparent); } }
.question-submit.is-focus { outline:2px solid var(--workspace-accent); outline-offset:3px; }
.question-submit:disabled { opacity:1; }
.question-error { padding:10px 12px; border:1px solid var(--workspace-divider); border-radius:8px; background:var(--workspace-soft); }
@media(max-width:480px) { .agent-question { padding:16px; }.question-options button { max-width:100%; white-space:normal; text-align:left; }.question-heading { gap:8px; } }

</style>
