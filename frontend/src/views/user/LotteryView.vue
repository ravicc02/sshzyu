<template>
  <AppLayout>
    <div class="mx-auto max-w-[1100px] space-y-6">
      <!-- 页头 -->
      <div>
        <h1 class="text-2xl font-bold">{{ t('lottery.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('lottery.description') }}
        </p>
      </div>

      <!-- 活动未开放 -->
      <div v-if="!loading && !activity" class="card p-8 text-center">
        <div class="mx-auto mb-3 w-fit rounded-xl bg-gray-100 p-3 text-gray-400 dark:bg-gray-800">
          <Icon name="gift" size="lg" />
        </div>
        <p class="font-medium text-gray-600 dark:text-gray-300">
          {{ t('lottery.noActivity') }}
        </p>
      </div>

      <div v-else-if="activity" class="flex flex-col gap-6 lg:flex-row">
        <!-- ============ 侧边:阶梯进度面板 ============ -->
        <aside class="w-full shrink-0 space-y-4 lg:w-[300px]" data-testid="tier-panel">
          <!-- 当前阶梯徽章 + 消耗进度 -->
          <div class="card p-5">
            <p class="text-xs font-medium uppercase tracking-wide text-gray-400 dark:text-gray-500">
              {{ t('lottery.tierPanel') }}
            </p>
            <div class="mt-2 flex items-center gap-2">
              <span class="text-3xl" aria-hidden="true">{{ tierIcon(currentTier) }}</span>
              <div>
                <p class="text-lg font-bold" data-testid="current-tier">
                  {{ tierLabel(currentTier) }}
                </p>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('lottery.spentSoFar', { spent: (status?.balance_spent ?? 0).toFixed(2) }) }}
                </p>
              </div>
            </div>

            <!-- 距下次抽奖解锁进度条 -->
            <div v-if="nextDrawAwayAmount !== null" class="mt-4">
              <div class="mb-1 flex justify-between text-xs text-gray-500 dark:text-gray-400">
                <span>{{ t('lottery.tierProgress') }}</span>
                <span>${{ (status?.next_threshold ?? 0).toFixed(2) }}</span>
              </div>
              <div class="h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-gray-800">
                <div
                  class="h-full rounded-full bg-primary-500 transition-all"
                  :style="{ width: tierProgressPercent + '%' }"
                />
              </div>
              <p class="mt-1.5 text-xs font-medium text-primary-600 dark:text-primary-400" data-testid="next-draw-away">
                {{ t('lottery.nextDrawAway', { amount: nextDrawAwayAmount.toFixed(2) }) }}
              </p>
            </div>
            <p v-else class="mt-3 text-xs font-medium text-green-600 dark:text-green-400">
              {{ t('lottery.tierMax') }}
            </p>
          </div>

          <!-- 阶梯一览 -->
          <div class="card p-5">
            <p class="mb-3 text-sm font-semibold">{{ t('lottery.tierList') }}</p>
            <ul class="space-y-1" data-testid="tier-list">
              <li
                v-for="tv in activity.tiers"
                :key="tv.tier"
                class="flex items-center justify-between rounded-lg px-2 py-1.5"
                :class="tv.tier === currentTier ? 'bg-primary-50 dark:bg-primary-900/30' : ''"
              >
                <span class="flex items-center gap-2 text-sm">
                  <span aria-hidden="true">{{ tierIcon(tv.tier) }}</span>
                  <span
                    class="font-medium"
                    :class="reachedTier(tv.tier) ? 'text-gray-900 dark:text-gray-100' : 'text-gray-400 dark:text-gray-500'"
                  >
                    {{ tierLabel(tv.tier) }}
                  </span>
                  <span
                    v-if="tv.tier === currentTier"
                    class="rounded-full bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-700 dark:bg-primary-900/50 dark:text-primary-300"
                  >
                    {{ t('lottery.tierCurrent') }}
                  </span>
                </span>
                <span class="text-xs text-gray-400 dark:text-gray-500">
                  {{ tv.threshold > 0 ? `$${tv.threshold.toFixed(0)}` : t('lottery.tierReached') }}
                </span>
              </li>
            </ul>
          </div>
        </aside>

        <!-- ============ 主区 ============ -->
        <div class="min-w-0 flex-1 space-y-6">
          <!-- 我的抽奖状态 -->
          <div class="card p-6">
            <div class="flex flex-wrap items-center justify-between gap-4">
              <div>
                <h2 class="text-lg font-semibold">{{ activity.name }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('lottery.rulesSummary') }}
                </p>
                <p
                  v-if="pointsExpiryLabel"
                  class="mt-2 text-xs text-amber-600 dark:text-amber-400"
                  data-testid="points-expiry"
                >
                  {{ t('lottery.pointsExpiresAt', { time: pointsExpiryLabel }) }}
                </p>
              </div>
              <div class="text-right">
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ t('lottery.availableDraws') }}
                </p>
                <p
                  class="text-4xl font-bold"
                  :class="status && status.available_draws > 0 ? 'text-primary-600 dark:text-primary-400' : 'text-gray-400 dark:text-gray-500'"
                  data-testid="available-draws"
                >
                  {{ status?.available_draws ?? 0 }}
                </p>
              </div>
            </div>

            <!-- 抽奖按钮 -->
            <div class="mt-6 flex flex-col items-center gap-2">
              <button
                class="btn btn-primary w-full py-3 text-base font-semibold disabled:cursor-not-allowed disabled:opacity-50 sm:w-auto sm:px-12"
                :disabled="!canDraw || drawing"
                data-testid="draw-button"
                @click="onDraw"
              >
                <span v-if="drawing">{{ t('lottery.drawing') }}</span>
                <span v-else-if="!status?.activity_open">{{ t('lottery.activityClosed') }}</span>
                <span v-else-if="(status?.available_draws ?? 0) <= 0">{{ t('lottery.noDrawsLeft') }}</span>
                <span v-else>{{ t('lottery.drawNow') }}</span>
              </button>
              <p v-if="status && status.available_draws <= 0 && status.next_threshold > 0" class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('lottery.howToEarnMore') }}
              </p>
            </div>
          </div>

          <!-- 奖品展示 -->
          <div class="card p-6">
            <h3 class="mb-4 font-semibold">{{ t('lottery.prizes') }}</h3>
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
              <div
                v-for="prize in activity.prizes"
                :key="prize.id"
                class="rounded-xl border border-gray-200 p-3 text-center dark:border-gray-700"
                :class="{ 'opacity-60': !prizeInStock(prize) }"
                data-testid="prize-item"
              >
                <p class="truncate text-sm font-semibold" :title="prize.name">
                  {{ prize.name }}
                </p>
                <p v-if="!prizeInStock(prize)" class="mt-1 text-xs text-gray-400 dark:text-gray-500">
                  {{ t('lottery.outOfStock') }}
                </p>
                <p v-if="prize.min_tier > 0" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
                  {{ tierIcon(prize.min_tier) }} {{ t('lottery.prizeMinTier', { tier: tierLabel(prize.min_tier) }) }}
                </p>
              </div>
            </div>
          </div>

          <!-- 中奖结果弹窗 -->
          <BaseDialog
            :show="showResult"
            :title="t('lottery.resultTitle')"
            @close="showResult = false"
          >
            <div v-if="lastResult" class="py-2 text-center">
              <template v-if="lastResult.draw.prize_type === 'none'">
                <p class="text-lg font-semibold text-gray-600 dark:text-gray-300">
                  {{ t('lottery.betterLuck') }}
                </p>
              </template>
              <template v-else>
                <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('lottery.congrats') }}</p>
                <p class="mt-2 text-2xl font-bold text-primary-600 dark:text-primary-400">
                  {{ lastResult.draw.prize_name }}
                </p>
                <p class="mt-2 text-xs" :class="fulfillmentClass(lastResult.draw.fulfillment_status)">
                  {{ fulfillmentLabel(lastResult.draw.fulfillment_status) }}
                </p>
              </template>
            </div>
            <template #footer>
              <button class="btn btn-primary w-full" data-testid="result-confirm" @click="showResult = false">
                {{ t('common.confirm') }}
              </button>
            </template>
          </BaseDialog>

          <!-- 抽奖记录 -->
          <div class="card p-6">
            <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
              <h3 class="font-semibold">{{ t('lottery.records') }}</h3>
              <RouterLink to="/redeem" class="text-sm text-primary-600 hover:underline dark:text-primary-400">
                {{ t('lottery.redeemHistory') }}
              </RouterLink>
            </div>
            <EmptyState v-if="records.length === 0" :description="t('lottery.noRecords')" />
            <ul v-else class="divide-y divide-gray-100 dark:divide-gray-800" data-testid="record-list">
              <li
                v-for="record in records"
                :key="record.id"
                class="flex items-center justify-between py-3"
              >
                <div>
                  <p class="text-sm font-medium">
                    {{ record.prize_type === 'none' ? t('lottery.betterLuck') : record.prize_name }}
                  </p>
                  <p class="text-xs text-gray-400 dark:text-gray-500">
                    {{ formatTime(record.created_at) }}
                  </p>
                </div>
                <span
                  v-if="record.prize_type !== 'none'"
                  class="text-xs"
                  :class="fulfillmentClass(record.fulfillment_status)"
                >
                  {{ fulfillmentLabel(record.fulfillment_status) }}
                </span>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import {
  lotteryAPI,
  type LotteryActivity,
  type LotteryDrawRecord,
  type LotteryDrawResult,
  type LotteryPrize,
  type LotteryStatus
} from '@/api/lottery'

