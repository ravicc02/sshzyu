<template>
  <BaseDialog
    :show="show"
    :title="editing ? t('admin.channelMonitor.editTitle') : t('admin.channelMonitor.createTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <form id="channel-monitor-form" @submit.prevent="handleSubmit" class="space-y-5">
      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.name') }} <span class="text-red-500">*</span></label>
        <input v-model="form.name" type="text" required class="input" :placeholder="t('admin.channelMonitor.form.namePlaceholder')" />
      </div>

      <!-- 检测模式：probe（探活）/ quota（仅配额）/ quota_probe（探活+配额） -->
      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.checkMode') }}</label>
        <div class="grid gap-3 sm:grid-cols-3" data-testid="monitor-check-mode">
          <button
            v-for="opt in checkModeOptions"
            :key="opt.value"
            type="button"
            :data-testid="`monitor-check-mode-${opt.value}`"
            :aria-pressed="form.check_mode === opt.value"
            :disabled="opt.disabled"
            class="rounded-lg border-2 px-3 py-2 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-50"
            :class="checkModeButtonClass(opt.value)"
            @click="selectCheckMode(opt.value)"
          >
            <span class="block text-sm font-semibold">{{ opt.label }}</span>
            <span class="mt-0.5 block text-xs opacity-80">{{ opt.hint }}</span>
          </button>
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.provider') }} <span class="text-red-500">*</span></label>
        <div v-if="isCompositeGroup" class="rounded-xl border border-primary-200 bg-primary-50/70 p-4 dark:border-primary-800/60 dark:bg-primary-900/20" data-testid="monitor-composite-routing">
          <div class="flex items-start gap-3">
            <span class="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-white text-primary-600 shadow-sm dark:bg-dark-800 dark:text-primary-300">
              <ProviderIcon provider="composite" :size="22" />
            </span>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <strong class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.channelMonitor.form.compositeRoutingTitle') }}</strong>
                <span class="rounded-full bg-white px-2 py-0.5 text-[10px] font-semibold text-primary-700 ring-1 ring-primary-200 dark:bg-dark-800 dark:text-primary-300 dark:ring-primary-800">
                  {{ t('admin.channelMonitor.form.compositeRoutingBadge') }}
                </span>
              </div>
              <p class="mt-1 text-xs leading-5 text-gray-600 dark:text-gray-300">{{ t('admin.channelMonitor.form.compositeRoutingHint') }}</p>
              <div v-if="compositeSelectedModels.length" class="mt-2 flex flex-wrap gap-1.5">
                <span v-for="model in compositeSelectedModels" :key="model" class="inline-flex items-center gap-1.5 rounded-lg bg-white px-2 py-1 text-[11px] text-gray-700 ring-1 ring-gray-200 dark:bg-dark-800 dark:text-gray-200 dark:ring-dark-600">
                  <ProviderIcon :provider="compositeRouteProvider(model)" :size="12" />
                  {{ providerLabel(compositeRouteProvider(model)) }}
                  <span class="max-w-44 truncate text-gray-400">{{ model }}</span>
                </span>
              </div>
            </div>
          </div>
        </div>
        <div v-else class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <button
            v-for="opt in providerOptions"
            :key="opt.value"
            type="button"
            :data-testid="`monitor-provider-${opt.value}`"
            :aria-pressed="form.provider === opt.value"
            class="flex items-center justify-center gap-2 rounded-lg border-2 px-3 py-2.5 text-sm font-medium transition-colors"
            :class="providerPickerClass(opt.value, form.provider === opt.value)"
            @click="selectProvider(opt.value)"
          >
            <ProviderIcon :provider="opt.value" :size="18" />
            <span>{{ opt.label }}</span>
          </button>
        </div>
      </div>

      <!-- 配额模式数据源：关联账号（复用账号侧用量/余额服务） -->
      <div v-if="usesQuotaMode">
        <label class="input-label">
          {{ t('admin.channelMonitor.form.linkedAccount') }} <span class="text-red-500">*</span>
        </label>
        <div data-testid="monitor-linked-account">
          <Select
            v-model="accountSelectValue"
            :options="accountOptions"
            :placeholder="t('admin.channelMonitor.form.linkedAccountPlaceholder')"
            remote
            :loading="accountsLoading"
            @search="onAccountSearch"
          />
        </div>
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.form.linkedAccountHint') }}</p>
        <p v-if="form.provider === PROVIDER_OPENAI" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
          {{ t('admin.channelMonitor.form.openAIQuotaProbeHint') }}
        </p>
        <p v-if="accountHydrationFailed" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
          {{ t('admin.channelMonitor.form.linkedAccountMissing') }}
        </p>
        <p v-if="accountOptions.length === 0 && !accountsLoading && !accountSearchQuery" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
          {{ t('admin.channelMonitor.form.linkedAccountEmpty') }}
        </p>
      </div>

      <div v-if="!isCompositeGroup && form.provider === PROVIDER_OPENAI && usesProbePart" class="rounded-lg border border-blue-100 bg-blue-50/50 p-3 dark:border-blue-500/20 dark:bg-blue-500/10">
        <label class="input-label">{{ t('admin.channelMonitor.form.apiMode') }}</label>
        <div class="grid gap-3 sm:grid-cols-2">
          <button
            v-for="opt in apiModeOptions"
            :key="opt.value"
            type="button"
            :aria-pressed="form.api_mode === opt.value"
            class="rounded-lg border-2 px-3 py-2 text-left transition-colors"
            :class="apiModeButtonClass(opt.value)"
            @click="form.api_mode = opt.value"
          >
            <span class="block text-sm font-semibold">{{ opt.label }}</span>
            <span class="mt-0.5 block text-xs opacity-80">{{ opt.hint }}</span>
          </button>
        </div>
      </div>

      <div v-if="usesProbePart">
        <label class="input-label">{{ t('admin.channelMonitor.form.endpoint') }} <span class="text-red-500">*</span></label>
        <div class="flex gap-2">
          <input v-model="form.endpoint" data-testid="monitor-endpoint" type="text" required class="input flex-1" :placeholder="t('admin.channelMonitor.form.endpointPlaceholder')" />
          <button type="button" @click="useCurrentDomain" class="btn btn-secondary whitespace-nowrap">
            {{ t('admin.channelMonitor.form.useCurrentDomain') }}
          </button>
        </div>
      </div>

      <div v-if="usesProbePart">
        <label class="input-label">
          {{ t('admin.channelMonitor.form.apiKey') }}<span v-if="!editing" class="text-red-500"> *</span>
        </label>
        <div class="flex gap-2">
          <input
            v-model="form.api_key"
            type="password"
            :required="!editing"
            class="input flex-1"
            :placeholder="editing ? t('admin.channelMonitor.form.apiKeyEditPlaceholder') : t('admin.channelMonitor.form.apiKeyPlaceholder')"
          />
          <button type="button" @click="openMyKeyPicker" class="btn btn-secondary whitespace-nowrap">
            {{ t('admin.channelMonitor.form.useMyKey') }}
          </button>
        </div>
        <p v-if="editing && editing.api_key_masked" class="mt-1 text-xs text-gray-400">{{ editing.api_key_masked }}</p>
      </div>

      <div v-if="usesProbePart">
        <label class="input-label">{{ t('admin.channelMonitor.form.primaryModel') }} <span class="text-red-500">*</span></label>
        <input
          v-model="form.primary_model"
          data-testid="monitor-primary-model"
          type="text"
          required
          class="input font-medium"
          :class="getPlatformTextClass(form.provider)"
          :placeholder="t('admin.channelMonitor.form.primaryModelPlaceholder')"
        />
      </div>

      <div v-if="usesProbePart">
        <div class="mb-1.5 flex items-center justify-between gap-3">
          <label class="input-label mb-0">{{ t('admin.channelMonitor.form.extraModels') }}</label>
          <button type="button" class="btn btn-ghost btn-sm" @click="openGroupModelPicker">
            <Icon name="download" size="sm" />
            {{ t('admin.channelMonitor.form.fetchGroupModels') }}
          </button>
        </div>
        <ModelTagInput
          :models="form.extra_models"
          :platform="form.provider"
          :placeholder="t('admin.channelMonitor.form.extraModelsPlaceholder')"
          @update:models="form.extra_models = $event"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.groupName') }}</label>
        <Select
          v-model="groupSelectValue"
          :options="groupOptions"
          :placeholder="t('admin.channelMonitor.form.groupNamePlaceholder')"
          searchable
        />
        <p class="mt-1 text-xs text-gray-400">
          {{ t('admin.channelMonitor.form.groupNameLinkedHint') }}
        </p>
      </div>

      <div v-if="!isCompositeGroup && form.provider === PROVIDER_OPENAI" class="rounded-2xl border border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-900/30">
        <div class="flex items-center justify-between gap-4">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.channelMonitor.form.intelligenceEnabled') }}</p>
          <Toggle v-model="form.intelligence_enabled" />
        </div>
        <div v-if="form.intelligence_enabled" class="mt-3 border-t border-gray-200 pt-3 dark:border-dark-700">
          <label class="input-label">{{ t('admin.channelMonitor.form.intelligenceIntervalMinutes') }}</label>
          <input
            v-model.number="intelligenceIntervalMinutes"
            type="number"
            min="5"
            max="1440"
            step="5"
            class="input"
          />
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.intervalSeconds') }} <span class="text-red-500">*</span></label>
        <input v-model.number="form.interval_seconds" type="number" min="15" max="3600" required class="input" />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.form.intervalSecondsHint') }}</p>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.jitterSeconds') }}</label>
        <input v-model.number="form.jitter_seconds" type="number" min="0" :max="maxJitterSeconds" class="input" />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.form.jitterSecondsHint') }}</p>
      </div>

      <div class="flex items-center justify-between">
        <label class="input-label mb-0">{{ t('admin.channelMonitor.form.enabled') }}</label>
        <Toggle v-model="form.enabled" />
      </div>

      <!-- 高级设置区：请求模板 + 自定义 headers/body（仅探活模式有意义） -->
      <details v-if="usesProbePart" class="rounded-lg border border-gray-200 bg-gray-50/50 p-3 dark:border-dark-700 dark:bg-dark-900/30">
        <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.channelMonitor.advanced.section') }}
        </summary>
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.advanced.sectionHint') }}</p>

        <div class="mt-4 space-y-4">
          <div>
            <label class="input-label">{{ t('admin.channelMonitor.templateField.label') }}</label>
            <Select
              v-model="templateSelectValue"
              :options="templateOptions"
              :placeholder="t('admin.channelMonitor.templateField.placeholder')"
            />
            <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.templateField.applyHint') }}</p>
          </div>

          <MonitorAdvancedRequestConfig
            :provider="form.provider"
            :api-mode="form.api_mode"
            :extra-headers="form.extra_headers"
            :body-override-mode="form.body_override_mode"
            :body-override="form.body_override"
            @update:extra-headers="form.extra_headers = $event"
            @update:body-override-mode="form.body_override_mode = $event"
            @update:body-override="form.body_override = $event"
          />
        </div>
      </details>
    </form>

    <BaseDialog
      :show="showGroupModelPicker"
      :title="t('admin.channelMonitor.form.groupModelsTitle')"
      width="wide"
      @close="showGroupModelPicker = false"
    >
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.channelMonitor.form.groupModelsHint') }}</p>
      <input v-model.trim="groupModelSearch" type="search" class="input mt-3 w-full" :placeholder="t('admin.channelMonitor.form.groupModelsSearch')" />
      <div v-if="groupModelsLoading" class="py-8 text-center text-sm text-gray-400">{{ t('common.loading') }}</div>
      <div v-else-if="filteredGroupModels.length" class="mt-3 grid max-h-[min(55vh,460px)] grid-cols-1 gap-1 overflow-y-auto sm:grid-cols-2">
        <label v-for="model in filteredGroupModels" :key="model" class="flex min-w-0 cursor-pointer items-center gap-2 rounded-xl px-3 py-2.5 text-sm hover:bg-gray-50 dark:hover:bg-dark-700">
          <input
            type="checkbox"
            class="h-4 w-4 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500/40"
            :checked="groupModelChoices.includes(model)"
            @change="toggleGroupModelChoice(model)"
          />
          <span class="min-w-0 flex-1 truncate font-medium text-gray-800 dark:text-gray-100" :title="model">{{ model }}</span>
        </label>
      </div>
      <p v-else class="py-8 text-center text-sm text-gray-500">{{ t('admin.channelMonitor.form.groupModelsEmpty') }}</p>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="showGroupModelPicker = false">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="groupModelChoices.length === 0" @click="applyGroupModels">
          {{ t('admin.channelMonitor.form.groupModelsApply', { count: groupModelChoices.length }) }}
        </button>
      </template>
    </BaseDialog>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" type="button" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="channel-monitor-form"
          :disabled="submitting"
          class="btn btn-primary"
        >
          {{ submitting
            ? t('common.submitting')
            : editing ? t('common.update') : t('common.create') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <MonitorKeyPickerDialog
    :show="showKeyPicker"
    :loading="myKeysLoading"
    :keys="myActiveKeys"
    :provider="form.provider"
    :group-id="form.group_id"
    :user-group-rates="userGroupRates"
    @close="showKeyPicker = false"
    @pick="pickMyKey"
  />
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import { keysAPI } from '@/api/keys'
import { userGroupsAPI } from '@/api/groups'
import type {
  BodyOverrideMode,
  ChannelMonitor,
  CreateParams,
  APIMode,
  CheckMode,
  Provider,
  UpdateParams,
} from '@/api/admin/channelMonitor'
import type { ChannelMonitorTemplate } from '@/api/admin/channelMonitorTemplate'
import type { AdminGroup, ApiKey, CompositeModelRoute } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelTagInput from '@/components/admin/channel/ModelTagInput.vue'
import { getPlatformTextClass } from '@/components/admin/channel/types'
import MonitorKeyPickerDialog from '@/components/admin/monitor/MonitorKeyPickerDialog.vue'
import MonitorAdvancedRequestConfig from '@/components/admin/monitor/MonitorAdvancedRequestConfig.vue'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
  PROVIDER_GROK,
  PROVIDER_ANTIGRAVITY,
  PROVIDER_KIMI,
  PROVIDER_ZHIPU,
  PROVIDER_DEEPSEEK,
  API_MODE_CHAT_COMPLETIONS,
  API_MODE_RESPONSES,
  CHECK_MODE_PROBE,
  CHECK_MODE_QUOTA,
  CHECK_MODE_QUOTA_PROBE,
  DEFAULT_GROK_ENDPOINT,
  DEFAULT_GROK_MODEL,
  DEFAULT_KIMI_ENDPOINT,
  DEFAULT_ZHIPU_ENDPOINT,
  DEFAULT_DEEPSEEK_ENDPOINT,
  DEFAULT_INTERVAL_SECONDS,
} from '@/constants/channelMonitor'

const props = defineProps<{
  show: boolean
  monitor: ChannelMonitor | null
  initialGroup?: AdminGroup | null
  initialEnabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { providerPickerClass } = useChannelMonitorFormat()

// System-configured default interval for new monitors. Falls back to the static
// constant when public settings haven't loaded yet or store the legacy 0 value.
const systemDefaultInterval = computed<number>(() => {
  const configured = appStore.cachedPublicSettings?.channel_monitor_default_interval_seconds
  return configured && configured > 0 ? configured : DEFAULT_INTERVAL_SECONDS
})

// editing is true when we have an existing monitor
const editing = computed<ChannelMonitor | null>(() => props.monitor)

const submitting = ref(false)

// API key picker
const showKeyPicker = ref(false)
const myKeysLoading = ref(false)
const myActiveKeys = ref<ApiKey[]>([])
const userGroupRates = ref<Record<number, number>>({})
const showGroupModelPicker = ref(false)
const groupModelsLoading = ref(false)
const groupModelSearch = ref('')
const groupModelOptions = ref<string[]>([])
const groupModelChoices = ref<string[]>([])
const filteredGroupModels = computed(() => {
  const query = groupModelSearch.value.toLocaleLowerCase()
  return query
    ? groupModelOptions.value.filter((model) => model.toLocaleLowerCase().includes(query))
    : groupModelOptions.value
})

interface MonitorForm {
  name: string
  provider: Provider
  api_mode: APIMode
  check_mode: CheckMode
  account_id: number | null
  endpoint: string
  api_key: string
  primary_model: string
  extra_models: string[]
  group_name: string
  group_id: number | null
  probe_fallback_enabled: boolean
  intelligence_enabled: boolean
  intelligence_interval_seconds: number
  interval_seconds: number
  jitter_seconds: number
  enabled: boolean
  // 高级设置快照
  template_id: number | null
  extra_headers: Record<string, string>
  body_override_mode: BodyOverrideMode
  body_override: Record<string, unknown> | null
}

const form = reactive<MonitorForm>({
  name: '',
  provider: PROVIDER_ANTHROPIC,
  api_mode: API_MODE_CHAT_COMPLETIONS,
  check_mode: CHECK_MODE_PROBE,
  account_id: null,
  endpoint: '',
  api_key: '',
  primary_model: '',
  extra_models: [],
  group_name: '',
  group_id: null,
  probe_fallback_enabled: true,
  intelligence_enabled: true,
  intelligence_interval_seconds: 1800,
  interval_seconds: systemDefaultInterval.value,
  jitter_seconds: 0,
  enabled: true,
  template_id: null,
  extra_headers: {},
  body_override_mode: 'off',
  body_override: null,
})

const intelligenceIntervalMinutes = computed({
  get: () => Math.max(5, Math.round(form.intelligence_interval_seconds / 60)),
  set: (value: number) => {
    const minutes = Math.min(1440, Math.max(5, Math.round(Number(value) || 30)))
    form.intelligence_interval_seconds = minutes * 60
  },
})

// quota / quota_probe 需要关联账号；probe / quota_probe 需要探活字段。
const usesQuotaMode = computed(() => form.check_mode !== CHECK_MODE_PROBE)
const usesProbePart = computed(() => form.check_mode !== CHECK_MODE_QUOTA)
const monitorProvider = computed<Provider>(() => isCompositeGroup.value ? PROVIDER_OPENAI : form.provider)

const monitorGroups = ref<Array<{ id: number; name: string; platform: string; status?: string }>>([])
const groupsLoading = ref(false)
const groupOptions = computed(() =>
  monitorGroups.value.map((group) => ({
    value: String(group.id),
    label: `${group.name} · ${group.platform}${group.status === 'inactive' ? ` · ${t('channelMonitorV2.settings.inactiveGroup')}` : ''}`,
  })),
)
const selectedGroup = computed(() => {
  if (form.group_id == null) return null
  return monitorGroups.value.find((item) => item.id === form.group_id)
    ?? (props.initialGroup?.id === form.group_id ? props.initialGroup : null)
})
const isCompositeGroup = computed(() => selectedGroup.value?.platform === 'composite')
const compositeRoutes = ref<CompositeModelRoute[]>([])
const compositeSelectedModels = computed(() => [...new Set([form.primary_model, ...form.extra_models].map((model) => model.trim()).filter(Boolean))])
const groupSelectValue = computed<string>({
  get: () => (form.group_id == null ? '' : String(form.group_id)),
  set: (raw: string) => {
    if (raw === '') {
      form.group_id = null
      form.group_name = ''
      return
    }
    const id = Number(raw)
    const group = monitorGroups.value.find((item) => item.id === id)
    if (!group) return
    form.group_id = group.id
    form.group_name = group.name
  },
})

function compositeRouteProvider(model: string): Provider {
  const normalized = model.trim()
  const exact = compositeRoutes.value.find((route) =>
    route.enabled && route.match_type === 'exact' && route.public_model === normalized
  )
  if (exact) return monitorProviderFor(exact.target_platform)
  const prefix = compositeRoutes.value
    .filter((route) => route.enabled && route.match_type === 'prefix' && normalized.startsWith(route.public_model))
    .sort((a, b) => b.public_model.length - a.public_model.length)[0]
  if (prefix) return monitorProviderFor(prefix.target_platform)
  if (/glm|zhipu/i.test(normalized)) return PROVIDER_ZHIPU
  if (/kimi|moonshot/i.test(normalized)) return PROVIDER_KIMI
  if (/deepseek/i.test(normalized)) return PROVIDER_DEEPSEEK
  if (/grok/i.test(normalized)) return PROVIDER_GROK
  if (/gemini/i.test(normalized)) return PROVIDER_GEMINI
  if (/claude|anthropic/i.test(normalized)) return PROVIDER_ANTHROPIC
  return PROVIDER_OPENAI
}

function monitorProviderFor(platform: string): Provider {
  switch (platform) {
    case PROVIDER_OPENAI:
    case PROVIDER_ANTHROPIC:
    case PROVIDER_GEMINI:
    case PROVIDER_GROK:
    case PROVIDER_ANTIGRAVITY:
    case PROVIDER_KIMI:
    case PROVIDER_ZHIPU:
    case PROVIDER_DEEPSEEK:
      return platform
    default:
      return PROVIDER_OPENAI
  }
}

async function loadCompositeRoutes(groupId: number) {
  compositeRoutes.value = []
  if (!isCompositeGroup.value) return
  try {
    compositeRoutes.value = await adminAPI.groups.listCompositeRoutes(groupId)
  } catch (error) {
    console.warn('load composite monitor routes failed', error)
  }
}

async function loadMonitorGroups() {
  if (groupsLoading.value) return
  groupsLoading.value = true
  try {
    if (!adminAPI.groups?.getAllIncludingInactive) return
    const rows = await adminAPI.groups.getAllIncludingInactive()
    monitorGroups.value = rows.map((group) => ({
      id: group.id,
      name: group.name,
      platform: group.platform,
      status: group.status,
    }))
    if (form.group_id == null && form.group_name.trim()) {
      const legacy = monitorGroups.value.find((group) => group.name === form.group_name.trim())
      if (legacy) form.group_id = legacy.id
    }
  } catch (err: unknown) {
    console.warn('load monitor groups failed', err)
  } finally {
    groupsLoading.value = false
  }
}

watch(
  () => [form.group_id, isCompositeGroup.value] as const,
  ([groupId, composite]) => {
    if (!composite || groupId == null) {
      compositeRoutes.value = []
      return
    }
    form.provider = PROVIDER_OPENAI
    form.api_mode = API_MODE_CHAT_COMPLETIONS
    form.check_mode = CHECK_MODE_PROBE
    form.intelligence_enabled = false
    if (!form.endpoint.trim()) form.endpoint = currentServiceEndpoint()
    void loadCompositeRoutes(groupId)
  },
  { immediate: true },
)

// jitter 上限与后端校验一致：interval - jitter 不得低于最小检测间隔 15 秒。
const maxJitterSeconds = computed<number>(() => Math.max(0, (form.interval_seconds || 0) - 15))

let suppressFormWatchers = false

// 可用模板列表（进入 dialog 时一次性拉取 cache；按 provider / api mode 过滤）。
const templatesCache = ref<ChannelMonitorTemplate[]>([])
const templatesLoading = ref(false)

const templateOptions = computed(() => {
  const items = templatesCache.value.filter((t) => {
    if (t.provider !== form.provider) return false
    if (form.provider !== PROVIDER_OPENAI) return true
    return normalizeAPIMode(t.api_mode) === form.api_mode
  })
  return [
    { value: '', label: t('admin.channelMonitor.templateField.none') },
    ...items.map((t) => ({ value: String(t.id), label: templateOptionLabel(t) })),
  ]
})

async function loadTemplates() {
  if (templatesCache.value.length > 0) return
  templatesLoading.value = true
  try {
    const { items } = await adminAPI.channelMonitorTemplate.list()
    templatesCache.value = items
  } catch (err: unknown) {
    // 模板拉取失败不阻塞监控表单，用户可以不选模板
    console.warn('load monitor templates failed', err)
  } finally {
    templatesLoading.value = false
  }
}

// 模板下拉绑定：value 是 string（Select 组件约束），需要与 number | null 互转。
const templateSelectValue = computed<string>({
  get: () => (form.template_id == null ? '' : String(form.template_id)),
  set: (raw: string) => {
    if (raw === '') {
      form.template_id = null
      return
    }
    const id = Number(raw)
    if (!Number.isFinite(id)) return
    form.template_id = id
    // 应用模板 = 拷贝快照
    const tpl = templatesCache.value.find((t) => t.id === id)
    if (tpl) {
      suppressFormWatchers = true
      form.api_mode = normalizeAPIMode(tpl.api_mode)
      form.template_id = id
      form.extra_headers = { ...(tpl.extra_headers || {}) }
      form.body_override_mode = tpl.body_override_mode
      form.body_override = tpl.body_override ? { ...tpl.body_override } : null
      suppressFormWatchers = false
    }
  },
})

const apiModeOptions = computed<{ value: APIMode; label: string; hint: string }[]>(() => [
  {
    value: API_MODE_CHAT_COMPLETIONS,
    label: t('admin.channelMonitor.form.apiModeChatCompletions'),
    hint: t('admin.channelMonitor.form.apiModeChatCompletionsHint'),
  },
  {
    value: API_MODE_RESPONSES,
    label: t('admin.channelMonitor.form.apiModeResponses'),
    hint: t('admin.channelMonitor.form.apiModeResponsesHint'),
  },
])

function normalizeAPIMode(mode: APIMode | undefined | null): APIMode {
  return mode === API_MODE_RESPONSES ? API_MODE_RESPONSES : API_MODE_CHAT_COMPLETIONS
}

function apiModeButtonClass(mode: APIMode): string {
  const active = form.api_mode === mode
  if (active) {
    return 'border-primary-500 bg-white text-primary-700 shadow-sm dark:border-primary-400 dark:bg-primary-500/15 dark:text-primary-300'
  }
  return 'border-blue-100 bg-white/70 text-gray-600 hover:border-primary-300 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400'
}

function templateOptionLabel(tpl: ChannelMonitorTemplate): string {
  if (tpl.provider !== PROVIDER_OPENAI) return tpl.name
  const labelKey = normalizeAPIMode(tpl.api_mode) === API_MODE_RESPONSES
    ? 'admin.channelMonitor.form.apiModeResponses'
    : 'admin.channelMonitor.form.apiModeChatCompletions'
  return `${tpl.name} · ${t(labelKey)}`
}

function clearRequestSnapshot() {
  form.template_id = null
  form.extra_headers = {}
  form.body_override_mode = 'off'
  form.body_override = null
}

interface ProviderOption {
  value: Provider
  label: string
}

const providerOptions = computed<ProviderOption[]>(() => [
  { value: PROVIDER_ANTHROPIC, label: t('monitorCommon.providers.anthropic') },
  { value: PROVIDER_OPENAI, label: t('monitorCommon.providers.openai') },
  { value: PROVIDER_GEMINI, label: t('monitorCommon.providers.gemini') },
  { value: PROVIDER_GROK, label: t('monitorCommon.providers.grok') },
  { value: PROVIDER_ANTIGRAVITY, label: t('monitorCommon.providers.antigravity') },
  { value: PROVIDER_KIMI, label: t('monitorCommon.providers.kimi') },
  { value: PROVIDER_ZHIPU, label: t('monitorCommon.providers.zhipu') },
  { value: PROVIDER_DEEPSEEK, label: t('monitorCommon.providers.deepseek') },
])

function providerLabel(provider: Provider): string {
  return providerOptions.value.find((option) => option.value === provider)?.label || provider
}

// 国产 provider 预填的官方 endpoint（仅探活侧；配额模式 endpoint 可留空）。
const PROVIDER_DEFAULT_ENDPOINTS: Partial<Record<Provider, string>> = {
  [PROVIDER_KIMI]: DEFAULT_KIMI_ENDPOINT,
  [PROVIDER_ZHIPU]: DEFAULT_ZHIPU_ENDPOINT,
  [PROVIDER_DEEPSEEK]: DEFAULT_DEEPSEEK_ENDPOINT,
}

interface CheckModeOption {
  value: CheckMode
  label: string
  hint: string
  disabled: boolean
}

const checkModeOptions = computed<CheckModeOption[]>(() => [
  {
    value: CHECK_MODE_PROBE,
    label: t('admin.channelMonitor.form.checkModeProbe'),
    hint: t('admin.channelMonitor.form.checkModeProbeHint'),
    disabled: !isCompositeGroup.value && form.provider === PROVIDER_ANTIGRAVITY,
  },
  {
    value: CHECK_MODE_QUOTA,
    label: t('admin.channelMonitor.form.checkModeQuota'),
    hint: t('admin.channelMonitor.form.checkModeQuotaHint'),
    disabled: isCompositeGroup.value,
  },
  {
    value: CHECK_MODE_QUOTA_PROBE,
    label: t('admin.channelMonitor.form.checkModeQuotaProbe'),
    hint: t('admin.channelMonitor.form.checkModeQuotaProbeHint'),
    disabled: isCompositeGroup.value || form.provider === PROVIDER_ANTIGRAVITY,
  },
])

function checkModeButtonClass(mode: CheckMode): string {
  const active = form.check_mode === mode
  if (active) {
    return 'border-primary-500 bg-white text-primary-700 shadow-sm dark:border-primary-400 dark:bg-primary-500/15 dark:text-primary-300'
  }
  return 'border-blue-100 bg-white/70 text-gray-600 hover:border-primary-300 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400'
}

function selectCheckMode(mode: CheckMode) {
  if (checkModeOptions.value.find((opt) => opt.value === mode)?.disabled) return
  form.check_mode = mode
  if (!usesQuotaMode.value) form.account_id = null
}

// --- 关联账号选择器 ---

interface LinkedAccount {
  id: number
  name: string
}

const linkedAccounts = ref<LinkedAccount[]>([])
const accountsLoading = ref(false)
// 当前搜索词（用于空态文案区分「平台无账号」与「搜索无命中」）。
const accountSearchQuery = ref('')
// 已绑定账号回填失败（getById 失败或平台失配）时提示用户重新选择。
const accountHydrationFailed = ref(false)
// 固定选项：已绑定/已选中但不在当前结果页里的账号，保证搜索后 label 仍可见。
const pinnedAccount = ref<LinkedAccount | null>(null)
let accountSearchSeq = 0
let accountSearchAbort: AbortController | null = null
const hydrationAttempted = new Set<number>()

const accountOptions = computed(() => {
  const opts = linkedAccounts.value.map((a) => ({
    value: String(a.id),
    label: `${a.name} (#${a.id})`,
  }))
  const pinned = pinnedAccount.value
  if (pinned && !linkedAccounts.value.some((a) => a.id === pinned.id)) {
    opts.unshift({ value: String(pinned.id), label: `${pinned.name} (#${pinned.id})` })
  }
  return opts
})

// Select 组件绑定 string，与 number | null 互转。
const accountSelectValue = computed<string>({
  get: () => (form.account_id == null ? '' : String(form.account_id)),
  set: (raw: string) => {
    if (raw === '') {
      form.account_id = null
      pinnedAccount.value = null
      accountHydrationFailed.value = false
      return
    }
    const id = Number(raw)
    if (Number.isFinite(id)) {
      form.account_id = id
      pinnedAccount.value = linkedAccounts.value.find((a) => a.id === id) ?? pinnedAccount.value
    }
  },
})

// 服务端搜索当前 provider 平台的账号（支持关键字，避免大分页截断取不齐）。
// seq + abort 防止快速切换 provider / 连续输入时乱序响应覆盖新结果。
// 失败不阻塞表单：下拉为空 + 空态提示。
async function loadLinkedAccounts(search = '') {
  if (!usesQuotaMode.value || !props.show) return
  accountSearchQuery.value = search
  const seq = ++accountSearchSeq
  accountSearchAbort?.abort()
  const controller = new AbortController()
  accountSearchAbort = controller
  accountsLoading.value = true
  try {
    const res = await adminAPI.accounts.list(
      1,
      50,
      { platform: form.provider, ...(search ? { search } : {}) },
      { signal: controller.signal },
    )
    if (seq !== accountSearchSeq) return
    linkedAccounts.value = (res.items || []).map((a) => ({ id: a.id, name: a.name }))
    await ensureSelectedAccountHydrated()
  } catch (err: unknown) {
    if (controller.signal.aborted) return
    console.warn('load linked accounts failed', err)
    if (!search) linkedAccounts.value = []
  } finally {
    if (seq === accountSearchSeq) accountsLoading.value = false
  }
}

// 编辑已有 quota 监控时，已绑定账号可能不在搜索结果第一页：用 getById
// 回填为固定选项，绑定不因分页截断而丢失。仅当账号确实无法加载或平台
// 失配时才清空绑定（带可见提示），否则绑定只在用户显式切换 provider 时清空。
async function ensureSelectedAccountHydrated() {
  const id = form.account_id
  if (id == null || !usesQuotaMode.value) return
  if (linkedAccounts.value.some((a) => a.id === id) || pinnedAccount.value?.id === id) return
  if (hydrationAttempted.has(id)) return
  hydrationAttempted.add(id)
  try {
    const account = await adminAPI.accounts.getById(id)
    if (form.account_id !== id) return
    if (String(account.platform) !== form.provider) {
      form.account_id = null
      pinnedAccount.value = null
      accountHydrationFailed.value = true
      return
    }
    pinnedAccount.value = { id: account.id, name: account.name }
  } catch {
    if (form.account_id === id) {
      form.account_id = null
      pinnedAccount.value = null
      accountHydrationFailed.value = true
    }
  }
}

function onAccountSearch(query: string) {
  void loadLinkedAccounts(query)
}

watch(
  () => [props.show, form.provider, form.check_mode] as const,
  ([show, provider], prev) => {
    const [prevShow, prevProvider] = prev ?? []
    if (!show) {
      accountSearchAbort?.abort()
      return
    }
    // 弹窗重开 / provider 真正变化时重置回填状态（check_mode 变化不重置，
    // 避免 probe↔quota 切换时无谓地重拉列表）。
    if (show !== prevShow || provider !== prevProvider) {
      hydrationAttempted.clear()
      accountHydrationFailed.value = false
      pinnedAccount.value = null
    }
    void loadLinkedAccounts()
  },
  { immediate: true },
)

function selectProvider(provider: Provider) {
  if (form.provider === provider) return
  const previousProvider = form.provider
  const clearGrokEndpoint =
    previousProvider === PROVIDER_GROK && form.endpoint === DEFAULT_GROK_ENDPOINT
  const clearGrokModel =
    previousProvider === PROVIDER_GROK && form.primary_model === DEFAULT_GROK_MODEL
  const clearPrevDefaultEndpoint =
    !!PROVIDER_DEFAULT_ENDPOINTS[previousProvider] && form.endpoint === PROVIDER_DEFAULT_ENDPOINTS[previousProvider]
  form.provider = provider
  // 关联账号与平台绑定：切换 provider 时显式清空（这是唯一主动清空的入口）。
  form.account_id = null
  pinnedAccount.value = null
  accountHydrationFailed.value = false
  // antigravity 仅配额模式：切到它时强制 quota（checkModeOptions 同步禁用其余项）。
  if (provider === PROVIDER_ANTIGRAVITY && form.check_mode !== CHECK_MODE_QUOTA) {
    form.check_mode = CHECK_MODE_QUOTA
  }
  // 对称还原：从 antigravity 切走时撤掉强制 quota，否则编辑存量 antigravity
  // 监控换平台后仍停留在 quota（目标平台未必支持），update 会携带残留配置。
  // 同步清掉 quota 占位模型（loadFromMonitor 回填的 'quota'），否则切回
  // probe 后拿 'quota' 当探活模型（与离开 grok 清 DEFAULT_GROK_MODEL 同理）。
  if (previousProvider === PROVIDER_ANTIGRAVITY && form.check_mode === CHECK_MODE_QUOTA) {
    form.check_mode = CHECK_MODE_PROBE
    if (form.primary_model.trim() === 'quota') form.primary_model = ''
  }
  if (provider === PROVIDER_GROK) {
    if (!form.endpoint.trim()) form.endpoint = DEFAULT_GROK_ENDPOINT
    if (!form.primary_model.trim()) form.primary_model = DEFAULT_GROK_MODEL
    return
  }
  if (clearGrokEndpoint || clearPrevDefaultEndpoint) form.endpoint = ''
  if (clearGrokModel) form.primary_model = ''
  const defaultEndpoint = PROVIDER_DEFAULT_ENDPOINTS[provider]
  if (defaultEndpoint && !form.endpoint.trim()) form.endpoint = defaultEndpoint
}

// Clear api_key whenever provider changes to avoid cross-provider key mismatch.
// Editing mode loads api_key='' via loadFromMonitor and only sets it on user
// typing, so clearing on provider change is always a safe no-op until the user
// picks a new key.
// 同时清空 template_id（模板有 provider 归属，跨平台不通用）。
watch(() => form.provider, () => {
  if (suppressFormWatchers) return
  form.api_key = ''
  if (form.provider !== PROVIDER_OPENAI) {
    form.api_mode = API_MODE_CHAT_COMPLETIONS
  }
  clearRequestSnapshot()
}, { flush: 'sync' })

watch(() => form.api_mode, () => {
  if (suppressFormWatchers) return
  if (form.provider === PROVIDER_OPENAI) {
    clearRequestSnapshot()
  }
}, { flush: 'sync' })

function resetForm() {
  suppressFormWatchers = true
  form.name = ''
  form.provider = PROVIDER_ANTHROPIC
  form.api_mode = API_MODE_CHAT_COMPLETIONS
  form.check_mode = CHECK_MODE_PROBE
  form.account_id = null
  pinnedAccount.value = null
  accountHydrationFailed.value = false
  form.endpoint = ''
  form.api_key = ''
  form.primary_model = ''
  form.extra_models = []
  form.group_name = ''
  form.group_id = null
  form.probe_fallback_enabled = true
  form.intelligence_enabled = true
  form.intelligence_interval_seconds = 1800
  form.interval_seconds = systemDefaultInterval.value
  form.jitter_seconds = 0
  form.enabled = true
  form.template_id = null
  form.extra_headers = {}
  form.body_override_mode = 'off'
  form.body_override = null
  suppressFormWatchers = false
}

function loadFromMonitor(m: ChannelMonitor) {
  suppressFormWatchers = true
  form.name = m.name
  form.provider = m.provider
  form.api_mode = normalizeAPIMode(m.api_mode)
  form.check_mode = m.check_mode || CHECK_MODE_PROBE
  form.account_id = m.account_id ?? null
  form.endpoint = m.endpoint
  form.api_key = ''
  form.primary_model = m.primary_model
  form.extra_models = [...(m.extra_models || [])]
  form.group_name = m.group_name || ''
  form.group_id = m.group_id ?? null
  form.probe_fallback_enabled = true
  form.intelligence_enabled = m.intelligence_enabled !== false
  form.intelligence_interval_seconds = m.intelligence_interval_seconds || 1800
  form.interval_seconds = m.interval_seconds || systemDefaultInterval.value
  form.jitter_seconds = m.jitter_seconds || 0
  form.enabled = m.enabled
  form.template_id = m.template_id ?? null
  form.extra_headers = { ...(m.extra_headers || {}) }
  form.body_override_mode = m.body_override_mode || 'off'
  form.body_override = m.body_override ? { ...m.body_override } : null
  suppressFormWatchers = false
}

async function loadInitialGroupModels(group: AdminGroup) {
  try {
    const models = await adminAPI.groups.getAvailableModels(group.id)
    if (!props.show || props.monitor || props.initialGroup?.id !== group.id || models.length === 0) return
    form.primary_model = models[0] || ''
    form.extra_models = models.slice(1)
  } catch (error) {
    console.warn('load initial monitor models failed', error)
  }
}

async function openGroupModelPicker() {
  if (form.group_id == null) {
    appStore.showError(t('admin.channelMonitor.form.groupModelsSelectGroupFirst'))
    return
  }
  showGroupModelPicker.value = true
  groupModelsLoading.value = true
  groupModelSearch.value = ''
  groupModelChoices.value = []
  try {
    groupModelOptions.value = await adminAPI.groups.getAvailableModels(form.group_id)
    const current = new Set([form.primary_model.trim(), ...form.extra_models].filter(Boolean))
    groupModelChoices.value = groupModelOptions.value.filter((model) => current.has(model))
  } catch (error) {
    groupModelOptions.value = []
    appStore.showError(extractApiErrorMessage(error, t('admin.channelMonitor.form.groupModelsLoadFailed')))
  } finally {
    groupModelsLoading.value = false
  }
}

function toggleGroupModelChoice(model: string) {
  groupModelChoices.value = groupModelChoices.value.includes(model)
    ? groupModelChoices.value.filter((item) => item !== model)
    : [...groupModelChoices.value, model]
}

function applyGroupModels() {
  const selected = [...new Set(groupModelChoices.value)].sort()
  if (!form.primary_model.trim() && selected.length > 0) {
    form.primary_model = selected.shift() || ''
  }
  form.extra_models = selected
    .filter((model) => model !== form.primary_model)
    .sort()
  showGroupModelPicker.value = false
}

function applyInitialGroup(group: AdminGroup) {
  const provider = providerOptions.value.find((option) => option.value === group.platform)?.value || PROVIDER_OPENAI
  selectProvider(provider)
  form.name = t('admin.channelMonitor.form.groupProbeName', { group: group.name })
  form.group_id = group.id
  form.group_name = group.name
  form.enabled = props.initialEnabled !== false
  form.primary_model = ''
  form.extra_models = []
  void loadInitialGroupModels(group)
}

// Re-sync form whenever the dialog is opened or the target monitor changes.
// 同时拉取模板列表（cache 过的话一次性返回）。
watch(
  () => [props.show, props.monitor, props.initialGroup, props.initialEnabled] as const,
  ([show, m, group]) => {
    if (!show) return
    void loadTemplates()
    void loadMonitorGroups()
    if (m) loadFromMonitor(m)
    else {
      resetForm()
      if (group) applyInitialGroup(group)
    }
  },
  { immediate: true },
)

function currentServiceEndpoint(): string {
  const origin = window.location.origin
  try {
    const hostname = new URL(origin).hostname.toLowerCase()
    if (hostname === 'localhost' || hostname === '::1' || hostname.startsWith('127.')) {
      return origin.replace(/\/+$/, '')
    }
  } catch {
    // Fall through to the configured service endpoint.
  }
  const endpoint = appStore.apiBaseUrl.trim() || window.location.origin
  return endpoint.replace(/\/+$/, '')
}

function useCurrentDomain() {
  form.endpoint = currentServiceEndpoint()
}

async function openMyKeyPicker() {
  showKeyPicker.value = true
  if (myActiveKeys.value.length > 0) return
  myKeysLoading.value = true
  try {
    const [res, rates] = await Promise.all([
      keysAPI.list(1, 100, { status: 'active' }),
      userGroupsAPI.getUserGroupRates(),
    ])
    const items = res.items || []
    const now = Date.now()
    myActiveKeys.value = items.filter(k => {
      if (k.status !== 'active') return false
      if (!k.expires_at) return true
      return new Date(k.expires_at).getTime() > now
    })
    userGroupRates.value = rates
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.channelMonitor.form.noActiveKey')))
  } finally {
    myKeysLoading.value = false
  }
}

function pickMyKey(k: ApiKey) {
  form.api_key = k.key
  showKeyPicker.value = false
}

function buildPayload(): CreateParams {
  return {
    name: form.name.trim(),
    provider: monitorProvider.value,
    api_mode: monitorProvider.value === PROVIDER_OPENAI ? form.api_mode : API_MODE_CHAT_COMPLETIONS,
    check_mode: form.check_mode,
    account_id: usesQuotaMode.value ? form.account_id : null,
    endpoint: usesProbePart.value ? form.endpoint.trim() : '',
    api_key: usesProbePart.value ? form.api_key.trim() : '',
    primary_model: usesProbePart.value ? form.primary_model.trim() : 'quota',
    extra_models: usesProbePart.value ? form.extra_models : [],
    group_name: form.group_name.trim(),
    group_id: form.group_id,
      probe_fallback_enabled: true,
    intelligence_enabled: !isCompositeGroup.value && form.provider === PROVIDER_OPENAI ? form.intelligence_enabled : false,
    intelligence_interval_seconds: form.intelligence_interval_seconds,
    enabled: form.enabled,
    interval_seconds: form.interval_seconds,
    jitter_seconds: form.jitter_seconds || 0,
    template_id: usesProbePart.value ? form.template_id : null,
    extra_headers: form.extra_headers,
    body_override_mode: form.body_override_mode,
    body_override: form.body_override,
  }
}

async function handleSubmit() {
  if (submitting.value) return
  if (!form.name.trim()) {
    appStore.showError(t('admin.channelMonitor.nameRequired'))
    return
  }
  if (usesQuotaMode.value && form.account_id == null) {
    appStore.showError(t('admin.channelMonitor.linkedAccountRequired'))
    return
  }
  if (usesProbePart.value && !form.primary_model.trim()) {
    appStore.showError(t('admin.channelMonitor.primaryModelRequired'))
    return
  }

  submitting.value = true
  try {
    const target = editing.value
    if (target) {
      const { api_key, ...rest } = buildPayload()
      const req: UpdateParams = { ...rest }
      if (req.group_id == null) req.group_id = 0
      // Only send api_key if user typed a new value
      if (api_key) req.api_key = api_key
      // template_id=null 用 clear_template=true 明确告诉后端清空（pointer 语义）
      if (usesProbePart.value && form.template_id == null) {
        req.clear_template = true
        delete req.template_id
      }
      // account_id 同理：probe 模式不带账号，update 发 0 显式解绑存量关联
      // （后端 0=清空、null=不动）。仅 update——create 发 0 会落 &0 触发 FK 违约。
      if (!usesQuotaMode.value) req.account_id = 0
      await adminAPI.channelMonitor.update(target.id, req)
      appStore.showSuccess(t('admin.channelMonitor.updateSuccess'))
    } else {
      await adminAPI.channelMonitor.create(buildPayload())
      appStore.showSuccess(t('admin.channelMonitor.createSuccess'))
    }
    emit('saved')
    emit('close')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    submitting.value = false
  }
}
</script>
