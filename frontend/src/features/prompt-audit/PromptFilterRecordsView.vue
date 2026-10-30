<template>
  <section class="card overflow-hidden" data-test="prompt-filter-records">
    <div class="flex flex-col gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">
          {{ t('admin.promptAudit.filterRecords.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 text-pretty dark:text-dark-300">
          {{ t('admin.promptAudit.filterRecords.description') }}
        </p>
      </div>
      <button
        type="button"
        class="btn btn-secondary btn-sm self-start"
        :disabled="loading"
        data-test="prompt-filter-records-refresh"
        @click="loadRecords"
      >
        <Icon name="refresh" size="sm" />
        {{ loading ? t('common.loading') : t('common.refresh') }}
      </button>
    </div>

    <div class="space-y-4 border-b border-gray-100 p-5 dark:border-dark-700">
      <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-6">
        <Select v-model="filters.decision" :options="resultOptions" @change="reloadFromFirstPage" />
        <Select v-model="filters.group_id" :options="groupOptions" @change="reloadFromFirstPage" />
        <Select v-model="filters.endpoint" :options="endpointOptions" @change="reloadFromFirstPage" />
        <input
          v-model.trim="filters.keyword"
          type="search"
          class="input"
          :placeholder="t('admin.promptAudit.filterRecords.searchPlaceholder')"
          @keyup.enter="reloadFromFirstPage"
        />
        <input v-model="filters.start_at" type="datetime-local" class="input" :title="t('admin.promptAudit.events.startAt')" @change="reloadFromFirstPage" />
        <input v-model="filters.end_at" type="datetime-local" class="input" :title="t('admin.promptAudit.events.endAt')" @change="reloadFromFirstPage" />
      </div>
      <p v-if="errorMessage" class="flex items-center gap-2 text-sm text-red-600 dark:text-red-300">
        <Icon name="exclamationCircle" size="sm" />
        {{ errorMessage }}
      </p>
    </div>

    <div class="overflow-x-auto">
      <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
        <thead class="bg-gray-50 dark:bg-dark-800">
          <tr>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.time') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.identity') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.group') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.route') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.result') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.categories') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.latency') }}</th>
            <th class="px-5 py-3 text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.preview') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-800 dark:bg-dark-800">
          <tr v-if="loading">
            <td colspan="8" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-300">
              {{ t('common.loading') }}
            </td>
          </tr>
          <tr v-else-if="records.length === 0">
            <td colspan="8" class="px-5 py-12 text-center text-sm text-gray-500 dark:text-dark-300">
              {{ t('admin.promptAudit.filterRecords.empty') }}
            </td>
          </tr>
          <template v-else>
            <tr v-for="record in records" :key="record.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
              <td class="whitespace-nowrap px-5 py-4 text-sm tabular-nums text-gray-700 dark:text-dark-200">
                {{ formatDateTime(record.created_at) }}
              </td>
              <td class="max-w-64 px-5 py-4 text-sm text-gray-700 dark:text-dark-200">
                <p class="truncate">{{ record.user_email || '-' }}</p>
                <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-dark-400">
                  {{ record.api_key_name || '-' }}<span v-if="record.user_id"> · UID {{ record.user_id }}</span>
                </p>
              </td>
              <td class="max-w-48 px-5 py-4 text-sm text-gray-700 dark:text-dark-200">
                <p class="truncate">{{ record.group_name || '-' }}</p>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">#{{ record.group_id || '-' }}</p>
              </td>
              <td class="max-w-64 px-5 py-4 text-sm text-gray-700 dark:text-dark-200">
                <p class="truncate">{{ record.endpoint || '-' }}</p>
                <p class="mt-0.5 truncate font-mono text-xs text-gray-500 dark:text-dark-400">
                  {{ record.model || '-' }} · {{ record.protocol || '-' }}
                </p>
              </td>
              <td class="whitespace-nowrap px-5 py-4">
                <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="resultClass(record.result)">
                  {{ resultLabel(record.result) }}
                </span>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ riskLabel(record.risk_level) }}</p>
              </td>
              <td class="max-w-56 px-5 py-4 text-sm text-gray-700 dark:text-dark-200">
                <div v-if="record.categories.length" class="flex flex-wrap gap-1">
                  <span
                    v-for="category in record.categories"
                    :key="category"
                    class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-dark-300"
                  >
                    {{ scannerLabel(category) }}
                  </span>
                </div>
                <span v-else>-</span>
              </td>
              <td class="whitespace-nowrap px-5 py-4 text-sm tabular-nums text-gray-700 dark:text-dark-200">
                {{ record.latency_ms }} ms
              </td>
              <td class="w-[300px] max-w-sm px-5 py-4 text-sm text-gray-700 dark:text-dark-200">
                <button
                  type="button"
                  class="group flex w-full min-w-0 items-center gap-2 rounded-lg px-2 py-1.5 text-left hover:bg-gray-100 dark:hover:bg-dark-700"
                  :title="record.redacted_preview"
                  @click="selectedRecord = record"
                >
                  <span class="min-w-0 flex-1 truncate">{{ record.redacted_preview || '-' }}</span>
                  <Icon name="eye" size="xs" class="shrink-0 text-gray-400" />
                </button>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <Pagination
      v-if="pagination.total > 0"
      :page="pagination.page"
      :total="pagination.total"
      :page-size="pagination.page_size"
      @update:page="changePage"
      @update:pageSize="changePageSize"
    />
  </section>

  <BaseDialog
    :show="Boolean(selectedRecord)"
    :title="t('admin.promptAudit.filterRecords.detailTitle')"
    width="wide"
    @close="selectedRecord = null"
  >
    <div v-if="selectedRecord" class="space-y-4">
      <div class="grid gap-3 text-sm sm:grid-cols-3">
        <div>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.result') }}</p>
          <p class="mt-1 font-medium text-gray-900 dark:text-white">{{ resultLabel(selectedRecord.result) }}</p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.model') }}</p>
          <p class="mt-1 font-mono text-gray-900 dark:text-white">{{ selectedRecord.model || '-' }}</p>
        </div>
        <div>
          <p class="text-xs text-gray-500 dark:text-dark-400">Request ID</p>
          <p class="mt-1 truncate font-mono text-gray-900 dark:text-white">{{ selectedRecord.request_id || '-' }}</p>
        </div>
      </div>
      <div>
        <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.promptAudit.filterRecords.redactedPreview') }}</p>
        <pre class="mt-2 max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-50 p-4 font-mono text-sm text-gray-800 dark:bg-dark-900 dark:text-dark-100">{{ selectedRecord.redacted_preview || '-' }}</pre>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import { promptAuditAPI } from './api'
