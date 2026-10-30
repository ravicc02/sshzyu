<template>
  <AppLayout>
    <div class="space-y-5 pb-12">
      <section class="monitor-toolbar flex flex-wrap items-center gap-2" :aria-label="t('channelMonitorV2.title')">
        <FilterMultiSelect
          v-model="filter.platforms"
          compact
          :label="t('channelMonitorV2.filters.platform')"
          :all-label="t('channelMonitorV2.filters.allPlatforms')"
          :options="platformOptions"
        />
        <FilterMultiSelect
          v-model="selectedGroupIds"
          compact
          :label="t('channelMonitorV2.filters.group')"
          :all-label="t('channelMonitorV2.filters.allGroups')"
          :options="groupOptions"
        />
        <FilterMultiSelect
          v-model="filter.models"
          compact
          :label="t('channelMonitorV2.filters.model')"
          :all-label="t('channelMonitorV2.filters.allModels')"
          :options="modelOptions"
        />
        <button
          type="button"
          class="btn btn-ghost btn-sm shrink-0 !px-2 !py-1 text-xs"
          :disabled="!hasDimensionFilter"
          :class="!hasDimensionFilter ? 'opacity-40' : ''"
          @click="clearDimensions"
        >
          {{ t('channelMonitorV2.clearFilters') }}
        </button>

        <div class="ml-auto flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
          <LoadingSpinner v-if="refreshing" size="sm" />
          <span v-if="dataThrough" class="tabular-nums">{{ t('channelMonitorV2.updatedTo', { time: formatTime(dataThrough) }) }}</span>
          <button
            class="btn btn-secondary btn-icon flex h-8 w-8 items-center justify-center rounded-lg"
            type="button"
            :title="t('common.refresh')"
            :disabled="refreshing"
            @click="reload(false)"
          >
            <Icon name="refresh" size="sm" :class="refreshing ? 'animate-spin' : ''" />
          </button>
        </div>
      </section>

      <GroupStatusCards
        :rows="groupMatrix?.items || []"
        :coverage="groupMatrix?.coverage || null"
        :probes="probes"
        :hybrid="isHybridMode"
        :loading="loading"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import FilterMultiSelect from '@/features/channel-monitor-v2/FilterMultiSelect.vue'
import GroupStatusCards from '@/features/channel-monitor-v2/GroupStatusCards.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getChannelMonitorMode } from '@/utils/featureFlags'
import * as api from '@/api/channelMonitorV2'
import { list as listActiveProbes, type UserMonitorView } from '@/api/channelMonitor'
import type { MonitorDimensions, MonitorFilter, MonitorMatrixResponse } from '@/api/channelMonitorV2'

const AUTO_REFRESH_MS = 60_000
const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const isAdmin = computed(() => authStore.isAdmin)
const isHybridMode = computed(
  () => appStore.cachedPublicSettings?.channel_monitor_enabled !== false && getChannelMonitorMode() === 'hybrid',
)

const filter = ref<MonitorFilter>({
  range: '90m',
  platforms: csv(route.query.platform),
  groupIds: csv(route.query.group).map(Number).filter(Boolean),
  models: csv(route.query.model),
})
const dimensions = ref<MonitorDimensions>({ platforms: [], groups: [], models: [] })
const groupMatrix = ref<MonitorMatrixResponse | null>(null)
const probes = ref<UserMonitorView[]>([])
const loading = ref(false)
const refreshing = ref(false)
let controller: AbortController | null = null
let sequence = 0
let autoRefreshTimer: number | null = null

