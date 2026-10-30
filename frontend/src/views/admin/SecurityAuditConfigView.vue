<template>
  <div class="space-y-5">
    <section class="card overflow-hidden">
      <div class="flex flex-col gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
        <div class="flex min-w-0 items-center gap-4">
          <button
            type="button"
            role="switch"
            :aria-checked="masterEnabled"
            :aria-label="t('admin.securityAudit.unified.masterSwitch')"
            class="relative inline-flex h-7 w-12 shrink-0 items-center rounded-full border-2 border-transparent transition-colors"
            :class="masterEnabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'"
            @click="masterEnabled = !masterEnabled"
          >
            <span
              class="inline-block h-5 w-5 rounded-full bg-white shadow transition-transform"
              :class="masterEnabled ? 'translate-x-5' : 'translate-x-0'"
            />
          </button>
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-lg font-semibold text-gray-950 dark:text-white">{{ t('admin.securityAudit.unified.title') }}</h2>
              <span class="rounded-full px-2.5 py-0.5 text-xs font-medium" :class="masterEnabled ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'">
                {{ masterEnabled ? t('admin.securityAudit.unified.enabled') : t('admin.securityAudit.unified.disabled') }}
              </span>
            </div>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.securityAudit.unified.description') }}</p>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-secondary" :disabled="loading || saving" @click="loadAll">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('common.reset') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="loading || saving || !promptConfig" @click="saveAll">
            <Icon name="check" size="sm" />
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>
      <div v-else class="p-5">
        <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <div class="rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-900/40">
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.securityAudit.unified.process') }}</p>
            <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ processStatus }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-900/40">
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.securityAudit.unified.mode') }}</p>
            <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ effectiveMode }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-900/40">
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.securityAudit.unified.version') }}</p>
            <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ activeVersion }} / {{ expectedVersion }}</p>
          </div>
          <div class="rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-900/40">
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.securityAudit.unified.guardMetrics') }}</p>
            <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">{{ allowedCount }} / {{ blockedCount }}</p>
            <p class="mt-0.5 text-[11px] text-gray-500 dark:text-dark-400">{{ t('admin.securityAudit.unified.allowBlock') }}</p>
          </div>
          <label class="rounded-lg bg-gray-50 px-4 py-3 dark:bg-dark-900/40">
            <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.securityAudit.unified.queueCapacity') }}</span>
            <input v-model.number="queueCapacity" class="mt-1 w-full border-0 bg-transparent p-0 text-sm font-semibold text-gray-900 focus:ring-0 dark:text-white" type="number" min="1" max="100000" />
          </label>
        </div>
      </div>
    </section>

    <div class="grid gap-5 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,.85fr)]">
      <section class="card">
        <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.securityAudit.unified.scope') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.securityAudit.unified.scopeHint') }}</p>
        </div>
        <div class="space-y-4 p-5">
          <div class="flex flex-wrap gap-5 text-sm text-gray-700 dark:text-dark-200">
            <label class="flex items-center gap-2">
              <input v-model="allGroups" type="radio" :value="true" />
              {{ t('admin.riskControl.allGroups') }}
            </label>
            <label class="flex items-center gap-2">
              <input v-model="allGroups" type="radio" :value="false" />
              {{ t('admin.riskControl.selectedGroups') }}
            </label>
          </div>
          <div v-if="!allGroups">
            <input v-model.trim="groupSearch" type="search" class="input w-full" :placeholder="t('admin.riskControl.searchGroups')" />
            <div class="mt-3 grid max-h-64 gap-1 overflow-y-auto rounded-lg border border-gray-200 p-2 dark:border-dark-700 sm:grid-cols-2">
              <label v-for="group in filteredGroups" :key="group.id" class="flex min-w-0 items-center justify-between gap-3 rounded-md px-2 py-2 text-sm hover:bg-gray-50 dark:hover:bg-dark-800">
                <span class="flex min-w-0 items-center gap-2">
                  <input type="checkbox" :checked="groupIds.includes(group.id)" @change="toggleGroup(group.id)" />
                  <span class="truncate text-gray-800 dark:text-dark-100">{{ group.name }}</span>
                </span>
                <span class="flex-shrink-0 text-xs text-gray-500 dark:text-dark-400">{{ group.platform }}</span>
              </label>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.securityAudit.unified.selectedGroups', { count: groupIds.length }) }}</p>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.securityAudit.unified.keywords') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.securityAudit.unified.keywordsHint') }}</p>
        </div>
        <div class="p-5">
          <div class="rounded-xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-900/30">
            <div class="flex items-start justify-between gap-4">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.securityAudit.unified.keywordCount', { count: keywords.length }) }}</p>
                <div class="mt-3 flex max-h-24 flex-wrap gap-1.5 overflow-hidden">
                  <span v-for="keyword in keywords.slice(0, 10)" :key="keyword" class="rounded-md bg-white px-2 py-1 text-xs text-gray-600 shadow-sm dark:bg-dark-800 dark:text-dark-300">
                    {{ keyword }}
                  </span>
                  <span v-if="keywords.length > 10" class="rounded-md bg-white px-2 py-1 text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">
                    +{{ keywords.length - 10 }}
                  </span>
                </div>
              </div>
              <button type="button" class="btn btn-secondary btn-sm flex-shrink-0" @click="openKeywordEditor">
                <Icon name="edit" size="sm" />
                {{ t('common.edit') }}
              </button>
            </div>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.securityAudit.unified.rules') }}</h2>
        </div>
        <div class="divide-y divide-gray-100 dark:divide-dark-700">
          <details class="group px-5 py-4">
            <summary class="flex cursor-pointer list-none items-center justify-between gap-3">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.securityAudit.unified.thresholds') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.securityAudit.unified.thresholdsHint') }}</p>
              </div>
              <Icon name="chevronDown" size="sm" class="transition-transform group-open:rotate-180" />
            </summary>
            <div class="mt-4 grid gap-3 sm:grid-cols-2">
              <label v-for="category in thresholdCategories" :key="category" class="space-y-1">
                <span class="block truncate font-mono text-[11px] text-gray-500 dark:text-dark-400">{{ category }}</span>
                <input v-model.number="thresholds[category]" class="input w-full" type="number" min="0" max="100" step="0.1" />
              </label>
            </div>
          </details>

          <div class="px-5 py-4">
            <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.securityAudit.unified.notification') }}</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.securityAudit.unified.notificationHint') }}</p>
            <div class="mt-4 grid gap-4 sm:grid-cols-2">
              <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
                <span>{{ t('admin.securityAudit.unified.blockStatus') }}</span>
                <input v-model.number="blockStatus" class="input w-full" type="number" min="400" max="599" />
              </label>
              <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
                <span>{{ t('admin.securityAudit.unified.banThreshold') }}</span>
                <input v-model.number="banThreshold" class="input w-full" type="number" min="1" max="1000" :disabled="!autoBanEnabled" />
              </label>
              <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200 sm:col-span-2">
                <span>{{ t('admin.securityAudit.unified.blockMessage') }}</span>
                <input v-model.trim="blockMessage" class="input w-full" maxlength="500" />
              </label>
              <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
                <span>{{ t('admin.securityAudit.unified.violationWindow') }}</span>
                <input v-model.number="violationWindowHours" class="input w-full" type="number" min="1" max="8760" :disabled="!autoBanEnabled" />
              </label>
              <div class="space-y-3 pt-1">
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
                  <input v-model="emailOnHit" type="checkbox" />
                  {{ t('admin.securityAudit.unified.emailOnHit') }}
                </label>
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
                  <input v-model="autoBanEnabled" type="checkbox" />
                  {{ t('admin.securityAudit.unified.autoBan') }}
                </label>
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
                  <input v-model="cyberPolicyExcludeFromBanCount" type="checkbox" :disabled="!autoBanEnabled" />
                  {{ t('admin.securityAudit.unified.cyberBanExclude') }}
                </label>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.securityAudit.unified.records') }}</h2>
        </div>
        <div class="grid gap-4 p-5 sm:grid-cols-2">
          <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
            <span>{{ t('admin.securityAudit.unified.hitRetention') }}</span>
            <input v-model.number="hitRetentionDays" class="input w-full" type="number" min="1" max="3650" />
          </label>
          <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
            <span>{{ t('admin.securityAudit.unified.nonHitRetention') }}</span>
            <input v-model.number="nonHitRetentionDays" class="input w-full" type="number" min="1" max="3" />
          </label>
          <label class="space-y-1 text-sm text-gray-700 dark:text-dark-200">
            <span>{{ t('admin.securityAudit.unified.sampleRate') }}</span>
            <input v-model.number="sampleRate" class="input w-full" type="number" min="0" max="100" />
          </label>
          <label class="flex items-center gap-2 self-end pb-2 text-sm text-gray-700 dark:text-dark-200">
            <input v-model="recordNonHits" type="checkbox" />
            {{ t('admin.securityAudit.unified.recordNonHits') }}
          </label>
        </div>
      </section>
    </div>

    <section v-if="promptConfig" class="card">
      <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.securityAudit.unified.modelAudit') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-300">{{ t('admin.securityAudit.unified.modelAuditHint') }}</p>
      </div>
      <div class="space-y-5 p-5">
        <EndpointPool
          :endpoints="promptConfig.endpoints"
          :probe-results="probeResults"
          :probing-ids="probingIds"
          @update:endpoints="updateEndpoints"
          @probe="runProbe"
        />

        <div class="grid gap-4 lg:grid-cols-2">
          <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
            <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.securityAudit.unified.modelPolicy') }}</h3>
            <div class="mt-3 space-y-3 text-sm text-gray-700 dark:text-dark-200">
              <label class="flex items-center gap-2">
                <input v-model="latestTurnOnly" type="checkbox" />
                {{ t('admin.promptAudit.saveBar.blockingLatestTurnOnly') }}
              </label>
              <label class="flex items-center gap-2">
                <input v-model="storePassEvents" type="checkbox" />
                {{ t('admin.promptAudit.saveBar.storePass') }}
              </label>
            </div>
          </div>

          <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/40">
            <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.securityAudit.unified.categories') }}</h3>
            <div class="mt-3 grid gap-2 sm:grid-cols-2">
              <label v-for="scanner in SCANNER_CATALOG" :key="scanner.id" class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
                <input type="checkbox" :checked="selectedScanners.includes(scanner.id)" @change="toggleScanner(scanner.id)" />
                {{ t(`admin.promptAudit.scanners.${scanner.id}`) }}
              </label>
            </div>
          </div>
        </div>
      </div>
    </section>

    <BaseDialog :show="showKeywordEditor" :title="t('admin.securityAudit.unified.editKeywords')" width="wide" @close="showKeywordEditor = false">
      <div>
        <p class="text-sm text-gray-500 dark:text-dark-300">{{ t('admin.securityAudit.unified.keywordEditorHint') }}</p>
        <textarea v-model="keywordEditorText" class="input mt-3 min-h-56 w-full font-mono text-sm leading-6" />
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" @click="showKeywordEditor = false">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" @click="applyKeywordEditor">{{ t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { ContentModerationConfig, ContentModerationRuntimeStatus, UpdateContentModerationConfig } from '@/api/admin/riskControl'
import type { AdminGroup } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import EndpointPool from '@/features/prompt-audit/components/EndpointPool.vue'
import promptAuditAPI from '@/features/prompt-audit/api'
import { buildUpdateRequest, configToDraft, SCANNER_CATALOG } from '@/features/prompt-audit/viewModel'
import type { PromptAuditDraft, PromptAuditEndpointDraft, PromptAuditRuntime, PromptProbeResult } from '@/features/prompt-audit/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const thresholdDefaults: Record<string, number> = {
  harassment: 98,
  'harassment/threatening': 90,
  hate: 65,
  'hate/threatening': 65,
  illicit: 95,
  'illicit/violent': 95,
  'self-harm': 65,
  'self-harm/intent': 85,
  'self-harm/instructions': 65,
  sexual: 65,
  'sexual/minors': 65,
  violence: 95,
  'violence/graphic': 95,
}
const thresholdCategories = Object.keys(thresholdDefaults)

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const promptConfig = ref<PromptAuditDraft | null>(null)
const promptRuntime = ref<PromptAuditRuntime | null>(null)
const contentConfig = ref<ContentModerationConfig | null>(null)
const contentStatus = ref<ContentModerationRuntimeStatus | null>(null)
const groups = ref<AdminGroup[]>([])
const groupSearch = ref('')
const masterEnabled = ref(false)
const allGroups = ref(true)
const groupIds = ref<number[]>([])
const keywords = ref<string[]>([])
const queueCapacity = ref(32768)
const thresholds = reactive<Record<string, number>>({ ...thresholdDefaults })
const blockStatus = ref(403)
const blockMessage = ref('')
const emailOnHit = ref(true)
const autoBanEnabled = ref(true)
const banThreshold = ref(10)
const violationWindowHours = ref(720)
const cyberPolicyExcludeFromBanCount = ref(false)
const hitRetentionDays = ref(180)
const nonHitRetentionDays = ref(3)
const sampleRate = ref(100)
const recordNonHits = ref(false)
const latestTurnOnly = ref(false)
const storePassEvents = ref(false)
const selectedScanners = ref<string[]>([])
const probingIds = ref<string[]>([])
const probeResults = reactive<Record<string, PromptProbeResult>>({})
const showKeywordEditor = ref(false)
const keywordEditorText = ref('')

const filteredGroups = computed(() => {
  const query = groupSearch.value.toLowerCase()
  if (!query) return groups.value
  return groups.value.filter((group) => `${group.name} ${group.id} ${group.platform}`.toLowerCase().includes(query))
})
const processStatus = computed(() => {
  const status = promptRuntime.value?.process_status || 'disabled'
  const key = `admin.promptAudit.status.${status}`
  const translated = t(key)
  return translated === key ? status : translated
})
const effectiveMode = computed(() => {
  const mode = promptRuntime.value?.effective_mode || 'off'
  return t(`admin.promptAudit.mode.${mode}`)
})
const activeVersion = computed(() => promptRuntime.value?.active_config_version ?? '-')
const expectedVersion = computed(() => promptRuntime.value?.expected_config_version ?? '-')
const allowedCount = computed(() => promptRuntime.value?.guard_metrics.allowed ?? 0)
const blockedCount = computed(() => promptRuntime.value?.guard_metrics.blocked ?? 0)

async function loadAll() {
  loading.value = true
  try {
    const [content, prompt, runtime, filterRuntime, groupItems] = await Promise.all([
      adminAPI.riskControl.getConfig(),
      promptAuditAPI.getConfig(),
      promptAuditAPI.getRuntime(),
      adminAPI.riskControl.getStatus(),
      adminAPI.groups.getAll(),
    ])
    contentConfig.value = content
    promptConfig.value = configToDraft(prompt)
    promptRuntime.value = runtime
    contentStatus.value = filterRuntime
    groups.value = groupItems
    masterEnabled.value = content.enabled || prompt.enabled
    allGroups.value = content.all_groups && prompt.all_groups
    groupIds.value = [...new Set([...(content.group_ids ?? []), ...(prompt.group_ids ?? [])])]
    keywords.value = [...(prompt.keywords ?? [])]
    queueCapacity.value = Math.max(1, content.queue_size || prompt.queue_capacity || 32768)
    blockMessage.value = content.block_message || t('admin.riskControl.defaultBlockMessage')
    blockStatus.value = content.block_status || 403
    emailOnHit.value = content.email_on_hit
    autoBanEnabled.value = content.auto_ban_enabled
    banThreshold.value = content.ban_threshold || 10
    violationWindowHours.value = content.violation_window_hours || 720
    cyberPolicyExcludeFromBanCount.value = content.cyber_policy_exclude_from_ban_count
    hitRetentionDays.value = content.hit_retention_days
    nonHitRetentionDays.value = content.non_hit_retention_days
    sampleRate.value = content.sample_rate
    recordNonHits.value = content.record_non_hits
    latestTurnOnly.value = prompt.blocking_latest_turn_only
    storePassEvents.value = prompt.store_pass_events
    selectedScanners.value = [...(prompt.scanners ?? [])]
    for (const [category, value] of Object.entries(content.thresholds ?? {})) {
      thresholds[category] = Number((value * 100).toFixed(1))
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.securityAudit.unified.loadFailed')))
  } finally {
    loading.value = false
  }
}

function toggleGroup(id: number) {
  const index = groupIds.value.indexOf(id)
  if (index >= 0) groupIds.value.splice(index, 1)
  else groupIds.value.push(id)
}

function toggleScanner(id: string) {
  const index = selectedScanners.value.indexOf(id)
  if (index >= 0) selectedScanners.value.splice(index, 1)
  else selectedScanners.value.push(id)
}

function openKeywordEditor() {
  keywordEditorText.value = keywords.value.join(', ')
  showKeywordEditor.value = true
}

function applyKeywordEditor() {
  const seen = new Set<string>()
  keywords.value = keywordEditorText.value
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter((item) => {
      if (!item || seen.has(item.toLowerCase())) return false
      seen.add(item.toLowerCase())
      return true
    })
    .slice(0, 500)
  showKeywordEditor.value = false
}

function updateEndpoints(endpoints: PromptAuditEndpointDraft[]) {
  if (promptConfig.value) promptConfig.value = { ...promptConfig.value, endpoints }
}

async function runProbe(endpoint: PromptAuditEndpointDraft) {
  if (probingIds.value.includes(endpoint.id)) return
  probingIds.value.push(endpoint.id)
  try {
    probeResults[endpoint.id] = await promptAuditAPI.probeEndpoint(endpoint)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.promptAudit.errors.probe')))
  } finally {
    probingIds.value = probingIds.value.filter((id) => id !== endpoint.id)
  }
}

async function saveAll() {
  if (!promptConfig.value || !contentConfig.value) return
  saving.value = true
  try {
    const thresholdPayload = Object.fromEntries(thresholdCategories.map((category) => [
      category,
      Number((Math.min(100, Math.max(0, thresholds[category] ?? thresholdDefaults[category])) / 100).toFixed(4)),
    ]))
    const contentPayload: UpdateContentModerationConfig = {
      enabled: masterEnabled.value,
      mode: contentConfig.value.mode,
      all_groups: allGroups.value,
      group_ids: allGroups.value ? [] : [...groupIds.value],
      queue_size: queueCapacity.value,
      sample_rate: sampleRate.value,
      record_non_hits: recordNonHits.value,
      thresholds: thresholdPayload,
      block_status: blockStatus.value,
      block_message: blockMessage.value,
      email_on_hit: emailOnHit.value,
      auto_ban_enabled: autoBanEnabled.value,
      ban_threshold: banThreshold.value,
      violation_window_hours: violationWindowHours.value,
      cyber_policy_exclude_from_ban_count: cyberPolicyExcludeFromBanCount.value,
      hit_retention_days: hitRetentionDays.value,
      non_hit_retention_days: nonHitRetentionDays.value,
      blocked_keywords: [],
      keyword_blocking_mode: 'api_only',
    }
    const nextPrompt = {
      ...promptConfig.value,
      enabled: masterEnabled.value,
      blocking_enabled: masterEnabled.value,
      blocking_latest_turn_only: latestTurnOnly.value,
      store_pass_events: storePassEvents.value,
      block_message: blockMessage.value,
      all_groups: allGroups.value,
      group_ids: allGroups.value ? [] : [...groupIds.value],
      keywords: [...keywords.value],
      queue_capacity: queueCapacity.value,
      scanners: [...selectedScanners.value],
    }
    await Promise.all([
      adminAPI.riskControl.updateConfig(contentPayload),
      promptAuditAPI.updateConfig(buildUpdateRequest(nextPrompt)),
    ])
    appStore.showSuccess(t('admin.securityAudit.unified.saved'))
    await loadAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.securityAudit.unified.saveFailed')))
  } finally {
    saving.value = false
  }
}

onMounted(loadAll)
</script>