import type { PromptEventFilters, PromptFilterRecord } from './types'
import { emptyEventFilters } from './viewModel'
import type { SelectOption } from '@/types'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()

const loading = ref(false)
const errorMessage = ref('')
const records = ref<PromptFilterRecord[]>([])
const selectedRecord = ref<PromptFilterRecord | null>(null)
const groups = ref<Array<{ id: number; name: string }>>([])
const filters = reactive<PromptEventFilters>(emptyEventFilters())
const pagination = reactive({ page: 1, page_size: 20, total: 0, pages: 1 })

const resultOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.promptAudit.filterRecords.results.all') },
  { value: 'pass', label: t('admin.promptAudit.filterRecords.results.pass') },
  { value: 'flag', label: t('admin.promptAudit.filterRecords.results.flag') },
  { value: 'critical', label: t('admin.promptAudit.filterRecords.results.critical') },
  { value: 'pending', label: t('admin.promptAudit.filterRecords.results.pending') },
  { value: 'error', label: t('admin.promptAudit.filterRecords.results.error') },
])

const groupOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('admin.promptAudit.filterRecords.allGroups') },
  ...groups.value.map(group => ({ value: String(group.id), label: group.name })),
])

const endpointOptions: SelectOption[] = [
  { value: '', label: t('admin.riskControl.filters.allEndpoints') },
  { value: '/v1/messages', label: '/v1/messages' },
  { value: '/v1/responses', label: '/v1/responses' },
  { value: '/v1/chat/completions', label: '/v1/chat/completions' },
  { value: '/v1beta/models', label: '/v1beta/models' },
  { value: '/v1/images/generations', label: '/v1/images/generations' },
  { value: '/v1/images/edits', label: '/v1/images/edits' },
]

function resultLabel(result: string): string {
  return t(`admin.promptAudit.filterRecords.results.${result || 'pending'}`)
}

function resultClass(result: string): string {
  if (result === 'pass') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (result === 'critical') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (result === 'flag') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  if (result === 'error') return 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
}

function riskLabel(risk: string): string {
  return t(`admin.promptAudit.riskLevels.${risk || 'low'}`)
}

function scannerLabel(category: string): string {
  return t(`admin.promptAudit.scanners.${category}`)
}

function reloadFromFirstPage() {
  pagination.page = 1
  void loadRecords()
}

function changePage(page: number) {
  pagination.page = page
  void loadRecords()
}

function changePageSize(pageSize: number) {
  pagination.page = 1
  pagination.page_size = pageSize
  void loadRecords()
}

async function loadGroups() {
  try {
    groups.value = await promptAuditAPI.listGroups()
  } catch {
    groups.value = []
  }
}

async function loadRecords() {
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await promptAuditAPI.listFilterRecords(filters, pagination.page, pagination.page_size)
    records.value = result.items
    pagination.total = result.total
    pagination.page = result.page
    pagination.page_size = result.page_size
    pagination.pages = result.pages
  } catch (error: unknown) {
    errorMessage.value = extractApiErrorMessage(error, t('admin.promptAudit.filterRecords.loadFailed'))
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadGroups(), loadRecords()])
})
</script>
