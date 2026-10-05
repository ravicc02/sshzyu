<template>
  <button class="rounded-lg bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-800 dark:text-dark-200" @click="open">
    sshzy {{ currentVersion }}
    <span v-if="info?.has_update" class="ml-1 text-amber-700 dark:text-amber-300">↑</span>
  </button>
  <BaseDialog :show="visible" :title="t('customUpdate.title')" width="normal" :close-on-escape="!stepUp.visible.value" @close="visible = false">
    <div class="space-y-5 text-sm text-gray-700 dark:text-dark-200">
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2">
        <dt>{{ t('customUpdate.current') }}</dt><dd class="break-all font-medium">{{ currentVersion }}</dd>
        <dt>{{ t('customUpdate.source') }}</dt><dd>ravicc02/sshzyu</dd>
        <dt>{{ t('customUpdate.baseline') }}</dt><dd>{{ info?.upstream_version || '—' }}</dd>
      </dl>
      <details v-if="info?.commit" class="text-xs text-gray-500 dark:text-dark-400">
        <summary class="cursor-pointer">{{ t('customUpdate.versionDetails') }}</summary>
        <p class="mt-2 break-all font-mono">Commit: {{ info.commit }}</p>
      </details>
      <p v-if="info?.official_notice?.has_update" class="rounded-lg bg-amber-50 p-3 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
        {{ t('customUpdate.officialNotice', { version: info.official_notice.version }) }}
      </p>
      <p v-if="info?.check_status !== 'verified'" class="rounded-lg bg-gray-100 p-3 dark:bg-dark-700">
        {{ t('customUpdate.notConfigured') }}
      </p>
      <p v-else-if="!info?.can_update">{{ t('customUpdate.activationDisabled') }}</p>
      <div class="flex flex-wrap items-start gap-3">
        <button data-test="check-custom-update" class="btn btn-secondary" :disabled="busy || activeOperation" @click="refresh">{{ t('customUpdate.check') }}</button>
        <div class="space-y-1">
          <button data-test="rollback-custom-update" class="btn btn-secondary" :disabled="busy || activeOperation || !info?.can_update || !rollbackTarget" @click="prepare('rollback')">{{ t('customUpdate.rollback') }}</button>
          <p v-if="checked" data-test="rollback-version" class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('customUpdate.rollbackVersion') }}: {{ rollbackTarget?.manifest.version || t('customUpdate.noRollback') }}
          </p>
        </div>
      </div>
      <section v-if="checked && info?.check_status === 'verified' && !activeOperation" class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('customUpdate.latest') }}</p>
            <p data-test="latest-custom-version" class="mt-1 font-semibold">{{ latest?.manifest.version || currentVersion }}</p>
          </div>
          <button v-if="updateTarget" data-test="update-latest" class="btn btn-primary" :disabled="busy || !info?.can_update" @click="prepare('update')">{{ t('customUpdate.update') }}</button>
          <span v-else role="status">{{ t('customUpdate.upToDate') }}</span>
        </div>
      </section>
      <p v-if="notice" class="rounded-lg bg-gray-100 p-3 dark:bg-dark-700" role="status">{{ notice }}</p>
      <p v-if="reconnecting && !operation" role="status">{{ t('customUpdate.reconnecting') }}</p>
      <section v-if="operation" data-test="custom-operation" class="space-y-3" aria-live="polite">
        <h4 class="font-semibold">{{ t(operation.kind === 'rollback' ? 'customUpdate.rollbackTask' : 'customUpdate.updateTask') }} · {{ operation.target.version }}</h4>
        <p>{{ t(`customUpdate.stages.${operation.stage}`) }}</p>
        <p v-if="reconnecting" role="status">{{ t('customUpdate.reconnecting') }}</p>
        <p v-if="operation.error" class="break-all rounded-lg bg-red-50 p-3 text-red-800 dark:bg-red-950/40 dark:text-red-200" role="alert">
          {{ translatedCode(operation.error) }}
        </p>
        <template v-if="operation.stage === 'ready'">
          <p>{{ t('customUpdate.pendingMigrations', { count: operation.pending_migrations.length }) }}</p>
          <ul v-if="operation.pending_migrations.length" class="space-y-2">
            <li v-for="migration in operation.pending_migrations" :key="migration.filename" class="break-words">
              <span class="font-mono text-xs">{{ migration.filename }}</span>
              <p class="mt-1">{{ migration.description }}</p>
              <p v-if="migration.risk !== 'backward-compatible' || migration.non_transactional" class="text-red-700 dark:text-red-300">{{ t('customUpdate.manualMigration') }}</p>
            </li>
          </ul>
          <p class="rounded-lg bg-amber-50 p-3 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">{{ t('customUpdate.switchUpstream') }}</p>
          <label class="flex items-start gap-3">
            <input v-model="confirmDowntime" type="checkbox" class="mt-1 shrink-0" />
            <span>{{ t('customUpdate.confirmDowntime') }}</span>
          </label>
          <label v-if="operation.pending_migrations.length" class="flex items-start gap-3">
            <input v-model="confirmMigrations" type="checkbox" class="mt-1 shrink-0" />
            <span>{{ t('customUpdate.confirmMigrations') }}</span>
          </label>
          <div class="flex flex-wrap gap-2">
            <button class="btn btn-primary" :disabled="busy || !info?.can_update || !canConfirm" @click="activate">{{ t(operation.kind === 'rollback' ? 'customUpdate.confirmRollback' : 'customUpdate.activate') }}</button>
            <button class="btn btn-secondary" :disabled="busy" @click="cancel">{{ t('customUpdate.cancel') }}</button>
          </div>
        </template>
        <button v-if="operation.stage === 'completed' || operation.stage === 'rolled_back'" class="btn btn-primary" @click="reload">{{ t('customUpdate.reload') }}</button>
      </section>
      <p v-if="error" class="break-words text-red-700 dark:text-red-300" role="alert">{{ error }}</p>
      <details v-if="operation?.error || errorCode" class="text-xs text-gray-500 dark:text-dark-400">
        <summary class="cursor-pointer">{{ t('customUpdate.technicalDetails') }}</summary>
        <code class="mt-2 block break-all">{{ operation?.error || errorCode }}</code>
      </details>
    </div>
  </BaseDialog>
  <Teleport to="body"><TotpStepUpDialog :controller="stepUp" /></Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import BaseDialog from './BaseDialog.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import {
  activateCustomUpdate, cancelCustomUpdate, canConfirmOperation, getCustomOperation,
  listCustomReleases, prepareCustomUpdate, savedOperationID, terminalStages,
  clearSavedOperation, compareCustomVersions, latestCustomRelease, rollbackCustomRelease, operationStorageKey,
  type CustomRelease, type CustomVersionInfo, type UpdateOperation
} from '@/api/admin/customUpdate'

