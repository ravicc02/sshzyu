<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <!-- Tab 切换 -->
          <div class="flex rounded-lg border border-gray-200 p-0.5 dark:border-dark-600">
            <button
              v-for="tab in tabs"
              :key="tab.key"
              @click="switchTab(tab.key)"
              :class="[
                'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                activeTab === tab.key
                  ? 'bg-blue-600 text-white'
                  : 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
              ]"
            >
              {{ t(tab.label) }}
            </button>
          </div>

          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <!-- 中奖记录 tab 的筛选 -->
            <template v-if="activeTab === 'draws'">
              <input
                v-model="drawUserIdInput"
                type="number"
                min="1"
                :placeholder="t('admin.lottery.filterUserId')"
                class="input w-36"
                @keydown.enter="handleApplyUserFilter"
              />
              <Select
                v-model="sortOrder"
                :options="sortOptions"
                class="w-32"
                @change="loadDraws(1)"
              />
            </template>
            <!-- 参数配置 tab 的活动选择 -->
            <template v-else>
              <Select
                v-model="activeActivityId"
                :options="activityOptions"
                class="w-56"
                @change="loadPrizes"
              />
            </template>
            <button @click="refresh" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <!-- ====================== 中奖记录 ====================== -->
      <template v-if="activeTab === 'draws'" #table>
        <DataTable
          :columns="drawColumns"
          :data="draws"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleDrawSort"
        >
          <template #cell-id="{ value }">
            <span class="text-xs text-gray-400">#{{ value }}</span>
          </template>

          <template #cell-user="{ row }">
            <div v-if="row.user" class="min-w-0">
              <div class="flex items-center gap-1.5 text-sm font-medium text-gray-900 dark:text-white">
                <span class="truncate">{{ row.user.username || row.user.email }}</span>
                <span v-if="row.user.deleted" class="badge badge-gray">{{ t('admin.lottery.userDeleted') }}</span>
              </div>
              <div class="truncate text-xs text-gray-500 dark:text-dark-400">{{ row.user.email }}</div>
            </div>
            <span v-else class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.lottery.userIdLabel') }} #{{ row.user_id }}</span>
          </template>

          <template #cell-prize="{ row }">
            <div v-if="row.prize_type !== 'none'" class="text-sm">
              <div class="font-medium text-gray-900 dark:text-white">{{ row.prize_name }}</div>
              <div class="text-xs text-gray-500 dark:text-dark-400">${{ row.prize_value.toFixed(2) }}</div>
            </div>
            <span v-else class="badge badge-gray">{{ row.prize_name || t('admin.lottery.prizeNone') }}</span>
          </template>

          <template #cell-source="{ value }">
            <span class="badge" :class="value === 'first' ? 'badge-purple' : 'badge-gray'">
              {{ t(`admin.lottery.sourceLabels.${value}`, value) }}
            </span>
          </template>

          <template #cell-fulfillment="{ row }">
            <span
              :class="[
                'badge',
                row.fulfillment_status === 'granted'
                  ? 'badge-success'
                  : row.fulfillment_status === 'pending'
                    ? 'badge-warning'
                    : 'badge-danger'
              ]"
            >
              {{ t(`admin.lottery.fulfillmentLabels.${row.fulfillment_status}`) }}
            </span>
            <p v-if="row.fulfillment_error" class="mt-1 max-w-48 truncate text-xs text-red-500" :title="row.fulfillment_error">
              {{ row.fulfillment_error }}
            </p>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <button
              v-if="row.fulfillment_status !== 'granted'"
              @click="handleRetry(row)"
              :disabled="retryingId === row.id"
              class="btn btn-secondary btn-sm"
            >
              <Icon name="refresh" size="sm" :class="retryingId === row.id ? 'animate-spin' : ''" />
              {{ t('admin.lottery.retryFulfillment') }}
            </button>
          </template>

          <template #empty>
            <EmptyState :title="t('empty.noData')" :description="t('admin.lottery.noDraws')" />
          </template>
        </DataTable>
      </template>

      <!-- ====================== 参数配置 ====================== -->
      <template v-else #table>
        <div class="space-y-6">
          <!-- 活动配置 -->
          <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600 dark:bg-dark-700/50">
            <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.lottery.activitySection') }}</h3>
            <div v-if="activityForm" class="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.lottery.activityName') }}</label>
                <input v-model="activityForm.name" type="text" class="input" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.lottery.activityStatus') }}</label>
                <Select v-model="activityForm.status" :options="activityStatusOptions" />
                <p class="input-hint">{{ t('admin.lottery.activityStatusHint') }}</p>
              </div>
              <div>
                <label class="input-label">{{ t('admin.lottery.startsAt') }}</label>
                <input
                  v-model="activityForm.starts_at_input"
                  type="datetime-local"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{ t('admin.lottery.endsAt') }}</label>
                <input
                  v-model="activityForm.ends_at_input"
                  type="datetime-local"
                  class="input"
                />
              </div>
              <div class="md:col-span-2">
                <button @click="handleSaveActivity" :disabled="savingActivity" class="btn btn-primary">
                  {{ t('common.save') }}
                </button>
                <span v-if="activity?.rules_version" class="ml-3 text-xs text-gray-400">
                  {{ t('admin.lottery.rulesVersion') }}: v{{ activity?.rules_version }}
                </span>
              </div>
            </div>
            <p v-else class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.lottery.noActivity') }}</p>
          </div>

          <!-- 奖品配置 -->
          <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600 dark:bg-dark-700/50">
            <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.lottery.prizeSection') }}</h3>
            <DataTable :columns="prizeColumns" :data="prizes" :loading="loading">
              <template #cell-name="{ row }">
                <div class="text-sm">
                  <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
                  <span v-if="!row.enabled" class="badge badge-gray ml-2">{{ t('admin.lottery.prizeDisabled') }}</span>
                </div>
              </template>
              <template #cell-prize_type="{ value }">
                <span class="badge" :class="value === 'balance_bonus' ? 'badge-success' : value === 'quota' ? 'badge-info' : 'badge-gray'">
                  {{ t(`admin.lottery.prizeTypeLabels.${value}`) }}
                </span>
              </template>
              <template #cell-value="{ value }">
                <span class="text-sm">${{ value.toFixed(2) }}</span>
              </template>
              <template #cell-weight="{ row }">
                <span class="text-sm">{{ row.weight }}</span>
                <div v-if="Object.keys(row.tier_weights || {}).length" class="text-xs text-gray-400">
                  {{ t('admin.lottery.tierOverrideHint') }}
                </div>
              </template>
              <template #cell-min_tier="{ value }">
                <span class="badge badge-gray">{{ t(`admin.lottery.tierNames.${value}`) }}</span>
              </template>
              <template #cell-stock="{ row }">
                <div class="text-sm">
                  {{ row.stock === -1 ? t('admin.lottery.stockUnlimited') : `${row.stock_issued}/${row.stock}` }}
                </div>
              </template>
              <template #cell-actions="{ row }">
                <button @click="openPrizeEdit(row)" class="btn btn-secondary btn-sm">
                  <Icon name="edit" size="sm" />
                  {{ t('common.edit') }}
                </button>
              </template>
              <template #empty>
                <EmptyState :title="t('empty.noData')" :description="t('admin.lottery.noPrizes')" />
              </template>
            </DataTable>
          </div>

          <!-- 次数调整 -->
          <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600 dark:bg-dark-700/50">
            <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.lottery.adjustSection') }}</h3>
            <div class="flex flex-wrap items-end gap-3">
              <div>
                <label class="input-label">{{ t('admin.lottery.filterUserId') }}</label>
                <input v-model="adjustUserId" type="number" min="1" class="input w-32" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.lottery.adjustDelta') }}</label>
                <input v-model="adjustDelta" type="number" class="input w-28" />
              </div>
              <button @click="handleAdjust" :disabled="adjusting" class="btn btn-primary">
                {{ t('admin.lottery.adjustApply') }}
              </button>
              <p class="input-hint">{{ t('admin.lottery.adjustHint') }}</p>
            </div>
          </div>
        </div>
      </template>

      <template #pagination>
        <Pagination
          v-if="activeTab === 'draws' && drawPagination.total > 0"
          :page="drawPagination.page"
          :total="drawPagination.total"
          :page-size="drawPagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- 奖品编辑弹窗 -->
    <BaseDialog
      :show="showPrizeDialog"
      :title="t('admin.lottery.editPrizeTitle')"
      width="wide"
      @close="closePrizeEdit"
    >
      <form v-if="prizeForm" @submit.prevent="handleSavePrize" class="space-y-4">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.lottery.prizeName') }}</label>
            <input v-model="prizeForm.name" type="text" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('admin.lottery.prizeType') }}</label>
            <Select v-model="prizeForm.prize_type" :options="prizeTypeOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.lottery.prizeValue') }}</label>
            <input v-model.number="prizeForm.value" type="number" step="0.01" min="0" class="input" />
            <p class="input-hint">{{ t('admin.lottery.prizeValueHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.lottery.prizeWeight') }}</label>
            <input v-model.number="prizeForm.weight" type="number" min="0" class="input" />
            <p class="input-hint">{{ t('admin.lottery.prizeWeightHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.lottery.prizeMinTier') }}</label>
            <Select v-model="prizeForm.min_tier" :options="tierOptions" />
            <p class="input-hint">{{ t('admin.lottery.prizeMinTierHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.lottery.prizeStock') }}</label>
            <input v-model.number="prizeForm.stock" type="number" min="-1" class="input" />
            <p class="input-hint">{{ t('admin.lottery.prizeStockHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.lottery.prizeSortOrder') }}</label>
            <input v-model.number="prizeForm.sort_order" type="number" class="input" />
          </div>
          <div class="flex items-end pb-1">
            <label class="flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
              <input v-model="prizeForm.enabled" type="checkbox" class="h-4 w-4" />
              {{ t('admin.lottery.prizeEnabled') }}
            </label>
          </div>
        </div>

        <div>
          <label class="input-label">{{ t('admin.lottery.tierWeightsSection') }}</label>
          <div class="grid grid-cols-2 gap-3 md:grid-cols-5">
            <div v-for="tier in 5" :key="tier - 1">
              <label class="mb-1 block text-xs text-gray-500 dark:text-dark-400">
                {{ t(`admin.lottery.tierNames.${tier - 1}`) }}
              </label>
              <input
                v-model="prizeForm.tier_weight_inputs[tier - 1]"
                type="number"
                min="0"
                :placeholder="t('admin.lottery.tierWeightFallback')"
                class="input"
              />
            </div>
          </div>
          <p class="input-hint">{{ t('admin.lottery.tierWeightsHint') }}</p>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button type="button" @click="closePrizeEdit" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="submit" :disabled="savingPrize" class="btn btn-primary">{{ t('common.save') }}</button>
        </div>
      </form>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import type { Column } from '@/components/common/types'
