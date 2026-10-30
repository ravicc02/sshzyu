<template>
  <button
    type="button"
    class="group flex min-h-[295px] w-full min-w-0 flex-col rounded-lg border border-gray-200 bg-white p-4 text-left shadow-sm transition-colors hover:border-emerald-400 dark:border-dark-600 dark:bg-[#172a31] dark:hover:border-emerald-600"
    @click="emit('click')"
  >
    <div class="flex min-w-0 items-start gap-2.5">
      <span class="grid h-9 w-9 shrink-0 place-items-center rounded-md bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300">
        <ProviderIcon :provider="item.provider" :size="21" />
      </span>
      <div class="min-w-0 flex-1">
        <div class="break-words text-sm font-bold leading-5 text-gray-900 dark:text-white">{{ item.name }}</div>
        <div class="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-gray-500 dark:text-gray-400">
          <span class="font-medium text-emerald-700 dark:text-emerald-300">{{ providerLabel(item.provider) }}</span>
          <span v-if="item.group_name" class="min-w-0 break-words">{{ item.group_name }}</span>
          <span class="min-w-0 break-all">{{ formatMonitorModel(item.primary_model) }}</span>
        </div>
      </div>
      <span class="shrink-0 rounded px-1.5 py-0.5 text-[11px] font-semibold" :class="statusBadgeClass(healthStale ? '' : item.primary_status)">
        {{ healthStale ? t('channelStatus.intelligenceStale') : statusLabel(item.primary_status) }}
      </span>
    </div>

    <div class="mt-5 grid grid-cols-3 divide-x divide-gray-200 border-y border-gray-100 py-3 dark:divide-dark-600 dark:border-dark-600">
      <div class="min-w-0 pr-2">
        <div class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('channelStatus.probeRate') }}</div>
        <div class="mt-1 text-base font-bold tabular-nums text-gray-800 dark:text-gray-100">{{ probeRate }}</div>
      </div>
      <div class="min-w-0 px-2">
        <div class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('channelStatus.availability') }} · {{ windowLabel }}</div>
        <div class="mt-1 text-base font-bold tabular-nums text-emerald-700 dark:text-emerald-300">{{ availability }}</div>
      </div>
      <div class="min-w-0 pl-2">
        <div class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('channelStatus.responseLatency') }}</div>
        <div class="mt-1 text-base font-bold tabular-nums text-gray-800 dark:text-gray-100">{{ healthStale ? '—' : formatLatency(item.primary_latency_ms) }}<small v-if="!healthStale && item.primary_latency_ms != null" class="ml-0.5 text-[10px] font-normal">ms</small></div>
      </div>
    </div>

    <MonitorQuotaView v-if="quotaVisible" :snapshot="item.latest_quota" class="mt-2" />

    <div class="mt-4">
      <MonitorStatusStrip :label="t('channelStatus.healthChecks')" :points="item.timeline || []" :countdown-seconds="countdownSeconds" />
    </div>
    <div v-if="item.provider === 'openai'" class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-600">
      <div class="mb-2 flex items-center justify-between gap-2 text-xs">
        <strong class="font-semibold text-gray-700 dark:text-gray-100">{{ t('channelStatus.intelligenceChecks') }}</strong>
        <span class="text-right" :class="intelligenceStatusClass">{{ intelligenceLabel }}</span>
      </div>
      <MonitorStatusStrip v-if="item.intelligence_enabled" :label="formatMonitorModel(item.primary_model)" :points="item.intelligence_timeline || []" />
    </div>
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView, MonitorTimelinePoint } from '@/api/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import { isChannelMonitorQuotaVisible } from '@/utils/featureFlags'
import MonitorQuotaView from '@/components/common/MonitorQuotaView.vue'
import ProviderIcon from './ProviderIcon.vue'
import MonitorStatusStrip from './MonitorStatusStrip.vue'

const props = defineProps<{
  item: UserMonitorView
  window: '7d' | '15d' | '30d'
  availabilityValue: number | null
  countdownSeconds: number
}>()
const emit = defineEmits<{ (e: 'click'): void }>()
const { t } = useI18n()
const { providerLabel, formatLatency, formatMonitorModel, statusBadgeClass, statusLabel } = useChannelMonitorFormat()
const quotaVisible = computed(() => isChannelMonitorQuotaVisible() && !!props.item.latest_quota)
const windowLabel = computed(() => t(`channelStatus.windowTab.${props.window}`))
const availability = computed(() => props.item.timeline?.length && props.availabilityValue != null ? `${props.availabilityValue.toFixed(1)}%` : '—')
const probeRate = computed(() => {
  const checks = (props.item.timeline || []).slice(0, 20)
  return checks.length ? `${((checks.filter(check => check.status === 'operational' || check.status === 'degraded').length / checks.length) * 100).toFixed(0)}%` : '—'
})
function stale(point?: MonitorTimelinePoint): boolean {
  if (!point) return true
  const time = Date.parse(point.checked_at)
  return !Number.isFinite(time) || Date.now() - time > Math.max(150, (props.item.interval_seconds || 60) * 2.5) * 1000
}
const healthStale = computed(() => stale(props.item.timeline?.[0]))
const intelligenceStale = computed(() => stale(props.item.intelligence_timeline?.[0]))
const intelligenceLabel = computed(() => {
  if (!props.item.intelligence_enabled) return t('channelStatus.intelligenceUnavailable')
  if (!props.item.intelligence_timeline?.length) return t('channelStatus.intelligencePending')
  if (intelligenceStale.value) return t('channelStatus.intelligenceStale')
  if (props.item.intelligence_timeline[0]?.status === 'error') return t('channelStatus.intelligenceError')
  return props.item.intelligence_timeline[0]?.status === 'operational'
    ? t('channelStatus.intelligencePassed') : t('channelStatus.intelligenceFailed')
})
const intelligenceStatusClass = computed(() =>
  !props.item.intelligence_enabled || intelligenceStale.value ? 'text-gray-500 dark:text-gray-400'
    : props.item.intelligence_timeline?.[0]?.status === 'operational' ? 'text-emerald-600 dark:text-emerald-300' : 'text-amber-600 dark:text-amber-300'
)
</script>