const props = defineProps<{ version: string; info: CustomVersionInfo | null }>()
const { t } = useI18n()
const app = useAppStore()
const stepUp = useStepUp()
const visible = ref(false)
const busy = ref(false)
const error = ref('')
const reconnecting = ref(false)
const checked = ref(false)
const refreshedInfo = ref<CustomVersionInfo | null>(props.info)
const info = computed(() => refreshedInfo.value || props.info)
const currentVersion = computed(() => info.value?.current_version || props.version)
const notice = ref('')
const errorCode = ref('')
const releases = ref<CustomRelease[]>([])
const operation = ref<UpdateOperation | null>(null)
const confirmDowntime = ref(false)
const confirmMigrations = ref(false)
const latest = computed(() => latestCustomRelease(releases.value))
const updateTarget = computed(() => latest.value && compareCustomVersions(latest.value.manifest.version, currentVersion.value)! > 0 ? latest.value : undefined)
const rollbackTarget = computed(() => rollbackCustomRelease(releases.value, currentVersion.value))
const activeOperation = computed(() => reconnecting.value || !!operation.value &&
  (!terminalStages.has(operation.value.stage) || operation.value.stage === 'manual_intervention'))
const canConfirm = computed(() => canConfirmOperation(operation.value, confirmDowntime.value, confirmMigrations.value))
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
let polls = 0
let pollingID = ''

watch(() => props.info, (value) => { if (value) refreshedInfo.value = value })

function translatedCode(code: string): string {
  const key = `customUpdate.errors.${code}`
  const translated = t(key)
  return translated === key ? t('customUpdate.failed') : translated
}

function codeOf(failure: unknown): string {
  if (!failure || typeof failure !== 'object') return ''
  const details = failure as { reason?: unknown; error?: unknown; code?: unknown; message?: unknown }
  return [details.reason, details.error, details.code, details.message]
    .find((value): value is string => typeof value === 'string' && /^[A-Z][A-Z_0-9]+$/.test(value)) || ''
}

function messageOf(failure: unknown): string {
  const code = codeOf(failure)
  return code ? translatedCode(code) : t('customUpdate.failed')
}

async function loadVersion(force: boolean): Promise<CustomVersionInfo> {
  const current = await app.fetchVersion(force)
  if (!current) throw new Error('VERSION_CHECK_FAILED')
  if (!disposed) refreshedInfo.value = current
  return current
}

async function open() {
  visible.value = true
  checked.value = false
  error.value = ''
  errorCode.value = ''
  notice.value = ''
  confirmDowntime.value = false
  confirmMigrations.value = false
  if (operation.value && terminalStages.has(operation.value.stage) && operation.value.stage !== 'manual_intervention') operation.value = null
  busy.value = true
  try {
    const current = await loadVersion(true)
    if (disposed) return
    if (current.active_operation) {
      operation.value = current.active_operation
      pollingID = operation.value.id
      localStorage.setItem(operationStorageKey, pollingID)
    }
  } catch (failure) {
    error.value = messageOf(failure)
  } finally {
    busy.value = false
  }
  const id = savedOperationID()
  if (operation.value) {
    await poll(operation.value.id)
  } else if (id) {
    pollingID = id
    await poll(id, true)
  }
}

