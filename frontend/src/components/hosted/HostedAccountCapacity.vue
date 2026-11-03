<template>
  <CapacityBadge :color-class="capacityClass" :current="'—'" :max="maxConcurrency" tooltip="当前并发未提供，仅显示容量上限">
    <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z"
      />
    </svg>
  </CapacityBadge>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import CapacityBadge from '@/components/account/CapacityBadge.vue'

const props = defineProps<{
  concurrency: number
}>()

const maxConcurrency = computed(() => (props.concurrency > 0 ? props.concurrency : '∞'))
const capacityClass = computed(() =>
  props.concurrency > 0
    ? 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
    : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400'
)
</script>