import {
  listDraws,
  getActivities,
  getPrizes,
  updateActivity,
  updatePrize,
  retryFulfillment,
  adjustDraws,
  type AdminLotteryDraw,
  type AdminLotteryActivity,
  type AdminLotteryPrize
} from '@/api/admin/lottery'

import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const activeTab = ref<'draws' | 'config'>('draws')

const tabs = computed(() => [
  { key: 'draws' as const, label: 'admin.lottery.tabDraws' },
  { key: 'config' as const, label: 'admin.lottery.tabConfig' }
])

function switchTab(tab: 'draws' | 'config') {
  activeTab.value = tab
}

async function refresh() {
  if (activeTab.value === 'draws') {
    await loadDraws()
  } else {
    await loadPrizes()
  }
}

// ==================== 中奖记录 ====================
const draws = ref<AdminLotteryDraw[]>([])
const drawUserIdInput = ref('')
const drawUserId = ref<number | undefined>(undefined)
const sortOrder = ref<'asc' | 'desc'>('desc')
const retryingId = ref<number | null>(null)

const drawPagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const sortOptions = computed(() => [
  { value: 'desc', label: t('admin.lottery.sortNewest') },
  { value: 'asc', label: t('admin.lottery.sortOldest') }
])

const drawColumns = computed<Column[]>(() => [
  { key: 'id', label: t('admin.lottery.columns.id') },
  { key: 'user', label: t('admin.lottery.columns.user') },
  { key: 'prize', label: t('admin.lottery.columns.prize') },
  { key: 'source', label: t('admin.lottery.columns.source') },
  { key: 'fulfillment', label: t('admin.lottery.columns.fulfillment') },
  { key: 'created_at', label: t('admin.lottery.columns.createdAt'), sortable: true },
  { key: 'actions', label: t('admin.lottery.columns.actions') }
])

