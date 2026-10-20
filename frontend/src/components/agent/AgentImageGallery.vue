<template>
  <section v-if="assets.length" class="image-gallery" role="region" aria-roledescription="轮播" aria-label="生成的图片" tabindex="0" @keydown="onKeydown">
    <header class="gallery-heading"><span>{{ assets.length }} 张图片</span></header>
    <div class="gallery-stage" @touchstart.passive="onTouchStart" @touchend="onTouchEnd" @touchcancel="touchStart = undefined">
      <div v-for="card in visibleCards" :key="card.asset.id" v-asset-visible="card.asset.id" class="gallery-card" :class="{ current: card.offset === 0, previous: card.offset < 0, next: card.offset > 0 }" :aria-hidden="card.offset !== 0 || undefined" :inert="card.offset !== 0 || undefined">
        <button v-if="available(card.asset) && !card.asset.failed" type="button" class="gallery-image" :aria-label="`预览第 ${selectedIndex + 1} 张图片`" @click="emit('preview', card.asset.id)">
          <img loading="lazy" :src="card.asset.url" :alt="card.asset.filename || `生成图片 ${card.index + 1}`" draggable="false" @load="emit('loaded')" @error="emit('imageError', card.asset.id)" />
          <span class="gallery-zoom" aria-hidden="true"><Icon name="search" size="md" /></span>
        </button>
        <div v-else class="gallery-placeholder" role="status">
          <Icon name="photograph" size="lg" />
          <span>{{ expired(card.asset) ? '图片已过期' : card.asset.failed ? '图片暂时无法加载' : '正在加载图片…' }}</span>
          <button v-if="card.asset.failed && !expired(card.asset)" type="button" @click="emit('retry', card.asset.id)">重新加载</button>
        </div>
      </div>
      <AgentImageNavigation :index="selectedIndex" :total="assets.length" @select="select" />
    </div>
    <div ref="thumbnails" class="gallery-thumbnails" aria-label="选择图片">
      <button v-for="(asset, index) in assets" :key="asset.id" v-asset-visible="asset.id" type="button" :class="{ selected: asset.id === selectedId }" :aria-label="`查看第 ${index + 1} 张图片`" :aria-pressed="asset.id === selectedId" @click="select(index)">
        <img v-if="available(asset) && !asset.failed" :src="asset.url" alt="" loading="lazy" draggable="false" @error="emit('imageError', asset.id)" />
        <Icon v-else name="photograph" size="sm" />
        <span class="thumbnail-number">{{ index + 1 }}</span>
      </button>
    </div>
  </section>
