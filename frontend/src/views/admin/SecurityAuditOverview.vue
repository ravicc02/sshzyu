<template>
  <section class="card overflow-hidden" data-test="security-audit-overview">
    <div class="flex flex-col gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">
            {{ t('admin.securityAudit.overview.title') }}
          </h2>
          <span
            class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium"
            :class="overallState.className"
          >
            <span class="size-1.5 rounded-full bg-current" />
            {{ overallState.label }}
          </span>
        </div>
        <p class="mt-1 text-sm text-gray-500 text-pretty dark:text-dark-300">
          {{ t('admin.securityAudit.overview.description') }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <span class="text-xs tabular-nums text-gray-500 dark:text-dark-400">
          {{ lastLoadedAt ? t('admin.securityAudit.overview.updatedAt', { time: formatDateTimeToMinute(lastLoadedAt) }) : t('admin.securityAudit.overview.notLoaded') }}
        </span>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loading || refreshing"
          data-test="security-audit-overview-refresh"
          @click="loadOverview"
        >
          <Icon name="refresh" size="sm" />
          {{ refreshing ? t('admin.securityAudit.overview.refreshing') : t('common.refresh') }}
        </button>
      </div>
    </div>

    <div class="grid gap-px bg-gray-100 dark:bg-dark-700 sm:grid-cols-2 xl:grid-cols-4">
      <div
        v-for="metric in metrics"
        :key="metric.key"
        class="min-w-0 bg-white px-5 py-5 dark:bg-dark-800"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ metric.label }}</p>
            <p class="mt-2 truncate text-2xl font-semibold tabular-nums text-gray-950 dark:text-white">
              {{ metric.value }}
            </p>
            <p class="mt-1 truncate text-xs text-gray-500 dark:text-dark-400">{{ metric.meta }}</p>
          </div>
          <span class="flex size-9 shrink-0 items-center justify-center rounded-lg" :class="metric.iconClass">
            <Icon :name="metric.icon" size="sm" />
          </span>
        </div>
      </div>
    </div>

    <div class="grid gap-4 border-t border-gray-100 px-5 py-4 dark:border-dark-700 sm:grid-cols-2 xl:grid-cols-3">
      <div v-for="item in runtimeItems" :key="item.key" class="min-w-0">
        <p class="text-xs text-gray-500 dark:text-dark-400">{{ item.label }}</p>
        <p class="mt-1 flex min-w-0 items-center gap-2 text-sm font-medium text-gray-900 dark:text-dark-100">
          <span class="size-2 shrink-0 rounded-full" :class="item.dotClass" />
          <span class="truncate">{{ item.value }}</span>
        </p>
      </div>
    </div>

    <p
      v-if="partialError"
      class="flex items-center gap-2 border-t border-amber-100 bg-amber-50 px-5 py-2.5 text-xs text-amber-800 dark:border-amber-900/40 dark:bg-amber-900/15 dark:text-amber-200"
    >
      <Icon name="exclamationTriangle" size="sm" />
      {{ t('admin.securityAudit.overview.partialError') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { riskControlAPI, type ContentModerationRuntimeStatus } from '@/api/admin/riskControl'
import { promptAuditAPI } from '@/features/prompt-audit/api'
import { emptyEventFilters } from '@/features/prompt-audit/viewModel'
import type { PromptAuditRuntime } from '@/features/prompt-audit/types'
import { formatDateTime, formatDateTimeToMinute, formatNumber } from '@/utils/format'

const { t } = useI18n()

const loading = ref(true)
const refreshing = ref(false)
const hasLoaded = ref(false)
const partialError = ref(false)
const lastLoadedAt = ref<Date | null>(null)
const filterTotal = ref<number | null>(null)
const modelTotal = ref<number | null>(null)
const latestFilterAt = ref<string | null>(null)
const latestModelAt = ref<string | null>(null)
const contentStatus = ref<ContentModerationRuntimeStatus | null>(null)
const promptRuntime = ref<PromptAuditRuntime | null>(null)

const filterEnabled = computed(() => Boolean(
  contentStatus.value?.risk_control_enabled && contentStatus.value.enabled,
))
const promptState = computed(() => promptRuntime.value?.process_status ?? 'unknown')
const promptStateLabel = computed(() => {
  if (['running', 'degraded', 'disabled', 'error', 'unknown'].includes(promptState.value)) {
    return t(`admin.securityAudit.overview.states.${promptState.value}`)
  }
  return promptState.value
})

const overallState = computed(() => {
  if (promptState.value === 'degraded' || promptState.value === 'error' || partialError.value) {
    return {
      label: t('admin.securityAudit.overview.states.degraded'),
      className: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-200',
    }
  }
  if (filterEnabled.value || promptState.value === 'running') {
    return {
      label: t('admin.securityAudit.overview.states.running'),
      className: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-200',
    }
  }
  if (!loading.value && contentStatus.value && promptRuntime.value) {
    return {
      label: t('admin.securityAudit.overview.states.disabled'),
      className: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200',
    }
  }
  return {
    label: t('admin.securityAudit.overview.states.unknown'),
    className: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300',
  }
})

const metrics = computed(() => [
  {
    key: 'filter-records',
    label: t('admin.securityAudit.overview.filterRecords'),
    value: filterTotal.value === null ? '—' : formatNumber(filterTotal.value),
    meta: t('admin.securityAudit.overview.retainedRecords'),
    icon: 'filter' as const,
    iconClass: 'bg-sky-50 text-sky-700 dark:bg-sky-900/25 dark:text-sky-300',
  },
  {
    key: 'model-records',
    label: t('admin.securityAudit.overview.modelRecords'),
    value: modelTotal.value === null ? '—' : formatNumber(modelTotal.value),
    meta: t('admin.securityAudit.overview.auditedPrompts'),
    icon: 'shield' as const,
    iconClass: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/25 dark:text-emerald-300',
  },
  {
    key: 'guard-result',
    label: t('admin.securityAudit.overview.guardResult'),
    value: promptRuntime.value
      ? `${formatNumber(promptRuntime.value.guard_metrics.allowed)} / ${formatNumber(promptRuntime.value.guard_metrics.blocked)}`
      : '—',
    meta: t('admin.securityAudit.overview.allowBlock'),
    icon: 'checkCircle' as const,
    iconClass: 'bg-violet-50 text-violet-700 dark:bg-violet-900/25 dark:text-violet-300',
  },
  {
    key: 'queue',
    label: t('admin.securityAudit.overview.activeQueue'),
    value: promptRuntime.value
      ? `${formatNumber(promptRuntime.value.queue.active)} / ${formatNumber(promptRuntime.value.queue_capacity)}`
      : '—',
    meta: promptRuntime.value
      ? t('admin.securityAudit.overview.queueDetail', {
          queued: formatNumber(promptRuntime.value.queue.queued),
          processing: formatNumber(promptRuntime.value.queue.processing),
          retry: formatNumber(promptRuntime.value.queue.retry),
        })
      : t('admin.securityAudit.overview.noRuntimeData'),
    icon: 'cube' as const,
    iconClass: 'bg-amber-50 text-amber-700 dark:bg-amber-900/25 dark:text-amber-300',
  },
])

function statusTone(healthy: boolean): string {
  return healthy ? 'bg-emerald-500' : 'bg-amber-500'
}

function dependencyHealthy(value?: string): boolean {
  return ['ok', 'healthy', 'up', 'connected'].includes(String(value ?? '').toLowerCase())
}

const runtimeItems = computed(() => [
  {
    key: 'filter-engine',
    label: t('admin.securityAudit.overview.filterEngine'),
    value: contentStatus.value
      ? filterEnabled.value
        ? t('admin.securityAudit.overview.states.running')
        : t('admin.securityAudit.overview.states.disabled')
      : t('admin.securityAudit.overview.states.unknown'),
    dotClass: statusTone(filterEnabled.value),
  },
  {
    key: 'model-audit',
    label: t('admin.securityAudit.overview.modelAudit'),
    value: promptStateLabel.value,
    dotClass: statusTone(promptState.value === 'running'),
  },
  {
    key: 'workers',
    label: t('admin.securityAudit.overview.workers'),
    value: promptRuntime.value
      ? `${formatNumber(promptRuntime.value.worker_active)} / ${formatNumber(promptRuntime.value.worker_total)}`
      : '—',
    dotClass: statusTone(Boolean(promptRuntime.value)),
  },
  {
    key: 'version',
    label: t('admin.securityAudit.overview.configVersion'),
    value: promptRuntime.value
      ? `${promptRuntime.value.active_config_version} / ${promptRuntime.value.expected_config_version}`
      : '—',
    dotClass: statusTone(
      Boolean(promptRuntime.value)
      && promptRuntime.value?.active_config_version === promptRuntime.value?.expected_config_version,
    ),
  },
  {
    key: 'dependencies',
    label: t('admin.securityAudit.overview.dependencies'),
    value: promptRuntime.value
      ? `${promptRuntime.value.database_status} / ${promptRuntime.value.redis_status}`
      : '—',
    dotClass: statusTone(
      dependencyHealthy(promptRuntime.value?.database_status)
      && dependencyHealthy(promptRuntime.value?.redis_status),
    ),
  },
  {
    key: 'last-processed',
    label: t('admin.securityAudit.overview.lastProcessed'),
    value: latestActivityAt.value
      ? formatDateTime(latestActivityAt.value)
      : t('admin.securityAudit.overview.never'),
    dotClass: statusTone(Boolean(latestActivityAt.value)),
  },
])

const latestActivityAt = computed(() => {
  const timestamps = [
    promptRuntime.value?.last_processed_at,
    latestFilterAt.value,
    latestModelAt.value,
  ]
    .filter((value): value is string => Boolean(value))
    .map(value => ({ value, time: new Date(value).getTime() }))
    .filter(item => Number.isFinite(item.time))
    .sort((left, right) => right.time - left.time)
  return timestamps[0]?.value ?? null
})

async function loadOverview() {
  const initial = !hasLoaded.value
  loading.value = initial
  refreshing.value = !initial
  try {
    const [filterResult, modelResult, statusResult, runtimeResult] = await Promise.allSettled([
      promptAuditAPI.listFilterRecords(emptyEventFilters(), 1, 1),
      promptAuditAPI.listEvents(emptyEventFilters(), 1, 1),
      riskControlAPI.getStatus(),
      promptAuditAPI.getRuntime(),
    ])

    if (filterResult.status === 'fulfilled') {
      filterTotal.value = filterResult.value.total
      latestFilterAt.value = filterResult.value.items[0]?.created_at ?? null
    }
    if (modelResult.status === 'fulfilled') {
      modelTotal.value = modelResult.value.total
      latestModelAt.value = modelResult.value.items[0]?.created_at ?? null
    }
    if (statusResult.status === 'fulfilled') contentStatus.value = statusResult.value
    if (runtimeResult.status === 'fulfilled') promptRuntime.value = runtimeResult.value

    partialError.value = [filterResult, modelResult, statusResult, runtimeResult]
      .some(result => result.status === 'rejected')
    lastLoadedAt.value = new Date()
    hasLoaded.value = true
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

onMounted(loadOverview)
</script>
