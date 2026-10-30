<template>
  <div class="space-y-3">
    <div v-if="loading" class="space-y-2" aria-hidden="true">
      <div v-for="n in 3" :key="n" class="h-16 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700" />
    </div>
    <div v-else-if="!groups.length" class="rounded-2xl border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
      {{ t('channelMonitorV2.settings.probeEmpty') }}
    </div>
    <div v-else class="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
      <div
        v-for="group in orderedGroups"
        :key="group.id"
        class="grid cursor-pointer grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_auto_auto] items-center gap-3 border-b border-gray-100 px-4 py-3 transition hover:bg-gray-50/70 last:border-b-0 dark:border-dark-700 dark:hover:bg-dark-700/40"
        @click="emit('configure', group, monitorFor(group))"
      >
        <div class="flex min-w-0 items-center gap-3 text-left">
          <span class="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-gray-50 dark:bg-dark-700">
            <ProviderIcon :provider="group.platform" :size="20" />
          </span>
          <span class="min-w-0">
            <strong class="block truncate text-sm font-semibold text-gray-900 dark:text-gray-100">{{ group.name }}</strong>
            <small class="mt-0.5 block truncate text-[11px] text-gray-500 dark:text-gray-400">{{ platformLabel(group.platform) }} · #{{ group.id }}</small>
          </span>
        </div>

        <div class="min-w-0 text-xs text-gray-600 dark:text-gray-300">
          <template v-if="monitorFor(group)">
            <span class="block truncate font-medium">{{ monitorFor(group)?.primary_model || '—' }}</span>
            <span class="mt-0.5 block text-[11px] text-gray-400">
              {{ monitorModelSummary(monitorFor(group)) }}
            </span>
          </template>
          <span v-else class="text-gray-400">{{ t('channelMonitorV2.settings.probeUnconfigured') }}</span>
        </div>

        <span class="whitespace-nowrap rounded-full px-2.5 py-1 text-[11px] font-semibold" :class="probeStateClass(monitorFor(group))">
          {{ probeStateLabel(monitorFor(group)) }}
        </span>

        <div class="flex items-center justify-end gap-2" @click.stop>
          <Toggle
            :model-value="monitorFor(group)?.enabled === true"
            :disabled="busyGroupId === group.id"
            :data-probe-toggle-group-id="group.id"
            @update:model-value="(value) => emit('toggle', group, monitorFor(group), value)"
          />
          <button type="button" class="btn btn-secondary btn-sm whitespace-nowrap" @click="emit('configure', group, monitorFor(group))">
            {{ t('channelMonitorV2.settings.probeDetails') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import type { AdminGroup } from '@/types'
import Toggle from '@/components/common/Toggle.vue'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'

const props = withDefaults(defineProps<{
  groups: AdminGroup[]
  monitors: ChannelMonitor[]
  loading?: boolean
  busyGroupId?: number | null
}>(), {
  groups: () => [],
  monitors: () => [],
  loading: false,
  busyGroupId: null,
})

const emit = defineEmits<{
  (e: 'configure', group: AdminGroup, monitor: ChannelMonitor | null): void
  (e: 'toggle', group: AdminGroup, monitor: ChannelMonitor | null, enabled: boolean): void
}>()

const { t } = useI18n()
const orderedGroups = computed(() =>
  [...props.groups].sort((a, b) => {
    const platformOrder = a.platform.localeCompare(b.platform)
    return platformOrder || a.name.localeCompare(b.name)
  })
)

function monitorFor(group: AdminGroup): ChannelMonitor | null {
  return props.monitors.find((monitor) => monitor.group_id === group.id) || null
}

function monitorModelSummary(monitor: ChannelMonitor | null): string {
  if (!monitor) return ''
  const extra = monitor.extra_models?.length || 0
  return extra > 0
    ? t('channelMonitorV2.settings.probeModelSummary', { count: extra })
    : t('channelMonitorV2.settings.probePrimaryOnly')
}

function probeStateLabel(monitor: ChannelMonitor | null): string {
  if (!monitor) return t('channelMonitorV2.settings.probeNotConfigured')
  return monitor.enabled
    ? t('channelMonitorV2.settings.probeEnabled')
    : t('channelMonitorV2.settings.probeDisabled')
}

function probeStateClass(monitor: ChannelMonitor | null): string {
  if (!monitor) return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  return monitor.enabled
    ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
    : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function platformLabel(platform: string): string {
  return ({
    composite: t('channelMonitorV2.settings.compositePlatform'),
    anthropic: 'Anthropic',
    openai: 'OpenAI',
    gemini: 'Gemini',
    grok: 'Grok',
    kiro: 'Kiro',
    deepseek: 'DeepSeek',
    kimi: 'Kimi',
    zhipu: 'GLM',
    minimax: 'MiniMax',
  } as Record<string, string>)[platform] || platform
}
</script>
