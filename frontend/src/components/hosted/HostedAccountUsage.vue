<template>
  <div class="hosted-account-usage">
    <div v-if="hasUsage" class="usage-windows">
      <UsageProgressBar
        v-if="account.quota.five_hour"
        label="5h"
        :utilization="account.quota.five_hour.used_percent"
        :resets-at="account.quota.five_hour.reset_at"
        :show-now-when-idle="true"
        color="indigo"
      />
      <div v-else class="usage-empty">
        <span>5h</span>
        <i />
        <em>暂无数据</em>
      </div>
      <UsageProgressBar
        v-if="account.quota.weekly"
        label="7d"
        :utilization="account.quota.weekly.used_percent"
        :resets-at="account.quota.weekly.reset_at"
        :show-now-when-idle="true"
        color="emerald"
      />
      <div v-else class="usage-empty">
        <span>7d</span>
        <i />
        <em>暂无数据</em>
      </div>
    </div>
    <div v-else class="usage-empty-state">
      <span>5h</span>
      <i />
      <em>暂无数据</em>
      <span>7d</span>
      <i />
      <em>暂无数据</em>
    </div>

    <div v-if="account.quota.error" class="usage-note is-error" :title="account.quota.error">
      {{ account.quota.error }}
    </div>
    <div v-else-if="account.quota.fetched_at" class="usage-note">
      更新于 {{ formatTime(account.quota.fetched_at) }}
    </div>
    <div v-else class="usage-note">刷新后读取实时窗口</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import UsageProgressBar from '@/components/account/UsageProgressBar.vue'
import type { HostedOpenAIAccount } from '@/api/hostedOpenAIAccounts'

const props = defineProps<{
  account: HostedOpenAIAccount
}>()

const hasUsage = computed(() => Boolean(props.account.quota.five_hour || props.account.quota.weekly))

function formatTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })
}
</script>

<style scoped>
.hosted-account-usage {
  display: grid;
  min-width: 0;
  gap: 5px;
}

.usage-windows,
.usage-empty-state {
  display: grid;
  gap: 4px;
}

.usage-empty-state {
  grid-template-columns: auto 48px auto;
  align-items: center;
  justify-content: start;
  column-gap: 7px;
  row-gap: 6px;
}

.usage-empty {
  display: grid;
  grid-template-columns: 30px 48px auto;
  align-items: center;
  gap: 7px;
  min-height: 18px;
}

.usage-empty span,
.usage-empty-state > span {
  display: inline-flex;
  width: fit-content;
  min-width: 30px;
  height: 18px;
  align-items: center;
  justify-content: center;
  padding: 0 5px;
  border-radius: 4px;
  color: var(--text-secondary);
  background: var(--surface-secondary);
  font-size: 9px;
  font-weight: 650;
}

.usage-empty i,
.usage-empty-state > i {
  display: block;
  width: 48px;
  height: 4px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--text-tertiary) 30%, transparent);
}

.usage-empty em,
.usage-empty-state > em {
  color: var(--text-tertiary);
  font-size: 9px;
  font-style: normal;
  white-space: nowrap;
}

.usage-note {
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 9px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.usage-note.is-error {
  color: var(--error-600);
}
</style>