async function loadDraws(page = drawPagination.page) {
  loading.value = true
  try {
    const data = await listDraws(page, drawPagination.page_size, drawUserId.value, sortOrder.value)
    draws.value = data.items
    drawPagination.page = data.page
    drawPagination.total = data.total
    drawPagination.pages = data.pages
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  } finally {
    loading.value = false
  }
}

function handleApplyUserFilter() {
  const raw = drawUserIdInput.value.trim()
  drawUserId.value = raw === '' ? undefined : Number(raw)
  loadDraws(1)
}

function handleDrawSort(_key: string, order: 'asc' | 'desc') {
  sortOrder.value = order
  loadDraws(1)
}

function handlePageChange(page: number) {
  loadDraws(page)
}

function handlePageSizeChange(pageSize: number) {
  drawPagination.page_size = pageSize
  loadDraws(1)
}

async function handleRetry(row: AdminLotteryDraw) {
  retryingId.value = row.id
  try {
    const { draw } = await retryFulfillment(row.id)
    const idx = draws.value.findIndex((d) => d.id === row.id)
    if (idx >= 0) draws.value[idx] = { ...draws.value[idx], ...draw }
    appStore.showSuccess(t('admin.lottery.retrySuccess'))
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  } finally {
    retryingId.value = null
  }
}

