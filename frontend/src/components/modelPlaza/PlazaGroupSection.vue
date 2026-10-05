<template>
  <section
    class="overflow-hidden rounded-2xl border bg-white shadow-card transition-shadow hover:shadow-lg dark:bg-dark-800/50"
    :class="[platformBorderStrongClass(group.platform)]"
  >
    <header class="group/header relative px-5 pb-5 pt-5 sm:px-6">
        <div class="absolute inset-x-0 top-0 h-1" :class="platformAccentBarClass(group.platform)"></div>
        <div class="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <GroupBadge
                :name="group.name"
                :platform="group.platform as GroupPlatform"
                :subscription-type="(group.subscription_type || 'standard') as SubscriptionType"
                :rate-multiplier="group.rate_multiplier"
                :user-rate-multiplier="group.user_rate_multiplier ?? null"
                :peak-rate-enabled="group.peak_rate_enabled"
                :peak-start="group.peak_start"
                :peak-end="group.peak_end"
                :peak-rate-multiplier="group.peak_rate_multiplier"
                always-show-rate
              />
              <span
                v-if="group.is_exclusive"
                class="inline-flex items-center gap-1 rounded-md bg-purple-50 px-2 py-0.5 text-xs font-medium text-purple-600 dark:bg-purple-900/20 dark:text-purple-400"
              >
                <Icon name="shield" size="xs" class="h-3 w-3" />
                {{ t('modelPlaza.badges.exclusive') }}
              </span>
              <span
                v-if="group.subscription_type === 'subscription'"
                class="inline-flex items-center rounded-md bg-violet-50 px-2 py-0.5 text-xs font-medium text-violet-600 dark:bg-violet-900/20 dark:text-violet-400"
              >
                {{ t('modelPlaza.badges.subscription') }}
              </span>
            </div>
            <p v-if="group.description" class="mt-3 max-w-3xl text-sm leading-6 text-gray-500 dark:text-dark-400">
              {{ group.description }}
            </p>
            <div class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-gray-500 dark:text-dark-400">
              <span class="inline-flex items-center gap-1.5 font-semibold text-gray-700 dark:text-dark-200">
                <Icon name="cube" size="xs" class="h-3.5 w-3.5" />
                {{ t('modelPlaza.card.modelCount', { count: group.models.length }) }}
              </span>
              <span v-if="discountLabel" class="inline-flex items-center gap-1 font-semibold text-emerald-600 dark:text-emerald-400">
                <Icon name="trendingUp" size="xs" class="h-3.5 w-3.5 rotate-180" />
                {{ discountLabel }}
              </span>
              <span v-if="peakNote" class="inline-flex items-center gap-1 text-amber-600 dark:text-amber-400">
                <Icon name="clock" size="xs" class="h-3.5 w-3.5" />
                {{ peakNote }}
              </span>
            </div>
          </div>
          <button
            type="button"
            class="inline-flex shrink-0 items-center gap-2 self-start rounded-lg bg-gray-50 px-3 py-2 text-xs font-semibold text-gray-600 transition-colors hover:bg-gray-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:bg-dark-900/60 dark:text-dark-300 dark:hover:bg-dark-900"
            :aria-expanded="expanded"
            @click="expanded = !expanded"
          >
            {{ expanded ? t('modelPlaza.card.hidePricing') : t('modelPlaza.card.showPricing') }}
            <Icon :name="expanded ? 'chevronUp' : 'chevronDown'" size="sm" class="h-4 w-4" />
          </button>
        </div>

        <!-- 始终先展示模型清单；按需展开逐项定价。 -->
        <div class="mt-5 grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
          <span
            v-for="model in group.models"
            :key="`${model.platform}:${model.name}`"
            class="min-w-0 rounded-xl border border-gray-200 bg-gray-50/70 p-3 transition-colors group-hover/header:border-gray-300 group-hover/header:bg-white dark:border-dark-700 dark:bg-dark-900/50 dark:group-hover/header:border-dark-600 dark:group-hover/header:bg-dark-900"
          >
            <span class="flex min-w-0 items-center gap-1.5 font-mono text-xs font-medium text-gray-700 dark:text-dark-200">
              <PlatformIcon :platform="model.platform as GroupPlatform" size="xs" />
              <span class="truncate">{{ model.name }}</span>
            </span>
            <span v-if="previewPrice(model)" class="mt-2 grid grid-cols-[auto_minmax(0,1fr)] items-baseline gap-x-2 gap-y-1 pl-5">
              <span class="text-[10px] text-gray-400 dark:text-dark-500">{{ t('modelPlaza.card.basePrice') }}</span>
              <span class="truncate font-mono text-[11px] text-gray-400 dark:text-dark-500" :class="previewRate(model) === 1 ? '' : 'line-through'">{{ previewPrice(model)!.standard }}</span>
              <span class="text-[10px] font-semibold text-gray-500 dark:text-dark-400">{{ t('modelPlaza.card.paidPrice') }}</span>
              <span class="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
                <strong class="font-mono text-sm font-bold" :class="previewRate(model) < 1 ? 'text-emerald-600 dark:text-emerald-400' : previewRate(model) > 1 ? 'text-amber-700 dark:text-amber-300' : 'text-gray-700 dark:text-dark-200'">{{ previewPrice(model)!.paid }}</strong>
                <span v-if="previewRate(model) !== 1" class="rounded-md px-1.5 py-0.5 text-[10px] font-semibold" :class="previewRate(model) < 1 ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300' : 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300'">{{ discountFor(model) }}</span>
              </span>
            </span>
          </span>
        </div>
    </header>

    <Transition name="plaza-details">
      <div v-if="expanded" class="border-t border-gray-100 dark:border-dark-700/60">
        <div class="flex flex-wrap items-center justify-between gap-2 bg-gray-50/70 px-5 py-3 text-xs text-gray-500 dark:bg-dark-900/30 dark:text-dark-400 sm:px-6">
          <span class="font-medium">{{ t('modelPlaza.card.pricingHint') }}</span>
          <span>{{ t('modelPlaza.card.standardRate', { rate: effectiveRate }) }}</span>
        </div>
        <div class="px-0 pb-1 pt-1">
          <PlazaModelPricingTable
            v-if="group.models.length > 0"
            :models="group.models"
            :platform="group.platform"
            :rate-multiplier="group.rate_multiplier"
            :user-rate-multiplier="group.user_rate_multiplier ?? null"
            :image-rate-independent="group.image_rate_independent"
            :image-rate-multiplier="group.image_rate_multiplier"
            :peak-window="peakWindow"
            :peak-rate-multiplier="group.peak_rate_multiplier"
          />
          <p v-else class="px-5 py-4 text-center text-sm text-gray-400 dark:text-dark-500">
            {{ t('modelPlaza.detail.noModels') }}
          </p>
        </div>
        <p
          v-if="longContextNote"
          class="flex items-center gap-1 px-5 pb-4 text-xs text-gray-500 dark:text-dark-400 sm:px-6"
        >
          <Icon name="infoCircle" size="xs" class="h-3 w-3" />
          {{ longContextNote }}
        </p>
      </div>
    </Transition>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import PlazaModelPricingTable from './PlazaModelPricingTable.vue'
