<template>
  <section class="space-y-6" :aria-label="t('channelMonitorV2.groupCards.title')">
    <div v-if="loading && !rows.length" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3" aria-hidden="true">
      <div v-for="n in 3" :key="n" class="h-[310px] animate-pulse rounded-[28px] bg-gray-100 dark:bg-dark-800" />
    </div>
    <div v-else-if="!groupRows.length" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('channelMonitorV2.groupCards.empty') }}
    </div>
    <template v-else>
      <div v-if="inactiveCount && groupRows.length > 12" class="flex justify-end">
        <button type="button" class="inline-flex items-center gap-1 text-xs text-gray-600 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white" :aria-expanded="showInactive" @click="showInactive = !showInactive">
          <Icon name="chevronDown" size="sm" :class="showInactive ? 'rotate-180' : ''" />
          {{ t(showInactive ? 'channelMonitorV2.groupCards.hideInactive' : 'channelMonitorV2.groupCards.showInactive', { count: inactiveCount }) }}
        </button>
      </div>
      <section v-for="section in sections" :key="section.platform" class="space-y-3" :aria-label="platformLabel(section.platform)">
        <header class="flex items-center gap-2.5">
          <span class="grid h-9 w-9 shrink-0 place-items-center rounded-2xl shadow-sm" :class="providerTone(section.platform)"><ProviderIcon :provider="section.platform" :size="20" /></span>
          <h2 class="text-base font-bold text-gray-900 dark:text-gray-100">{{ platformLabel(section.platform) }}</h2>
          <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs tabular-nums text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ section.rows.length }}</span>
        </header>
        <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          <article v-for="row in section.rows" :key="`${row.platform}:${row.group_id}`" class="group flex min-w-0 flex-col rounded-[28px] border border-gray-200/70 bg-gradient-to-br from-white via-white to-gray-50/70 p-5 shadow-[0_10px_30px_rgba(15,23,42,0.06)] ring-1 ring-transparent transition duration-200 hover:-translate-y-0.5 hover:border-gray-300 hover:shadow-[0_18px_40px_rgba(15,23,42,0.10)] dark:border-white/10 dark:from-[#172a31] dark:via-[#172a31] dark:to-[#14262d] dark:hover:border-white/20">
            <header class="flex min-h-[62px] min-w-0 items-start gap-3">
              <span class="grid h-12 w-12 shrink-0 place-items-center rounded-[18px] shadow-sm ring-1 ring-black/5 dark:ring-white/10" :class="providerTone(row.platform)"><ProviderIcon :provider="row.platform" :size="24" /></span>
              <div class="min-w-0 flex-1">
                <h3 class="break-words text-[15px] font-bold leading-5 text-gray-900 dark:text-white">{{ row.group_name || t('channelMonitorV2.filters.group') }}</h3>
                <div class="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-gray-500 dark:text-gray-400">
                  <span class="font-medium" :class="providerText(row.platform)">{{ platformLabel(row.platform) }}</span>
                  <span v-if="hasTraffic(row) || hasProbe(row)" :class="hasTraffic(row) ? '' : 'text-sky-700 dark:text-sky-300'">{{ sourceLabel(row) }}</span>
                  <span v-if="row.rate_multiplier != null" class="font-semibold text-teal-700 dark:text-teal-300">{{ t('channelMonitorV2.groupCards.multiplier', { value: formatMultiplier(row.rate_multiplier) }) }}</span>
                </div>
                <div v-if="probeModels(row).length" class="mt-1 flex min-w-0 items-center gap-1.5 text-[10px] text-gray-500 dark:text-gray-400">
                  <span class="shrink-0">{{ t('channelMonitorV2.groupCards.probeModelsLabel') }}</span>
                  <span class="truncate" :title="probeModels(row).join(', ')">{{ probeModels(row).join(', ') }}</span>
                </div>
              </div>
              <span class="shrink-0 rounded-full px-2.5 py-1 text-[11px] font-semibold ring-1 ring-inset ring-black/5 dark:ring-white/10" :class="stateClass(row)">{{ stale ? t('channelMonitorV2.groupCards.stale') : stateLabel(row) }}</span>
            </header>

            <div class="mt-4 grid grid-cols-3 divide-x divide-gray-200/80 rounded-[18px] border border-gray-200/70 bg-white/75 py-3.5 shadow-[inset_0_1px_0_rgba(255,255,255,0.8)] dark:divide-white/10 dark:border-white/10 dark:bg-white/[0.045] dark:shadow-none">
              <div class="min-w-0 px-3"><div class="text-[10px] font-medium text-gray-500 dark:text-gray-400">{{ t('channelMonitorV2.metrics.cacheRate') }}</div><strong class="mt-1.5 block text-sm tabular-nums text-gray-900 dark:text-gray-100" :title="cacheRateTitle(row)">{{ cacheRateDisplay(row) }}</strong></div>
              <div class="min-w-0 px-3"><div class="text-[10px] font-medium text-gray-500 dark:text-gray-400">{{ t('channelMonitorV2.groupCards.availability') }}</div><strong class="mt-1.5 block text-sm tabular-nums" :class="availabilityClass(row)">{{ availabilityValue(row) }}</strong></div>
              <div class="min-w-0 px-3"><div class="text-[10px] font-medium text-gray-500 dark:text-gray-400">{{ latencyLabel(row) }}</div><strong class="mt-1.5 block text-sm tabular-nums text-gray-900 dark:text-gray-100">{{ latencyValue(row) }}</strong></div>
            </div>

            <div v-if="row.platform === 'composite'" class="mt-3 rounded-[16px] border border-gray-200/70 bg-gray-50/65 p-2.5 dark:border-white/10 dark:bg-white/[0.035]">
              <div class="mb-1.5 flex items-center justify-between gap-2 px-1">
                <span class="text-[10px] font-semibold text-gray-600 dark:text-gray-300">{{ t('channelMonitorV2.groupCards.providerBreakdown') }}</span>
                <span class="text-[10px] text-gray-400">{{ t('channelMonitorV2.groupCards.providerCount', { count: compositeProviderCount(row) }) }}</span>
              </div>
              <div v-if="compositeProviderDetails(row).length" class="space-y-1">
                <div class="grid grid-cols-[minmax(0,1fr)_auto_auto_auto] items-center gap-1.5 px-2 text-[9px] text-gray-400">
                  <span>{{ t('channelMonitorV2.groupCards.provider') }}</span>
                  <span>{{ t('channelMonitorV2.groupCards.availability') }}</span>
                  <span>{{ t('channelMonitorV2.groupCards.latency') }}</span>
                  <span>{{ t('channelMonitorV2.groupCards.groupMultiplier') }}</span>
                </div>
                <div v-for="provider in compositeProviderDetails(row)" :key="provider.platform" class="grid grid-cols-[minmax(0,1fr)_auto_auto_auto] items-center gap-1.5 rounded-lg bg-white/75 px-2 py-1.5 dark:bg-dark-800/70" :title="providerModelTitle(provider)">
                  <span class="flex min-w-0 items-center gap-1.5 text-[11px] font-medium text-gray-700 dark:text-gray-200">
                    <ProviderIcon :provider="provider.platform" :size="13" />
                    <span class="truncate">{{ platformLabel(provider.platform) }}</span>
                  </span>
                  <span class="text-[10px] font-semibold" :class="probeStateTextClass(provider.health)">{{ provider.availability }}</span>
                  <span class="text-[10px] tabular-nums text-gray-500 dark:text-gray-400">{{ provider.latency }}</span>
                  <span class="text-[10px] font-semibold tabular-nums text-teal-700 dark:text-teal-300">{{ provider.multiplier }}</span>
                </div>
              </div>
              <p v-else class="px-1 py-1 text-[10px] text-gray-400">{{ t('channelMonitorV2.groupCards.providerNoData') }}</p>
            </div>

            <div class="mt-4 flex items-center justify-between gap-2 text-[11px] text-gray-500 dark:text-gray-400">
              <span>{{ timelineWindowLabel(row) }}</span>
              <time v-if="lastUpdatedAt(row)" :datetime="lastUpdatedAt(row) || undefined">{{ formatTime(lastUpdatedAt(row) || '') }}</time>
            </div>
            <div class="mt-2 grid h-5 grid-cols-[repeat(20,minmax(0,1fr))] gap-1.5" :aria-label="timelineAria(row)">
              <span v-for="(slot, index) in timelineSlots(row)" :key="index" class="min-w-0 rounded-full transition-transform group-hover:scale-y-110" :class="slotColor(slot?.state || null)" :title="slotTitle(slot)" />
            </div>
            <div class="mt-1 flex justify-between text-[10px] text-gray-400 dark:text-gray-500"><span>{{ t('channelStatus.earlier') }}</span><span>{{ t('channelStatus.now') }}</span></div>

            <div v-if="row.platform === 'openai' && (!hybrid || hasIntelligenceMonitor(row))" class="mt-4 border-t border-gray-100 pt-3 text-xs dark:border-dark-600">
              <div class="flex items-center justify-between gap-2"><strong class="text-gray-700 dark:text-gray-100">{{ t('channelStatus.intelligenceChecks') }}</strong><span :class="intelligenceSummaryClass(row)">{{ intelligenceSummary(row) }}</span></div>
              <div v-if="hybrid && intelligenceProbe(row) && intelligenceProbe(row)?.intelligence_source !== 'account_aggregate'" class="mt-3 grid h-2 grid-cols-[repeat(20,minmax(0,1fr))] gap-1" :aria-label="t('channelMonitorV2.groupCards.intelligenceActive')">
                <span v-for="(slot, index) in intelligenceSlots(row)" :key="index" class="rounded-full" :class="slotColor(slot?.state || null)" :title="slotTitle(slot)" />
              </div>
            </div>
          </article>
        </div>
      </section>
      <div class="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-gray-500 dark:text-gray-400">
        <span v-for="state in legend" :key="state.key" class="inline-flex items-center gap-1.5"><i class="h-2 w-2 rounded-full" :class="slotColor(state.value)" />{{ t(`channelMonitorV2.groupCards.legend.${state.key}`) }}</span>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { HealthState, MonitorCoverage, MonitorMatrixRow } from '@/api/channelMonitorV2'
