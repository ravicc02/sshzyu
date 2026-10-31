<template>
  <section class="mx-auto w-full max-w-6xl space-y-5 px-1 py-2 sm:px-2">
    <header
      class="page-header mb-0 flex flex-wrap items-center justify-between gap-3 rounded-3xl bg-white p-5 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 sm:p-6"
    >
      <div class="min-w-0">
        <h2 class="page-title flex items-center gap-2 text-xl font-black text-gray-900 dark:text-white">
          <span class="inline-flex h-8 w-8 items-center justify-center rounded-xl bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400">
            <Icon name="chart" size="sm" />
          </span>
          {{ t('channelMonitorV2.settings.title') }}
        </h2>
        <p class="page-description mt-1.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t('channelMonitorV2.settings.description') }}
        </p>
      </div>
      <button
        type="button"
        class="btn btn-primary"
        :disabled="saving || !dirty"
        @click="save"
      >
        <Icon name="check" size="sm" />
        {{ t('channelMonitorV2.settings.save') }}
      </button>
    </header>

    <div
      v-if="loading"
      class="card flex min-h-[200px] items-center justify-center !rounded-3xl !border-0 text-sm text-gray-400 shadow-sm ring-1 ring-gray-900/5 dark:ring-dark-700"
    >
      <span class="animate-pulse">{{ t('channelMonitorV2.settings.loading') }}</span>
    </div>

    <template v-else-if="draft">
      <div class="card divide-y divide-gray-100 !rounded-3xl !border-0 shadow-sm ring-1 ring-gray-900/5 dark:divide-dark-700 dark:!bg-dark-800 dark:ring-dark-700">
        <div class="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
          <div>
            <strong class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('channelMonitorV2.settings.enableTitle') }}</strong>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
              {{ t('channelMonitorV2.settings.enableHint') }}
            </p>
          </div>
          <Toggle v-model="draft.enabled" />
        </div>
        <div class="flex flex-wrap items-center justify-between gap-4 px-5 py-4">
          <div>
            <strong class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('channelMonitorV2.settings.refreshTitle') }}</strong>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('channelMonitorV2.settings.refreshHint') }}</p>
          </div>
          <div class="tabs inline-flex w-auto" role="group" :aria-label="t('channelMonitorV2.settings.refreshAria')">
            <button
              type="button"
              class="tab"
              :class="draft.refresh_interval_seconds === 60 ? 'tab-active' : ''"
              @click="draft.refresh_interval_seconds = 60"
            >
              1 min
            </button>
            <button
              type="button"
              class="tab"
              :class="draft.refresh_interval_seconds === 300 ? 'tab-active' : ''"
              @click="draft.refresh_interval_seconds = 300"
            >
              5 min
            </button>
          </div>
        </div>
      </div>

      <div class="card overflow-hidden !rounded-3xl !border-0 shadow-sm ring-1 ring-gray-900/5 dark:!bg-dark-800 dark:ring-dark-700">
        <div class="card-header !py-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('channelMonitorV2.settings.platformsTitle') }}</h3>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
            {{ t('channelMonitorV2.settings.platformsHint') }}
          </p>
          <div class="mt-3 flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="saving || availablePlatforms.length === 0"
              @click="openPlatformPicker"
            >
              <Icon name="plus" size="sm" />
              {{ t('channelMonitorV2.settings.addPlatform') }}
            </button>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('channelMonitorV2.settings.platformsSelected', { count: draft.platforms.length }) }}
            </span>
          </div>
        </div>
        <div class="divide-y divide-gray-100 dark:divide-dark-700">
          <div
            v-for="platform in draft.platforms"
            :key="platform.platform"
            class="grid grid-cols-1 items-center gap-3 px-5 py-3 sm:grid-cols-[auto_7rem_minmax(0,1fr)_auto_auto]"
          >
            <Toggle v-model="platform.enabled" />
            <strong class="text-sm font-medium text-gray-900 dark:text-white">{{ platformLabel(platform.platform) }}</strong>
            <input
              class="input"
              :value="platform.models.join(', ')"
              type="text"
              :placeholder="t('channelMonitorV2.settings.modelsPlaceholder')"
              @change="setModels(platform, $event)"
            />
            <button
              type="button"
              class="btn btn-ghost btn-sm whitespace-nowrap"
              :disabled="saving || syncingPlatform === platform.platform"
              @click="syncPlatformModels(platform)"
            >
              {{ syncingPlatform === platform.platform ? t('channelMonitorV2.settings.modelSyncing') : t('channelMonitorV2.settings.modelSync') }}
            </button>
            <button
              type="button"
              class="rounded-lg p-2 text-gray-400 transition hover:bg-rose-50 hover:text-rose-600 dark:hover:bg-rose-500/10"
              :aria-label="t('channelMonitorV2.settings.removePlatform', { platform: platformLabel(platform.platform) })"
              @click="removePlatform(platform.platform)"
            >
              <Icon name="x" size="sm" />
            </button>
          </div>
          <p v-if="draft.platforms.length === 0" class="px-5 py-7 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('channelMonitorV2.settings.platformsEmpty') }}</p>
        </div>
      </div>

      <div class="card overflow-hidden !rounded-lg !border-0 shadow-sm ring-1 ring-gray-900/5 dark:!bg-dark-800 dark:ring-dark-700">
        <div class="card-header flex flex-wrap items-center justify-between gap-2 !py-3">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('channelMonitorV2.settings.groupsTitle') }}</h3>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
              {{
                draft.group_ids.length
                  ? t('channelMonitorV2.settings.groupsSelected', { count: draft.group_ids.length })
                  : t('channelMonitorV2.settings.groupsAll')
              }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('channelMonitorV2.settings.groupsHint') }}</p>
          </div>
          <div class="flex items-center gap-1">
            <button type="button" class="btn btn-ghost btn-sm" :disabled="refreshingGroups" @click="refreshMonitorGroups">
              <Icon name="refresh" size="sm" :class="refreshingGroups ? 'animate-spin' : ''" />
              {{ refreshingGroups ? t('channelMonitorV2.settings.groupsRefreshing') : t('channelMonitorV2.settings.groupsRefresh') }}
            </button>
            <button v-if="selectableGroups.length" type="button" class="btn btn-ghost btn-sm" @click="selectAllVisibleGroups">
              {{ t('channelMonitorV2.settings.groupsSelectAll') }}
            </button>
            <button v-if="draft.group_ids.length" type="button" class="btn btn-ghost btn-sm" @click="draft.group_ids = []">
              {{ t('channelMonitorV2.settings.groupsClear') }}
            </button>
          </div>
        </div>
        <div class="border-t border-gray-100 px-4 py-3 dark:border-dark-700">
          <input v-model.trim="groupSearch" type="search" class="input w-full sm:max-w-xs" :aria-label="t('channelMonitorV2.settings.searchGroups')" :placeholder="t('channelMonitorV2.settings.searchGroups')" />
        </div>
        <div class="max-h-[min(55vh,460px)] overflow-y-auto px-4 py-2">
          <section v-for="section in groupedOptions" :key="section.platform" class="border-b border-gray-100 py-3 last:border-b-0 dark:border-dark-700" :aria-label="platformLabel(section.platform)">
            <h4 class="mb-2 flex items-center gap-2 text-xs font-semibold text-gray-700 dark:text-gray-200">
              {{ platformLabel(section.platform) }}
              <span class="font-normal tabular-nums text-gray-400">{{ section.groups.length }}</span>
            </h4>
            <div class="grid grid-cols-1 gap-1 sm:grid-cols-2">
              <label v-for="group in section.groups" :key="group.id" class="flex min-w-0 items-center gap-2 rounded-md px-2 py-2 text-sm hover:bg-gray-50 dark:hover:bg-dark-700" :class="group.status !== 'active' && !draft.group_ids.includes(group.id) ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'">
                <input type="checkbox" class="h-4 w-4 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500/40" :checked="draft.group_ids.includes(group.id)" :disabled="group.status !== 'active' && !draft.group_ids.includes(group.id)" @change="toggleGroup(group.id)" />
                <span class="min-w-0 flex-1 truncate font-medium text-gray-800 dark:text-gray-100" :title="group.name">{{ group.name }}</span>
                <small v-if="group.status !== 'active'" class="shrink-0 text-xs text-gray-500">{{ t('channelMonitorV2.settings.inactiveGroup') }}</small>
                <small class="shrink-0 text-xs text-gray-400">#{{ group.id }}</small>
              </label>
            </div>
          </section>
          <p v-if="selectableGroups.length === 0" class="empty-state py-8 text-sm text-gray-400">{{ t('channelMonitorV2.settings.groupsEmpty') }}</p>
          <p v-else-if="!groupedOptions.length" class="py-6 text-center text-xs text-gray-500">{{ t('channelMonitorV2.settings.noMatchingGroups') }}</p>
        </div>
      </div>

      <slot name="probe-monitor" :groups="selectedMonitorGroups" />

      <div class="card overflow-hidden !rounded-3xl !border-0 shadow-sm ring-1 ring-gray-900/5 dark:!bg-dark-800 dark:ring-dark-700">
        <div class="card-header !py-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('channelMonitorV2.settings.errorsTitle') }}</h3>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
            {{ t('channelMonitorV2.settings.errorsHint') }}
          </p>
        </div>
        <div class="max-h-[min(40vh,320px)] overflow-y-auto px-3 py-2 sm:px-4">
          <div class="grid grid-cols-1 gap-1 sm:grid-cols-2">
            <label
              v-for="category in errorCategories"
              :key="category"
              class="flex cursor-pointer items-center gap-3 rounded-xl px-3 py-2.5 text-sm transition hover:bg-gray-50 dark:hover:bg-dark-800/60"
            >
              <input
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500/40"
                :checked="isCategoryIgnored(category)"
                @change="toggleIgnoredCategory(category)"
              />
              <span class="min-w-0 flex-1 truncate font-medium text-gray-800 dark:text-gray-100">
                {{ categoryLabel(category) }}
              </span>
              <small class="shrink-0 font-mono text-[10px] text-gray-400">{{ category }}</small>
            </label>
          </div>
        </div>
        <div class="border-t border-gray-100 px-5 py-3 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-400">
          {{
            t('channelMonitorV2.settings.ignoredSummary', {
              ignored: draft.ignored_error_categories?.length || 0,
              counted: countedErrorCategoryCount,
            })
          }}
        </div>
      </div>

      <div class="card overflow-hidden !rounded-3xl !border-0 shadow-sm ring-1 ring-gray-900/5 dark:!bg-dark-800 dark:ring-dark-700">
        <div class="card-header !py-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('channelMonitorV2.settings.healthTitle') }}</h3>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
            {{ t('channelMonitorV2.settings.healthHint') }}
          </p>
        </div>
        <div class="grid grid-cols-1 gap-4 px-5 py-4 sm:grid-cols-2 lg:grid-cols-4">
          <label class="block">
            <span class="input-label">{{ t('channelMonitorV2.settings.fields.minimumSample') }}</span>
            <input v-model.number="draft.health_thresholds.minimum_sample" class="input" type="number" min="1" max="10000" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('channelMonitorV2.settings.fields.warningError') }}</span>
            <input v-model.number="warningErrorPercent" class="input" type="number" min="0" max="100" step="0.1" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('channelMonitorV2.settings.fields.criticalError') }}</span>
            <input v-model.number="criticalErrorPercent" class="input" type="number" min="0" max="100" step="0.1" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('channelMonitorV2.settings.fields.targetTtft') }}</span>
            <input v-model.number="draft.health_thresholds.target_ttft_ms" class="input" type="number" min="1" step="100" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('channelMonitorV2.settings.fields.warningTtft') }}</span>
            <input v-model.number="draft.health_thresholds.warning_ttft_ms" class="input" type="number" min="1" step="100" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('channelMonitorV2.settings.fields.criticalTtft') }}</span>
            <input v-model.number="draft.health_thresholds.critical_ttft_ms" class="input" type="number" min="1" step="100" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('channelMonitorV2.settings.fields.warningCache') }}</span>
            <input v-model.number="warningCachePercent" class="input" type="number" min="0" max="100" step="0.1" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('channelMonitorV2.settings.fields.criticalCache') }}</span>
            <input v-model.number="criticalCachePercent" class="input" type="number" min="0" max="100" step="0.1" />
          </label>
        </div>
      </div>

      <div class="space-y-2">
        <div class="rounded-2xl border border-primary-200 bg-primary-50/80 px-4 py-3 text-sm text-primary-900 dark:border-primary-800/50 dark:bg-primary-900/20 dark:text-primary-100">
          <template v-if="namedModelCount === 0">
            {{ t('channelMonitorV2.settings.namedModelsEmpty') }}
          </template>
          <template v-else>
            {{ t('channelMonitorV2.settings.namedModelsCount', { count: namedModelCount }) }}
          </template>
        </div>
        <div class="rounded-2xl border border-gray-200 bg-gray-50/80 px-4 py-3 text-xs text-gray-600 dark:border-dark-600 dark:bg-dark-800/50 dark:text-gray-300">
          <p class="font-medium text-gray-800 dark:text-gray-100">{{ t('channelMonitorV2.settings.userContractTitle') }}</p>
          <ul class="mt-1.5 list-disc space-y-0.5 pl-4">
            <li>{{ t('channelMonitorV2.settings.userContract.health') }}</li>
            <li>{{ t('channelMonitorV2.settings.userContract.trend') }}</li>
            <li>{{ t('channelMonitorV2.settings.userContract.latency') }}</li>
            <li>{{ t('channelMonitorV2.settings.userContract.models') }}</li>
          </ul>
        </div>
      </div>
    </template>

    <BaseDialog
      :show="showPlatformPicker"
      :title="t('channelMonitorV2.settings.platformPickerTitle')"
      width="wide"
      @close="showPlatformPicker = false"
    >
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('channelMonitorV2.settings.platformPickerHint') }}</p>
      <div class="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-2">
        <label
          v-for="platform in availablePlatforms"
          :key="platform.value"
          class="flex cursor-pointer items-center gap-3 rounded-2xl border px-4 py-3 transition"
          :class="platformChoices.includes(platform.value) ? 'border-primary-300 bg-primary-50/70 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-200 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
        >
          <input
            type="checkbox"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500/40"
            :checked="platformChoices.includes(platform.value)"
            @change="togglePlatformChoice(platform.value)"
          />
          <ProviderIcon :provider="platform.value" :size="20" />
          <span class="min-w-0 flex-1">
            <strong class="block truncate text-sm font-semibold text-gray-900 dark:text-gray-100">{{ platform.label }}</strong>
            <small class="block text-[11px] text-gray-500 dark:text-gray-400">{{ t('channelMonitorV2.settings.platformGroupCount', { count: groupCountForPlatform(platform.value) }) }}</small>
          </span>
        </label>
      </div>
      <p v-if="availablePlatforms.length === 0" class="py-8 text-center text-sm text-gray-500">{{ t('channelMonitorV2.settings.platformsAllSelected') }}</p>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="showPlatformPicker = false">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="platformChoices.length === 0 || syncingPlatforms" @click="addSelectedPlatforms">
          {{ syncingPlatforms ? t('channelMonitorV2.settings.platformAdding') : t('channelMonitorV2.settings.platformAddSelected', { count: platformChoices.length }) }}
        </button>
      </template>
    </BaseDialog>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { GROUP_PLATFORM_OPTIONS } from '@/constants/platforms'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  getConfig,
  updateConfig,
  MONITOR_ERROR_CATEGORIES,
  type MonitorConfig,
} from '@/api/channelMonitorV2'
import { adminAPI } from '@/api/admin'
import type { AdminGroup } from '@/types'

