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
            <div class="flex gap-1">
              <!-- pending_review: 审核通过/驳回 -->
              <template v-if="row.fulfillment_status === 'pending_review'">
                <button
                  @click="handleApprove(row)"
                  :disabled="actionLoadingId === row.id"
                  class="btn btn-success btn-sm"
                >
                  <Icon name="checkCircle" size="sm" :class="actionLoadingId === row.id ? 'animate-spin' : ''" />
                  {{ t('admin.lottery.approve') }}
                </button>
                <button
                  @click="handleReject(row)"
                  :disabled="actionLoadingId === row.id"
                  class="btn btn-danger btn-sm"
                >
                  <Icon name="xCircle" size="sm" />
                  {{ t('admin.lottery.reject') }}
                </button>
              </template>
              <!-- 已发放余额奖励可静默撤回；不会新增用户侧通知。 -->
              <template v-else-if="row.fulfillment_status === 'granted' && row.prize_type === 'balance_bonus'">
                <button
                  @click="handleReverse(row)"
                  :disabled="actionLoadingId === row.id"
                  class="btn btn-danger btn-sm"
                >
                  <Icon name="sync" size="sm" :class="actionLoadingId === row.id ? 'animate-spin' : ''" />
                  {{ t('admin.lottery.reverseGrant') }}
                </button>
              </template>
              <!-- 仅初始待处理或失败记录允许重试。 -->
              <template v-else-if="row.fulfillment_status === 'pending' || row.fulfillment_status === 'failed'">
                <button
                  @click="handleRetry(row)"
                  :disabled="retryingId === row.id"
                  class="btn btn-secondary btn-sm"
                >
                  <Icon name="refresh" size="sm" :class="retryingId === row.id ? 'animate-spin' : ''" />
                  {{ t('admin.lottery.retryFulfillment') }}
                </button>
              </template>
            </div>
          </template>

          <template #empty>
            <EmptyState :title="t('empty.noData')" :description="t('admin.lottery.noDraws')" />
          </template>
        </DataTable>
      </template>

      <!-- ====================== 参数配置 ====================== -->
      <template v-else #table>
        <div class="lottery-config-scroll space-y-6">
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
                <label class="input-label">{{ t('admin.lottery.tierMode') }}</label>
                <Select v-model="activityForm.tier_mode" :options="tierModeOptions" />
                <p class="input-hint">{{ t('admin.lottery.tierModeHint') }}</p>
              </div>
              <div v-if="activityForm.tier_mode === 'custom'" class="md:col-span-2">
                <div class="mb-2 flex items-center justify-between gap-3">
                  <label class="input-label">{{ t('admin.lottery.customTierThresholds') }}</label>
                  <button type="button" class="btn btn-secondary btn-sm" @click="addTier">
                    {{ t('admin.lottery.addTier') }}
                  </button>
                </div>
                <div class="space-y-3">
                  <div v-for="(tier, index) in activityForm.tier_definitions" :key="tier.id" class="grid grid-cols-[auto_1fr_1fr_auto] items-end gap-3">
                    <span class="pb-2 text-xs font-semibold text-gray-500 dark:text-dark-400">{{ index }}</span>
                    <div>
                      <label class="mb-1 block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.lottery.tierName') }}</label>
                      <input v-model="tier.name" type="text" class="input" maxlength="50" />
                    </div>
                    <div>
                      <label class="mb-1 block text-xs text-gray-500 dark:text-dark-400">{{ t('admin.lottery.tierThreshold') }}</label>
                      <input v-model.number="tier.threshold_dollars" type="number" min="0" step="0.01" class="input" :disabled="index === 0" />
                    </div>
                    <button v-if="index > 0" type="button" class="btn btn-danger btn-sm" @click="removeTier(index)">
                      {{ t('common.delete') }}
                    </button>
                  </div>
                </div>
                <p class="input-hint">{{ t('admin.lottery.customTierThresholdsHint') }}</p>
                <p class="input-hint">{{ t('admin.lottery.newTierSaveOrderHint') }}</p>
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
            <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.lottery.prizeSection') }}</h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.lottery.probabilityConfigHint') }}</p>
              </div>
              <button @click="openWeightsEdit" :disabled="!prizes.length" class="btn btn-secondary btn-sm">
                <Icon name="edit" size="sm" />
                {{ t('admin.lottery.editProbabilities') }}
              </button>
            </div>
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
              <template #cell-min_tier="{ value }">
                <span class="badge badge-gray">{{ tierDefinitions.find((tier) => tier.index === value)?.name || t('admin.lottery.tierFallback', { tier: value + 1 }) }}</span>
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
              <div class="relative min-w-64">
                <label class="input-label">{{ t('admin.lottery.adjustUser') }}</label>
                <input
                  v-model="adjustUserQuery"
                  type="search"
                  :placeholder="t('admin.lottery.adjustUserPlaceholder')"
                  class="input w-full"
                  @input="searchAdjustUsers"
                />
                <div v-if="adjustUserResults.length" class="absolute z-10 mt-1 max-h-48 w-full overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800">
                  <button
                    v-for="user in adjustUserResults"
                    :key="user.id"
                    type="button"
                    class="block w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                    @click="selectAdjustUser(user)"
                  >
                    {{ user.email }} (#{{ user.id }})
                  </button>
                </div>
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

    <!-- 活动级概率编辑弹窗：所有奖品一次保存，保证每档总和恒为100%。 -->
    <BaseDialog
      :show="showWeightsDialog"
      :title="t('admin.lottery.editProbabilities')"
      width="wide"
      @close="closeWeightsEdit"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('admin.lottery.probabilityDialogHint') }}</p>
        <div class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-left text-xs text-gray-500 dark:border-dark-600 dark:text-dark-400">
                <th class="px-2 py-2">{{ t('admin.lottery.prizeName') }}</th>
                <th class="min-w-20 px-2 py-2 text-center">{{ t('admin.lottery.prizeEnabled') }}</th>
                <th class="min-w-28 px-2 py-2 text-center">{{ t('admin.lottery.prizeMinTier') }}</th>
                <th v-for="tier in tierDefinitions" :key="tier.index" class="min-w-24 px-2 py-2 text-center">
                  {{ tier.name }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in weightForms" :key="item.id" class="border-b border-gray-100 dark:border-dark-600/60">
                <td class="px-2 py-2 text-gray-900 dark:text-white">
                  {{ item.name }}
                </td>
                <td class="px-2 py-2 text-center">
                  <input v-model="item.enabled" type="checkbox" class="h-4 w-4" />
                </td>
                <td class="px-2 py-2">
                  <select v-model.number="item.minTier" class="input min-w-24">
                    <option v-for="tier in tierDefinitions" :key="tier.index" :value="tier.index">
                      {{ tier.name }}
                    </option>
                  </select>
                </td>
                <td v-for="tier in tierDefinitions" :key="tier.index" class="px-2 py-2">
                  <input
                    v-model.number="item.weights[tier.index]"
                    type="number"
                    min="0"
                    max="100"
                    class="input min-w-20 text-center"
                    :disabled="!isWeightApplicable(item, tier.index)"
                  />
                </td>
              </tr>
            </tbody>
            <tfoot>
              <tr class="font-semibold">
                <td colspan="3" class="px-2 py-3 text-gray-700 dark:text-dark-200">{{ t('admin.lottery.probabilityTotal') }}</td>
                <td v-for="tier in tierDefinitions" :key="tier.index" class="px-2 py-3 text-center">
                  <span :class="weightTotal(tier.index) === 100 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'">
                    {{ weightTotal(tier.index) }}%
                  </span>
                </td>
              </tr>
            </tfoot>
          </table>
        </div>
        <p v-if="!allWeightTotalsValid" class="text-sm text-red-600 dark:text-red-400">{{ t('admin.lottery.probabilityTotalInvalid') }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" @click="closeWeightsEdit" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="button" @click="handleSaveWeights" :disabled="savingWeights || !allWeightTotalsValid" class="btn btn-primary">
            {{ t('common.save') }}
          </button>
        </div>
      </div>
    </BaseDialog>

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
          <div class="flex items-end pb-1 md:col-span-2">
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.lottery.probabilityManagedSeparately') }}</p>
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
import { searchUsers, type SimpleUser } from '@/api/admin/usage'
import type { Column } from '@/components/common/types'
import {
  listDraws,
  getActivities,
  getPrizes,
  updateActivity,
  updatePrize,
  updatePrizeWeights,
  retryFulfillment,
  adjustDraws,
  approveDraw,
  rejectDraw,
  reverseGrant,
  type AdminLotteryDraw,
  type AdminLotteryActivity,
  type AdminLotteryPrize,
  type AdminUpdateActivityRequest
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
	const actionLoadingId = ref<number | null>(null)

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

async function handleApprove(row: AdminLotteryDraw) {
	  actionLoadingId.value = row.id
	  try {
	    const { draw } = await approveDraw(row.id)
	    const idx = draws.value.findIndex((d) => d.id === row.id)
	    if (idx >= 0) draws.value[idx] = { ...draws.value[idx], ...draw }
	    appStore.showSuccess(t('admin.lottery.approveSuccess'))
	  } catch (e) {
	    appStore.showError(e instanceof Error ? e.message : String(e))
	  } finally {
	    actionLoadingId.value = null
	  }
	}

	async function handleReject(row: AdminLotteryDraw) {
	  if (!window.confirm(t('admin.lottery.rejectConfirm'))) return
	  actionLoadingId.value = row.id
	  try {
	    const { draw } = await rejectDraw(row.id)
	    const idx = draws.value.findIndex((d) => d.id === row.id)
	    if (idx >= 0) draws.value[idx] = { ...draws.value[idx], ...draw }
	    appStore.showSuccess(t('admin.lottery.rejectSuccess'))
	  } catch (e) {
	    appStore.showError(e instanceof Error ? e.message : String(e))
	  } finally {
	    actionLoadingId.value = null
	  }
	}

	async function handleReverse(row: AdminLotteryDraw) {
  if (!window.confirm(t('admin.lottery.reverseGrantConfirm'))) return
  actionLoadingId.value = row.id
  try {
    const { draw } = await reverseGrant(row.id)
    const idx = draws.value.findIndex((d) => d.id === row.id)
    if (idx >= 0) draws.value[idx] = { ...draws.value[idx], ...draw }
    appStore.showSuccess(t('admin.lottery.reverseGrantSuccess'))
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  } finally {
    actionLoadingId.value = null
  }
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
  tier_mode: 'fixed' | 'custom'
  tier_definitions: Array<{ id: string; name: string; threshold_dollars: number }>
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

const tierModeOptions = computed(() => [
  { value: 'fixed', label: t('admin.lottery.tierModeFixed') },
  { value: 'custom', label: t('admin.lottery.tierModeCustom') }
])

const defaultTierDefinitions = [
  { name: '青铜', threshold_dollars: 0 },
  { name: '白银', threshold_dollars: 5 },
  { name: '黄金', threshold_dollars: 55 },
  { name: '钻石', threshold_dollars: 105 },
  { name: '王者', threshold_dollars: 155 }
]

const tierDefinitions = computed(() =>
  (activityForm.value?.tier_definitions || []).map((tier, index) => ({ ...tier, index }))
)

const prizeColumns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.lottery.prizeName') },
  { key: 'prize_type', label: t('admin.lottery.prizeType') },
  { key: 'value', label: t('admin.lottery.prizeValue') },
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

function addTier() {
  if (!activityForm.value) return
  const last = activityForm.value.tier_definitions.at(-1)
  activityForm.value.tier_definitions.push({
    id: `tier-${Date.now()}-${activityForm.value.tier_definitions.length}`,
    name: `${t('admin.lottery.tierName')} ${activityForm.value.tier_definitions.length}`,
    threshold_dollars: last ? Number(last.threshold_dollars) + 50 : 0
  })
}

function removeTier(index: number) {
  if (!activityForm.value || index <= 0 || activityForm.value.tier_definitions.length <= 1) return
  activityForm.value.tier_definitions.splice(index, 1)
}

function syncActivityForm() {
  const current = activities.value.find((a) => a.id === activeActivityId.value) || null
  activity.value = current
  activityForm.value = current
    ? {
        name: current.name,
        status: current.status,
        tier_mode: current.tier_mode || 'fixed',
        tier_definitions:
          current.tier_mode === 'custom' && current.tier_definitions?.length
            ? current.tier_definitions.map((tier, index) => ({
                id: `tier-${current.id}-${index}`,
                name: tier.name,
                threshold_dollars: tier.threshold_cents / 100
              }))
            : defaultTierDefinitions.map((tier, index) => ({
                id: `tier-${current.id}-${index}`,
                ...tier
              })),
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
  const form = activityForm.value
  const tier_definitions = form.tier_definitions.map((tier) => ({
    name: tier.name.trim(),
    threshold_cents: Math.round(Number(tier.threshold_dollars) * 100)
  }))
  if (form.tier_mode === 'custom') {
    const valid = tier_definitions.length > 0 && tier_definitions.length <= 100 &&
      tier_definitions[0].threshold_cents === 0 &&
      tier_definitions.every((tier, index) =>
        tier.name.length > 0 && tier.threshold_cents >= 0 &&
        (index === 0 || tier.threshold_cents > tier_definitions[index - 1].threshold_cents))
    if (!valid) {
      appStore.showError(t('admin.lottery.customTierThresholdsInvalid'))
      return
    }
  }

  savingActivity.value = true
  try {
    const starts = parseDateTimeLocalInput(form.starts_at_input)
    const ends = parseDateTimeLocalInput(form.ends_at_input)
    const savedActivity = activity.value
    const savedDefinitions = savedActivity.tier_definitions || []
    const tierConfigChanged = form.tier_mode !== savedActivity.tier_mode || (
      form.tier_mode === 'custom' && (
        tier_definitions.length !== savedDefinitions.length ||
        tier_definitions.some((tier, index) =>
          tier.name !== savedDefinitions[index]?.name ||
          tier.threshold_cents !== savedDefinitions[index]?.threshold_cents)
      )
    )
    const request: AdminUpdateActivityRequest = {
      name: form.name,
      status: form.status,
      starts_at: starts !== null ? new Date(starts * 1000).toISOString() : null,
      ends_at: ends !== null ? new Date(ends * 1000).toISOString() : null
    }
    if (tierConfigChanged) {
      request.tier_mode = form.tier_mode
      request.tier_thresholds = []
      request.tier_definitions = form.tier_mode === 'custom' ? tier_definitions : []
    }
    await updateActivity(savedActivity.id, request)

    appStore.showSuccess(t('admin.lottery.saveSuccess'))
    await loadActivities()
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  } finally {
    savingActivity.value = false
  }
}

// ---- 活动级概率编辑弹窗 ----
type WeightFormItem = {
  id: number
  name: string
  enabled: boolean
  minTier: number
  weights: number[]
}

const showWeightsDialog = ref(false)
const savingWeights = ref(false)
const weightForms = ref<WeightFormItem[]>([])

function openWeightsEdit() {
  weightForms.value = prizes.value.map((prize) => ({
    id: prize.id,
    name: prize.name,
    enabled: prize.enabled,
    minTier: prize.min_tier,
    weights: tierDefinitions.value.map((tier) => {
      const configured = prize.tier_weights?.[String(tier.index)]
      return typeof configured === 'number' ? configured : prize.weight
    })
  }))
  showWeightsDialog.value = true
}

function closeWeightsEdit() {
  showWeightsDialog.value = false
  weightForms.value = []
}

function isWeightApplicable(item: WeightFormItem, tier: number) {
  return item.enabled && tier >= item.minTier
}

function weightTotal(tier: number) {
  return weightForms.value.reduce((total, item) =>
    isWeightApplicable(item, tier) ? total + Math.max(0, Number(item.weights[tier]) || 0) : total, 0)
}

const allWeightTotalsValid = computed(() =>
  tierDefinitions.value.every((tier) => weightTotal(tier.index) === 100)
)

async function handleSaveWeights() {
  if (!activity.value || !allWeightTotalsValid.value) return
  savingWeights.value = true
  try {
    await updatePrizeWeights(
      activity.value.id,
      weightForms.value.map((item) => ({
        id: item.id,
        // 每档均显式保存；后端据此执行活动级原子100%校验。
        weight: Math.max(0, Number(item.weights[0]) || 0),
        tier_weights: Object.fromEntries(
          tierDefinitions.value.map((tier) => [
            String(tier.index),
            isWeightApplicable(item, tier.index) ? Math.max(0, Number(item.weights[tier.index]) || 0) : 0
          ])
        ),
        enabled: item.enabled,
        min_tier: item.minTier
      }))
    )
    appStore.showSuccess(t('admin.lottery.probabilitySaveSuccess'))
    closeWeightsEdit()
    // 新档位须先预配概率，再保存活动；刷新奖品时保留尚未提交的档位草稿。
    const pendingActivityForm = activityForm.value
    const pendingActivityId = activity.value.id
    await loadPrizes()
    await loadActivities()
    if (activity.value?.id === pendingActivityId && pendingActivityForm) {
      activityForm.value = pendingActivityForm
    }
  } catch (e) {
    appStore.showError(e instanceof Error ? e.message : String(e))
  } finally {
    savingWeights.value = false
  }
}

// ---- 奖品编辑弹窗 ----
const showPrizeDialog = ref(false)
const editingPrizeId = ref<number | null>(null)
const prizeForm = ref<{
  name: string
  prize_type: AdminLotteryPrize['prize_type']
  value: number
  stock: number
  sort_order: number
} | null>(null)

function openPrizeEdit(row: AdminLotteryPrize) {
  editingPrizeId.value = row.id
  prizeForm.value = {
    name: row.name,
    prize_type: row.prize_type,
    value: row.value,
    stock: row.stock,
    sort_order: row.sort_order
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
    await updatePrize(editingPrizeId.value, {
      name: prizeForm.value.name,
      prize_type: prizeForm.value.prize_type,
      value: prizeForm.value.value,
      stock: prizeForm.value.stock,
      sort_order: prizeForm.value.sort_order
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
const adjustUserQuery = ref('')
const adjustUserResults = ref<SimpleUser[]>([])
const adjustDelta = ref('1')
const adjusting = ref(false)
let adjustSearchTimer: ReturnType<typeof setTimeout> | undefined

function searchAdjustUsers() {
  const query = adjustUserQuery.value.trim()
  adjustUserId.value = ''
  adjustUserResults.value = []
  if (adjustSearchTimer) clearTimeout(adjustSearchTimer)
  if (!query) return
  adjustSearchTimer = setTimeout(async () => {
    try {
      adjustUserResults.value = await searchUsers(query)
    } catch (e) {
      appStore.showError(e instanceof Error ? e.message : String(e))
    }
  }, 250)
}

function selectAdjustUser(user: SimpleUser) {
  adjustUserId.value = String(user.id)
  adjustUserQuery.value = `${user.email} (#${user.id})`
  adjustUserResults.value = []
}

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

<style scoped>
.lottery-config-scroll {
  max-height: calc(100dvh - 250px);
  overflow-y: auto;
  padding-right: 0.5rem;
  scrollbar-gutter: stable;
}

@media (max-width: 1023px) {
  .lottery-config-scroll {
    max-height: none;
    overflow-y: visible;
    padding-right: 0;
  }
}
</style>
