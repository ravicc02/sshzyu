<template>
  <div class="hosted-account-meta">
    <PlatformTypeBadge
      :platform="account.account_platform || 'openai'"
      :type="account.account_type || 'oauth'"
      :auth-mode="account.auth_mode"
      :plan-type="account.quota.plan_type || account.plan_type"
      :privacy-mode="account.privacy_mode"
    />
    <div v-if="compactMeta" :class="['compact-state', compactMeta.className]" :title="compactMeta.title">
      <span :class="['compact-dot', compactMeta.dotClass]" />
      <span>{{ compactMeta.label }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import type { HostedOpenAIAccount } from '@/api/hostedOpenAIAccounts'

const props = defineProps<{
  account: HostedOpenAIAccount
}>()

const compactMeta = computed(() => {
  const state = props.account.openai_compact_state
  if (!state) return null
  if (state === 'active') {
    return {
      label: 'Compact On',
      title: 'OpenAI Compact 已启用',
      className: 'is-active',
      dotClass: 'is-active'
    }
  }
  if (state === 'blocked') {
    return {
      label: 'Compact Off',
      title: 'OpenAI Compact 不可用',
      className: 'is-blocked',
      dotClass: 'is-blocked'
    }
  }
  return {
    label: 'Compact Auto',
    title: 'OpenAI Compact 自动判断',
    className: 'is-auto',
    dotClass: 'is-auto'
  }
})
</script>

<style scoped>
.hosted-account-meta {
  display: grid;
  min-width: 0;
  width: fit-content;
  gap: 5px;
}

.hosted-account-meta :deep(.inline-flex.flex-col) {
  max-width: 100%;
}

.hosted-account-meta :deep(.inline-flex.flex-col > div) {
  max-width: 100%;
  flex-wrap: wrap;
}

.compact-state {
  display: inline-flex;
  align-items: center;
  width: fit-content;
  gap: 6px;
  padding-left: 1px;
  color: var(--text-tertiary);
  font-size: 10px;
  font-weight: 550;
  line-height: 1.2;
}

.compact-state.is-active {
  color: #059669;
}

.compact-state.is-blocked {
  color: #e11d48;
}

.compact-dot {
  width: 6px;
  height: 6px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--text-tertiary);
}

.compact-dot.is-active {
  background: #10b981;
  box-shadow: 0 0 0 2px rgb(16 185 129 / 0.14);
}

.compact-dot.is-blocked {
  background: #f43f5e;
  box-shadow: 0 0 0 2px rgb(244 63 94 / 0.14);
}

.compact-dot.is-auto {
  background: #94a3b8;
}
</style>
