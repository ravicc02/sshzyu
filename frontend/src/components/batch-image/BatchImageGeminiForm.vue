<template>
  <div class="space-y-5" data-testid="gemini-create-form">
    <div class="grid gap-4 md:grid-cols-2">
      <div>
        <label class="input-label">{{ t('batchImage.create.model') }}</label>
        <select v-model="form.model" class="input" :disabled="loadingModels || models.length === 0 || submitting">
          <option v-if="loadingModels" value="">{{ t('batchImage.messages.loadingModels') }}</option>
          <option v-else-if="models.length === 0" value="">{{ t('batchImage.messages.noModels') }}</option>
          <option v-for="model in models" :key="model.value" :value="model.value">{{ model.label }}</option>
        </select>
        <p v-if="modelError" class="input-error-text mt-1">{{ modelError }}</p>
      </div>

      <div>
        <label class="input-label">{{ t('batchImage.create.outputFormat') }}</label>
        <select v-model="form.responseMimeType" class="input" :disabled="submitting">
          <option v-for="mime in outputMimeOptions" :key="mime" :value="mime">{{ mimeLabel(mime) }}</option>
        </select>
      </div>

      <div>
        <label class="input-label">{{ t('batchImage.create.imageSize') }}</label>
        <select v-model="form.imageSize" class="input" :disabled="submitting" data-testid="gemini-image-size">
          <option v-for="size in imageSizeOptions" :key="size" :value="size">{{ size }}</option>
        </select>
        <p class="input-hint">{{ t('batchImage.create.imageSizeHint') }}</p>
      </div>

      <div>
        <label class="input-label">{{ t('batchImage.create.aspectRatio') }}</label>
        <select v-model="form.aspectRatio" class="input" :disabled="submitting" data-testid="gemini-aspect-ratio">
          <option v-for="[ratio, pixels] in aspectRatioOptions" :key="ratio" :value="ratio">{{ ratio }} · {{ pixels }}</option>
        </select>
      </div>
    </div>

    <section class="space-y-3">
      <div class="flex items-center justify-between gap-3">
        <label class="input-label mb-0">Prompt</label>
        <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('batchImage.create.promptAdded', { count: rows.length }) }}</span>
      </div>

      <div class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700">
        <textarea v-model="draft.prompt" rows="4" class="input resize-y text-sm leading-6" :disabled="submitting" :placeholder="t('batchImage.create.promptPlaceholder')" />
        <div class="grid gap-2 md:grid-cols-[minmax(0,1fr)_112px_auto_auto] md:items-center">
          <input v-model="draft.customId" type="text" maxlength="255" class="input h-9 text-sm" :disabled="submitting" :placeholder="t('batchImage.create.customIdPlaceholder')" />
          <select v-model.number="draft.outputCount" class="input h-9 text-sm" :disabled="submitting" :aria-label="t('batchImage.create.outputCountPerPrompt')">
            <option v-for="count in outputCountOptions" :key="count" :value="count">{{ t('batchImage.create.outputCountOption', { n: count }, count) }}</option>
          </select>
          <label class="btn btn-secondary h-9 cursor-pointer justify-center text-sm" :class="uploadDisabled ? 'pointer-events-none opacity-60' : ''" @dragover.prevent @drop.prevent="dropReferenceFiles">
            <Icon name="upload" size="sm" class="mr-1.5" />
            {{ t('batchImage.create.referenceImage') }}
            <input type="file" accept="image/png,image/jpeg,image/webp" multiple class="hidden" :disabled="uploadDisabled" data-testid="gemini-reference-input" @change="loadReferenceFiles" />
          </label>
          <button type="button" class="btn btn-secondary h-9 justify-center px-4 text-sm" :disabled="submitting || !draft.prompt.trim()" @click="addRow">
            <Icon name="plus" size="sm" class="mr-1.5" />
            {{ t('common.add') }}
          </button>
        </div>

        <div v-if="draft.references.length" class="flex flex-wrap gap-2">
          <span v-for="(reference, index) in draft.references" :key="`${reference.name}-${index}`" class="inline-flex max-w-full items-center gap-2 rounded-md border border-gray-200 bg-gray-50 px-2 py-1 text-xs dark:border-dark-700 dark:bg-dark-900">
            <span class="max-w-[180px] truncate">{{ reference.name }}</span>
            <button type="button" class="btn-icon text-gray-400 hover:text-red-600" :title="t('batchImage.create.removeReferenceImage')" @click="removeDraftReference(index)">
              <Icon name="x" size="xs" />
            </button>
          </span>
        </div>
        <p v-if="referenceError" class="input-error-text" role="alert">{{ referenceError }}</p>
        <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('batchImage.create.limitsHint', { maxPerItem: 4, maxPerJob: 200, refLimit: referenceLimit }) }}</p>
      </div>

      <div v-if="rows.length" class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
        <div v-for="(row, index) in rows" :key="row.localId" class="flex items-center gap-3 border-b border-gray-100 px-3 py-2 last:border-b-0 dark:border-dark-700">
          <span class="w-20 flex-shrink-0 font-mono text-xs text-gray-500">{{ row.custom_id }}</span>
          <p class="min-w-0 flex-1 truncate text-sm text-gray-800 dark:text-gray-100">{{ row.prompt }}</p>
          <span v-if="row.output_count > 1" class="text-xs text-gray-500">×{{ row.output_count }}</span>
          <span v-if="row.reference_images.length" class="text-xs text-gray-500">{{ t('batchImage.create.referenceCount', { n: row.reference_images.length }, row.reference_images.length) }}</span>
          <button type="button" class="btn-ghost btn-icon flex-shrink-0 text-red-600" :title="t('common.delete')" :disabled="submitting" @click="removeRow(index)">
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
      <div v-else class="rounded-lg border border-dashed border-gray-200 px-3 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
        {{ t('batchImage.create.noPrompts') }}
      </div>
    </section>

    <div class="flex justify-end">
      <button type="button" class="btn btn-primary inline-flex min-w-[156px] justify-center" :disabled="!canSubmit" data-testid="submit-gemini" @click="submit">
        <Icon v-if="submitting" name="refresh" size="sm" class="mr-2 animate-spin" />
        {{ submitting ? t('common.submitting') : t('batchImage.config.submit') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { batchImageMimeTypes, geminiImageSizes } from '@/utils/batchImage'
import type { BatchImageReferenceImage, BatchImageSubmitItem, BatchImageSubmitRequest } from '@/api/batchImage'

type ModelOption = { value: string; label: string; supported_image_sizes?: string[]; supported_mime_types?: string[] }
type ReferenceDraft = BatchImageReferenceImage & { name: string; size: number }
type PromptRow = { localId: string; custom_id: string; prompt: string; output_count: number; reference_images: BatchImageReferenceImage[] }

const props = defineProps<{ models: ModelOption[]; loadingModels: boolean; modelError?: string; submitting: boolean }>()
const emit = defineEmits<{ submit: [payload: BatchImageSubmitRequest] }>()
const { t } = useI18n()
const outputCountOptions = [1, 2, 3, 4]
const rows = ref<PromptRow[]>([])
const referenceError = ref('')
const form = reactive({ model: '', imageSize: '1K', aspectRatio: '1:1', responseMimeType: 'image/png' })
const draft = reactive({ prompt: '', customId: '', outputCount: 1, references: [] as ReferenceDraft[] })

const selectedModel = computed(() => props.models.find(model => model.value === form.model))
const imageSizeOptions = computed(() => Object.keys(geminiImageSizes).filter(size => !selectedModel.value?.supported_image_sizes || selectedModel.value.supported_image_sizes.includes(size)))
const aspectRatioOptions = computed(() => Object.entries(geminiImageSizes[form.imageSize] || {}))
const outputMimeOptions = computed(() => batchImageMimeTypes.filter(mime => !selectedModel.value?.supported_mime_types || selectedModel.value.supported_mime_types.includes(mime)))
const referenceLimit = computed(() => {
  const model = form.model.toLowerCase()
  if (model.includes('pro-image')) return 14
  if (model.includes('flash-image')) return 3
  return 0
})
const uploadDisabled = computed(() => props.submitting || referenceLimit.value <= 0 || draft.references.length >= referenceLimit.value)
const estimatedOutputs = computed(() => rows.value.reduce((sum, row) => sum + row.output_count, 0))
const canSubmit = computed(() => !props.submitting && !props.loadingModels && !!form.model && rows.value.length > 0 && estimatedOutputs.value <= 200 && !!geminiImageSizes[form.imageSize]?.[form.aspectRatio])

watch(() => props.models, models => {
  if (!models.some(model => model.value === form.model)) form.model = models[0]?.value || ''
}, { immediate: true })
watch(imageSizeOptions, sizes => {
  if (!sizes.includes(form.imageSize)) form.imageSize = sizes[0] || ''
})
watch(aspectRatioOptions, options => {
  if (!options.some(([ratio]) => ratio === form.aspectRatio)) form.aspectRatio = options[0]?.[0] || ''
})
watch(outputMimeOptions, options => {
  if (!options.includes(form.responseMimeType)) form.responseMimeType = options[0] || ''
})

function normalizeID(raw: string, index: number) {
  return raw.replace(/[^\w.-]+/g, '_').replace(/^_+|_+$/g, '') || `img_${String(index + 1).padStart(3, '0')}`
}

function uniqueID(raw: string) {
  const used = new Set(rows.value.map(row => row.custom_id))
  const base = normalizeID(raw, rows.value.length)
  let candidate = base
  let suffix = 2
  while (used.has(candidate)) candidate = `${base}_${suffix++}`
  return candidate
}

function addRow() {
  const prompt = draft.prompt.trim()
  if (!prompt) return
  rows.value.push({
    localId: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    custom_id: uniqueID(draft.customId),
    prompt,
    output_count: Math.min(4, Math.max(1, Math.floor(Number(draft.outputCount) || 1))),
    reference_images: draft.references.map(({ name: _name, size: _size, ...reference }) => ({ ...reference })),
  })
  draft.prompt = ''
  draft.customId = ''
  draft.outputCount = 1
  draft.references = []
}

function removeRow(index: number) {
  rows.value.splice(index, 1)
}

function removeDraftReference(index: number) {
  draft.references.splice(index, 1)
}

async function loadReferenceFiles(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  await addReferenceFiles(files)
}

async function dropReferenceFiles(event: DragEvent) {
  await addReferenceFiles(Array.from(event.dataTransfer?.files || []))
}

async function addReferenceFiles(files: File[]) {
  referenceError.value = ''
  const slots = Math.max(0, referenceLimit.value - draft.references.length)
  if (files.length > slots) referenceError.value = t('batchImage.create.refLimitExceededIgnored', { limit: referenceLimit.value })
  for (const file of files.slice(0, slots)) {
    if (!batchImageMimeTypes.includes(file.type)) {
      referenceError.value = t('batchImage.create.refFormatUnsupported')
      continue
    }
    if (!file.size || file.size > 10 * 1024 * 1024) {
      referenceError.value = t('batchImage.create.refFileTooLarge', { name: file.name })
      continue
    }
    const data = await readBase64(file)
    draft.references.push({ id: file.name, type: 'reference', mime_type: file.type, data, name: file.name, size: file.size })
  }
}

function readBase64(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.onload = () => {
      const value = String(reader.result || '')
      resolve(value.includes(',') ? value.slice(value.indexOf(',') + 1) : value)
    }
    reader.readAsDataURL(file)
  })
}

