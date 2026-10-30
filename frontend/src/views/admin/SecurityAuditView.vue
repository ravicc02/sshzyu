<template>
  <AppLayout>
    <div class="space-y-6">
      <header class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-400">
            {{ t('nav.securityAudit') }}
          </p>
          <h1 class="mt-1 text-2xl font-semibold text-gray-950 dark:text-white">{{ t('nav.securityAudit') }}</h1>
          <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-300">{{ t('admin.securityAudit.description') }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <div class="tabs inline-flex w-fit" role="tablist" :aria-label="t('nav.securityAudit')">
            <button
              v-for="tab in tabs"
              :key="tab.id"
              type="button"
              role="tab"
              class="tab"
              :class="{ 'tab-active': activeTab === tab.id }"
              :aria-selected="activeTab === tab.id"
              :data-test="`security-audit-tab-${tab.id}`"
              @click="setTab(tab.id)"
            >
              {{ tab.label }}
            </button>
          </div>
          <RouterLink to="/admin/risk-control/config" class="btn btn-secondary" data-test="security-audit-settings">
            <Icon name="cog" size="sm" />
            {{ t('admin.securityAudit.settings') }}
          </RouterLink>
        </div>
      </header>

      <SecurityAuditOverview />
      <PromptFilterRecordsView v-if="activeTab === 'filter'" />
      <PromptAuditView v-else embedded mode="records" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import SecurityAuditOverview from './SecurityAuditOverview.vue'
import PromptAuditView from '@/features/prompt-audit/PromptAuditView.vue'
import PromptFilterRecordsView from '@/features/prompt-audit/PromptFilterRecordsView.vue'

type SecurityAuditTab = 'filter' | 'model'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const tabs = computed(() => [
  { id: 'filter' as const, label: t('admin.securityAudit.tabs.filterRecords') },
  { id: 'model' as const, label: t('admin.securityAudit.tabs.modelRecords') },
])
const activeTab = computed<SecurityAuditTab>(() => {
  if (route.query.tab === 'model' || route.query.tab === 'prompt') return 'model'
  return 'filter'
})

function setTab(tab: SecurityAuditTab) {
  void router.replace({ query: { ...route.query, tab } })
}

watch(
  () => route.query.tab,
  (value) => {
    if (value === 'config') {
      void router.replace('/admin/risk-control/config')
      return
    }
    if (value !== undefined && !['content', 'filter', 'prompt', 'model'].includes(String(value))) {
      void router.replace({ query: { ...route.query, tab: 'filter' } })
    }
  },
  { immediate: true },
)
</script>
