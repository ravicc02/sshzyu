<template>
  <nav class="account-management-tabs" :aria-label="t('nav.accounts')">
    <RouterLink to="/admin/accounts" :class="{ active: !hosted }" :aria-current="!hosted ? 'page' : undefined">
      <Icon name="globe" size="sm" />
      {{ t('nav.platformAccounts') }}
    </RouterLink>
    <RouterLink to="/admin/accounts/hosted" :class="{ active: hosted }" :aria-current="hosted ? 'page' : undefined">
      <Icon name="users" size="sm" />
      {{ t('nav.hostedAccounts') }}
    </RouterLink>
  </nav>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ hosted: boolean }>()
const { t } = useI18n()
</script>

<style scoped>
.account-management-tabs { display: flex; align-items: center; gap: 24px; margin-bottom: 16px; border-bottom: 1px solid var(--workspace-divider); }
.account-management-tabs a { display: inline-flex; align-items: center; gap: 8px; min-height: 40px; padding: 8px 0; border-bottom: 2px solid transparent; color: var(--workspace-muted); font-size: 13px; white-space: nowrap; }
.account-management-tabs a:hover { color: var(--workspace-ink); }
.account-management-tabs a.active { border-bottom-color: var(--workspace-accent); color: var(--workspace-accent); font-weight: 600; }
</style>