import type { MonitorTimelinePoint, UserMonitorView } from '@/api/channelMonitor'
import { formatMonitorMs, formatMonitorPercent } from './monitorFormat'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  rows: MonitorMatrixRow[]
  coverage: MonitorCoverage | null
  probes?: UserMonitorView[]
  hybrid?: boolean
  loading?: boolean
  stale?: boolean
}>()
const { t } = useI18n()
const showInactive = ref(false)
const groupRows = computed(() => props.rows.filter(row => row.group_id != null && row.group_id > 0))
const cardRows = computed(() => groupRows.value.filter(row => row.group_platform !== 'composite' || row.platform === 'composite'))
const probesByGroupID = computed(() => {
  const result = new Map<number, UserMonitorView[]>()
  for (const probe of props.probes || []) {
    if (probe.group_id == null || probe.group_id <= 0) continue
    const rows = result.get(probe.group_id) || []
    rows.push(probe)
    result.set(probe.group_id, rows)
  }
  return result
})
const probesByGroupName = computed(() => {
  const result = new Map<string, UserMonitorView[]>()
  for (const probe of props.probes || []) {
    const name = normalizeGroupName(probe.group_name)
    if (!name) continue
    const rows = result.get(name) || []
    rows.push(probe)
    result.set(name, rows)
  }
  return result
})
const inactiveCount = computed(() => cardRows.value.filter(row => !hasEffectiveData(row)).length)
const platformOrder = ['composite', 'openai', 'anthropic', 'gemini', 'deepseek', 'zhipu', 'kimi', 'grok', 'kiro']
const legend = [{ key: 'healthy', value: 'healthy' }, { key: 'warning', value: 'warning' }, { key: 'critical', value: 'critical' }, { key: 'noData', value: null }] as const
const sections = computed(() => {
  const byPlatform = new Map<string, MonitorMatrixRow[]>()
  for (const row of cardRows.value.filter(row => cardRows.value.length <= 12 || showInactive.value || hasEffectiveData(row))) {
    const rows = byPlatform.get(row.platform) || []
    rows.push(row)
    byPlatform.set(row.platform, rows)
  }
  return [...byPlatform.entries()]
    .sort(([a], [b]) => (platformOrder.indexOf(a) < 0 ? 99 : platformOrder.indexOf(a)) - (platformOrder.indexOf(b) < 0 ? 99 : platformOrder.indexOf(b)) || a.localeCompare(b))
    .map(([platform, rows]) => ({ platform, rows: rows.sort((a, b) => (a.group_name || '').localeCompare(b.group_name || '')) }))
})
function platformLabel(platform: string) {
  return ({
    composite: t('channelMonitorV2.settings.compositePlatform'),
    openai: 'OpenAI',
    anthropic: 'Anthropic',
    gemini: 'Gemini',
    deepseek: 'DeepSeek',
    zhipu: 'GLM',
    kimi: 'Kimi',
    grok: 'Grok',
    kiro: 'Kiro',
  } as Record<string, string>)[platform] || platform
}
function compositeProviderRows(row: MonitorMatrixRow): MonitorMatrixRow[] {
  if (row.platform !== 'composite' || row.group_id == null) return []
  return groupRows.value
    .filter(child => child.group_id === row.group_id && child.group_platform === 'composite' && child.platform !== 'composite')
    .sort((a, b) => platformOrder.indexOf(a.platform) - platformOrder.indexOf(b.platform) || a.platform.localeCompare(b.platform))
}