const { t, te } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const draft = ref<MonitorConfig | null>(null)
const original = ref('')
const groups = ref<AdminGroup[]>([])
const groupSearch = ref('')
const groupModels = ref<Record<number, string[]>>({})
const showPlatformPicker = ref(false)
const platformChoices = ref<string[]>([])
const syncingPlatforms = ref(false)
const syncingPlatform = ref('')
const refreshingGroups = ref(false)
const selectedPlatformKeys = computed(() => new Set(draft.value?.platforms.map((item) => item.platform) || []))
const selectableGroups = computed(() => groups.value.filter((group) =>
  group.platform === 'composite' || selectedPlatformKeys.value.has(group.platform)
))
const selectedMonitorGroups = computed(() => {
  const selected = new Set(draft.value?.group_ids || [])
  return selectableGroups.value.filter((group) => selected.has(group.id))
})
const groupedOptions = computed(() => {
  const match = groupSearch.value.toLocaleLowerCase()
  const byPlatform = new Map<string, AdminGroup[]>()
  for (const group of selectableGroups.value) {
    if (match && !`${group.name} ${group.id} ${group.platform}`.toLocaleLowerCase().includes(match)) continue
    const platform = group.platform || 'unknown'
    const list = byPlatform.get(platform) || []
    list.push(group)
    byPlatform.set(platform, list)
  }
  return [...byPlatform.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([platform, list]) => ({ platform, groups: list.sort((a, b) => a.name.localeCompare(b.name)) }))
})
const availablePlatforms = computed(() =>
  GROUP_PLATFORM_OPTIONS.filter(
    (option) => !draft.value?.platforms.some((platform) => platform.platform === option.value)
  ).map((option) => ({ ...option, label: platformLabel(option.value) }))
)

