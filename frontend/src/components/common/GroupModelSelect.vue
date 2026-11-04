<template>
  <div class="space-y-2">
    <div class="flex items-center justify-between gap-2">
      <label class="input-label mb-0">{{ t('batchKeys.modelRestriction') }}</label>
      <button v-if="modelValue.length" type="button" class="text-xs text-primary-600 hover:underline dark:text-primary-400" @click="clearRestriction">{{ t('batchKeys.clearModels') }}</button>
    </div>
    <div class="flex items-center justify-between gap-3">
      <span class="text-sm text-gray-600 dark:text-gray-300">{{ modelValue.length ? t('batchKeys.restrictedModels', { count: modelValue.length }) : t('batchKeys.unrestrictedModels') }}</span>
      <button type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="!groupId || loading" @click="loadModels">{{ t(loading ? 'batchKeys.fetchingGroupModels' : 'batchKeys.fetchGroupModels') }}</button>
    </div>
    <p v-if="modelValue.length" class="line-clamp-2 break-all text-xs text-gray-500 dark:text-gray-400" :title="modelValue.join('、')">{{ modelValue.join('、') }}</p>
    <p v-if="!groupId" class="input-hint">{{ t('batchKeys.selectGroupFirst') }}</p>
    <p v-else-if="failed" class="text-sm text-red-500" role="alert">{{ t('batchKeys.modelsLoadFailed') }}</p>
    <template v-else-if="fetched">
      <input v-model="query" class="input" type="search" :placeholder="t('batchKeys.searchModels')" :aria-label="t('batchKeys.searchModels')" />
      <div class="max-h-44 overflow-y-auto rounded-lg border border-gray-200 p-1 dark:border-dark-600">
        <label v-for="model in filteredModels" :key="model" class="flex cursor-pointer items-start gap-2 rounded px-2 py-2 text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-dark-700">
          <input type="checkbox" class="mt-0.5 h-4 w-4 shrink-0 accent-primary-600" :checked="modelValue.includes(model)" @change="toggleModel(model)" />
          <span class="min-w-0 break-all">{{ model }}</span>
        </label>
        <p v-if="!filteredModels.length" class="p-3 text-sm text-gray-500">{{ t('batchKeys.noModels') }}</p>
      </div>
    </template>
    <p class="input-hint">{{ t('batchKeys.modelsHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { userGroupsAPI } from '@/api/groups'
const props = defineProps<{ groupId: number | null; modelValue: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()
const { t } = useI18n()
const models = ref<string[]>([])
const query = ref('')
const loading = ref(false)
const failed = ref(false)
const fetched = ref(false)
let controller: AbortController | null = null
const filteredModels = computed(() => models.value.filter(model => model.toLowerCase().includes(query.value.trim().toLowerCase())))
function toggleModel(model: string) {
  emit('update:modelValue', props.modelValue.includes(model) ? props.modelValue.filter(value => value !== model) : [...props.modelValue, model])
}
function resetCatalog() {
  controller?.abort(); controller = null
  models.value = []; query.value = ''; loading.value = false; failed.value = false; fetched.value = false
}
function clearRestriction() {
  resetCatalog()
  emit('update:modelValue', [])
}
async function loadModels() {
  resetCatalog()
  if (!props.groupId) return
  const request = new AbortController(); controller = request
  loading.value = true
  try {
    const result = await userGroupsAPI.getModels(props.groupId, { signal: request.signal })
    if (controller === request && !request.signal.aborted) { models.value = result; fetched.value = true }
  } catch {
    if (controller === request && !request.signal.aborted) failed.value = true
  } finally {
    if (controller === request) loading.value = false
  }
}
// Changing the group clears fetched candidates; discovery only runs on a click.
watch(() => props.groupId, resetCatalog, { immediate: true })
onBeforeUnmount(resetCatalog)
</script>
