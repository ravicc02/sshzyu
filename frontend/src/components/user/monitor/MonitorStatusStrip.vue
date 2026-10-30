<template>
  <div class="min-w-0">
    <div class="mb-2 flex items-center justify-between gap-2 text-xs">
      <strong class="font-semibold text-gray-700 dark:text-gray-100">{{ label }}</strong>
      <span class="shrink-0 text-gray-500 dark:text-gray-400">{{ countdownSeconds == null ? (points.length ? t('channelStatus.recentChecks') : t('channelStatus.noChecks')) : t('channelStatus.refreshIn', { n: countdownSeconds }) }}</span>
    </div>
    <div class="grid h-5 grid-cols-[repeat(20,minmax(0,1fr))] gap-0.5" :aria-label="label">
      <span
        v-for="(point, index) in slots"
        :key="index"
        class="min-w-0 rounded-[2px]"
        :class="point ? color(point.status) : 'bg-gray-200 dark:bg-dark-600'"
        :title="point ? `${new Date(point.checked_at).toLocaleString()} · ${statusLabel(point.status)}` : t('channelStatus.noChecks')"
      />
    </div>
    <div class="mt-1 flex justify-between text-[10px] text-gray-400 dark:text-gray-500">
      <span>{{ t('channelStatus.earlier') }}</span><span>{{ t('channelStatus.now') }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorTimelinePoint } from '@/api/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

const props = defineProps<{ label: string; points: MonitorTimelinePoint[]; countdownSeconds?: number }>()
const { t } = useI18n()
const { statusLabel } = useChannelMonitorFormat()
const slots = computed(() => {
  const recent = props.points.slice(0, 20).reverse()
  return [...Array<null>(20 - recent.length).fill(null), ...recent]
})
function color(status: string) {
  if (status === 'operational') return 'bg-emerald-500'
  if (status === 'degraded') return 'bg-amber-400'
  return 'bg-rose-500'
}
</script>