function openPlatformPicker() {
  platformChoices.value = []
  showPlatformPicker.value = true
}

function togglePlatformChoice(platform: string) {
  platformChoices.value = platformChoices.value.includes(platform)
    ? platformChoices.value.filter((item) => item !== platform)
    : [...platformChoices.value, platform]
}

function groupCountForPlatform(platform: string) {
  return groups.value.filter((group) => group.platform === platform).length
}

async function groupModelsFor(group: AdminGroup, force = false): Promise<string[]> {
  const cached = groupModels.value[group.id]
  if (cached && !force) return cached
  try {
    const models = await adminAPI.groups.getAvailableModels(group.id)
    groupModels.value = { ...groupModels.value, [group.id]: models }
    return models
  } catch (error) {
    console.warn('load group model catalog failed', { groupId: group.id, error })
    return []
  }
}

async function modelsForPlatform(platform: string, force = false): Promise<string[]> {
  const platformGroups = groups.value.filter((group) => group.platform === platform && group.status === 'active')
  const catalogs = await Promise.all(platformGroups.map((group) => groupModelsFor(group, force)))
  return [...new Set(catalogs.flat())].sort()
}

async function addSelectedPlatforms() {
  if (!draft.value || platformChoices.value.length === 0) return
  syncingPlatforms.value = true
  try {
    const additions = await Promise.all(
      platformChoices.value.map(async (platform) => ({
        platform,
        enabled: true,
        models: await modelsForPlatform(platform),
      }))
    )
    draft.value.platforms.push(...additions)
    draft.value.platforms.sort((a, b) => a.platform.localeCompare(b.platform))
    showPlatformPicker.value = false
    platformChoices.value = []
  } finally {
    syncingPlatforms.value = false
  }
}