const { t, locale } = useI18n()

// 阶梯图标(与后端 tier 序号一一对应)。
const TIER_ICONS = ['🥉', '🥈', '🥇', '💎', '👑', '🏆', '🌟', '✨', '🔥', '🚀']

const loading = ref(true)
const drawing = ref(false)
const activity = ref<LotteryActivity | null>(null)
const status = ref<LotteryStatus | null>(null)
const records = ref<LotteryDrawRecord[]>([])
const lastResult = ref<LotteryDrawResult | null>(null)
const showResult = ref(false)

const canDraw = computed(
  () => !!status.value?.activity_open && (status.value?.available_draws ?? 0) > 0
)

// 当前阶梯(服务端为准;无状态时按青铜处理)。
const currentTier = computed(() => Math.max(status.value?.current_tier ?? 0, 0))

function tierIcon(tier: number): string {
  return TIER_ICONS[Math.min(Math.max(tier, 0), TIER_ICONS.length - 1)]
}

function tierLabel(tier: number): string {
  const configured = activity.value?.tiers.find((item) => item.tier === tier)
  return configured?.name || t('lottery.tierFallback', { tier: tier + 1 })
}

function reachedTier(tier: number): boolean {
  return tier <= currentTier.value
}

// 距离下次抽奖解锁还差金额(美元);null 表示暂无可解锁阈值(status 未加载或异常)。
// 基准为服务端 next_threshold($5/$15/$25/…每 $10 递进),最高阶梯后仍持续计算,不封顶。
const nextDrawAwayAmount = computed<number | null>(() => {
  if (!status.value || !(status.value.next_threshold > 0)) {
    return null
  }
  return Math.max(0, status.value.next_threshold - status.value.balance_spent)
})

