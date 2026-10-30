<template>
  <HelpTooltip
    width-class="w-64"
    panel-class="!bg-gray-950 !p-2.5"
    @click.stop
  >
    <div class="space-y-1.5">
      <div
        v-for="entry in rates"
        :key="entry.platform"
        class="flex items-center gap-2 rounded-lg px-2 py-1.5 ring-1 ring-white/10"
        :class="platformBadgeLightClass(entry.platform)"
      >
        <span class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-white/60 dark:bg-black/20">
          <PlatformIcon :platform="entry.platform" size="sm" />
        </span>
        <span class="min-w-0 flex-1 truncate font-medium">{{ platformLabel(entry.platform) }}</span>
        <span class="font-mono text-xs font-semibold">{{ Number(entry.rate_multiplier.toFixed(3)) }}x</span>
      </div>
    </div>
  </HelpTooltip>
</template>

<script setup lang="ts">
import HelpTooltip from './HelpTooltip.vue'
import PlatformIcon from './PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import { platformBadgeLightClass, platformLabel } from '@/utils/platformColors'

defineProps<{
  rates: Array<{ platform: GroupPlatform; rate_multiplier: number }>
}>()
</script>