async function syncPlatformModels(platform: MonitorConfig['platforms'][number]) {
  syncingPlatform.value = platform.platform
  try {
    platform.models = await modelsForPlatform(platform.platform, true)
    appStore.showSuccess(t('channelMonitorV2.settings.modelSyncSuccess', { count: platform.models.length }))
  } finally {
    syncingPlatform.value = ''
  }
}

function removePlatform(platform: string) {
  if (!draft.value) return
  const removedGroupIDs = new Set(groups.value.filter((group) => group.platform === platform).map((group) => group.id))
  draft.value.platforms = draft.value.platforms.filter((item) => item.platform !== platform)
  draft.value.group_ids = draft.value.group_ids.filter((id) => !removedGroupIDs.has(id))
}

function selectAllVisibleGroups() {
  if (!draft.value) return
  const visible = new Set(selectableGroups.value.map((group) => group.id))
  for (const id of draft.value.group_ids) visible.add(id)
  draft.value.group_ids = [...visible].sort((a, b) => a - b)
}

async function refreshMonitorGroups() {
  if (refreshingGroups.value) return
  refreshingGroups.value = true
  try {
    groups.value = await adminAPI.groups.getAllIncludingInactive()
    if (draft.value) {
      const allowed = new Set(selectableGroups.value.map((group) => group.id))
      draft.value.group_ids = draft.value.group_ids.filter((id) => allowed.has(id))
    }
    appStore.showSuccess(t('channelMonitorV2.settings.groupsRefreshSuccess', { count: selectableGroups.value.length }))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.settings.groupsRefreshFailed')))
  } finally {
    refreshingGroups.value = false
  }
}