const hasDimensionFilter = computed(
  () => filter.value.platforms.length + filter.value.groupIds.length + filter.value.models.length > 0,
)
const dataThrough = computed(() => groupMatrix.value?.coverage.data_through || null)
const platformOptions = computed(() =>
  (dimensions.value.platforms || []).map((item) => ({ value: item.value, label: item.label })),
)
const selectedPlatforms = computed(() => new Set(filter.value.platforms))
const groupOptions = computed(() =>
  (dimensions.value.groups || [])
    .filter(
      (item) =>
        selectedPlatforms.value.size === 0 ||
        !item.platform ||
        selectedPlatforms.value.has(item.platform),
    )
    .map((item) => ({
      value: String(item.id),
      label: item.platform ? `${item.platform} / ${item.name || `#${item.id}`}` : item.name || `#${item.id}`,
    })),
)
const modelOptions = computed(() =>
  (dimensions.value.models || [])
    .filter(
      (item) =>
        selectedPlatforms.value.size === 0 ||
        !item.platform ||
        selectedPlatforms.value.has(item.platform),
    )
    .map((item) => ({
      value: item.value,
      label:
        item.platform && !item.label.includes(item.platform)
          ? `${item.platform} / ${item.label}`
          : item.label,
    })),
)
const selectedGroupIds = computed({
  get: () => filter.value.groupIds.map(String),
  set: (value: string[]) => {
    filter.value.groupIds = value.map(Number).filter((id) => Number.isInteger(id) && id > 0)
  },
})

function csv(value: unknown) {
  return typeof value === 'string' ? value.split(',').filter(Boolean) : []
}

function syncQuery() {
  void router.replace({
    query: {
      platform: filter.value.platforms.join(',') || undefined,
      group: filter.value.groupIds.join(',') || undefined,
      model: filter.value.models.join(',') || undefined,
    },
  })
}

async function loadDimensions(signal?: AbortSignal, id = sequence) {
  const next = await api.getDimensions(
    { range: '90m', platforms: [], groupIds: [], models: [] },
    isAdmin.value,
    signal,
  )
  if (id === sequence) dimensions.value = next
}

async function loadCards(signal?: AbortSignal, id = sequence) {
  const next = await api.getMatrix(filter.value, 'platform_group', isAdmin.value, signal)
  if (id === sequence) groupMatrix.value = next
}

async function loadProbes(signal?: AbortSignal, id = sequence) {
  if (!isHybridMode.value) {
    probes.value = []
    return
  }
  try {
    const next = await listActiveProbes({ signal })
    if (id === sequence) probes.value = next.items || []
  } catch (error) {
    if ((error as { name?: string }).name === 'CanceledError') throw error
    if (id === sequence) probes.value = []
    console.warn('load active channel probes failed', error)
  }
}

async function reload(silent = true) {
  controller?.abort()
  const request = new AbortController()
  controller = request
  const id = ++sequence
  refreshing.value = true
  if (!silent) loading.value = true
  try {
    await Promise.all([
      loadDimensions(request.signal, id),
      loadCards(request.signal, id),
      loadProbes(request.signal, id),
    ])
  } catch (error) {
    if ((error as { name?: string }).name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.loadFailed')))
    }
  } finally {
    if (id === sequence) {
      loading.value = false
      refreshing.value = false
    }
  }
}

async function reloadCards() {
  controller?.abort()
  const request = new AbortController()
  controller = request
  const id = ++sequence
  refreshing.value = true
  try {
    await Promise.all([loadCards(request.signal, id), loadProbes(request.signal, id)])
  } catch (error) {
    if ((error as { name?: string }).name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.loadFailed')))
    }
  } finally {
    if (id === sequence) refreshing.value = false
  }
}

function clearDimensions() {
  filter.value = { ...filter.value, platforms: [], groupIds: [], models: [] }
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat(locale.value || undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

watch(
  [groupOptions, modelOptions],
  () => {
    if (groupOptions.value.length > 0) {
      const allowed = new Set(groupOptions.value.map((item) => item.value))
      const next = filter.value.groupIds.filter((id) => allowed.has(String(id)))
      if (next.length !== filter.value.groupIds.length) filter.value.groupIds = next
    }
    if (modelOptions.value.length > 0) {
      const allowed = new Set(modelOptions.value.map((item) => item.value))
      const next = filter.value.models.filter((model) => allowed.has(model))
      if (next.length !== filter.value.models.length) filter.value.models = next
    }
  },
  { flush: 'post' },
)

watch(
  filter,
  () => {
    syncQuery()
    void reloadCards()
  },
  { deep: true },
)

onMounted(() => {
  void reload(false)
  autoRefreshTimer = window.setInterval(() => void reloadCards(), AUTO_REFRESH_MS)
})

onBeforeUnmount(() => {
  controller?.abort()
  if (autoRefreshTimer) window.clearInterval(autoRefreshTimer)
})
</script>
