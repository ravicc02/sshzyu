<template>
  <button class="rounded-lg bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-800 dark:text-dark-200" @click="open">
    sshzy {{ info?.current_version || version }}
    <span v-if="info?.has_update" class="ml-1 text-amber-700 dark:text-amber-300">↑</span>
  </button>
  <BaseDialog :show="visible" :title="t('customUpdate.title')" width="normal" :close-on-escape="!stepUp.visible.value" @close="visible = false">
    <div class="space-y-5 text-sm text-gray-700 dark:text-dark-200">
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2">
        <dt>{{ t('customUpdate.current') }}</dt><dd class="break-all font-medium">{{ info?.current_version || version }}</dd>
        <dt>{{ t('customUpdate.source') }}</dt><dd>ravicc02/sshzyu</dd>
        <dt>{{ t('customUpdate.baseline') }}</dt><dd>{{ info?.upstream_version || '—' }}</dd>
        <dt>Commit</dt><dd class="break-all font-mono text-xs">{{ info?.commit || '—' }}</dd>
      </dl>
      <p v-if="info?.official_notice?.has_update" class="rounded-lg bg-amber-50 p-3 text-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
        {{ t('customUpdate.officialNotice', { version: info.official_notice.version }) }}
      </p>
      <p v-if="info?.check_status !== 'verified'" class="rounded-lg bg-gray-100 p-3 dark:bg-dark-700">
        {{ t('customUpdate.notConfigured') }}
      </p>
      <p v-else-if="!info?.can_update">{{ t('customUpdate.activationDisabled') }}</p>
      <div v-if="releases.length && (!operation || terminalStages.has(operation.stage))" class="space-y-3">
        <label for="custom-update-version" class="block font-medium">{{ t('customUpdate.target') }}</label>
        <select id="custom-update-version" v-model="selectedHash" class="input w-full">
          <option v-for="target in releases" :key="target.manifest_hash" :value="target.manifest_hash">
            {{ target.manifest.version }}
          </option>
        </select>
        <p class="break-all font-mono text-xs">{{ selected?.manifest.source_sha }}</p>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-primary" :disabled="busy || !selected" @click="prepare('update')">{{ t('customUpdate.prepare') }}</button>
          <button class="btn btn-secondary" :disabled="busy || !selected" @click="prepare('rollback')">{{ t('customUpdate.prepareRollback') }}</button>
        </div>
      </div>
      <section v-if="operation" class="space-y-3" aria-live="polite">
        <h4 class="font-semibold">{{ operation.target.version }}</h4>
        <p>{{ t(`customUpdate.stages.${operation.stage}`) }}</p>
        <p v-if="reconnecting" role="status">{{ t('customUpdate.reconnecting') }}</p>
        <p v-if="operation.error" class="break-all rounded-lg bg-red-50 p-3 text-red-800 dark:bg-red-950/40 dark:text-red-200" role="alert">
          {{ operation.error }}
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
            <button class="btn btn-primary" :disabled="busy || !info?.can_update || !canConfirm" @click="activate">{{ t('customUpdate.activate') }}</button>
            <button class="btn btn-secondary" :disabled="busy" @click="cancel">{{ t('customUpdate.cancel') }}</button>
          </div>
        </template>
        <button v-if="operation.stage === 'completed' || operation.stage === 'rolled_back'" class="btn btn-primary" @click="reload">{{ t('customUpdate.reload') }}</button>
      </section>
      <p v-if="error" class="break-words text-red-700 dark:text-red-300" role="alert">{{ error }}</p>
      <button class="btn btn-secondary" :disabled="busy" @click="refresh">{{ t('customUpdate.check') }}</button>
    </div>
  </BaseDialog>
  <Teleport to="body"><TotpStepUpDialog :controller="stepUp" /></Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import BaseDialog from './BaseDialog.vue'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { isStepUpCancelled, useStepUp } from '@/composables/useStepUp'
import {
  activateCustomUpdate, cancelCustomUpdate, canConfirmOperation, getCustomOperation,
  listCustomReleases, prepareCustomUpdate, savedOperationID, terminalStages,
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
const releases = ref<CustomRelease[]>([])
const selectedHash = ref('')
const operation = ref<UpdateOperation | null>(null)
const confirmDowntime = ref(false)
const confirmMigrations = ref(false)
const selected = computed(() => releases.value.find(target => target.manifest_hash === selectedHash.value))
const canConfirm = computed(() => canConfirmOperation(operation.value, confirmDowntime.value, confirmMigrations.value))
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
let polls = 0

function messageOf(failure: unknown): string {
  const failureData = failure as { message?: string; reason?: string }
  return failureData.reason || failureData.message || t('customUpdate.failed')
}

async function open() {
  visible.value = true
  await refresh()
  const id = savedOperationID()
  if (id && !operation.value) await poll(id)
}

async function refresh() {
  busy.value = true
  error.value = ''
  try {
    const current = await app.fetchVersion(true)
    if (current?.check_status === 'verified') {
      releases.value = await listCustomReleases()
      selectedHash.value = current.custom_release?.manifest_hash || releases.value[0]?.manifest_hash || ''
    }
  } catch (failure) {
    error.value = messageOf(failure)
  } finally {
    busy.value = false
  }
}

async function prepare(kind: 'update' | 'rollback') {
  if (!selected.value || busy.value) return
  busy.value = true
  error.value = ''
  confirmDowntime.value = false
  confirmMigrations.value = false
  try {
    operation.value = await prepareCustomUpdate(selected.value, kind)
    polls = 0
    void poll(operation.value.id)
  } catch (failure) {
    error.value = messageOf(failure)
  } finally {
    busy.value = false
  }
}

async function activate() {
  if (!operation.value || busy.value || !canConfirm.value || !props.info?.can_update) return
  busy.value = true
  error.value = ''
  try {
    operation.value = await stepUp.run(() => activateCustomUpdate(operation.value!))
    polls = 0
    void poll(operation.value.id)
  } catch (failure) {
    if (!isStepUpCancelled(failure)) {
      error.value = messageOf(failure)
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
    operation.value = await cancelCustomUpdate(operation.value)
  } catch (failure) {
    error.value = messageOf(failure)
  } finally {
    busy.value = false
  }
}

async function poll(id: string) {
  if (timer) clearTimeout(timer)
  if (disposed) return
  try {
    const next = await getCustomOperation(id)
    if (disposed) return
    operation.value = next
    reconnecting.value = false
    if (terminalStages.has(next.stage) || next.stage === 'ready') return
  } catch {
    reconnecting.value = true
  }
  polls++
  if (!disposed && polls < 900) timer = setTimeout(() => void poll(id), 2000)
}

function reload() { window.location.reload() }

onBeforeUnmount(() => {
  disposed = true
  if (timer) clearTimeout(timer)
})
</script>
