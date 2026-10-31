<template>
  <AppLayout>
    <div class="w-full min-w-0 space-y-6 pb-8">
      <MonitorSettingsPanel>
        <template #probe-monitor="{ groups }">
          <section class="space-y-3 border-t border-gray-100 pt-5 dark:border-dark-700">
            <div class="flex items-start justify-between gap-3">
              <div>
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('channelMonitorV2.settings.probeTitle') }}</h3>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('channelMonitorV2.settings.probeDescription') }}</p>
              </div>
              <button type="button" class="btn btn-ghost btn-sm shrink-0" :disabled="loading" @click="reload(true)">
                <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
                {{ loading ? t('channelMonitorV2.settings.probeRefreshing') : t('channelMonitorV2.settings.probeRefresh') }}
              </button>
            </div>
            <ActiveProbeGroupList
              :groups="groups"
              :monitors="monitors"
              :loading="loading"
              :busy-group-id="busyGroupId"
              @configure="openProbeDetails"
              @toggle="toggleGroupProbe"
            />
            <MonitorRequestLogs :groups="groups" />
          </section>
        </template>
      </MonitorSettingsPanel>
    </div>

    <MonitorFormDialog
      :show="showDialog"
      :monitor="editing"
      :initial-group="editingGroup"
      :initial-enabled="initialMonitorEnabled"
      @close="closeDialog"
      @saved="reload"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import type { AdminGroup } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'
import MonitorSettingsPanel from '@/features/channel-monitor-v2/MonitorSettingsPanel.vue'
import ActiveProbeGroupList from '@/features/channel-monitor-v2/ActiveProbeGroupList.vue'
import MonitorRequestLogs from '@/features/channel-monitor-v2/MonitorRequestLogs.vue'

const { t } = useI18n()
const appStore = useAppStore()

const monitors = ref<ChannelMonitor[]>([])
const loading = ref(false)
const showDialog = ref(false)
const editing = ref<ChannelMonitor | null>(null)
const editingGroup = ref<AdminGroup | null>(null)
const initialMonitorEnabled = ref(false)
const busyGroupId = ref<number | null>(null)

async function reload(notify = false) {
  loading.value = true
  try {
    const res = await adminAPI.channelMonitor.list({ page: 1, page_size: 200 })
    monitors.value = res.items || []
    if (notify) appStore.showSuccess(t('channelMonitorV2.settings.probeRefreshSuccess'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.channelMonitor.loadError')))
  } finally {
    loading.value = false
  }
}

function openProbeDetails(group: AdminGroup, monitor: ChannelMonitor | null) {
  editing.value = monitor
  editingGroup.value = monitor ? null : group
  initialMonitorEnabled.value = monitor?.enabled === true
  showDialog.value = true
}

async function toggleGroupProbe(group: AdminGroup, monitor: ChannelMonitor | null, enabled: boolean) {
  if (busyGroupId.value != null) return
  if (!monitor) {
    if (!enabled) return
    editing.value = null
    editingGroup.value = group
    initialMonitorEnabled.value = true
    showDialog.value = true
    return
  }
  busyGroupId.value = group.id
  try {
    await adminAPI.channelMonitor.update(monitor.id, { enabled })
    await reload()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    busyGroupId.value = null
  }
}

function closeDialog() {
  showDialog.value = false
  editing.value = null
  editingGroup.value = null
  initialMonitorEnabled.value = false
}

onMounted(() => reload())
</script>
