<template>
  <div class="space-y-2">
    <label
      class="flex cursor-pointer items-center gap-3 rounded-md border border-dashed border-gray-300 bg-white p-3 transition-colors hover:border-primary-400 focus-within:ring-2 focus-within:ring-primary-500 dark:border-dark-600 dark:bg-dark-800"
      :class="[compact ? 'min-h-12' : 'min-h-[92px]', uploadDisabled ? 'pointer-events-none opacity-60' : '']"
      @dragover.prevent
      @drop.prevent="dropFiles"
    >
      <input
        type="file"
        accept="image/png,image/jpeg,image/webp"
        class="sr-only"
        :multiple="multiple"
        :disabled="uploadDisabled"
        :data-testid="testId"
        :aria-label="label"
        @change="selectFiles"
      />
      <img v-if="!multiple && images[0]?.data" :src="source(images[0])" :alt="t('batchImage.config.referencePreview')" class="h-12 w-12 rounded-md object-cover" />
      <Icon v-else name="upload" size="md" class="flex-shrink-0 text-gray-500 dark:text-gray-300" />
      <span class="min-w-0 flex-1">
        <span class="block truncate text-sm font-medium text-gray-800 dark:text-gray-100">{{ !multiple && names[0] ? names[0] : label }}</span>
        <span class="mt-1 block text-xs text-gray-600 dark:text-gray-300">PNG / JPEG / WebP · 10 MB</span>
      </span>
      <span v-if="multiple" class="flex-shrink-0 text-xs text-gray-600 dark:text-gray-300">{{ images.length }} / {{ limit }}</span>
      <span v-else-if="images.length" class="text-xs text-primary-600 dark:text-primary-400">{{ t('batchImage.config.replaceReference') }}</span>
    </label>
    <div v-if="multiple && images.length" class="space-y-2">
      <div v-for="(image, index) in images" :key="index" class="flex min-w-0 items-center gap-3">
        <img v-if="image.data" :src="source(image)" :alt="t('batchImage.config.referencePreview')" class="h-12 w-12 flex-shrink-0 rounded-md object-cover" />
        <Icon v-else name="document" size="md" class="flex-shrink-0 text-gray-500" />
        <span class="min-w-0 flex-1 break-all text-xs text-gray-700 dark:text-gray-200">{{ names[index] || t('batchImage.config.referenceReupload', { index: index + 1 }) }}</span>
        <button type="button" class="btn btn-ghost btn-icon flex-shrink-0" :disabled="disabled || loading" :title="t('batchImage.config.removeReference')" :aria-label="t('batchImage.config.removeReference')" :data-testid="removeTestId" @click="emit('remove', index)">
          <Icon name="x" size="sm" />
        </button>
      </div>
    </div>
    <div class="flex items-center justify-between gap-3">
      <p v-if="loading" class="text-xs text-gray-600 dark:text-gray-300" role="status">{{ t('batchImage.config.referenceLoading') }}</p>
      <p v-else-if="error" class="text-xs text-red-600 dark:text-red-400" role="alert">{{ error }}</p>
      <p v-else-if="limit <= 0" class="text-xs text-gray-600 dark:text-gray-300">{{ t('batchImage.config.referenceUnsupported') }}</p>
      <p v-else-if="required && !images.length" class="text-xs text-amber-700 dark:text-amber-300">{{ t('batchImage.config.referenceRequired') }}</p>
      <p v-else-if="multiple" class="text-xs text-gray-600 dark:text-gray-300">{{ t('batchImage.config.referenceLimit', { limit }) }}</p>
      <span v-else />
      <button v-if="images.length || error" type="button" class="btn btn-ghost btn-sm flex-shrink-0" :disabled="disabled" :data-testid="clearTestId" @click="emit('clear')">{{ t('batchImage.config.removeReference') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { BatchImageReferenceImage } from '@/api/batchImage'

const props = defineProps<{
  images: BatchImageReferenceImage[]
  names: string[]
  label: string
  limit: number
  multiple: boolean
  disabled: boolean
  loading: boolean
  error?: string
  required?: boolean
  compact?: boolean
  testId: string
  removeTestId?: string
  clearTestId?: string
}>()
const emit = defineEmits<{ files: [files: File[]]; remove: [index: number]; clear: [] }>()
const { t } = useI18n()
const uploadDisabled = computed(() => props.disabled || props.loading || props.limit <= 0 || (props.multiple && props.images.filter(image => image.data).length >= props.limit))

function source(image: BatchImageReferenceImage) {
  return `data:${image.mime_type};base64,${image.data}`
}

function selectFiles(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!uploadDisabled.value && files.length) emit('files', files)
}

function dropFiles(event: DragEvent) {
  const files = Array.from(event.dataTransfer?.files || [])
  if (!uploadDisabled.value && files.length) emit('files', files)
}
</script>
