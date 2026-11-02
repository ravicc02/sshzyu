<template>
  <div class="quota-row">
    <div class="quota-label">
      <span>{{ label }}</span>
      <strong v-if="window">{{ remainingLabel }}</strong>
      <strong v-else>暂无数据</strong>
    </div>
    <div class="quota-track" :class="levelClass">
      <span :style="{ width: progressWidth }" />
    </div>
    <div class="quota-foot">
      <span>{{ window ? `${formatPercent(window.used_percent)} 已用` : '—' }}</span>
      <span>{{ resetLabel }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { HostedQuotaWindow } from '@/api/hostedOpenAIAccounts'

const props = defineProps<{
  label: string
  window?: HostedQuotaWindow
}>()

const used = computed(() => Math.max(0, Math.min(100, props.window?.used_percent ?? 0)))
const progressWidth = computed(() => `${used.value}%`)
const remainingLabel = computed(() => `${formatPercent(Math.max(0, 100 - used.value))} 可用`)
const levelClass = computed(() => {
  if (used.value >= 90) return 'is-critical'
  if (used.value >= 70) return 'is-warning'
  return 'is-healthy'
})
const resetLabel = computed(() => {
  if (!props.window?.reset_at) return '—'
  const date = new Date(props.window.reset_at)
  if (Number.isNaN(date.getTime())) return '—'
  return `${date.toLocaleDateString('zh-CN', { month: 'numeric', day: 'numeric' })} ${date.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })} 重置`
})

function formatPercent(value: number) {
  return `${Number(value.toFixed(value >= 10 ? 0 : 1))}%`
}
</script>

<style scoped>
.quota-row { display: grid; gap: 7px; }
.quota-label, .quota-foot { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.quota-label span { font-size: 11px; font-weight: 600; }
.quota-label strong { font-size: 10px; color: var(--text-secondary); }
.quota-track { height: 6px; overflow: hidden; border-radius: 999px; background: var(--border-color); }
.quota-track span { display: block; height: 100%; border-radius: inherit; transition: width 0.25s ease; }
.quota-track.is-healthy span { background: #10b981; }
.quota-track.is-warning span { background: #d59b22; }
.quota-track.is-critical span { background: #e05252; }
.quota-foot { color: var(--text-tertiary); font-size: 9px; }
</style>