import type { ModelPlazaGroup } from '@/api/modelPlaza'
import type { GroupPlatform, SubscriptionType } from '@/types'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { formatScaled } from '@/utils/pricing'
import { BILLING_MODE_IMAGE, BILLING_MODE_TOKEN, BILLING_MODE_VIDEO, type BillingMode } from '@/constants/channel'
import { platformAccentBarClass, platformBorderStrongClass } from '@/utils/platformColors'
import { hasPeakRate, formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { useAppStore } from '@/stores/app'

const props = defineProps<{
  group: ModelPlazaGroup
}>()

const { t } = useI18n()
const appStore = useAppStore()
const expanded = ref(false)

const effectiveRate = computed(() => props.group.user_rate_multiplier ?? props.group.rate_multiplier)

interface PreviewPrice {
  standard: string
  paid: string
}

function previewRate(model: ModelPlazaGroup['models'][number]): number {
  const rate = modelRate(model)
  return Math.round(rate * 1000) / 1000
}

function previewPrice(model: ModelPlazaGroup['models'][number]): PreviewPrice | null {
  const pricing = model.pricing
  if (!pricing) return null
  const rate = previewRate(model)
  const scale = billingMode(model) === BILLING_MODE_TOKEN ? 1_000_000 : 1
  const unit = t(billingMode(model) === BILLING_MODE_TOKEN ? 'modelPlaza.table.unitPerMillionShort' : modelUnitKey(model))

  if (pricing.billing_mode === BILLING_MODE_TOKEN) {
    if (pricing.input_price == null && pricing.output_price == null) return null
    const values = [
      [t('modelPlaza.table.inputShort'), pricing.input_price],
      [t('modelPlaza.table.outputShort'), pricing.output_price]
    ] as const
    return {
      standard: values.filter(([, price]) => price != null).map(([label, price]) => `${label} ${formatPrice(price ?? 0, scale)}`).join(' · ') + ` ${unit}`,
      paid: values.filter(([, price]) => price != null).map(([label, price]) => `${label} ${formatPrice((price ?? 0) * rate, scale)}`).join(' · ') + ` ${unit}`
    }
  }

  if (pricing.billing_mode === BILLING_MODE_VIDEO) return null
  const raw = pricing.intervals?.find((interval) => interval.per_request_price != null)?.per_request_price ?? pricing.per_request_price
  if (raw == null) return null
  return {
    standard: `${formatPrice(raw, scale)} ${unit}`,
    paid: `${formatPrice(raw * rate, scale)} ${unit}`
  }
}

function modelUnitKey(model: ModelPlazaGroup['models'][number]): string {
  const mode = billingMode(model)
  if (mode === BILLING_MODE_IMAGE) return 'modelPlaza.table.perUnitImage'
  return 'modelPlaza.table.perUnitRequest'
}

function hasIndependentRate(model: ModelPlazaGroup['models'][number]): boolean {
  const mode = billingMode(model)
  return (mode === BILLING_MODE_IMAGE && props.group.image_rate_independent) ||
    (mode === BILLING_MODE_VIDEO && props.group.video_rate_independent === true)
}

function discountFor(model: ModelPlazaGroup['models'][number]): string {
  if (hasIndependentRate(model)) return t('modelPlaza.card.independentRate')
  const rate = previewRate(model)
  const percent = Math.round(Math.abs(1 - rate) * 100)
  return rate < 1
    ? t('modelPlaza.card.discount', { percent })
    : t('modelPlaza.card.surcharge', { percent })
}

function formatPrice(value: number, scale: number): string {
  return formatScaled(value, scale, 2)
}

const discountLabel = computed(() => {
  const rates = props.group.models
    .filter((model) => previewPrice(model) != null)
    .map((model) => previewRate(model))
  if (rates.length === 0 || new Set(rates).size !== 1) return ''
  const rate = rates[0]
  if (!Number.isFinite(rate) || rate === 1) return ''
  const percent = Math.round(Math.abs(1 - rate) * 100)
  return rate < 1
    ? t('modelPlaza.card.discount', { percent })
    : t('modelPlaza.card.surcharge', { percent })
})

function billingMode(model: ModelPlazaGroup['models'][number]): BillingMode {
  return (model.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
}

function modelRate(model: ModelPlazaGroup['models'][number]): number {
  const mode = billingMode(model)
  if (mode === BILLING_MODE_IMAGE && props.group.image_rate_independent) return props.group.image_rate_multiplier ?? 1
  if (mode === BILLING_MODE_VIDEO && props.group.video_rate_independent) return props.group.video_rate_multiplier ?? 1
  return effectiveRate.value
}

/** 高峰窗口描述(含倍率与服务器时区标注);分组未启用高峰为空串。 */
const peakWindow = computed(() => {
  if (!hasPeakRate(props.group)) return ''
  return formatPeakRateWindow(
    props.group,
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
})

const peakNote = computed(() => {
  if (!peakWindow.value) return ''
  return t('modelPlaza.detail.peakNote', {
    window: peakWindow.value,
    multiplier: props.group.peak_rate_multiplier
  })
})

/**
 * 分组关闭了长上下文阶梯、但组内有模型官方带阶梯时提示:实付列只展示基础档,
 * 官方阶梯仅供参考。字段缺失(旧后端)不提示。
 */
const longContextNote = computed(() => {
  if (props.group.long_context_pricing_enabled !== false) return ''
  const hasOfficialLadder = props.group.models.some(
    (m) => (m.official_pricing?.intervals?.length ?? 0) > 1
  )
  return hasOfficialLadder ? t('modelPlaza.detail.longContextDisabledNote') : ''
})
</script>

<style scoped>
.plaza-details-enter-active,
.plaza-details-leave-active {
  overflow: hidden;
  transition: opacity 180ms ease, transform 180ms ease;
}

.plaza-details-enter-from,
.plaza-details-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

@media (prefers-reduced-motion: reduce) {
  .plaza-details-enter-active,
  .plaza-details-leave-active {
    transition: none;
  }
}
</style>
