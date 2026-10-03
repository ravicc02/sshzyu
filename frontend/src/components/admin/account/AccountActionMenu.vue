<template>
  <Teleport to="body">
    <div v-if="show && (anchorRect || position)">
      <!-- Backdrop: click anywhere outside to close -->
      <div class="fixed inset-0 z-[9998]" @click="emit('close')"></div>
      <div
        ref="menuRef"
        class="action-menu-content fixed z-[9999] w-52 overflow-y-auto overscroll-contain rounded-xl bg-white shadow-lg ring-1 ring-black/5 dark:bg-dark-800"
        :style="menuStyle"
        @click.stop
      >
        <div class="py-1">
          <template v-if="account">
            <button @click="$emit('test', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="play" size="sm" class="text-green-500" :stroke-width="2" />
              {{ t('admin.accounts.testConnection') }}
            </button>
            <button @click="$emit('stats', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="chart" size="sm" class="text-indigo-500" />
              {{ t('admin.accounts.viewStats') }}
            </button>
            <button @click="$emit('schedule', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="clock" size="sm" class="text-orange-500" />
              {{ t('admin.scheduledTests.schedule') }}
            </button>
            <button v-if="canDuplicate" @click="$emit('duplicate', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="copy" size="sm" class="text-sky-500" />
              {{ t('admin.accounts.duplicateAccount') }}
            </button>
            <!-- 影子账号不持凭据:重授权/刷新 token 对其无效(后端拒绝),故隐藏(外审 G4)。 -->
            <template v-if="(account.type === 'oauth' || account.type === 'setup-token') && !isShadow">
              <button @click="$emit('reauth', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-blue-600 hover:bg-gray-100 dark:hover:bg-dark-700">
                <Icon name="link" size="sm" />
                {{ t('admin.accounts.reAuthorize') }}
              </button>
              <button @click="$emit('refresh-token', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-purple-600 hover:bg-gray-100 dark:hover:bg-dark-700">
                <Icon name="refresh" size="sm" />
                {{ t('admin.accounts.refreshToken') }}
              </button>
            </template>
            <button v-if="isOpenAIOAuthParent" @click="$emit('create-spark-shadow', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-amber-600 hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="sparkles" size="sm" />
              {{ t('admin.accounts.createSparkShadow') }}
            </button>
            <button v-if="supportsPrivacy" @click="$emit('set-privacy', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-emerald-600 hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="shield" size="sm" />
              {{ t('admin.accounts.setPrivacy') }}
            </button>
            <div v-if="hasRecoverableState" class="my-1 border-t border-gray-100 dark:border-dark-700"></div>
            <button v-if="hasRecoverableState" @click="$emit('recover-state', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-emerald-600 hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="sync" size="sm" />
              {{ t('admin.accounts.recoverState') }}
            </button>
            <button v-if="hasQuotaLimit" @click="$emit('reset-quota', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-teal-600 hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="refresh" size="sm" />
              {{ t('admin.accounts.resetQuota') }}
            </button>
          </template>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import type { Account } from '@/types'

const props = defineProps<{
  show: boolean
  account: Account | null
  anchorRect?: DOMRect | null
  position?: { top: number; left: number } | null
}>()
const emit = defineEmits(['close', 'test', 'stats', 'schedule', 'duplicate', 'reauth', 'refresh-token', 'recover-state', 'reset-quota', 'set-privacy', 'create-spark-shadow'])
const { t } = useI18n()
const menuRef = ref<HTMLElement | null>(null)
const viewport = ref({ width: window.innerWidth, height: window.innerHeight })
const measuredPosition = ref({ top: 8, left: 8 })
let resizeObserver: ResizeObserver | undefined
const menuStyle = computed(() => ({
  top: `${props.anchorRect ? measuredPosition.value.top : props.position?.top ?? 8}px`,
  left: `${props.anchorRect ? measuredPosition.value.left : props.position?.left ?? 8}px`,
  maxHeight: `${Math.max(0, viewport.value.height - 16)}px`,
  maxWidth: `${Math.max(0, viewport.value.width - 16)}px`
}))