function submit() {
  if (draft.prompt.trim()) addRow()
  if (!canSubmit.value) return
  const items: BatchImageSubmitItem[] = rows.value.map(row => ({
    custom_id: row.custom_id,
    prompt: row.prompt,
    ...(row.output_count > 1 ? { output_count: row.output_count } : {}),
    ...(row.reference_images.length ? { reference_images: row.reference_images.map(reference => ({ ...reference })) } : {}),
  }))
  emit('submit', {
    model: form.model,
    provider: '',
    image_size: form.imageSize,
    aspect_ratio: form.aspectRatio,
    response_mime_type: form.responseMimeType,
    items,
  })
}

function mimeLabel(mime: string) {
  return mime === 'image/webp' ? 'WebP' : mime.slice(6).toUpperCase()
}

function reset() {
  rows.value = []
  draft.prompt = ''
  draft.customId = ''
  draft.outputCount = 1
  draft.references = []
  referenceError.value = ''
  form.model = props.models[0]?.value || ''
  form.imageSize = '1K'
  form.aspectRatio = '1:1'
  form.responseMimeType = 'image/png'
}

function hasDraft() {
  return rows.value.length > 0 || !!draft.prompt.trim() || draft.references.length > 0
}

defineExpose({ reset, hasDraft })
</script>
