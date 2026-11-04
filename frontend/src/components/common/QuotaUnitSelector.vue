<template>
  <fieldset>
    <legend class="input-label">{{ t('keys.quotaUnit') }}</legend>
    <div class="grid grid-cols-2 gap-2">
      <label v-for="unit in units" :key="unit.value" class="flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2.5 text-sm" :class="modelValue === unit.value ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300' : 'border-gray-300 text-gray-600 dark:border-dark-600 dark:text-gray-300'">
        <input type="radio" :name="name" :value="unit.value" :checked="modelValue === unit.value" class="h-4 w-4 accent-primary-600" @change="$emit('update:modelValue', unit.value)" />
        {{ t(unit.label) }}
      </label>
    </div>
  </fieldset>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{ modelValue: 'usd' | 'tokens'; name: string }>()
defineEmits<{ 'update:modelValue': [value: 'usd' | 'tokens'] }>()
const { t } = useI18n()
const units = [{ value: 'usd' as const, label: 'keys.balanceUnit' }, { value: 'tokens' as const, label: 'keys.tokenUnit' }]
</script>