function updatePosition() {
  if (!props.show || !props.anchorRect || !menuRef.value) return
  const anchor = props.anchorRect
  const { width, height } = menuRef.value.getBoundingClientRect()
  const preferredLeft = viewport.value.width < 768
    ? anchor.left + anchor.width / 2 - width / 2
    : anchor.right - width
  const below = anchor.bottom + 4
  const preferredTop = below + height <= viewport.value.height - 8
    ? below
    : anchor.top - height - 4
  measuredPosition.value = {
    top: Math.max(8, Math.min(preferredTop, viewport.value.height - height - 8)),
    left: Math.max(8, Math.min(preferredLeft, viewport.value.width - width - 8))
  }
}

async function handleResize() {
  viewport.value = { width: window.innerWidth, height: window.innerHeight }
  await nextTick()
  updatePosition()
}
const canDuplicate = computed(() => {
  if (!props.account || props.account.parent_account_id != null) return false
  return ['apikey', 'upstream', 'bedrock', 'service_account'].includes(props.account.type)
})
const isRateLimited = computed(() => {
  if (props.account?.rate_limit_reset_at && new Date(props.account.rate_limit_reset_at) > new Date()) {
    return true
  }
  const modelLimits = (props.account?.extra as Record<string, unknown> | undefined)?.model_rate_limits as
    | Record<string, { rate_limit_reset_at: string }>
    | undefined
  if (modelLimits) {
    const now = new Date()
    return Object.values(modelLimits).some(info => new Date(info.rate_limit_reset_at) > now)
  }
  return false
})
const isOverloaded = computed(() => props.account?.overload_until && new Date(props.account.overload_until) > new Date())
const isTempUnschedulable = computed(() => props.account?.temp_unschedulable_until && new Date(props.account.temp_unschedulable_until) > new Date())
const hasRecoverableState = computed(() => {
  return props.account?.status === 'error' || Boolean(isRateLimited.value) || Boolean(isOverloaded.value) || Boolean(isTempUnschedulable.value)
})
const isAntigravityOAuth = computed(() => props.account?.platform === 'antigravity' && props.account?.type === 'oauth')
const isOpenAIOAuth = computed(() => props.account?.platform === 'openai' && props.account?.type === 'oauth')
// 影子账号(链接型,持 parent_account_id)不持凭据、type 不可变,凭据/隐私类操作对其无效。
const isShadow = computed(() => props.account?.parent_account_id != null)
// A "parent" OpenAI OAuth account is one that is NOT itself a shadow (parent_account_id == null)
const isOpenAIOAuthParent = computed(() => isOpenAIOAuth.value && !isShadow.value)
const supportsPrivacy = computed(() => (isAntigravityOAuth.value || isOpenAIOAuth.value) && !isShadow.value)
const hasQuotaLimit = computed(() => {
  return (props.account?.type === 'apikey' || props.account?.type === 'bedrock') && (
    (props.account?.quota_limit ?? 0) > 0 ||
    (props.account?.quota_daily_limit ?? 0) > 0 ||
    (props.account?.quota_weekly_limit ?? 0) > 0
  )
})

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') emit('close')
}

watch(() => [props.show, props.anchorRect, props.account], async () => {
  resizeObserver?.disconnect()
  window.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('resize', handleResize)
  if (!props.show) return
  viewport.value = { width: window.innerWidth, height: window.innerHeight }
  window.addEventListener('keydown', handleKeydown)
  window.addEventListener('resize', handleResize)
  await nextTick()
  if (!props.show || !menuRef.value) return
  updatePosition()
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(updatePosition)
    resizeObserver.observe(menuRef.value)
  }
}, { immediate: true })

onUnmounted(() => {
  resizeObserver?.disconnect()
  window.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('resize', handleResize)
})
</script>
