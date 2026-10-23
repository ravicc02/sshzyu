<template>
  <button type="button" class="block-copy" :class="{ 'icon-only': iconOnly }" :data-state="state" :disabled="disabled || state === 'loading'" :aria-label="statusLabel" :title="statusLabel" @click="copy">
    <Icon :name="state === 'success' ? 'check' : 'copy'" size="sm" />
    <span v-if="!iconOnly">{{ state === 'success' ? '已复制' : state === 'loading' ? '复制中' : state === 'error' ? '重试复制' : '复制' }}</span>
    <span class="sr-only" role="status">{{ state === 'success' ? '已复制到剪贴板' : state === 'error' ? '复制失败，请重试或选择内容手动复制' : '' }}</span>
  </button>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import Icon from '@/components/icons/Icon.vue'
const props = withDefaults(defineProps<{ text: string; label?: string; iconOnly?: boolean; disabled?: boolean }>(), { label: '复制此段内容', iconOnly: false, disabled: false })
const state = ref<'default' | 'loading' | 'success' | 'error'>('default')
let timer: ReturnType<typeof setTimeout> | undefined
const statusLabel = computed(() => state.value === 'success' ? `${props.label}：已复制` : state.value === 'error' ? `${props.label}失败，点击重试` : props.label)
async function copy() {
  if (props.disabled || state.value === 'loading') return
  clearTimeout(timer); state.value = 'loading'
  try {
    if (!navigator.clipboard?.writeText) throw new Error('Clipboard unavailable')
    await navigator.clipboard.writeText(props.text)
    state.value = 'success'
    timer = setTimeout(() => { state.value = 'default' }, 2000)
  } catch { state.value = 'error' }
}
onBeforeUnmount(() => clearTimeout(timer))
</script>
<style scoped>
/* finesse · component: block-copy · register=product
 * states: default · hover · focus-visible · active · disabled · loading · error · success
 * tokens: inherited workspace palette */
.block-copy { display:inline-flex; align-items:center; justify-content:center; gap:6px; min-width:88px; height:32px; padding:5px 9px; border:1px solid transparent; border-radius:6px; color:var(--workspace-ink); background:transparent; font-size:11px; font-weight:500; white-space:nowrap; cursor:pointer; transition:background-color .12s,color .12s; }
.block-copy.icon-only { min-width:32px; width:32px; padding:7px; }
@media(hover:hover) { .block-copy:hover:not(:disabled),.block-copy.is-hover { color:var(--workspace-ink); background:var(--workspace-divider); } }
.block-copy:focus-visible,.block-copy.is-focus { outline:2px solid var(--workspace-accent); outline-offset:2px; }
.block-copy:active:not(:disabled),.block-copy.is-active { transform:translateY(1px); background:var(--workspace-divider); }
.block-copy:disabled { opacity:.5; cursor:not-allowed; }
.block-copy[data-state=loading] { cursor:progress; }
.block-copy[data-state=error] { color:var(--workspace-ink); text-decoration:underline dotted; text-underline-offset:3px; }
.block-copy[data-state=success] { color:var(--workspace-accent); }
@media(hover:none),(pointer:coarse),(max-width:640px) { .block-copy { min-height:44px; }.block-copy.icon-only { width:44px; min-width:44px; } }
@media(prefers-reduced-motion:reduce) { .block-copy { transition:none; } }
</style>
