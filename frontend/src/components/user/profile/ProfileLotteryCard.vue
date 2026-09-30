<template>
  <div class="card p-6" data-testid="profile-lottery-card">
    <div class="flex items-center gap-4">
      <div class="rounded-xl bg-primary-100 p-3 text-primary-600 dark:bg-primary-900/40 dark:text-primary-400">
        <Icon name="gift" size="lg" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-center justify-between gap-3">
          <h3 class="font-semibold">{{ t('lottery.title') }}</h3>
          <span
            v-if="status"
            class="shrink-0 rounded-full bg-primary-100 px-3 py-1 text-xs font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300"
            data-testid="lottery-card-draws"
          >
            {{ t('lottery.drawsBadge', { count: status.available_draws }) }}
          </span>
        </div>
        <p class="mt-1 flex items-center gap-1 truncate text-sm text-gray-500 dark:text-gray-400">
          <template v-if="status">
            <span aria-hidden="true">{{ tierIcon(status.current_tier) }}</span>
            <span class="font-medium text-gray-700 dark:text-gray-300">{{ tierLabel(status.current_tier) }}</span>
            <span v-if="nextDrawAwayAmount !== null">
              · {{ t('lottery.nextDrawAway', { amount: nextDrawAwayAmount.toFixed(2) }) }}
            </span>
            <span v-else>· {{ t('lottery.tierMax') }}</span>
          </template>
          <template v-else>
            {{ t('lottery.cardIdle') }}
          </template>
        </p>
        <p v-if="pointsExpiryLabel" class="mt-1 truncate text-xs text-amber-600 dark:text-amber-400">
          {{ t('lottery.pointsExpiresAt', { time: pointsExpiryLabel }) }}
        </p>
      </div>
      <RouterLink
        to="/lottery"
        class="btn btn-primary shrink-0"
        data-testid="lottery-card-link"
      >
        {{ t('lottery.goDraw') }}
      </RouterLink>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { Icon } from '@/components/icons'
import { lotteryAPI, type LotteryStatus } from '@/api/lottery'

const { t, locale } = useI18n()
const status = ref<LotteryStatus | null>(null)

// 距离下次抽奖解锁还差金额(美元),与 LotteryView 侧边面板同一口径;null 表示暂无可解锁阈值。
const nextDrawAwayAmount = computed<number | null>(() => {
  if (!status.value || !(status.value.next_threshold > 0)) {
    return null
  }
  return Math.max(0, status.value.next_threshold - status.value.balance_spent)
})

// 阶梯图标与展示名(与 LotteryView 保持一致)。
const TIER_ICONS = ['🥉', '🥈', '🥇', '💎', '👑', '🏆', '🌟', '✨', '🔥', '🚀']

function tierIcon(tier: number): string {
  return TIER_ICONS[Math.min(Math.max(tier, 0), TIER_ICONS.length - 1)]
}

function tierLabel(tier: number): string {
  if (status.value?.tier_name && status.value.current_tier === tier) return status.value.tier_name
  return t(`lottery.tier${Math.min(Math.max(tier, 0), 4)}`)
}

const pointsExpiryLabel = computed(() => {
  if (!status.value?.points_expires_at) return ''
  const expiry = new Date(status.value.points_expires_at)
  if (Number.isNaN(expiry.getTime())) return ''
  return expiry.toLocaleString(locale.value === 'zh' ? 'zh-CN' : 'en-US')
})

onMounted(async () => {
  try {
    status.value = await lotteryAPI.getStatus()
  } catch (error) {
    // 抽奖功能不可用(如活动未配置)时静默降级,不阻塞个人中心
    console.error('Failed to load lottery status:', error)
  }
})
</script>