// ==================== 参数配置 ====================
const activities = ref<AdminLotteryActivity[]>([])
const activity = ref<AdminLotteryActivity | null>(null)
const activeActivityId = ref<number | ''>('')
const prizes = ref<AdminLotteryPrize[]>([])
const savingActivity = ref(false)
const savingPrize = ref(false)

const activityForm = ref<{
  name: string
  status: AdminLotteryActivity['status']
  starts_at_input: string
  ends_at_input: string
} | null>(null)

const activityOptions = computed(() =>
  activities.value.map((a) => ({ value: a.id, label: `${a.name} (#${a.id})` }))
)

const activityStatusOptions = computed(() =>
  (['draft', 'active', 'paused', 'ended'] as const).map((s) => ({
    value: s,
    label: t(`admin.lottery.activityStatusLabels.${s}`)
  }))
)

const prizeColumns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.lottery.prizeName') },
  { key: 'prize_type', label: t('admin.lottery.prizeType') },
  { key: 'value', label: t('admin.lottery.prizeValue') },
  { key: 'weight', label: t('admin.lottery.prizeWeight') },
  { key: 'min_tier', label: t('admin.lottery.prizeMinTier') },
  { key: 'stock', label: t('admin.lottery.prizeStock') },
  { key: 'sort_order', label: t('admin.lottery.prizeSortOrder') },
  { key: 'actions', label: t('admin.lottery.columns.actions') }
])

const prizeTypeOptions = computed(() =>
  (['none', 'balance_bonus', 'quota'] as const).map((v) => ({
    value: v,
    label: t(`admin.lottery.prizeTypeLabels.${v}`)
  }))
)

const tierOptions = computed(() =>
  [0, 1, 2, 3, 4].map((tier) => ({ value: tier, label: t(`admin.lottery.tierNames.${tier}`) }))
)

async function loadActivities(selectFirst = false) {
  try {
    const data = await getActivities()
    activities.value = data.activities
    if (selectFirst && data.activities.length > 0 && activeActivityId.value === '') {
      activeActivityId.value = data.activities[0].id
    }
    syncActivityForm()
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  }
}

function syncActivityForm() {
  const current = activities.value.find((a) => a.id === activeActivityId.value) || null
  activity.value = current
  activityForm.value = current
    ? {
        name: current.name,
        status: current.status,
        starts_at_input: toLocalInput(current.starts_at),
        ends_at_input: toLocalInput(current.ends_at)
      }
    : null
}