// 到下次抽奖解锁的进度百分比(与服务端 next_threshold 同基准)。
const tierProgressPercent = computed(() => {
  if (!status.value || !(status.value.next_threshold > 0)) {
    return 100
  }
  const pct = (status.value.balance_spent / status.value.next_threshold) * 100
  return Math.min(100, Math.max(0, Math.round(pct * 10) / 10))
})

const pointsExpiryLabel = computed(() => {
  if (!status.value?.points_expires_at) return ''
  const expiry = new Date(status.value.points_expires_at)
  if (Number.isNaN(expiry.getTime())) return ''
  return expiry.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US')
})

function prizeInStock(prize: LotteryPrize): boolean {
  return prize.stock < 0 || prize.stock_issued < prize.stock
}

function fulfillmentLabel(statusValue: LotteryDrawRecord['fulfillment_status']): string {
  switch (statusValue) {
    case 'granted':
      return t('lottery.fulfillmentGranted')
    case 'pending':
      return t('lottery.fulfillmentPending')
    case 'pending_review':
      return t('lottery.fulfillmentPendingReview')
    case 'rejected':
      return t('lottery.fulfillmentRejected')
    case 'revoked':
      return t('lottery.fulfillmentRevoked')
    default:
      return t('lottery.fulfillmentFailed')
  }
}

function fulfillmentClass(statusValue: LotteryDrawRecord['fulfillment_status']): string {
  switch (statusValue) {
    case 'granted':
      return 'text-green-600 dark:text-green-400'
    case 'pending':
    case 'pending_review':
      return 'text-amber-600 dark:text-amber-400'
    case 'rejected':
    case 'revoked':
      return 'text-gray-500 dark:text-gray-400'
    default:
      return 'text-red-600 dark:text-red-400'
  }
}

function formatTime(value: string): string {
  return new Date(value).toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US')
}

/**
 * 生成幂等键。同一请求重试由调用方持有同一 key;
 * 这里每次点击生成新 key,重试场景(网络超时)由用户再次点击,
 * 服务端以 (user_id, idempotency_key) 唯一约束兜底防重复扣次数。
 */
function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `draw-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`
}

async function onDraw(): Promise<void> {
  if (!canDraw.value || drawing.value) {
    return
  }
  drawing.value = true
  try {
    const result = await lotteryAPI.draw(newIdempotencyKey())
    lastResult.value = result
    showResult.value = true
    await Promise.all([refreshStatus(), refreshRecords()])
  } catch (error) {
    console.error('Lottery draw failed:', error)
  } finally {
    drawing.value = false
  }
}

async function refreshStatus(): Promise<void> {
  try {
    status.value = await lotteryAPI.getStatus()
  } catch (error) {
    console.error('Failed to load lottery status:', error)
  }
}

async function refreshRecords(): Promise<void> {
  try {
    const data = await lotteryAPI.getRecords(1, 20)
    records.value = data.items
  } catch (error) {
    console.error('Failed to load lottery records:', error)
  }
}

onMounted(async () => {
  try {
    const [activityResult] = await Promise.all([
      lotteryAPI.getActivity().catch((error) => {
        console.error('Failed to load lottery activity:', error)
        return null
      }),
      refreshStatus(),
      refreshRecords()
    ])
    activity.value = activityResult
  } finally {
    loading.value = false
  }
})
</script>
