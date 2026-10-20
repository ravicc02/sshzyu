<template>
  <component :is="embedded ? 'section' : 'dialog'" ref="dialog" class="image-preview" :class="{ embedded, 'natural-size': naturalSize, 'image-content': isImage }" :aria-label="isImage ? '图片预览' : '文件预览'" @cancel="emit('close')" @close="emit('close')" @click="onBackdrop">
    <header v-if="!embedded"><span>{{ isImage ? '图片预览' : '文件预览' }}</span><div><a v-if="url" :href="url" :download="filename">{{ isImage ? '下载原图' : '下载文件' }}</a><button type="button" :aria-label="isImage ? '关闭图片预览' : '关闭文件预览'" @click="emit('close')">×</button></div></header>
    <img v-if="url && isImage" :src="url" alt="图片完整预览" @error="emit('imageError')" />
    <div v-else class="file-preview-content"><h3>{{ filename || '文件附件' }}</h3><p>{{ attachmentType({ filename, mime_type: mime }) }} · {{ attachmentSize(size) }}</p><pre v-if="text">{{ text }}</pre><p v-else class="preview-note">可查看文件名称、类型和大小，下载后使用对应应用打开。</p></div>
  </component>
</template>
<script setup lang="ts">
import { computed, ref, watch, nextTick } from 'vue'
import { attachmentType, attachmentSize } from '@/utils/agentAttachments'
const props = defineProps<{ embedded?: boolean; naturalSize?: boolean; url?: string; filename?: string; mime?: string; size?: number; text?: string }>()
const isImage = computed(() => !props.mime || props.mime.startsWith('image/'))
const emit = defineEmits<{ close: []; imageError: [] }>()
const dialog = ref<HTMLDialogElement>()
watch(() => props.url, async url => { if (props.embedded) return; await nextTick(); if (url && !dialog.value?.open) dialog.value?.showModal(); else if (!url && dialog.value?.open) dialog.value?.close() }, { immediate: true })
function onBackdrop(event: MouseEvent) { if (!props.embedded && event.target === dialog.value) { const box = dialog.value.getBoundingClientRect(); if (event.clientX < box.left || event.clientX > box.right || event.clientY < box.top || event.clientY > box.bottom) emit('close') } }
</script>
<style scoped>
.image-preview { max-width: min(1100px, 94vw); max-height: 94dvh; padding: 12px; border: 1px solid var(--workspace-divider); border-radius: 12px; background: var(--workspace-surface); color: var(--workspace-ink); margin: auto; }
.image-preview::backdrop { background: rgb(0 0 0 / .75); } header, header div { display: flex; align-items: center; gap: 20px; } header { justify-content: space-between; padding: 2px 4px 12px; font-size: 13px; } header a { color: var(--workspace-accent); } header button { font-size: 24px; padding: 0 8px; } img { display: block; max-width: 100%; max-height: calc(94dvh - 80px); object-fit: contain; margin: auto; } button:focus-visible, a:focus-visible { outline: 2px solid var(--workspace-accent); outline-offset: 3px; }
.file-preview-content { width: min(720px, 85vw); padding: 16px; }.file-preview-content h3 { overflow-wrap: anywhere; font-weight: 600; margin-bottom: 8px; }.file-preview-content p { font-size: 12px; color: var(--workspace-muted); }.file-preview-content pre { max-height: 65dvh; overflow: auto; white-space: pre-wrap; overflow-wrap: anywhere; background: var(--workspace-soft); border-radius: 8px; padding: 16px; margin-top: 16px; font-size: 12px; line-height: 1.8; }.preview-note { margin-top: 28px; }
.image-preview.embedded { width:100%; max-width:none; max-height:none; margin:0; padding:0; border:0; border-radius:0; background:transparent; }.image-preview.embedded.image-content { display:flex; align-items:center; justify-content:center; min-height:100%; }.image-preview.embedded.image-content img { flex-shrink:0; }.image-preview.embedded img { width:100%; height:auto; max-height:none; }.image-preview.embedded.natural-size { width:max-content; min-width:100%; }.image-preview.embedded.natural-size img { width:auto; max-width:none; }.image-preview.embedded .file-preview-content { width:100%; padding:0; }.image-preview.embedded pre { max-height:none; }
</style>