type CompositeProbeProvider = {
  platform: string
  health: HealthState
  latency_ms: number | null
  models: string[]
}

type CompositeProviderDetail = {
  platform: string
  health: HealthState
  availability: string
  latency: string
  multiplier: string
  models: string[]
}

function compositeProbeProviders(row: MonitorMatrixRow): CompositeProbeProvider[] {
  const providers = new Map<string, CompositeProbeProvider>()
  const severity: Record<HealthState, number> = { unknown: 0, healthy: 1, warning: 2, critical: 3 }
  for (const probe of probesForRow(row)) {
    const models = [
      { model: probe.primary_model, status: probe.primary_status, latency_ms: probe.primary_latency_ms },
      ...(probe.extra_models || []),
    ]
    for (const item of models) {
      const platform = probeModelProvider(item.model, probe.provider)
      const health = probeStatusToHealth(item.status)
      const current = providers.get(platform)
      if (!current || severity[health] > severity[current.health]) {
        providers.set(platform, {
          platform,
          health,
          latency_ms: item.latency_ms ?? current?.latency_ms ?? null,
          models: [...new Set([...(current?.models || []), item.model].filter(Boolean))],
        })
      } else if (current.latency_ms == null && item.latency_ms != null) {
        current.latency_ms = item.latency_ms
        current.models = [...new Set([...current.models, item.model].filter(Boolean))]
      } else {
        current.models = [...new Set([...current.models, item.model].filter(Boolean))]
      }
    }
  }
  return [...providers.values()].sort((a, b) =>
    platformOrder.indexOf(a.platform) - platformOrder.indexOf(b.platform) || a.platform.localeCompare(b.platform)
  )
}