async function refresh() {
  if (busy.value || activeOperation.value) return
  busy.value = true
  error.value = ''
  errorCode.value = ''
  notice.value = ''
  releases.value = []
  checked.value = false
  if (operation.value && terminalStages.has(operation.value.stage)) operation.value = null
  try {
    const current = await loadVersion(true)
    if (current?.check_status === 'verified') {
      const targets = await listCustomReleases()
      if (disposed) return
      releases.value = targets
      checked.value = true
    }
  } catch (failure) {
    if (disposed) return
    error.value = messageOf(failure)
    errorCode.value = codeOf(failure)
  } finally {
    busy.value = false
  }
}

async function prepare(kind: 'update' | 'rollback') {
  const target = kind === 'update' ? updateTarget.value : rollbackTarget.value
  if (!target || busy.value || activeOperation.value || !info.value?.can_update) return
  busy.value = true
  error.value = ''
  errorCode.value = ''
  notice.value = ''
  confirmDowntime.value = false
  confirmMigrations.value = false
  try {
    const next = await prepareCustomUpdate(target, kind)
    if (disposed) return
    operation.value = next
    pollingID = operation.value.id
    polls = 0
    void poll(operation.value.id)
  } catch (failure) {
    if (disposed) return
    if (codeOf(failure) === 'ALREADY_INSTALLED') {
      notice.value = t('customUpdate.alreadyInstalled')
      checked.value = false
      releases.value = []
      await loadVersion(true).catch(() => undefined)
      return
    }
    if (codeOf(failure) === 'UPDATE_IN_PROGRESS') {
      const current = await loadVersion(true).catch(() => undefined)
      if (disposed) return
      if (current?.active_operation) {
        operation.value = current.active_operation
        pollingID = operation.value.id
        localStorage.setItem(operationStorageKey, pollingID)
        polls = 0
        void poll(pollingID)
        return
      }
    }
    error.value = messageOf(failure)
    errorCode.value = codeOf(failure)
  } finally {
    busy.value = false
  }
}

async function activate() {
  if (!operation.value || busy.value || !canConfirm.value || !info.value?.can_update) return
  busy.value = true
  error.value = ''
  try {
    const pending = operation.value
    const next = await stepUp.run(() => activateCustomUpdate(pending))
    if (disposed) return
    operation.value = next
    polls = 0
    void poll(operation.value.id)
  } catch (failure) {
    if (!isStepUpCancelled(failure)) {
      error.value = messageOf(failure)
      errorCode.value = codeOf(failure)
      void poll(operation.value.id)
    }
  } finally {
    busy.value = false
  }
}

async function cancel() {
  if (!operation.value || busy.value) return
  busy.value = true
  try {
    const next = await cancelCustomUpdate(operation.value)
    if (disposed) return
    operation.value = next
    pollingID = ''
    if (timer) clearTimeout(timer)
    clearSavedOperation(operation.value.id)
  } catch (failure) {
    error.value = messageOf(failure)
  } finally {
    busy.value = false
  }
}

async function poll(id: string, restoring = false) {
  if (timer) clearTimeout(timer)
  if (disposed || pollingID !== id) return
  try {
    const next = await getCustomOperation(id)
    if (disposed || pollingID !== id) return
    if (terminalStages.has(next.stage) && next.stage !== 'manual_intervention') {
      clearSavedOperation(id)
      if (restoring) {
        reconnecting.value = false
        return
      }
    }
    if (next.error === 'ALREADY_INSTALLED' && next.stage === 'failed') {
      operation.value = null
      reconnecting.value = false
      checked.value = false
      releases.value = []
      notice.value = t('customUpdate.alreadyInstalled')
      await loadVersion(true).catch(() => undefined)
      return
    }
    operation.value = next
    reconnecting.value = false
    if (terminalStages.has(next.stage)) {
      await loadVersion(true).catch(() => undefined)
      return
    }
    if (next.stage === 'ready') return
  } catch (failure) {
    if (disposed || pollingID !== id) return
    if (codeOf(failure) === 'OPERATION_NOT_FOUND') {
      clearSavedOperation(id)
      reconnecting.value = false
      return
    }
    reconnecting.value = true
  }
  polls++
  if (!disposed && polls < 900) timer = setTimeout(() => void poll(id, restoring), 2000)
}

function reload() { window.location.reload() }

onBeforeUnmount(() => {
  disposed = true
  if (timer) clearTimeout(timer)
})
</script>