const dirty = computed(() => (draft.value ? JSON.stringify(draft.value) !== original.value : false))
const namedModelCount = computed(
  () => draft.value?.platforms.filter((p) => p.enabled).reduce((sum, p) => sum + p.models.length, 0) || 0
)
const errorCategories = MONITOR_ERROR_CATEGORIES
const countedErrorCategoryCount = computed(
  () => errorCategories.length - (draft.value?.ignored_error_categories?.length || 0)
)
const defaultThresholds = {
  minimum_sample: 50,
  warning_error_rate: 0.05,
  critical_error_rate: 0.20,
  target_ttft_ms: 3000,
  warning_ttft_ms: 3000,
  critical_ttft_ms: 10000,
  // Higher is better: below 85% watch, below 60% critical.
  warning_cache_rate: 0.85,
  critical_cache_rate: 0.60,
  error_weight: 0.60,
  ttft_weight: 0.20,
  cache_weight: 0.20,
}

/** Factory ignored categories (matches backend DefaultChannelMonitorV2IgnoredErrorCategories). */
const defaultIgnoredErrorCategories = [
  'authentication',
  'client_cancelled',
  'content_policy',
  'context_limit',
  'group_access',
  'model_unsupported',
  'not_found',
  'quota_or_balance',
] as const
function percentModel(key: 'warning_error_rate' | 'critical_error_rate' | 'warning_cache_rate' | 'critical_cache_rate') {
  return computed({
    get: () => ((draft.value?.health_thresholds?.[key] ?? defaultThresholds[key]) * 100),
    set: (value: number) => {
      if (!draft.value) return
      draft.value.health_thresholds[key] = Math.max(0, Math.min(100, Number(value) || 0)) / 100
    },
  })
}
const warningErrorPercent = percentModel('warning_error_rate')
const criticalErrorPercent = percentModel('critical_error_rate')
const warningCachePercent = percentModel('warning_cache_rate')
const criticalCachePercent = percentModel('critical_cache_rate')