</template>
<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAgentAssetVisibility } from '@/composables/useAgentAssetVisibility'
import AgentImageNavigation from './AgentImageNavigation.vue'
import type { AgentAttachmentView } from '@/utils/agentAttachments'
const props = defineProps<{ assets: AgentAttachmentView[] }>()
const emit = defineEmits<{ request: [id: string]; preview: [id: string]; imageError: [id: string]; retry: [id: string]; loaded: [] }>()
const vAssetVisible = useAgentAssetVisibility(id => emit('request', id))
const selectedId = ref(''), thumbnails = ref<HTMLElement>()
const selectedIndex = computed(() => Math.max(0, props.assets.findIndex(asset => asset.id === selectedId.value)))
const visibleCards = computed(() => props.assets.map((asset, index) => ({ asset, index, offset: index - selectedIndex.value })).filter(card => Math.abs(card.offset) <= 1))
watch(() => props.assets.map(asset => asset.id), ids => {
  if (!ids.includes(selectedId.value)) selectedId.value = ids[0] || ''
}, { immediate: true })
function expired(asset: AgentAttachmentView) { return asset.status === 'expired' || asset.content_available === false }
function available(asset: AgentAttachmentView) { return !!asset.url && !expired(asset) }
function select(index: number) {
  const asset = props.assets[index]
  if (!asset) return
  selectedId.value = asset.id
  void nextTick(() => thumbnails.value?.querySelector('[aria-pressed="true"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest', behavior: 'auto' }))
}
function onKeydown(event: KeyboardEvent) {
  if (event.altKey || event.ctrlKey || event.metaKey || !['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  select(event.key === 'Home' ? 0 : event.key === 'End' ? props.assets.length - 1 : selectedIndex.value + (event.key === 'ArrowLeft' ? -1 : 1))
}
let touchStart: { x: number; y: number } | undefined
function onTouchStart(event: TouchEvent) {
  const touch = event.touches.length === 1 ? event.touches[0] : undefined
  touchStart = touch ? { x: touch.clientX, y: touch.clientY } : undefined
}
function onTouchEnd(event: TouchEvent) {
  const touch = event.changedTouches[0], start = touchStart
  touchStart = undefined
  if (!touch || !start) return
  const dx = touch.clientX - start.x, dy = touch.clientY - start.y
  if (Math.abs(dx) < 45 || Math.abs(dx) <= Math.abs(dy)) return
  event.preventDefault()
  select(selectedIndex.value + (dx < 0 ? 1 : -1))
}
</script>
<style scoped>
/* finesse · component: AgentImageGallery · register=product · tokens: inherited */
.image-gallery { width:min(560px,100%); min-width:0; margin:14px 0 6px; outline:none; }
.image-gallery:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:5px; border-radius:10px; }
.gallery-heading { display:flex; align-items:center; justify-content:space-between; gap:12px; min-height:24px; margin-bottom:4px; color:var(--workspace-muted); font-size:12px; }
.gallery-stage { position:relative; isolation:isolate; aspect-ratio:16/10; overflow:hidden; touch-action:pan-y; }
.gallery-card { position:absolute; top:8%; left:max(10%,50px); width:calc(100% - max(20%,100px)); height:84%; display:grid; place-items:center; overflow:hidden; border:1px solid var(--workspace-divider); border-radius:12px; background:var(--workspace-surface); box-shadow:0 8px 18px color-mix(in srgb,var(--workspace-ink) 9%,transparent); transition:transform 240ms ease, opacity 240ms ease; }
.gallery-card.current { z-index:3; transform:translateX(0) scale(1); }
.gallery-card.previous { z-index:1; transform:translateX(-15%) scale(.88) rotate(-4deg); opacity:.6; }
.gallery-card.next { z-index:2; transform:translateX(15%) scale(.88) rotate(4deg); opacity:.6; }
.gallery-image { display:block; position:relative; width:100%; height:100%; cursor:zoom-in; background:var(--workspace-soft); }
.gallery-image img { display:block; width:100%; height:100%; object-fit:contain; }
.gallery-zoom { position:absolute; right:10px; bottom:10px; display:grid; place-items:center; width:34px; height:34px; border-radius:50%; color:var(--workspace-ink); background:var(--workspace-surface); opacity:0; transition:opacity 160ms; }
.gallery-image:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:-3px; }.gallery-image:focus-visible .gallery-zoom { opacity:1; }
.gallery-placeholder { display:flex; align-items:center; justify-content:center; flex-direction:column; gap:12px; padding:16px; color:var(--workspace-muted); font-size:12px; text-align:center; }
.gallery-placeholder button { padding:8px 12px; border-radius:6px; color:var(--workspace-accent); background:var(--workspace-selected); }
.gallery-thumbnails { display:flex; gap:8px; overflow-x:auto; overscroll-behavior-x:contain; padding:6px 2px; scrollbar-width:thin; }
.gallery-thumbnails button { position:relative; flex:0 0 64px; height:46px; display:grid; place-items:center; overflow:hidden; border:1px solid var(--workspace-divider); border-radius:6px; color:var(--workspace-muted); background:var(--workspace-soft); opacity:.6; }
.gallery-thumbnails img { display:block; width:100%; height:100%; object-fit:contain; }
.gallery-thumbnails button.selected { opacity:1; border-color:var(--workspace-accent); box-shadow:0 0 0 1px var(--workspace-accent); }
.thumbnail-number { position:absolute; bottom:1px; right:2px; min-width:14px; border-radius:3px; background:var(--workspace-surface); color:var(--workspace-ink); font-size:9px; text-align:center; }
.gallery-thumbnails button:focus-visible,.gallery-placeholder button:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:-3px; }
.gallery-thumbnails button:active { transform:scale(.96); }
@media(hover:hover) { .gallery-image:hover .gallery-zoom { opacity:1; }.gallery-thumbnails button:hover { opacity:1; } }
@media(prefers-reduced-motion:reduce) { .gallery-card,.gallery-zoom { transition:none; } }
</style>
