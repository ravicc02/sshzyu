<template>
  <nav v-if="total > 1" class="image-navigation" aria-label="图片切换">
    <button type="button" aria-label="上一张图片" :disabled="index <= 0" @click="emit('select', index - 1)"><Icon name="chevronLeft" size="sm" /></button>
    <span role="status" aria-live="polite" aria-atomic="true">{{ index + 1 }} <span aria-hidden="true">/</span><span class="sr-only">，共</span> {{ total }}<span class="sr-only">张图片</span></span>
    <button type="button" aria-label="下一张图片" :disabled="index >= total - 1" @click="emit('select', index + 1)"><Icon name="chevronRight" size="sm" /></button>
  </nav>
</template>
<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
defineProps<{ index: number; total: number }>()
const emit = defineEmits<{ select: [index: number] }>()
</script>
<style scoped>
.image-navigation { position:absolute; inset:0; z-index:5; pointer-events:none; color:var(--workspace-muted); }
.image-navigation > span { position:absolute; bottom:0; left:50%; transform:translateX(-50%); min-width:54px; text-align:center; font-size:12px; font-variant-numeric:tabular-nums; white-space:nowrap; }
.image-navigation button { position:absolute; top:50%; transform:translateY(-50%); display:grid; place-items:center; width:36px; height:36px; border:1px solid var(--workspace-divider); border-radius:50%; pointer-events:auto; color:var(--workspace-ink); background:var(--workspace-surface); box-shadow:0 2px 8px color-mix(in srgb,var(--workspace-ink) 8%,transparent); }
.image-navigation button:first-child { left:5px; }.image-navigation button:last-child { right:5px; }
.image-navigation button:active:not(:disabled) { transform:translateY(-50%) scale(.94); }
.image-navigation button:disabled { opacity:.35; cursor:not-allowed; }
.image-navigation button:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:3px; }
@media(hover:hover) { .image-navigation button:hover:not(:disabled) { background:var(--workspace-selected); color:var(--workspace-accent); } }
@media(pointer:coarse) { .image-navigation button { width:44px; height:44px; } }
</style>
