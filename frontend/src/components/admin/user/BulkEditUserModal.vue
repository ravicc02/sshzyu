<template>
  <BaseDialog :show="show" :title="t('admin.users.bulkLimits.title')" width="normal" :show-close-button="!submitting" @close="closeDialog">
    <form id="bulk-edit-user-limits-form" class="space-y-5" @submit.prevent="handleSubmit">
      <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.users.bulkLimits.selectedCount', { count: enableBalance && balanceTargets.length ? balanceTargets.length : selectedIds.length }) }}</p>
      <div class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-700 dark:border-dark-700">
        <div class="space-y-3 py-4">
          <div class="flex items-center justify-between gap-4">
            <label for="bulk-concurrency" class="input-label mb-0">{{ t('admin.users.columns.concurrency') }}</label>
            <Toggle v-model="enableConcurrency" :disabled="submitting || !!balanceAttempt" :aria-label="t('admin.users.bulkLimits.enableConcurrency')" data-test="enable-concurrency" />
          </div>
          <input v-if="enableConcurrency" id="bulk-concurrency" v-model="concurrencyValue" :disabled="submitting" type="number" min="0" step="1" class="input" data-test="concurrency-input" />
        </div>
        <div class="space-y-3 py-4">
          <div class="flex items-center justify-between gap-4">
            <label for="bulk-rpm-limit" class="input-label mb-0">{{ t('admin.users.form.rpmLimit') }}</label>
            <Toggle v-model="enableRPMLimit" :disabled="submitting || !!balanceAttempt" :aria-label="t('admin.users.bulkLimits.enableRPMLimit')" data-test="enable-rpm-limit" />
          </div>
          <div v-if="enableRPMLimit">
            <input id="bulk-rpm-limit" v-model="rpmLimitValue" :disabled="submitting" type="number" min="0" step="1" class="input" data-test="rpm-limit-input" />
            <p v-if="parsedRPMLimit === 0" class="input-hint">{{ t('admin.users.bulkLimits.unlimited') }}</p>
          </div>
        </div>
        <div class="space-y-3 py-4">
          <div class="flex items-center justify-between gap-4">
            <label class="input-label mb-0">{{ t('admin.users.bulkBalance.title') }}</label>
            <Toggle v-model="enableBalance" :disabled="submitting || !!balanceAttempt" :aria-label="t('admin.users.bulkBalance.enable')" data-test="enable-balance" />
          </div>
          <template v-if="enableBalance">
            <div class="grid gap-3 sm:grid-cols-2">
              <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
                <span>{{ t('admin.users.bulkBalance.operation') }}</span>
                <select v-model="balanceOperation" class="input" :disabled="submitting || !!balanceAttempt" data-test="balance-operation">
                  <option value="add">{{ t('admin.users.bulkBalance.add') }}</option>
                  <option value="subtract">{{ t('admin.users.bulkBalance.subtract') }}</option>
                </select>
              </label>
              <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
                <span>{{ t('admin.users.bulkBalance.amount') }}</span>
                <input v-model="balanceValue" type="number" min="0" step="0.00000001" class="input" :disabled="submitting || !!balanceAttempt" data-test="balance-input" />
              </label>
            </div>
            <label class="block space-y-1 text-sm text-gray-700 dark:text-gray-300">
              <span>{{ t('admin.users.bulkBalance.notes') }}</span>
              <input v-model="balanceNotes" type="text" maxlength="3900" class="input" :disabled="submitting || !!balanceAttempt" data-test="balance-notes" />
            </label>
            <p class="input-hint">{{ t('admin.users.bulkBalance.hint') }}</p>
            <p v-if="parsedBalance === null" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ t('admin.users.bulkBalance.invalidAmount') }}</p>
          </template>
        </div>
      </div>
      <p v-if="hasInvalidValue" class="text-sm text-red-600 dark:text-red-400">{{ t('admin.users.bulkLimits.nonNegativeInteger') }}</p>
      <p v-if="selectionTooLarge" class="text-sm text-red-600 dark:text-red-400">{{ t('admin.users.bulkLimits.selectionLimit', { max: MAX_BATCH_USER_IDS }) }}</p>
      <p v-if="balanceAttempt && !submitting" role="alert" class="text-sm text-amber-700 dark:text-amber-300">{{ t('admin.users.bulkBalance.unconfirmed') }}</p>
      <section v-if="balanceResults.length" class="space-y-3" aria-live="polite">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.users.bulkBalance.results') }}</h3>
        <ul class="max-h-64 divide-y divide-gray-200 overflow-y-auto rounded-lg border border-gray-200 dark:divide-dark-700 dark:border-dark-600">
          <li v-for="item in balanceResults" :key="item.user_id" class="space-y-1 px-3 py-2 text-sm" :data-test="'balance-result-' + item.user_id">
            <div class="flex items-center justify-between gap-3">
              <span>{{ t('admin.users.bulkBalance.user', { id: item.user_id }) }}</span>
              <span :class="item.success ? 'text-green-700 dark:text-green-300' : 'text-red-600 dark:text-red-400'">{{ t(item.success ? 'admin.users.bulkBalance.succeeded' : item.retryable === false ? 'admin.users.bulkBalance.reviewRequired' : 'admin.users.bulkBalance.failed') }}</span>
            </div>
            <p v-if="item.error" class="break-words text-gray-600 dark:text-gray-300">{{ item.error }}</p>
          </li>
        </ul>
      </section>
    </form>
    <template #footer>
      <div class="flex flex-wrap justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="submitting" @click="closeDialog">{{ t(balanceResults.length ? 'common.close' : 'common.cancel') }}</button>
        <button type="submit" form="bulk-edit-user-limits-form" class="btn btn-primary" :disabled="!canSubmit" data-test="submit">
          {{ submitting ? t('admin.users.bulkLimits.applying') : balanceAttempt ? t('admin.users.bulkBalance.retryOriginal') : balanceResults.length && enableBalance ? t(pendingBalanceIds.length ? 'admin.users.bulkBalance.retryFailed' : 'admin.users.bulkBalance.done') : t('admin.users.bulkLimits.apply') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { BatchUpdateBalanceRequest, BatchUpdateBalanceResponse, BatchUpdateUserLimitsRequest } from '@/api/admin/users'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'

const props = defineProps<{ show: boolean; selectedIds: number[] }>()
const emit = defineEmits<{ close: []; success: [affected: number]; balanceResult: [result: BatchUpdateBalanceResponse] }>()
const { t } = useI18n()
const appStore = useAppStore()
const enableConcurrency = ref(false)
const enableRPMLimit = ref(false)
const enableBalance = ref(false)
const concurrencyValue = ref<string | number>('')
const rpmLimitValue = ref<string | number>('')
const balanceOperation = ref<'add' | 'subtract'>('add')
const balanceValue = ref<string | number>('')
const balanceNotes = ref('')
const balanceTargets = ref<number[]>([])
const balanceResults = ref<BatchUpdateBalanceResponse['items']>([])
const balanceAttempt = ref<{ request: BatchUpdateBalanceRequest; key: string } | null>(null)
const submitting = ref(false)
const balanceNotice = ref('')
const MAX_BATCH_USER_IDS = 500
const parseLimit = (value: string | number): number | null | undefined => {
  if (!String(value).trim()) return undefined
  const number = Number(value)
  return Number.isInteger(number) && number >= 0 ? number : null
}
const parsedConcurrency = computed(() => enableConcurrency.value ? parseLimit(concurrencyValue.value) : undefined)
const parsedRPMLimit = computed(() => enableRPMLimit.value ? parseLimit(rpmLimitValue.value) : undefined)
const parsedBalance = computed(() => {
  if (!enableBalance.value || !String(balanceValue.value).trim()) return undefined
  const amount = Number(balanceValue.value)
  return Number.isFinite(amount) && amount > 0 ? amount : null
})
const hasInvalidValue = computed(() => parsedConcurrency.value === null || parsedRPMLimit.value === null)
const pendingBalanceIds = computed(() => balanceResults.value.length
  ? balanceResults.value.filter(item => !item.success && item.retryable !== false).map(item => item.user_id)
  : props.selectedIds)
const targetCount = computed(() => enableBalance.value ? (balanceAttempt.value?.request.user_ids.length ?? pendingBalanceIds.value.length) : props.selectedIds.length)
const selectionTooLarge = computed(() => targetCount.value > MAX_BATCH_USER_IDS)
const hasUpdate = computed(() => enableBalance.value
  ? !!balanceAttempt.value || (parsedBalance.value !== undefined && parsedBalance.value !== null)
  : (parsedConcurrency.value !== undefined && parsedConcurrency.value !== null) || (parsedRPMLimit.value !== undefined && parsedRPMLimit.value !== null))
const canSubmit = computed(() => targetCount.value > 0 && !selectionTooLarge.value && hasUpdate.value && !hasInvalidValue.value && !submitting.value)
watch(enableBalance, enabled => { if (enabled) { enableConcurrency.value=false; enableRPMLimit.value=false } })
watch([enableConcurrency, enableRPMLimit], values => { if (values.some(Boolean)) enableBalance.value=false })
const reset = () => {
  enableConcurrency.value=false; enableRPMLimit.value=false; enableBalance.value=false
  concurrencyValue.value=''; rpmLimitValue.value=''; balanceValue.value=''; balanceNotes.value=''
  balanceOperation.value='add'; balanceTargets.value=[]; balanceResults.value=[]; balanceAttempt.value=null
}
watch(() => props.show, shown => { if (shown && !submitting.value) reset() })
function closeDialog() {
  if (submitting.value) return
  if (balanceAttempt.value && !window.confirm(t('admin.users.bulkBalance.closeUnconfirmed'))) return
  emit('close')
}
async function handleSubmit() {
  if (!canSubmit.value) return
  if (enableBalance.value) {
    if (!balanceAttempt.value) {
      const amount = parsedBalance.value
      if (amount == null) return
      const ids = [...pendingBalanceIds.value]
      if (!window.confirm(t('admin.users.bulkBalance.confirm', { count: ids.length, amount, operation: t(balanceOperation.value === 'add' ? 'admin.users.bulkBalance.add' : 'admin.users.bulkBalance.subtract') }))) return
      if (!balanceTargets.value.length) balanceTargets.value=[...ids]
      balanceAttempt.value={ request:{ user_ids:ids, amount, operation:balanceOperation.value, notes:balanceNotes.value }, key:'balance-' + (globalThis.crypto?.randomUUID?.() || Date.now().toString(36)+'-'+Math.random().toString(36).slice(2)) }
    }
    submitting.value=true
    if (balanceNotice.value) { appStore.hideToast(balanceNotice.value); balanceNotice.value='' }
    try {
      const result=await adminAPI.users.batchUpdateBalance(balanceAttempt.value.request, balanceAttempt.value.key)
      const byUser=new Map(balanceResults.value.map(item => [item.user_id,item]))
      for (const item of result.items) byUser.set(item.user_id,item)
      balanceResults.value=[...byUser.values()]
      balanceAttempt.value=null
      emit('balanceResult',result)
      if (result.failed) balanceNotice.value=appStore.showError(t('admin.users.bulkBalance.partial',{ success:result.success, failed:result.failed }))
      else balanceNotice.value=appStore.showSuccess(t('admin.users.bulkBalance.success',{ count:result.success }))
    } catch (error: any) {
      appStore.showError(error?.response?.data?.detail || error?.message || t('admin.users.bulkBalance.unconfirmed'))
    } finally { submitting.value=false }
    return
  }
  const request: BatchUpdateUserLimitsRequest={ user_ids:[...props.selectedIds], all:false }
  const fields: string[]=[]
  if (parsedConcurrency.value != null) { request.concurrency=parsedConcurrency.value; fields.push(t('admin.users.bulkLimits.concurrencyValue',{ value:parsedConcurrency.value })) }
  if (parsedRPMLimit.value != null) { request.rpm_limit=parsedRPMLimit.value; fields.push(parsedRPMLimit.value === 0 ? t('admin.users.bulkLimits.rpmUnlimitedValue') : t('admin.users.bulkLimits.rpmValue',{ value:parsedRPMLimit.value })) }
  if (!window.confirm(t('admin.users.bulkLimits.confirm',{ count:props.selectedIds.length, fields:fields.join(', ') }))) return
  submitting.value=true
  try {
    const result=await adminAPI.users.batchUpdateLimits(request)
    appStore.showSuccess(t('admin.users.bulkLimits.success',{ count:result.affected }))
    emit('success',result.affected); emit('close')
  } catch (error: any) { appStore.showError(error?.response?.data?.message || error?.response?.data?.detail || t('admin.users.bulkLimits.failed')) }
  finally { submitting.value=false }
}
</script>
