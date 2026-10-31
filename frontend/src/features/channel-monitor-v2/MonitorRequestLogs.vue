<template>
  <section class="space-y-3 border-t border-gray-100 pt-5 dark:border-dark-700">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('channelMonitorV2.requestLogs.title') }}
        </h3>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
          {{ t('channelMonitorV2.requestLogs.description') }}
        </p>
      </div>
      <button type="button" class="btn btn-ghost btn-sm shrink-0" :disabled="loading || !groups.length" @click="load">
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        {{ loading ? t('channelMonitorV2.requestLogs.refreshing') : t('channelMonitorV2.requestLogs.refresh') }}
      </button>
    </div>

    <p v-if="!groups.length" class="rounded-lg border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
      {{ t('channelMonitorV2.requestLogs.selectGroups') }}
    </p>
    <template v-else>
      <div class="flex flex-wrap items-center gap-2">
        <label class="relative min-w-[180px] flex-1">
          <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
          <input
            v-model.trim="searchQuery"
            type="search"
            class="input w-full pl-9"
            :placeholder="t('channelMonitorV2.requestLogs.searchPlaceholder')"
            :aria-label="t('channelMonitorV2.requestLogs.searchPlaceholder')"
          />
        </label>
        <select v-model="groupFilter" class="input w-full sm:w-[180px]" :aria-label="t('channelMonitorV2.requestLogs.filterGroup')">
          <option value="">{{ t('channelMonitorV2.requestLogs.allGroups') }}</option>
          <option v-for="group in groupOptions" :key="group.id" :value="String(group.id)">{{ group.name }}</option>
        </select>
        <select v-model="requestFilter" class="input w-full sm:w-[140px]" :aria-label="t('channelMonitorV2.requestLogs.filterResult')">
          <option value="">{{ t('channelMonitorV2.requestLogs.allResults') }}</option>
          <option value="success">{{ t('channelMonitorV2.requestLogs.success') }}</option>
          <option value="failed">{{ t('channelMonitorV2.requestLogs.failed') }}</option>
        </select>
        <select v-model="intelligenceFilter" class="input w-full sm:w-[150px]" :aria-label="t('channelMonitorV2.requestLogs.filterIntelligence')">
          <option value="">{{ t('channelMonitorV2.requestLogs.allIntelligence') }}</option>
          <option value="operational">{{ t('channelMonitorV2.groupCards.intelligenceNotDegraded') }}</option>
          <option value="failed">{{ t('channelMonitorV2.groupCards.intelligenceDegraded') }}</option>
          <option value="error">{{ t('channelMonitorV2.groupCards.intelligenceError') }}</option>
          <option value="none">{{ t('channelMonitorV2.requestLogs.noIntelligence') }}</option>
        </select>
      </div>

      <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
        <div class="overflow-x-auto">
          <table class="min-w-[720px] w-full text-left text-xs">
            <thead class="bg-gray-50 text-[11px] font-semibold text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-3 py-2.5">{{ t('channelMonitorV2.requestLogs.columns.time') }}</th>
                <th class="px-3 py-2.5">{{ t('channelMonitorV2.requestLogs.columns.group') }}</th>
                <th class="px-3 py-2.5">{{ t('channelMonitorV2.requestLogs.columns.account') }}</th>
                <th class="px-3 py-2.5">{{ t('channelMonitorV2.requestLogs.columns.model') }}</th>
                <th class="px-3 py-2.5">{{ t('channelMonitorV2.requestLogs.columns.status') }}</th>
                <th class="px-3 py-2.5">{{ t('channelMonitorV2.requestLogs.columns.intelligence') }}</th>
                <th class="px-3 py-2.5 text-right">{{ t('channelMonitorV2.requestLogs.columns.action') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-if="loading && !items.length">
                <td colspan="7" class="px-3 py-8 text-center text-gray-500">{{ t('common.loading') }}</td>
              </tr>
              <tr v-else-if="!filteredItems.length">
                <td colspan="7" class="px-3 py-8 text-center text-gray-500">{{ t('channelMonitorV2.requestLogs.filteredEmpty') }}</td>
              </tr>
              <template v-else>
                <tr v-for="item in pagedItems" :key="item.id" class="hover:bg-gray-50/70 dark:hover:bg-dark-700/40">
                  <td class="whitespace-nowrap px-3 py-2.5 text-gray-500 dark:text-gray-400">{{ formatTime(item.created_at) }}</td>
                  <td class="max-w-[180px] px-3 py-2.5">
                    <span class="block truncate font-medium text-gray-800 dark:text-gray-100" :title="item.group_name">{{ item.group_name || `#${item.group_id}` }}</span>
                    <span class="mt-0.5 block text-[10px] text-gray-400">{{ item.group_platform }}</span>
                  </td>
                  <td class="max-w-[150px] px-3 py-2.5">
                    <span class="block truncate text-gray-700 dark:text-gray-200" :title="item.account_name">{{ item.account_name || `#${item.account_id}` }}</span>
                    <span class="mt-0.5 block text-[10px] text-gray-400">{{ item.account_platform }}</span>
                  </td>
                  <td class="max-w-[190px] px-3 py-2.5">
                    <span class="block truncate font-mono text-[11px] text-gray-700 dark:text-gray-200" :title="modelTitle(item)">{{ item.model }}</span>
                  </td>
                  <td class="px-3 py-2.5">
                    <span class="rounded-full px-2 py-0.5 font-semibold" :class="requestStatusClass(item)">
                      {{ item.success ? t('channelMonitorV2.requestLogs.success') : t('channelMonitorV2.requestLogs.failed') }}
                    </span>
                  </td>
                  <td class="px-3 py-2.5">
                    <span v-if="item.intelligence_status" class="rounded-full px-2 py-0.5 font-semibold" :class="intelligenceClass(item)">
                      {{ intelligenceLabel(item) }}
                    </span>
                    <span v-else class="text-gray-400">—</span>
                  </td>
                  <td class="px-3 py-2.5 text-right">
                    <button type="button" class="btn btn-ghost btn-sm" @click="selected = item">
                      {{ t('channelMonitorV2.requestLogs.details') }}
                    </button>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </div>

      <div class="flex flex-wrap items-center justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ t('channelMonitorV2.requestLogs.retained', { count: items.length }) }}</span>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-ghost btn-sm" :aria-label="t('channelMonitorV2.requestLogs.previousPage')" :disabled="page <= 1" @click="page--">
            <Icon name="chevronLeft" size="sm" />
          </button>
          <span class="min-w-12 text-center tabular-nums">{{ page }} / {{ pageCount }}</span>
          <button type="button" class="btn btn-ghost btn-sm" :aria-label="t('channelMonitorV2.requestLogs.nextPage')" :disabled="page >= pageCount" @click="page++">
            <Icon name="chevronRight" size="sm" />
          </button>
        </div>
      </div>
    </template>

    <BaseDialog
      :show="selected != null"
      :title="t('channelMonitorV2.requestLogs.detailTitle')"
      width="wide"
      @close="selected = null"
    >
      <div v-if="selected" class="space-y-5">
        <dl class="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
          <div>
            <dt class="text-xs text-gray-500">{{ t('channelMonitorV2.requestLogs.columns.time') }}</dt>
            <dd class="mt-1 font-medium text-gray-900 dark:text-gray-100">{{ formatTime(selected.created_at, true) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500">{{ t('channelMonitorV2.requestLogs.requestId') }}</dt>
            <dd class="mt-1 break-all font-mono text-xs text-gray-700 dark:text-gray-300">{{ selected.request_id }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500">{{ t('channelMonitorV2.requestLogs.columns.group') }}</dt>
            <dd class="mt-1 font-medium text-gray-900 dark:text-gray-100">{{ selected.group_name || `#${selected.group_id}` }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500">{{ t('channelMonitorV2.requestLogs.columns.account') }}</dt>
            <dd class="mt-1 font-medium text-gray-900 dark:text-gray-100">
              {{ selected.account_name || `#${selected.account_id}` }}
              <span class="ml-1 text-xs text-gray-400">{{ selected.account_platform }}</span>
            </dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500">{{ t('channelMonitorV2.requestLogs.columns.model') }}</dt>
            <dd class="mt-1 break-all font-mono text-xs text-gray-700 dark:text-gray-300">{{ modelTitle(selected) }}</dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500">{{ t('channelMonitorV2.requestLogs.latency') }}</dt>
            <dd class="mt-1 text-gray-900 dark:text-gray-100">
              {{ selected.duration_ms == null ? '—' : `${selected.duration_ms} ms` }}
              /
              {{ selected.first_token_ms == null ? '—' : `${selected.first_token_ms} ms` }}
            </dd>
          </div>
          <div>
            <dt class="text-xs text-gray-500">{{ t('channelMonitorV2.requestLogs.tokens') }}</dt>
            <dd class="mt-1 text-gray-900 dark:text-gray-100">{{ selected.input_tokens }} / {{ selected.output_tokens }}</dd>
          </div>
        </dl>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <strong class="text-sm text-gray-900 dark:text-white">{{ t('channelMonitorV2.requestLogs.intelligenceResult') }}</strong>
            <span v-if="selected.intelligence_status" class="rounded-full px-2 py-0.5 text-xs font-semibold" :class="intelligenceClass(selected)">
              {{ intelligenceLabel(selected) }}
            </span>
          </div>
          <p v-if="selected.intelligence_checked_at" class="mt-1 text-xs text-gray-500">
            {{ formatTime(selected.intelligence_checked_at, true) }}
          </p>
          <pre class="mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-50 p-3 text-xs leading-5 text-gray-800 dark:bg-dark-900 dark:text-gray-200">{{ selected.intelligence_message || t('channelMonitorV2.requestLogs.noIntelligenceReply') }}</pre>
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="selected = null">{{ t('common.close') }}</button>
      </template>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminGroup } from '@/types'
import type { MonitorRequestLog } from '@/api/channelMonitorV2'
import * as api from '@/api/channelMonitorV2'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = withDefaults(defineProps<{
  groups: AdminGroup[]
}>(), {
  groups: () => [],
})

const { t, locale } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const items = ref<MonitorRequestLog[]>([])
const selected = ref<MonitorRequestLog | null>(null)
const searchQuery = ref('')
const groupFilter = ref('')
const requestFilter = ref('')
const intelligenceFilter = ref('')
const page = ref(1)
const pageSize = 10
let controller: AbortController | null = null
let timer: number | null = null

const groupOptions = computed(() => {
  const groups = new Map<number, string>()
  for (const item of items.value) groups.set(item.group_id, item.group_name || `#${item.group_id}`)
  return [...groups].map(([id, name]) => ({ id, name })).sort((a, b) => a.name.localeCompare(b.name))
})

const filteredItems = computed(() => {
  const query = searchQuery.value.toLowerCase()
  return items.value.filter((item) => {
    const matchesQuery = !query || [item.group_name, item.account_name, item.model, item.upstream_model]
      .some((value) => value?.toLowerCase().includes(query))
    const matchesGroup = !groupFilter.value || String(item.group_id) === groupFilter.value
    const matchesRequest = requestFilter.value === 'success' ? item.success : requestFilter.value === 'failed' ? !item.success : true
    const matchesIntelligence =
      intelligenceFilter.value === 'none' ? !item.intelligence_status :
      intelligenceFilter.value === 'error' ? ['error', 'degraded'].includes(item.intelligence_status) :
      intelligenceFilter.value ? item.intelligence_status === intelligenceFilter.value :
      true
    return matchesQuery && matchesGroup && matchesRequest && matchesIntelligence
  })
})

const pageCount = computed(() => Math.max(1, Math.ceil(filteredItems.value.length / pageSize)))
const pagedItems = computed(() => filteredItems.value.slice((page.value - 1) * pageSize, page.value * pageSize))

function groupScope() {
  return [...new Set(props.groups.map(group => group.id).filter(id => id > 0))].sort((a, b) => a - b)
}

async function load() {
  const groupIds = groupScope()
  if (!groupIds.length) {
    controller?.abort()
    items.value = []
    return
  }
  controller?.abort()
  const request = new AbortController()
  controller = request
  loading.value = true
  try {
    const result = await api.getRequestLogs({
      range: '24h',
      platforms: [],
      groupIds,
      models: [],
    }, true, 50, request.signal)
    items.value = result.items || []
  } catch (error) {
    if ((error as { name?: string }).name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.requestLogs.loadFailed')))
    }
  } finally {
    if (controller === request) loading.value = false
  }
}

function formatTime(value: string, full = false) {
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return value
  return new Intl.DateTimeFormat(locale.value || undefined, full
    ? { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }
    : { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }
  ).format(date)
}

function modelTitle(item: MonitorRequestLog) {
  return item.upstream_model && item.upstream_model !== item.model
    ? `${item.model} -> ${item.upstream_model}`
    : item.model
}

function requestStatusClass(item: MonitorRequestLog) {
  return item.success
    ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
    : 'bg-rose-50 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300'
}

function intelligenceLabel(item: MonitorRequestLog) {
  if (item.intelligence_status === 'operational') return t('channelMonitorV2.groupCards.intelligenceNotDegraded')
  if (item.intelligence_status === 'failed') return t('channelMonitorV2.groupCards.intelligenceDegraded')
  if (item.intelligence_status === 'error' || item.intelligence_status === 'degraded') return t('channelMonitorV2.groupCards.intelligenceError')
  return t('channelMonitorV2.requestLogs.noIntelligence')
}

function intelligenceClass(item: MonitorRequestLog) {
  if (item.intelligence_status === 'operational') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
  if (item.intelligence_status === 'failed') return 'bg-rose-50 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300'
  return 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
}

watch(
  () => props.groups.map(group => group.id).sort((a, b) => a - b).join(','),
  () => void load(),
  { immediate: true },
)

watch([searchQuery, groupFilter, requestFilter, intelligenceFilter], () => {
  page.value = 1
})

watch(pageCount, (count) => {
  if (page.value > count) page.value = count
})

onMounted(() => {
  timer = window.setInterval(() => {
    if (document.visibilityState === 'visible') void load()
  }, 60_000)
})

onBeforeUnmount(() => {
  controller?.abort()
  if (timer != null) window.clearInterval(timer)
})
</script>