function compositeProviderDetails(row: MonitorMatrixRow): CompositeProviderDetail[] {
  const probes = new Map(compositeProbeProviders(row).map((provider) => [provider.platform, provider]))
  const details = new Map<string, CompositeProviderDetail>()
  for (const child of compositeProviderRows(row)) {
    const probe = probes.get(child.platform)
    const latency = latencyValue(child)
    details.set(child.platform, {
      platform: child.platform,
      health: child.health.overall,
      availability: availabilityValue(child),
      latency: latency === '—' ? t('channelMonitorV2.groupCards.noLatencySample') : latency,
      multiplier: providerMultiplier(child.rate_multiplier ?? row.rate_multiplier),
      models: probe?.models || [],
    })
  }
  for (const probe of probes.values()) {
    if (details.has(probe.platform)) continue
    details.set(probe.platform, {
      platform: probe.platform,
      health: probe.health,
      availability: probeStateText(probe.health),
      latency: probe.latency_ms == null ? t('channelMonitorV2.groupCards.noLatencySample') : formatMonitorMs(probe.latency_ms),
      multiplier: providerMultiplier(row.rate_multiplier),
      models: probe.models,
    })
  }
  return [...details.values()].sort((a, b) =>
    platformOrder.indexOf(a.platform) - platformOrder.indexOf(b.platform) || a.platform.localeCompare(b.platform)
  )
}

function compositeProviderCount(row: MonitorMatrixRow) {
  return compositeProviderDetails(row).length
}

function providerMultiplier(value: number | null | undefined) {
  return value == null ? '—' : formatMultiplier(value)
}

