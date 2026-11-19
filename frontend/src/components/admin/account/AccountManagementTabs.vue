<template>
  <nav class="account-management-tabs" :aria-label="t('nav.accounts')">
    <RouterLink to="/admin/accounts" @click="$emit('standard')" :class="{ active: !hosted && !archived }" :aria-current="!hosted && !archived ? 'page' : undefined">
      <Icon name="globe" size="sm" />
      {{ t('nav.platformAccounts') }}
    </RouterLink>
    <RouterLink to="/admin/accounts/hosted" @click="$emit('standard')" :class="{ active: hosted && !archived }" :aria-current="hosted && !archived ? 'page' : undefined">
      <Icon name="users" size="sm" />
      {{ t('nav.hostedAccounts') }}
    </RouterLink>
    <button type="button" :class="{ active: archived }" :aria-pressed="!!archived" @click="$emit('archive')">
      <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" d="M3 4h18v4H3zM5 8v12h14V8M10 12h4" /></svg>
      {{ t('admin.accounts.archivedAccounts') }}
    </button>
  </nav>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ hosted: boolean; archived?: boolean }>()
defineEmits(['archive', 'standard'])
const { t } = useI18n()
</script>

<style scoped>
.account-management-tabs { display: flex; align-items: center; gap: 24px; margin-bottom: 16px; border-bottom: 1px solid var(--workspace-divider); }
.account-management-tabs a, .account-management-tabs button { display: inline-flex; align-items: center; gap: 8px; min-height: 40px; padding: 8px 0; border-bottom: 2px solid transparent; color: var(--workspace-muted); font-size: 13px; white-space: nowrap; }
.account-management-tabs a:hover, .account-management-tabs button:hover { color: var(--workspace-ink); }
.account-management-tabs a.active, .account-management-tabs button.active { border-bottom-color: var(--workspace-accent); color: var(--workspace-accent); font-weight: 600; }
</style>
