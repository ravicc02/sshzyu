<template>
  <figure v-asset-visible="asset?.id" class="attachment-card" :class="{ compact, 'image-card': isImage }">
    <button v-if="asset?.url && isImage" type="button" class="attachment-thumbnail" aria-label="预览图片" @click="emit('preview')"><img loading="lazy" :src="asset.url" :alt="asset.filename || '图片附件'" @load="imageLoaded" @error="emit('imageError')" /><span class="image-overlay" aria-hidden="true"><Icon name="search" size="md" /></span></button>
    <button v-else-if="asset?.filename && !isImage && asset.content_available !== false && asset.status !== 'expired'" type="button" class="document-preview" aria-label="预览文件" @click="emit('preview')"><span class="file-type-icon" aria-hidden="true">{{ type }}</span><span class="file-info"><strong :title="asset.filename">{{ asset.filename || '文件附件' }}</strong><small>{{ type }} · {{ size }}</small></span></button>
    <div v-else class="attachment-placeholder" role="status"><span>{{ asset?.status === 'expired' ? `${asset.filename || '文件'} · 已过期，请重新上传` : asset?.failed ? '附件暂时不可用' : '正在载入附件…' }}</span><button v-if="asset?.failed && asset?.status !== 'expired'" type="button" @click="emit('retry')">重新加载</button></div>
    <button v-if="isImage && removable && asset?.url" type="button" class="image-remove" :disabled="disabled" :aria-label="`移除附件 ${asset.filename || ''}`" title="移除图片" @click.stop="emit('remove')"><Icon name="x" size="xs" /></button>
    <figcaption v-if="asset?.filename && !isImage && asset.content_available !== false && asset.status !== 'expired'"><button type="button" class="preview-action" @click="emit('preview')">预览</button><a v-if="asset.url" :href="asset.url" :download="asset.filename || asset.id">下载</a><button v-else type="button" @click="emit('download')">下载</button><button v-if="removable" type="button" :disabled="disabled" :aria-label="`移除附件 ${asset.filename || ''}`" @click="emit('remove')">移除</button></figcaption>
  </figure>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAgentAssetVisibility } from '@/composables/useAgentAssetVisibility'
import { useAgentMotion } from '@/composables/useAgentMotion'
import { attachmentIsImage, attachmentSize, attachmentType, type AgentAttachmentView } from '@/utils/agentAttachments'
const props = defineProps<{ asset?: AgentAttachmentView; compact?: boolean; removable?: boolean; disabled?: boolean; canReference?: boolean }>()
const emit = defineEmits<{ download: []; request: [id: string]; preview: []; remove: []; retry: []; loaded: []; imageError: []; reference: [] }>()
const vAssetVisible = useAgentAssetVisibility(id => emit('request', id))
const motion = useAgentMotion()
function imageLoaded(event: Event) { motion.reveal(event.target as HTMLImageElement); emit('loaded') }
const isImage = computed(() => attachmentIsImage(props.asset)), type = computed(() => attachmentType(props.asset)), size = computed(() => attachmentSize(props.asset?.byte_size))
</script>
<style scoped>
/* finesse · component: AgentAttachmentCard · register=product · tokens: inherited */
.attachment-card { width: min(320px, 100%); max-width: 100%; margin: 12px 0; border: 1px solid var(--workspace-divider); border-radius: 9px; background: var(--workspace-surface); overflow: hidden; color: var(--workspace-ink); }
.attachment-thumbnail { display: block; width: 100%; cursor: zoom-in; }.attachment-thumbnail img { display: block; width: 100%; max-height: 240px; object-fit: contain; background: var(--workspace-soft); }
.document-preview { display: flex; align-items: center; gap: 12px; width: 100%; padding: 14px; text-align: left; }.file-type-icon { min-width: 42px; padding: 12px 4px; border: 1px solid var(--workspace-divider); border-radius: 5px; color: var(--workspace-muted); font-size: 10px; text-align: center; }.file-info { min-width: 0; }.file-info strong, .image-info { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }.file-info small, .image-info small { display: block; color: var(--workspace-muted); font-size: 10px; margin-top: 4px; }
figcaption { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding: 9px 12px; font-size: 11px; }.image-info { min-width: 0; flex: 1 1 100%; }figcaption button, figcaption a { color: var(--workspace-accent); }.attachment-placeholder { padding: 16px; font-size: 12px; color: var(--workspace-muted); }.attachment-placeholder button { margin-left: 8px; }
.compact { display: inline-flex; align-items: center; flex-wrap: wrap; width: min(350px, 100%); margin: 10px 12px 0; }.compact .attachment-thumbnail { flex: 0 0 64px; width: 64px; }.compact .attachment-thumbnail img { width: 64px; height: 64px; object-fit: cover; }.compact figcaption { flex: 1; min-width: 0; }.compact .document-preview { padding-bottom: 6px; }.compact .file-type-icon { padding: 8px 4px; }
button:focus-visible, a:focus-visible { outline: 2px solid var(--workspace-accent); outline-offset: -2px; }button:disabled { opacity: .5; }
.image-card { position:relative; width:192px; height:140px; margin:10px 0; background:var(--workspace-soft); border-radius:10px; }.image-card .attachment-thumbnail { position:relative; height:100%; }.image-card .attachment-thumbnail img { width:100%; height:100%; max-height:none; object-fit:contain; }.image-card.compact { display:block; width:76px; height:76px; margin:0; }.image-card.compact .attachment-thumbnail { width:100%; height:100%; }.image-card.compact .attachment-thumbnail img { width:100%; height:100%; object-fit:cover; }.image-overlay { position:absolute; inset:0; display:flex; align-items:center; justify-content:center; color:#fff; background:#0005; opacity:0; }.image-remove { position:absolute; top:3px; right:3px; display:flex; align-items:center; justify-content:center; width:22px; height:22px; border-radius:50%; color:#fff; background:#202124cc; opacity:0; }.image-card:focus-within .image-overlay, .image-card:focus-within .image-remove { opacity:1; }@media (hover: hover) {.image-card:hover .image-overlay, .image-card:hover .image-remove { opacity:1; } }@media(hover:none) { .image-remove { opacity:1; } }

.image-card { border-radius:10px; }
.image-overlay, .image-remove { transition:opacity 160ms ease-out; }
.image-remove:active { transform:scale(.94); }
@media(prefers-reduced-motion:reduce) { .image-overlay, .image-remove { transition:none; } }

</style>