function setModels(platform: MonitorConfig['platforms'][number], event: Event) {
  platform.models = [
    ...new Set(
      (event.target as HTMLInputElement).value
        .split(',')
        .map((v) => v.trim())
        .filter(Boolean)
    ),
  ].sort()
}

function toggleGroup(id: number) {
  if (!draft.value) return
  draft.value.group_ids = draft.value.group_ids.includes(id)
    ? draft.value.group_ids.filter((value) => value !== id)
    : [...draft.value.group_ids, id].sort((a, b) => a - b)
}

function isCategoryIgnored(category: string): boolean {
  return Boolean(draft.value?.ignored_error_categories?.includes(category))
}

function toggleIgnoredCategory(category: string) {
  if (!draft.value) return
  const current = new Set(draft.value.ignored_error_categories || [])
  if (current.has(category)) current.delete(category)
  else current.add(category)
  draft.value.ignored_error_categories = [...current].sort()
}

function categoryLabel(category: string) {
  const key = `channelMonitorV2.errorCategories.${category}`
  return te(key) ? t(key) : category
}

function platformLabel(value: string) {
  return (
    {
      anthropic: 'Anthropic',
      openai: 'OpenAI',
      grok: 'Grok',
      kiro: 'Kiro',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      deepseek: 'DeepSeek',
      kimi: 'Kimi',
      zhipu: 'GLM',
      minimax: 'MiniMax',
      composite: t('channelMonitorV2.settings.compositePlatform'),
    } as Record<string, string>
  )[value] || value
}

function normalizeConfig(value: MonitorConfig): MonitorConfig {
  const ignored = value.ignored_error_categories
  return {
    ...value,
    health_thresholds: { ...defaultThresholds, ...(value.health_thresholds || {}) },
    // Preserve explicit empty arrays from the server (operator cleared all).
    ignored_error_categories: [
      ...(ignored == null ? [...defaultIgnoredErrorCategories] : ignored),
    ].sort(),
  }
}

async function load() {
  loading.value = true
  try {
    const [value, groupRows] = await Promise.all([
      getConfig(),
      adminAPI.groups.getAllIncludingInactive(),
    ])
    const normalized = normalizeConfig(value)
    draft.value = structuredClone(normalized)
    groups.value = groupRows
    groupModels.value = {}
    original.value = JSON.stringify(normalized)
    const allowedGroupIDs = new Set(
      groupRows
        .filter((group) => normalized.platforms.some((platform) => platform.platform === group.platform))
        .map((group) => group.id)
    )
    draft.value.group_ids = draft.value.group_ids.filter((id) => allowedGroupIDs.has(id))
    const composite = draft.value.platforms.find((platform) => platform.platform === 'composite')
    if (composite) composite.models = await modelsForPlatform('composite', true)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.settings.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!draft.value) return
  saving.value = true
  try {
    const payload = normalizeConfig(draft.value)
    const selectableIDs = new Set(selectableGroups.value.map((group) => group.id))
    payload.group_ids = payload.group_ids.filter((id) => selectableIDs.has(id))
    const value = await updateConfig(payload)
    const normalized = normalizeConfig(value)
    draft.value = structuredClone(normalized)
    original.value = JSON.stringify(normalized)
    appStore.showSuccess(t('channelMonitorV2.settings.saveSuccess'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('channelMonitorV2.settings.saveFailed')))
    await load()
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
