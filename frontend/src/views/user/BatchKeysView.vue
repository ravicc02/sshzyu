<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <div class="flex flex-wrap items-center justify-end gap-2">
        <router-link to="/keys" class="btn btn-secondary">
          <Icon name="arrowLeft" size="sm" class="mr-1.5" />
          {{ t('batchKeys.backToKeys') }}
        </router-link>
        <button type="button" class="btn btn-primary" @click="openCreate">
          <Icon name="plus" size="sm" class="mr-1.5" />
          {{ t('batchKeys.createBatch') }}
        </button>
      </div>

      <section class="space-y-3">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-base font-bold text-gray-900 dark:text-white">{{ t('batchKeys.batches') }}</h2>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingBatches" @click="loadBatches()">
            <Icon name="refresh" size="sm" :class="loadingBatches ? 'animate-spin' : ''" />
          </button>
        </div>

        <div v-if="loadingBatches && batches.length === 0" class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <div v-for="n in 3" :key="n" class="h-40 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800" />
        </div>
        <div v-else-if="batches.length === 0" class="rounded-xl border border-dashed border-gray-300 bg-white py-14 text-center dark:border-dark-600 dark:bg-dark-800">
          <p class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('batchKeys.emptyBatches') }}</p>
          <button type="button" class="btn btn-primary btn-sm mt-4" @click="openCreate">{{ t('batchKeys.createBatch') }}</button>
        </div>
        <div v-else class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <button
            v-for="batch in batches"
            :key="batch.batch_id"
            type="button"
            class="min-w-0 rounded-xl border bg-white p-4 text-left shadow-sm transition hover:border-primary-400 hover:shadow-md dark:bg-dark-800"
            :class="batch.batch_id === selectedBatchId ? 'border-primary-500 ring-2 ring-primary-500/15' : 'border-gray-200 dark:border-dark-700'"
            @click="selectBatch(batch.batch_id)"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <h3 class="truncate font-bold text-gray-900 dark:text-white">{{ batch.batch_name || t('batchKeys.unnamedBatch') }}</h3>
                <p class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                  {{ batch.group_name || t('batchKeys.noGroup') }} · {{ formatDateTime(batch.created_at) }}
                </p>
              </div>
              <span class="shrink-0 rounded-full bg-gray-100 px-2 py-1 text-xs font-semibold text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                {{ t('batchKeys.keyCount', { count: batch.key_count }) }}
              </span>
            </div>
            <div class="mt-5 grid grid-cols-3 gap-3 text-xs">
              <div>
                <p class="text-gray-500 dark:text-gray-400">{{ t('batchKeys.active') }}</p>
                <p class="mt-1 font-bold tabular-nums text-emerald-600 dark:text-emerald-300">{{ batch.active_key_count }}</p>
              </div>
              <div>
                <p class="text-gray-500 dark:text-gray-400">{{ t('batchKeys.quotaPerKey') }}</p>
                <p class="mt-1 font-bold tabular-nums text-gray-900 dark:text-white">{{ formatMoney(batch.quota_per_key) }}</p>
              </div>
              <div>
                <p class="text-gray-500 dark:text-gray-400">{{ t('batchKeys.quotaUsed') }}</p>
                <p class="mt-1 font-bold tabular-nums text-gray-900 dark:text-white">{{ formatMoney(batch.quota_used_total) }}</p>
              </div>
            </div>
            <div class="mt-4 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
              <div class="h-full rounded-full bg-primary-500" :style="{ width: `${batchUsagePercent(batch)}%` }" />
            </div>
            <div v-if="batch.quota_tokens_total > 0" class="mt-3 flex items-center justify-between gap-2 text-xs">
              <span class="text-gray-500 dark:text-gray-400">{{ t('batchKeys.tokenQuotaUsed') }}</span>
              <strong class="tabular-nums text-gray-900 dark:text-white">
                {{ formatTokenQuota(batch.quota_tokens_used_total) }} / {{ formatTokenQuota(batch.quota_tokens_total) }}
              </strong>
            </div>
          </button>
        </div>
      </section>

      <section v-if="selectedBatch" class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 border-b border-gray-100 p-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
          <div class="min-w-0">
            <h2 class="truncate text-base font-bold text-gray-900 dark:text-white">{{ selectedBatch.batch_name }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ selectedBatch.group_name }} · {{ t('batchKeys.quotaSummary', {
                used: formatMoney(selectedBatch.quota_used_total),
                total: formatMoney(selectedBatch.quota_total),
              }) }}
              <template v-if="selectedBatch.quota_tokens_total > 0">
                · {{ t('batchKeys.tokenQuotaSummary', {
                  used: formatTokenQuota(selectedBatch.quota_tokens_used_total),
                  total: formatTokenQuota(selectedBatch.quota_tokens_total),
                }) }}
              </template>
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="!selectedIds.size" @click="copySelected">
              <Icon name="clipboard" size="sm" class="mr-1" />
              {{ t('batchKeys.copySelected') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="!batchKeys.length" @click="exportKeys(batchKeys, 'all')">
              <Icon name="download" size="sm" class="mr-1" />
              {{ t('batchKeys.exportAll') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="!selectedIds.size" @click="runBulk('active')">
              {{ t('batchKeys.enableSelected') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="!selectedIds.size" @click="runBulk('inactive')">
              {{ t('batchKeys.disableSelected') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="!selectedIds.size" @click="runBulk('reset_quota')">
              {{ t('batchKeys.resetQuota') }}
            </button>
            <button type="button" class="btn btn-danger btn-sm" :disabled="!selectedIds.size" @click="promptDelete">
              {{ t('batchKeys.deleteSelected') }}
            </button>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-left text-xs font-semibold uppercase tracking-wide text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
              <tr>
                <th class="w-12 px-4 py-3">
                  <input type="checkbox" :checked="allSelected" :aria-label="t('batchKeys.selectAll')" @change="toggleAll" />
                </th>
                <th class="px-4 py-3">{{ t('common.name') }}</th>
                <th class="px-4 py-3">{{ t('keys.apiKey') }}</th>
                <th class="px-4 py-3">{{ t('batchKeys.quotaUsage') }}</th>
                <th class="px-4 py-3">{{ t('common.status') }}</th>
                <th class="px-4 py-3">{{ t('keys.lastUsedAt') }}</th>
                <th class="px-4 py-3 text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-if="loadingKeys">
                <td colspan="7" class="px-4 py-12 text-center text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</td>
              </tr>
              <tr v-else-if="batchKeys.length === 0">
                <td colspan="7" class="px-4 py-12 text-center text-gray-500 dark:text-gray-400">{{ t('batchKeys.emptyKeys') }}</td>
              </tr>
              <template v-else>
                <tr v-for="key in batchKeys" :key="key.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
                  <td class="px-4 py-3">
                    <input type="checkbox" :checked="selectedIds.has(key.id)" :aria-label="t('batchKeys.selectKey', { name: key.name })" @change="toggleKey(key.id)" />
                  </td>
                  <td class="max-w-[240px] px-4 py-3">
                    <p class="truncate font-medium text-gray-900 dark:text-white">{{ key.name }}</p>
                    <p class="mt-0.5 text-xs text-gray-400">#{{ key.id }}</p>
                  </td>
                  <td class="px-4 py-3">
                    <code class="rounded bg-gray-100 px-2 py-1 font-mono text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ maskApiKey(key.key) }}</code>
                  </td>
                  <td class="min-w-[210px] px-4 py-3">
                    <div class="flex items-center justify-between gap-2 text-xs">
                      <span class="font-medium tabular-nums text-gray-900 dark:text-white">{{ formatMoney(key.quota_used) }} / {{ formatMoney(key.quota) }}</span>
                      <span class="tabular-nums text-gray-400">{{ quotaPercent(key) }}%</span>
                    </div>
                    <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                      <div class="h-full rounded-full" :class="quotaPercent(key) >= 100 ? 'bg-rose-500' : quotaPercent(key) >= 80 ? 'bg-amber-400' : 'bg-emerald-500'" :style="{ width: `${quotaPercent(key)}%` }" />
                    </div>
                    <div v-if="key.quota_tokens > 0" class="mt-2.5">
                      <div class="flex items-center justify-between gap-2 text-xs">
                        <span class="font-medium tabular-nums text-gray-900 dark:text-white">
                          {{ formatTokenQuota(key.quota_tokens_used) }} / {{ formatTokenQuota(key.quota_tokens) }} Token
                        </span>
                        <span class="tabular-nums text-gray-400">{{ tokenQuotaPercent(key) }}%</span>
                      </div>
                      <div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                        <div class="h-full rounded-full" :class="tokenQuotaPercent(key) >= 100 ? 'bg-rose-500' : tokenQuotaPercent(key) >= 80 ? 'bg-amber-400' : 'bg-sky-500'" :style="{ width: `${tokenQuotaPercent(key)}%` }" />
                      </div>
                    </div>
                  </td>
                  <td class="px-4 py-3">
                    <span class="badge" :class="key.status === 'active' ? 'badge-success' : key.status === 'quota_exhausted' ? 'badge-warning' : 'badge-gray'">
                      {{ t(`keys.status.${key.status}`) }}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">{{ key.last_used_at ? formatDateTime(key.last_used_at) : '—' }}</td>
                  <td class="px-4 py-3 text-right">
                    <button type="button" class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" :title="t('batchKeys.copyKey')" @click="copyKeys([key])">
                      <Icon name="clipboard" size="sm" />
                    </button>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </section>
    </div>

    <BaseDialog :show="showCreateDialog" :title="t('batchKeys.createBatch')" width="normal" @close="showCreateDialog = false">
      <form id="batch-key-create-form" class="space-y-5" @submit.prevent="submitCreate">
        <div>
          <label class="input-label">{{ t('batchKeys.batchName') }}</label>
          <input v-model="createForm.name" class="input" maxlength="100" :placeholder="t('batchKeys.batchNamePlaceholder')" />
        </div>
        <div>
          <label class="input-label">{{ t('batchKeys.quotaMode') }} <span class="text-red-500">*</span></label>
          <div
            role="radiogroup"
            :aria-label="t('batchKeys.quotaMode')"
            class="grid grid-cols-2 gap-1 rounded-xl border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-900/60"
          >
            <button
              type="button"
              role="radio"
              data-quota-mode="balance"
              :aria-checked="createForm.quota_mode === 'balance'"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors"
              :class="createForm.quota_mode === 'balance' ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-300' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
              @click="selectQuotaMode('balance')"
            >
              {{ t('batchKeys.quotaModeBalance') }}
            </button>
            <button
              type="button"
              role="radio"
              data-quota-mode="token"
              :aria-checked="createForm.quota_mode === 'token'"
              class="rounded-lg px-3 py-2 text-sm font-medium transition-colors"
              :class="createForm.quota_mode === 'token' ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-300' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
              @click="selectQuotaMode('token')"
            >
              {{ t('batchKeys.quotaModeToken') }}
            </button>
          </div>
          <p class="input-hint">{{ t('batchKeys.quotaModeHint') }}</p>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('batchKeys.count') }} <span class="text-red-500">*</span></label>
            <input v-model.number="createForm.count" type="number" min="1" max="50" required class="input" />
            <p class="input-hint">{{ t('batchKeys.countHint') }}</p>
          </div>
          <div v-if="createForm.quota_mode === 'balance'" data-quota-mode="balance">
            <label class="input-label">{{ t('batchKeys.balanceQuota') }}</label>
            <div class="relative">
              <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
              <input v-model.number="createForm.quota" type="number" min="0" step="0.01" class="input pl-7" />
            </div>
            <p class="input-hint">{{ t('batchKeys.balanceQuotaHint') }}</p>
          </div>
        </div>
        <div v-if="createForm.quota_mode === 'token'" data-quota-mode="token">
          <label class="input-label">{{ t('batchKeys.tokenQuotaPerKey') }}</label>
          <div class="flex gap-2">
            <input v-model.number="createForm.quota_token_amount" type="number" min="0" step="0.01" class="input" placeholder="0" />
            <select v-model="createForm.quota_token_unit" data-quota-unit class="input w-24">
              <option value="M">M</option>
              <option value="B">B</option>
            </select>
          </div>
          <p class="input-hint">{{ t('batchKeys.tokenQuotaHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('keys.groupLabel') }} <span class="text-red-500">*</span></label>
          <Select v-model="createForm.group_id" data-group-select :options="groupOptions" :placeholder="t('keys.selectGroup')" searchable />
        </div>
        <div class="rounded-xl border border-primary-100 bg-primary-50/70 p-4 dark:border-primary-500/20 dark:bg-primary-500/10">
          <div v-if="createForm.quota_mode === 'balance'" class="flex items-center justify-between gap-3 text-sm">
            <span class="text-gray-600 dark:text-gray-300">{{ t('batchKeys.estimatedTotal') }}</span>
            <strong class="tabular-nums text-primary-700 dark:text-primary-300">{{ formatMoney(estimatedTotal) }}</strong>
          </div>
          <div v-else class="flex items-center justify-between gap-3 text-sm">
            <span class="text-gray-600 dark:text-gray-300">{{ t('batchKeys.estimatedTokenTotal') }}</span>
            <strong class="tabular-nums text-primary-700 dark:text-primary-300">{{ formatTokenQuota(estimatedTokenTotal) }}</strong>
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="showCreateDialog = false">{{ t('common.cancel') }}</button>
          <button type="submit" form="batch-key-create-form" class="btn btn-primary" :disabled="creating">
            {{ creating ? t('common.submitting') : t('batchKeys.createNow') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('batchKeys.deleteSelected')"
      :message="t('batchKeys.deleteConfirm', { count: selectedIds.size })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import { batchKeysAPI, keysAPI, userGroupsAPI } from '@/api'
import type { APIKeyBatchSummary, CreateAPIKeyBatchRequest } from '@/api/batchKeys'
import type { ApiKey, Group, UpdateApiKeyRequest } from '@/types'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { maskApiKey } from '@/utils/maskApiKey'
import { formatTokenQuota, tokenAmountToBase, type TokenQuotaUnit } from '@/utils/tokenQuota'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const batches = ref<APIKeyBatchSummary[]>([])
const groups = ref<Group[]>([])
const batchKeys = ref<ApiKey[]>([])
const selectedBatchId = ref('')
const selectedIds = ref(new Set<number>())
const loadingBatches = ref(false)
const loadingKeys = ref(false)
const creating = ref(false)
const showCreateDialog = ref(false)
const showDeleteDialog = ref(false)
let batchesController: AbortController | null = null
let keysController: AbortController | null = null

const createForm = reactive({
  name: '',
  count: 10,
  quota_mode: 'balance' as 'balance' | 'token',
  quota: 1,
  quota_token_amount: 0,
  quota_token_unit: 'M' as TokenQuotaUnit,
  group_id: null as number | null,
})

const selectedBatch = computed(() => batches.value.find(batch => batch.batch_id === selectedBatchId.value) || null)
const groupOptions = computed(() => groups.value.map(group => ({ value: group.id, label: group.name })))
const estimatedTotal = computed(() => createForm.quota_mode === 'balance'
  ? Math.max(0, Number(createForm.count) || 0) * Math.max(0, Number(createForm.quota) || 0)
  : 0)
const quotaTokens = computed(() => tokenAmountToBase(createForm.quota_token_amount, createForm.quota_token_unit))
const estimatedTokenTotal = computed(() => createForm.quota_mode === 'token'
  ? Math.max(0, Number(createForm.count) || 0) * quotaTokens.value
  : 0)
const allSelected = computed(() => batchKeys.value.length > 0 && batchKeys.value.every(key => selectedIds.value.has(key.id)))

function setSelectedIds(ids: number[]) {
  selectedIds.value = new Set(ids)
}

async function loadGroups() {
  try {
    groups.value = await userGroupsAPI.getAvailable()
  } catch (error) {
    console.error('load batch key groups failed', error)
  }
}

async function loadBatches(signal?: AbortSignal) {
  batchesController?.abort()
  const controller = signal ? null : new AbortController()
  batchesController = controller
  loadingBatches.value = true
  try {
    const response = await batchKeysAPI.listBatches({ signal: signal || controller?.signal })
    batches.value = response.items || []
    if (!selectedBatchId.value && batches.value[0]) {
      selectedBatchId.value = batches.value[0].batch_id
    } else if (selectedBatchId.value && !batches.value.some(batch => batch.batch_id === selectedBatchId.value)) {
      selectedBatchId.value = batches.value[0]?.batch_id || ''
    }
  } catch (error) {
    if ((error as { name?: string }).name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('batchKeys.loadFailed')))
    }
  } finally {
    loadingBatches.value = false
  }
}

async function loadBatchKeys(batchId: string) {
  if (!batchId) {
    batchKeys.value = []
    setSelectedIds([])
    return
  }
  keysController?.abort()
  const controller = new AbortController()
  keysController = controller
  loadingKeys.value = true
  try {
    const response = await batchKeysAPI.getBatch(batchId, { signal: controller.signal })
    batchKeys.value = response.items || []
    setSelectedIds([])
  } catch (error) {
    if ((error as { name?: string }).name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('batchKeys.loadFailed')))
    }
  } finally {
    if (keysController === controller) loadingKeys.value = false
  }
}

function selectBatch(batchId: string) {
  selectedBatchId.value = batchId
}

function openCreate() {
  createForm.name = ''
  createForm.count = 10
  createForm.quota_mode = 'balance'
  createForm.quota = 1
  createForm.quota_token_amount = 0
  createForm.quota_token_unit = 'M'
  createForm.group_id = groups.value[0]?.id ?? null
  showCreateDialog.value = true
}

function selectQuotaMode(mode: 'balance' | 'token') {
  createForm.quota_mode = mode
  if (mode === 'balance') {
    createForm.quota_token_amount = 0
  } else {
    createForm.quota = 0
  }
}

async function submitCreate() {
  if (!createForm.group_id) {
    appStore.showError(t('keys.groupRequired'))
    return
  }
  const selectedQuota = createForm.quota_mode === 'balance'
    ? Number(createForm.quota) || 0
    : quotaTokens.value
  if (createForm.count < 1 || createForm.count > 50 || selectedQuota <= 0) {
    appStore.showError(t('batchKeys.invalidCreate'))
    return
  }
  creating.value = true
  try {
    const payload: CreateAPIKeyBatchRequest = {
      name: createForm.name.trim() || undefined,
      group_id: createForm.group_id,
      count: Number(createForm.count),
      quota: createForm.quota_mode === 'balance' ? Math.max(0, Number(createForm.quota) || 0) : 0,
      quota_tokens: createForm.quota_mode === 'token' ? quotaTokens.value : 0,
    }
    const response = await batchKeysAPI.createBatch(payload)
    const createdBatchID = response.items[0]?.batch_id || ''
    showCreateDialog.value = false
    await loadBatches()
    if (createdBatchID) selectedBatchId.value = createdBatchID
    appStore.showSuccess(t('batchKeys.createSuccess', { count: response.items.length }))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('batchKeys.createFailed')))
  } finally {
    creating.value = false
  }
}

function toggleKey(id: number) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = next
}

function toggleAll() {
  setSelectedIds(allSelected.value ? [] : batchKeys.value.map(key => key.id))
}

async function copyKeys(keys: ApiKey[]) {
  if (!keys.length) return
  await copyToClipboard(keys.map(key => key.key).join('\n'), t('batchKeys.copied', { count: keys.length }))
}

function copySelected() {
  void copyKeys(batchKeys.value.filter(key => selectedIds.value.has(key.id)))
}

async function runBulk(action: 'active' | 'inactive' | 'reset_quota') {
  const ids = [...selectedIds.value]
  if (!ids.length) return
  const updates: UpdateApiKeyRequest = action === 'reset_quota' ? { reset_quota: true } : { status: action }
  try {
    const result = await keysAPI.bulkUpdate(ids, updates)
    if (result.failures.length) {
      appStore.showError(t('batchKeys.bulkPartial', { count: result.failures.length }))
    } else {
      appStore.showSuccess(t('batchKeys.bulkSuccess', { count: result.succeededIds.length }))
    }
    await Promise.all([loadBatches(), loadBatchKeys(selectedBatchId.value)])
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('batchKeys.bulkFailed')))
  }
}

function promptDelete() {
  if (selectedIds.value.size) showDeleteDialog.value = true
}

async function confirmDelete() {
  const ids = [...selectedIds.value]
  showDeleteDialog.value = false
  if (!ids.length) return
  const results = await Promise.allSettled(ids.map(id => keysAPI.delete(id)))
  const failed = results.filter(result => result.status === 'rejected').length
  if (failed) appStore.showError(t('batchKeys.deletePartial', { count: failed }))
  else appStore.showSuccess(t('batchKeys.deleteSuccess', { count: ids.length }))
  await Promise.all([loadBatches(), loadBatchKeys(selectedBatchId.value)])
}

function exportKeys(keys: ApiKey[], mode: 'all' | 'selected') {
  if (!keys.length) return
  const content = keys.map(key => key.key).join('\n') + '\n'
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `${safeFilename(selectedBatch.value?.batch_name || 'batch-keys')}-${mode}.txt`
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

function safeFilename(value: string) {
  return value.replace(/[\\/:*?"<>|]+/g, '-').trim() || 'batch-keys'
}

function quotaPercent(key: ApiKey) {
  if (!key.quota || key.quota <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((key.quota_used / key.quota) * 100)))
}

function batchUsagePercent(batch: APIKeyBatchSummary) {
  if (batch.quota_total > 0) {
    return Math.max(0, Math.min(100, Math.round((batch.quota_used_total / batch.quota_total) * 100)))
  }
  if (batch.quota_tokens_total > 0) {
    return Math.max(0, Math.min(100, Math.round((batch.quota_tokens_used_total / batch.quota_tokens_total) * 100)))
  }
  return 0
}

function formatMoney(value: number) {
  return `$${Number(value || 0).toFixed(2)}`
}

function tokenQuotaPercent(key: ApiKey) {
  if (!key.quota_tokens || key.quota_tokens <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((key.quota_tokens_used / key.quota_tokens) * 100)))
}

watch(selectedBatchId, batchId => {
  void loadBatchKeys(batchId)
})

onMounted(() => {
  void Promise.all([loadGroups(), loadBatches()])
})
</script>