async function loadPrizes() {
  if (!activeActivityId.value) return
  loading.value = true
  try {
    const data = await getPrizes(Number(activeActivityId.value))
    prizes.value = data.prizes
    syncActivityForm()
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  } finally {
    loading.value = false
  }
}

/** ISO 时间字符串 -> datetime-local 输入值(秒) */
function toLocalInput(iso: string | null): string {
  if (!iso) return ''
  return formatDateTimeLocalInput(Math.floor(new Date(iso).getTime() / 1000))
}

async function handleSaveActivity() {
  if (!activity.value || !activityForm.value) return
  savingActivity.value = true
  try {
    const starts = parseDateTimeLocalInput(activityForm.value.starts_at_input)
    const ends = parseDateTimeLocalInput(activityForm.value.ends_at_input)
    await updateActivity(activity.value.id, {
      name: activityForm.value.name,
      status: activityForm.value.status,
      starts_at: starts !== null ? new Date(starts * 1000).toISOString() : null,
      ends_at: ends !== null ? new Date(ends * 1000).toISOString() : null
    })
    appStore.showSuccess(t('admin.lottery.saveSuccess'))
    await loadActivities()
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  } finally {
    savingActivity.value = false
  }
}

// ---- 奖品编辑弹窗 ----
const showPrizeDialog = ref(false)
const editingPrizeId = ref<number | null>(null)
const prizeForm = ref<{
  name: string
  prize_type: AdminLotteryPrize['prize_type']
  value: number
  weight: number
  min_tier: number
  stock: number
  sort_order: number
  enabled: boolean
  tier_weight_inputs: (number | null)[]
} | null>(null)

function openPrizeEdit(row: AdminLotteryPrize) {
  editingPrizeId.value = row.id
  prizeForm.value = {
    name: row.name,
    prize_type: row.prize_type,
    value: row.value,
    weight: row.weight,
    min_tier: row.min_tier,
    stock: row.stock,
    sort_order: row.sort_order,
    enabled: row.enabled,
    tier_weight_inputs: [0, 1, 2, 3, 4].map((tier) => {
      const raw = row.tier_weights?.[String(tier)]
      return typeof raw === 'number' ? raw : null
    })
  }
  showPrizeDialog.value = true
}

function closePrizeEdit() {
  showPrizeDialog.value = false
  editingPrizeId.value = null
  prizeForm.value = null
}

async function handleSavePrize() {
  if (!prizeForm.value || editingPrizeId.value == null) return
  savingPrize.value = true
  try {
    // tier_weights: 有输入的档位写入覆盖值;清空的档位从覆盖中移除(回退基础 weight)
    const tier_weights: Record<string, number> = {}
    prizeForm.value.tier_weight_inputs.forEach((input, tier) => {
      if (input !== null && !Number.isNaN(input)) tier_weights[String(tier)] = input
    })
    await updatePrize(editingPrizeId.value, {
      name: prizeForm.value.name,
      prize_type: prizeForm.value.prize_type,
      value: prizeForm.value.value,
      weight: prizeForm.value.weight,
      min_tier: prizeForm.value.min_tier,
      tier_weights,
      stock: prizeForm.value.stock,
      sort_order: prizeForm.value.sort_order,
      enabled: prizeForm.value.enabled
    })
    appStore.showSuccess?.(t('admin.lottery.saveSuccess'))
    closePrizeEdit()
    await loadPrizes()
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  } finally {
    savingPrize.value = false
  }
}

// ---- 次数调整 ----
const adjustUserId = ref('')
const adjustDelta = ref('1')
const adjusting = ref(false)

async function handleAdjust() {
  const uid = Number(adjustUserId.value)
  const delta = Number(adjustDelta.value)
  if (!uid || Number.isNaN(delta) || delta === 0) return
  adjusting.value = true
  try {
    await adjustDraws(uid, delta)
    appStore.showSuccess(t('admin.lottery.adjustSuccess'))
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  } finally {
    adjusting.value = false
  }
}

onMounted(async () => {
  await loadDraws(1)
  await loadActivities(true)
  await loadPrizes()
})
</script>