function providerModelTitle(provider: CompositeProviderDetail) {
  return provider.models.length
    ? t('channelMonitorV2.groupCards.providerModels', { models: provider.models.join(', ') })
    : t('channelMonitorV2.groupCards.providerModelsEmpty')
}
function probeModelProvider(model: string, fallback: string): string {
  if (/glm|zhipu/i.test(model)) return 'zhipu'
  if (/kimi|moonshot/i.test(model)) return 'kimi'
  if (/deepseek/i.test(model)) return 'deepseek'
  if (/grok/i.test(model)) return 'grok'
  if (/gemini/i.test(model)) return 'gemini'
  if (/claude|anthropic/i.test(model)) return 'anthropic'
  if (/gpt|openai/i.test(model)) return 'openai'
  return fallback || 'unknown'
}
function probeStateText(health: HealthState) {
  if (health === 'healthy') return t('channelMonitorV2.groupCards.states.activeOperational')
  if (health === 'warning') return t('channelMonitorV2.groupCards.states.activeDegraded')
  if (health === 'critical') return t('channelMonitorV2.groupCards.states.activeFailed')
  return t('channelMonitorV2.groupCards.states.unknown')
}
function probeStateTextClass(health: HealthState) {
  if (health === 'healthy') return 'text-emerald-700 dark:text-emerald-300'
  if (health === 'warning') return 'text-amber-700 dark:text-amber-300'
  if (health === 'critical') return 'text-rose-700 dark:text-rose-300'
  return 'text-gray-500 dark:text-gray-400'
}
function providerTone(platform: string) {
  if (platform === 'anthropic') return 'bg-orange-50 text-orange-700 dark:bg-orange-500/15 dark:text-orange-300'
  if (platform === 'openai') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
  return 'bg-blue-50 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300'
}
function providerText(platform: string) {
  return platform === 'anthropic' ? 'text-orange-700 dark:text-orange-300' : 'text-emerald-700 dark:text-emerald-300'
}
function formatMultiplier(value: number) {
  return `${Number(value.toFixed(4))}×`
}
function formatTime(value: string) {
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? new Date(timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '—'
}

function normalizeGroupName(value: string | null | undefined) {
  return (value || '').trim().toLocaleLowerCase()
}

function matchedProbesForRow(row: MonitorMatrixRow): UserMonitorView[] {
  if (row.group_id != null) {
    const exact = probesByGroupID.value.get(row.group_id)
    if (exact?.length) return exact
  }
  return probesByGroupName.value.get(normalizeGroupName(row.group_name)) || []
}

function probesForRow(row: MonitorMatrixRow): UserMonitorView[] {
  return matchedProbesForRow(row)
}

function probeModels(row: MonitorMatrixRow): string[] {
  return [...new Set(
    probesForRow(row)
      .flatMap((probe) => [probe.primary_model, ...(probe.extra_models || []).map((model) => model.model)])
      .map((model) => model?.trim())
      .filter((model): model is string => Boolean(model)),
  )]
}

function hasProbe(row: MonitorMatrixRow) {
  return probesForRow(row).length > 0
}

function hasTraffic(row: MonitorMatrixRow) {
  return row.metrics.has_requests === true || row.metrics.rpm > 0 || row.metrics.request_count > 0 || row.metrics.ttft.p50_ms != null ||
    row.metrics.cache_rate > 0 || row.metrics.cache_rate_denominator > 0 || row.health.overall !== 'unknown' ||
    row.buckets.some(bucket => bucket.metrics.rpm > 0 || bucket.metrics.request_count > 0 || bucket.metrics.ttft.p50_ms != null || bucket.metrics.cache_rate_denominator > 0 || bucket.health.overall !== 'unknown')
}

function cacheRateValue(row: MonitorMatrixRow): number | null {
  if (!hasTraffic(row)) return null
  const minimumSample = Math.max(1, row.health.thresholds?.minimum_sample ?? 50)
  if (row.metrics.cache_rate_denominator >= minimumSample) return row.metrics.cache_rate
  const buckets = [...row.buckets].sort((a, b) => Date.parse(b.bucket_start) - Date.parse(a.bucket_start))
  const representative = buckets.find((bucket) => bucket.metrics.cache_rate_denominator >= minimumSample) ||
    buckets.find((bucket) => bucket.metrics.cache_rate_denominator > 0)
  if (representative) return representative.metrics.cache_rate
  return row.metrics.cache_rate_denominator > 0 ? row.metrics.cache_rate : null
}

function cacheRateDisplay(row: MonitorMatrixRow) {
  const value = cacheRateValue(row)
  return value == null ? '—' : formatMonitorPercent(value)
}

function cacheRateTitle(row: MonitorMatrixRow) {
  const value = cacheRateValue(row)
  if (value == null) return t('channelMonitorV2.groupCards.cacheRateEmpty')
  const minimumSample = Math.max(1, row.health.thresholds?.minimum_sample ?? 50)
  return row.metrics.cache_rate_denominator < minimumSample
    ? t('channelMonitorV2.groupCards.cacheRateFallback')
    : t('channelMonitorV2.metrics.cacheRate')
}

function hasEffectiveData(row: MonitorMatrixRow) {
  return hasTraffic(row) || hasProbe(row)
}

function sourceLabel(row: MonitorMatrixRow) {
  return hasTraffic(row) || hasProbe(row)
    ? t('channelMonitorV2.groupCards.combined')
    : t('channelMonitorV2.groupCards.noData')
}

function probeState(row: MonitorMatrixRow): HealthState {
  const probes = probesForRow(row)
  if (probes.some(probe => probe.primary_status === 'failed' || probe.primary_status === 'error')) return 'critical'
  if (probes.some(probe => probe.primary_status === 'degraded')) return 'warning'
  if (probes.some(probe => probe.primary_status === 'operational')) return 'healthy'
  return 'unknown'
}

function effectiveState(row: MonitorMatrixRow): HealthState {
  return hasTraffic(row) ? row.health.overall : probeState(row)
}

function stateLabel(row: MonitorMatrixRow) {
  if (hasTraffic(row)) return t(`channelMonitorV2.groupCards.states.${row.health.overall}`)
  if (!hasProbe(row)) return t('channelMonitorV2.groupCards.noData')
  const state = probeState(row)
  if (state === 'healthy') return t('channelMonitorV2.groupCards.states.activeOperational')
  if (state === 'warning') return t('channelMonitorV2.groupCards.states.activeDegraded')
  if (state === 'critical') return t('channelMonitorV2.groupCards.states.activeFailed')
  return t('channelMonitorV2.groupCards.states.unknown')
}

function stateClass(row: MonitorMatrixRow) {
  const state = effectiveState(row)
  if (props.stale || state === 'unknown') return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  if (state === 'healthy') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
  if (state === 'critical') return 'bg-rose-50 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300'
  return 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
}

type TimelineSlot = { state: HealthState; timestamp: string; detail?: string }

function realSlots(row: MonitorMatrixRow): Array<TimelineSlot | null> {
  const coverage = props.coverage
  if (!coverage) return Array(20).fill(null)
  const start = Date.parse(coverage.requested_start)
  const end = Date.parse(coverage.requested_end || coverage.data_through)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return Array(20).fill(null)
  const width = (end - start) / 20
  const result: Array<TimelineSlot | null> = Array(20).fill(null)
  const priority: Record<HealthState, number> = { unknown: 0, healthy: 1, warning: 2, critical: 3 }
  for (const bucket of row.buckets) {
    const stamp = Date.parse(bucket.bucket_start)
    if (!Number.isFinite(stamp) || stamp < start || stamp >= end ||
      !(bucket.metrics.has_requests === true || bucket.metrics.rpm > 0 || bucket.metrics.request_count > 0 || bucket.metrics.ttft.p50_ms != null || bucket.health.overall !== 'unknown')) continue
    const index = Math.min(19, Math.floor((stamp - start) / width))
    const thresholds = bucket.health.thresholds || row.health.thresholds
    const state: HealthState = bucket.metrics.error_rate >= (thresholds?.critical_error_rate ?? 0.2) ? 'critical' : bucket.metrics.error_rate >= (thresholds?.warning_error_rate ?? 0.05) ? 'warning' : 'healthy'
    if (!result[index] || priority[state] > priority[result[index].state]) {
      result[index] = {
        state,
        timestamp: bucket.bucket_start,
        detail: t('channelMonitorV2.groupCards.availabilityValue', { value: formatMonitorPercent(1 - bucket.metrics.error_rate) }),
      }
    }
  }
  return result
}

function activeSlots(row: MonitorMatrixRow): Array<TimelineSlot | null> {
  const points = probesForRow(row)
    .flatMap(probe => probe.timeline || [])
    .filter(point => Number.isFinite(Date.parse(point.checked_at)))
    .sort((a, b) => Date.parse(b.checked_at) - Date.parse(a.checked_at))
    .slice(0, 20)
    .reverse()
  const result: Array<TimelineSlot | null> = Array(Math.max(0, 20 - points.length)).fill(null)
  result.push(...points.map(probeTimelineSlot))
  return result.slice(-20)
}

function probeTimelineSlot(point: MonitorTimelinePoint): TimelineSlot {
  return {
    state: probeStatusToHealth(point.status),
    timestamp: point.checked_at,
    detail: point.status === 'operational'
      ? t('channelMonitorV2.groupCards.states.activeOperational')
      : point.status === 'degraded'
        ? t('channelMonitorV2.groupCards.states.activeDegraded')
        : t('channelMonitorV2.groupCards.states.activeFailed'),
  }
}

function probeStatusToHealth(status: MonitorTimelinePoint['status']): HealthState {
  if (status === 'operational') return 'healthy'
  if (status === 'degraded') return 'warning'
  if (status === 'failed' || status === 'error') return 'critical'
  return 'unknown'
}

function timelineSlots(row: MonitorMatrixRow) {
  if (!hasTraffic(row)) return activeSlots(row)
  const result = realSlots(row)
  const coverage = props.coverage
  if (!coverage || !hasProbe(row)) return result
  const start = Date.parse(coverage.requested_start)
  const end = Date.parse(coverage.requested_end || coverage.data_through)
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return result
  const width = (end - start) / 20
  const points = probesForRow(row)
    .flatMap(probe => probe.timeline || [])
    .filter(point => Number.isFinite(Date.parse(point.checked_at)))
  for (const point of points) {
    const stamp = Date.parse(point.checked_at)
    if (stamp < start || stamp >= end) continue
    const index = Math.min(19, Math.floor((stamp - start) / width))
    if (result[index]) continue
    result[index] = probeTimelineSlot(point)
  }
  return result
}

function timelineAria(row: MonitorMatrixRow) {
  return hasTraffic(row) || hasProbe(row)
    ? t('channelMonitorV2.groupCards.combined')
    : t('channelMonitorV2.groupCards.noData')
}

function timelineWindowLabel(row: MonitorMatrixRow) {
  return hasTraffic(row) || hasProbe(row)
    ? t('channelMonitorV2.groupCards.combinedWindow')
    : t('channelMonitorV2.groupCards.noData')
}

function slotTitle(slot: TimelineSlot | null) {
  if (!slot) return t('channelMonitorV2.groupCards.noData')
  const state = t(`channelMonitorV2.groupCards.states.${slot.state}`)
  return `${new Date(slot.timestamp).toLocaleString()} · ${state}${slot.detail ? ` · ${slot.detail}` : ''}`
}

function probeAvailability(row: MonitorMatrixRow): number | null {
  const points = probesForRow(row).flatMap(probe => probe.timeline || [])
  if (points.length === 0) return null
  const available = points.filter(point => point.status === 'operational' || point.status === 'degraded').length
  return available / points.length
}

function availabilityValue(row: MonitorMatrixRow) {
  if (hasTraffic(row)) return formatMonitorPercent(1 - row.metrics.error_rate)
  const availability = probeAvailability(row)
  return availability == null ? '—' : formatMonitorPercent(availability)
}

function availabilityClass(row: MonitorMatrixRow) {
  const state = effectiveState(row)
  if (state === 'healthy') return 'text-emerald-700 dark:text-emerald-300'
  if (state === 'warning') return 'text-amber-700 dark:text-amber-300'
  if (state === 'critical') return 'text-rose-700 dark:text-rose-300'
  return 'text-gray-500 dark:text-gray-400'
}

function latestProbeTime(probe: UserMonitorView) {
  const values = [probe.timeline?.[0]?.checked_at, probe.intelligence_timeline?.[0]?.checked_at]
    .map(value => value ? Date.parse(value) : 0)
    .filter(Number.isFinite)
  return Math.max(0, ...values)
}

function probeLatency(row: MonitorMatrixRow): number | null {
  const probes = [...probesForRow(row)].sort((a, b) => latestProbeTime(b) - latestProbeTime(a))
  for (const probe of probes) {
    const pointLatency = probe.timeline?.find(point => point.latency_ms != null)?.latency_ms
    if (pointLatency != null) return pointLatency
    if (probe.primary_latency_ms != null) return probe.primary_latency_ms
  }
  return null
}

function latencyLabel(row: MonitorMatrixRow) {
  if (hasTraffic(row)) return t('channelMonitorV2.metrics.ttftP50')
  return hasProbe(row) ? t('channelMonitorV2.groupCards.activeLatency') : t('channelMonitorV2.groupCards.latency')
}

function latencyValue(row: MonitorMatrixRow) {
  if (hasTraffic(row)) return row.metrics.ttft.p50_ms == null ? '—' : `≈${formatMonitorMs(row.metrics.ttft.p50_ms)}`
  const latency = probeLatency(row)
  return latency == null ? '—' : formatMonitorMs(latency)
}

function lastUpdatedAt(row: MonitorMatrixRow): string | null {
  if (hasTraffic(row)) return props.coverage?.data_through || null
  const timestamps = probesForRow(row)
    .flatMap(probe => probe.timeline || [])
    .map(point => point.checked_at)
    .filter(value => Number.isFinite(Date.parse(value)))
    .sort((a, b) => Date.parse(b) - Date.parse(a))
  return timestamps[0] || null
}

function intelligenceProbe(row: MonitorMatrixRow): UserMonitorView | null {
  const probes = matchedProbesForRow(row)
    .filter(probe => probe.provider === 'openai' && probe.intelligence_enabled !== false)
    .sort((a, b) => (b.intelligence_timeline?.length || 0) - (a.intelligence_timeline?.length || 0))
  return probes[0] || null
}

function hasIntelligenceMonitor(row: MonitorMatrixRow): boolean {
  return matchedProbesForRow(row).some(probe => probe.provider === 'openai' && probe.intelligence_enabled !== false)
}

function intelligenceSlots(row: MonitorMatrixRow): Array<TimelineSlot | null> {
  const probe = intelligenceProbe(row)
  const points = (probe?.intelligence_timeline || [])
    .filter(point => Number.isFinite(Date.parse(point.checked_at)))
    .sort((a, b) => Date.parse(a.checked_at) - Date.parse(b.checked_at))
    .slice(-20)
  const result: Array<TimelineSlot | null> = Array(Math.max(0, 20 - points.length)).fill(null)
  result.push(...points.map(point => ({
    state: probeStatusToHealth(point.status),
    timestamp: point.checked_at,
    detail: point.status === 'operational'
      ? t('channelMonitorV2.groupCards.intelligenceNotDegraded')
      : point.status === 'failed'
        ? t('channelMonitorV2.groupCards.intelligenceDegraded')
        : t('channelMonitorV2.groupCards.intelligenceError'),
  })))
  return result
}

function intelligenceSummary(row: MonitorMatrixRow) {
  if (!props.hybrid) return t('channelMonitorV2.groupCards.intelligencePending')
  const probe = intelligenceProbe(row)
  if (!probe) return t('channelMonitorV2.groupCards.intelligenceNotConfigured')
  if (probe.intelligence_source === 'account_aggregate') {
    if (intelligenceProbeStale(probe)) return t('channelStatus.intelligenceStale')
    const samples = probe.intelligence_samples || 0
    const passed = probe.intelligence_passed || 0
    if (samples > 0) {
      return t('channelMonitorV2.groupCards.intelligenceAccountVote', {
        passed,
        samples,
      })
    }
    if ((probe.intelligence_errors || 0) > 0) {
      return t('channelMonitorV2.groupCards.intelligenceAccountErrors', {
        count: probe.intelligence_errors,
      })
    }
  }
  const latest = probe.intelligence_timeline?.[0]
  if (!latest) return t('channelMonitorV2.groupCards.intelligenceWaiting')
  if (latest.status === 'operational') return t('channelMonitorV2.groupCards.intelligenceNotDegraded')
  if (latest.status === 'failed') return t('channelMonitorV2.groupCards.intelligenceDegraded')
  return t('channelMonitorV2.groupCards.intelligenceError')
}

function intelligenceProbeStale(probe: UserMonitorView): boolean {
  const checkedAt = probe.intelligence_timeline?.[0]?.checked_at
  if (!checkedAt) return false
  const timestamp = Date.parse(checkedAt)
  const interval = Math.max(30, probe.intelligence_interval_seconds || probe.interval_seconds || 60)
  return !Number.isFinite(timestamp) || Date.now() - timestamp > interval * 1000 * 2
}

function intelligenceSummaryClass(row: MonitorMatrixRow) {
  if (!props.hybrid || !intelligenceProbe(row)) return 'text-gray-500 dark:text-gray-400'
  const probe = intelligenceProbe(row)
  const status = probe?.intelligence_status || probe?.intelligence_timeline?.[0]?.status
  const latest = probe?.intelligence_timeline?.[0]
  if (probe?.intelligence_source === 'account_aggregate' && status) {
    if (status === 'operational') return 'font-semibold text-emerald-700 dark:text-emerald-300'
    if (status === 'degraded') return 'font-semibold text-amber-700 dark:text-amber-300'
    if (status === 'failed') return 'font-semibold text-rose-700 dark:text-rose-300'
    return 'font-semibold text-amber-700 dark:text-amber-300'
  }
  if (!latest) return 'text-gray-500 dark:text-gray-400'
  if (latest.status === 'operational') return 'font-semibold text-emerald-700 dark:text-emerald-300'
  if (latest.status === 'failed') return 'font-semibold text-rose-700 dark:text-rose-300'
  return 'font-semibold text-amber-700 dark:text-amber-300'
}

function slotColor(state: HealthState | null) {
  if (state === 'healthy') return 'bg-emerald-500'
  if (state === 'warning') return 'bg-amber-400'
  if (state === 'critical') return 'bg-rose-500'
  return 'bg-gray-200 dark:bg-dark-600'
}
</script>
