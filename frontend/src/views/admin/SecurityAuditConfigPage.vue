<template>
  <AppLayout>
    <div class="space-y-5">
      <header class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-400">
            {{ t('nav.securityAudit') }}
          </p>
          <h1 class="mt-1 text-2xl font-semibold text-gray-950 dark:text-white">{{ t('admin.securityAudit.settings') }}</h1>
          <p class="mt-2 max-w-3xl text-sm text-gray-500 dark:text-dark-300">{{ t('admin.securityAudit.settingsDescription') }}</p>
        </div>
        <RouterLink to="/admin/risk-control" class="btn btn-secondary w-fit">
          <Icon name="chevronLeft" size="sm" />
          {{ t('admin.securityAudit.backToRecords') }}
        </RouterLink>
      </header>
      <SecurityAuditConfigView />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import SecurityAuditConfigView from './SecurityAuditConfigView.vue'

const { t } = useI18n()
</script>
