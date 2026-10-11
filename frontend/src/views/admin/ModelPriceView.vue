<template>
  <AppLayout>
    <div class="space-y-6 p-4 sm:p-6">
      <!-- 页头由 AppHeader 依路由 meta 渲染，页面内不再重复标题/描述 -->
      <div v-if="loading" class="py-12 text-center text-sm text-gray-500 dark:text-dark-400">
        {{ t('admin.modelPrice.loading') }}
      </div>

      <div
        v-else-if="platformSections.length === 0"
        class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400"
      >
        {{ t('admin.modelPrice.empty') }}
      </div>

      <template v-else>
        <p v-if="feedback" role="status" class="text-sm text-gray-600 dark:text-dark-300">{{ feedback }}</p>
        <!-- 按平台分类：每个平台一节，节内是该平台的全部分组 -->
        <section v-for="sec in platformSections" :key="sec.platform" class="space-y-3">
          <header class="flex items-center gap-2.5">
            <span
              class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg"
              :class="platformBadgeLightClass(sec.platform)"
            >
              <PlatformIcon :platform="sec.platform as GroupPlatform" size="sm" />
            </span>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
              {{ sectionLabel(sec.platform) }}
            </h2>
            <span class="text-xs text-gray-400 dark:text-dark-500">
              {{ t('admin.modelPrice.platformGroupCount', { count: sec.groups.length }) }}
            </span>
          </header>

          <div class="space-y-3">
            <article
              v-for="g in sec.groups"
              :key="g.id"
              class="overflow-hidden rounded-2xl border bg-white shadow-card transition-shadow hover:shadow-card-hover dark:bg-dark-800/50"
              :class="platformBorderStrongClass(g.platform)"
            >
              <div class="relative">
                <div
                  class="absolute inset-x-0 top-0 h-1"
                  :class="platformAccentBarClass(g.platform)"
                  aria-hidden="true"
                ></div>

                <!-- 分组标题可点击折叠；默认折叠，避免一次铺开过多模型 -->
                <button
                  type="button"
                  class="flex w-full flex-wrap items-center justify-between gap-2 px-4 pb-3 pt-4 text-left transition-colors hover:bg-gray-50 dark:hover:bg-dark-900/40"
                  :aria-expanded="isExpanded(g.id)"
                  :aria-label="
                    isExpanded(g.id) ? t('admin.modelPrice.collapse') : t('admin.modelPrice.expand')
                  "
                  @click="toggleGroup(g.id)"
                >
                  <div class="flex min-w-0 items-center gap-2">
                    <svg
                      class="h-4 w-4 shrink-0 text-gray-400 transition-transform dark:text-dark-500"
                      :class="isExpanded(g.id) ? 'rotate-90' : ''"
                      viewBox="0 0 20 20"
                      fill="currentColor"
                      aria-hidden="true"
                    >
                      <path
                        fill-rule="evenodd"
                        d="M7.21 14.77a.75.75 0 0 1 .02-1.06L11.168 10 7.23 6.29a.75.75 0 1 1 1.04-1.08l4.5 4.25a.75.75 0 0 1 0 1.08l-4.5 4.25a.75.75 0 0 1-1.06-.02Z"
                        clip-rule="evenodd"
                      />
                    </svg>
                    <!-- 分组默认倍率：置于分组名左侧 -->
                    <span
                      class="shrink-0 rounded-md bg-gray-100 px-1.5 py-0.5 font-mono text-xs font-semibold text-gray-700 dark:bg-dark-900 dark:text-dark-200"
                    >
                      {{ formatRate(g.rate_multiplier) }}
                    </span>
                    <h3 class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">
                      {{ g.name }}
                    </h3>
                  </div>
                  <div class="flex shrink-0 items-center gap-2 text-xs">
                    <span
                      v-if="customCount(g) > 0"
                      class="rounded-full bg-amber-100 px-2 py-0.5 font-medium text-amber-800 dark:bg-amber-900/40 dark:text-amber-300"
                    >
                      {{ t('admin.modelPrice.overrideCount', { count: customCount(g) }) }}
                    </span>
                    <span class="text-gray-500 dark:text-dark-400">
                      {{ t('admin.modelPrice.modelCount', { count: g.models.length }) }}
                    </span>
                  </div>
                </button>
              </div>

              <div v-if="isExpanded(g.id)">
                <div v-if="g.models.length" class="flex flex-wrap items-center gap-3 border-t border-gray-100 px-4 py-3 dark:border-dark-700">
                  <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
                    <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                      :data-testid="`select-all-${g.id}`"
                      :checked="selectedModels(g).length === g.models.length"
                      :indeterminate="selectedModels(g).length > 0 && selectedModels(g).length < g.models.length"
                      :disabled="saving" @change="toggleAll(g)" />
                    {{ t('admin.modelPrice.selectAll') }}
                  </label>
                  <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.modelPrice.selectedCount', { count: selectedModels(g).length }) }}</span>
                  <button type="button" class="btn btn-primary btn-sm ml-auto" :data-testid="`batch-adjust-${g.id}`"
                    :disabled="saving || selectedModels(g).length === 0" @click="openBatchDialog(g)">
                    {{ t('admin.modelPrice.batchAdjust') }}
                  </button>
                  <button v-if="selectedModels(g).length" type="button" class="btn btn-secondary btn-sm"
                    :data-testid="`clear-selection-${g.id}`" :disabled="saving" @click="clearSelection(g)">
                    {{ t('admin.modelPrice.clearSelection') }}
                  </button>
                </div>
                <div
                  v-if="g.models.length === 0"
                  class="border-t border-gray-100 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400"
                >
                  {{ t('admin.modelPrice.emptyGroup') }}
                </div>
                <ul
                  v-else
                  class="divide-y divide-gray-100 border-t border-gray-100 dark:divide-dark-700 dark:border-dark-700"
                >
                  <li
                    v-for="m in g.models"
                    :key="m.model"
                    class="flex flex-wrap items-center justify-between gap-2 px-4 py-2.5"
                  >
                    <div class="flex min-w-0 items-center gap-2">
                      <input type="checkbox" class="h-4 w-4 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                        :data-testid="`select-model-${g.id}-${m.model}`"
                        :aria-label="t('admin.modelPrice.selectModel', { model: m.model, group: g.name })"
                        :checked="selection.has(selectionKey(g.id, m.model))" :disabled="saving"
                        @change="toggleModel(g.id, m.model)" />
                      <span class="truncate text-sm text-gray-900 dark:text-gray-100">{{
                        m.model
                      }}</span>
                      <!-- 直接显示实际生效倍率：已覆盖为覆盖值（琥珀），否则为分组默认倍率（灰） -->
                      <span
                        class="inline-flex shrink-0 rounded-full px-2 py-0.5 font-mono text-xs font-medium"
                        :class="
                          m.custom_rate
                            ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300'
                            : 'bg-gray-100 text-gray-600 dark:bg-dark-900 dark:text-dark-300'
                        "
                        :title="
                          m.custom_rate
                            ? t('admin.modelPrice.overrideRateTitle')
                            : t('admin.modelPrice.groupDefaultRateTitle')
                        "
                      >
                        {{ formatRate(effectiveModelRate(g.rate_multiplier, m)) }}
                      </span>
                    </div>
                    <div class="flex shrink-0 items-center gap-2">
                      <button
                        type="button"
                        :disabled="saving"
                        class="rounded-md border border-gray-300 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-dark-200 dark:hover:bg-dark-900"
                        @click="openDialog(g.id, m)"
                      >
                        {{ t('admin.modelPrice.adjust') }}
                      </button>
                      <button
                        v-if="m.custom_rate"
                        type="button"
                        :disabled="saving"
                        class="rounded-md border border-gray-300 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-dark-200 dark:hover:bg-dark-900"
                        @click="clearRate(g.id, m)"
                      >
                        {{ t('admin.modelPrice.reset') }}
                      </button>
                    </div>
                  </li>
                </ul>
              </div>
            </article>
          </div>
        </section>
      </template>

      <BaseDialog :show="dialog.visible" :title="t(dialog.batch ? 'admin.modelPrice.batchDialogTitle' : 'admin.modelPrice.dialogTitle')"
        width="narrow" :show-close-button="!saving" @close="closeDialog">
          <p class="break-all text-sm text-gray-500 dark:text-dark-400">{{ dialog.groupName }}</p>
          <p class="mt-1 break-all text-sm text-gray-500 dark:text-dark-400">{{ dialog.models.join('、') }}</p>
          <label for="model-rate-input" class="mt-4 block text-sm text-gray-700 dark:text-dark-200">{{ t('admin.modelPrice.multiplier') }}</label>
          <input
            id="model-rate-input"
            v-model.number="dialog.value"
            type="number"
            step="0.001"
            min="0.001"
            class="input mt-2 w-full"
            :disabled="saving"
            :placeholder="t('admin.modelPrice.inputPlaceholder')"
            @keyup.enter="saveRate"
          />
          <p class="mt-2 text-xs text-gray-400 dark:text-dark-500">
            {{ t(dialog.batch ? 'admin.modelPrice.batchInputHint' : 'admin.modelPrice.inputHint') }}
          </p>
          <p v-if="dialog.error" role="alert" class="mt-3 text-sm text-red-600 dark:text-red-400">{{ dialog.error }}</p>
          <p v-if="saving" role="status" class="mt-3 text-sm text-gray-500">{{ t('admin.modelPrice.progress', { done: progress, total: dialog.models.length }) }}</p>
          <div class="mt-5 flex justify-end gap-2">
            <button
              type="button"
              data-testid="cancel-rate"
              :disabled="saving"
              class="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-dark-200 dark:hover:bg-dark-900"
              @click="closeDialog"
            >
              {{ t('admin.modelPrice.cancel') }}
            </button>
            <button
              type="button"
              data-testid="save-rate"
              class="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
              :disabled="saving"
              @click="saveRate"
            >
              {{ t(saving ? 'admin.modelPrice.saving' : 'admin.modelPrice.confirm') }}
            </button>
          </div>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { GROUP_PLATFORM_OPTIONS } from '@/constants/platforms'
import {
  platformAccentBarClass,
  platformBadgeLightClass,
  platformBorderStrongClass,
  platformLabel
} from '@/utils/platformColors'
import type { GroupPlatform } from '@/types'
import { buildPlatformSections, effectiveModelRate, formatRate } from './modelPriceView'
import {
  clearModelRateMultiplier,
  listAllGroupModelPrices,
  setModelRateMultiplier,
  type GroupModelPriceEntry,
  type GroupModelPriceGroup
} from '@/api/admin/modelPrice'

const { t } = useI18n()

const groups = ref<GroupModelPriceGroup[]>([])
const loading = ref(false)
const saving = ref(false)
const feedback = ref('')
const progress = ref(0)
const selection = reactive(new Set<string>())

function selectionKey(groupId: number, model: string) {
  return JSON.stringify([groupId, model])
}

function selectedModels(group: GroupModelPriceGroup) {
  return group.models.filter(model => selection.has(selectionKey(group.id, model.model)))
}

function toggleModel(groupId: number, model: string) {
  if (saving.value) return
  const key = selectionKey(groupId, model)
  if (selection.has(key)) selection.delete(key)
  else selection.add(key)
}

function clearSelection(group: GroupModelPriceGroup) {
  if (saving.value) return
  for (const model of group.models) selection.delete(selectionKey(group.id, model.model))
}

function toggleAll(group: GroupModelPriceGroup) {
  if (saving.value) return
  if (selectedModels(group).length === group.models.length) clearSelection(group)
  else for (const model of group.models) selection.add(selectionKey(group.id, model.model))
}

/** 平台分节的顺序：沿用平台目录声明的顺序，未知平台排在最后。 */
const PLATFORM_ORDER: readonly string[] = GROUP_PLATFORM_OPTIONS.map(
  (option) => option.value as string
)

/** 全部分组按平台归类成分节（按类型分类展示）。 */
const platformSections = computed(() => buildPlatformSections(groups.value, PLATFORM_ORDER))

function sectionLabel(platform: string): string {
  return platform ? platformLabel(platform) : t('admin.modelPrice.otherPlatform')
}

// 默认全部折叠，只展开用户点开的分组（reactive Set 的增删是响应式的）。
const expandedGroups = reactive(new Set<number>())

const dialog = reactive<{
  visible: boolean
  groupId: number
  models: string[]
  groupName: string
  batch: boolean
  error: string
  value: number | string | undefined
}>({
  visible: false,
  groupId: 0,
  models: [],
  groupName: '',
  batch: false,
  error: '',
  value: undefined
})

function isExpanded(groupId: number): boolean {
  return expandedGroups.has(groupId)
}

function toggleGroup(groupId: number) {
  if (expandedGroups.has(groupId)) {
    expandedGroups.delete(groupId)
  } else {
    expandedGroups.add(groupId)
  }
}

function customCount(group: GroupModelPriceGroup): number {
  return group.models.filter((m) => m.custom_rate).length
}

async function loadGroups() {
  loading.value = true
  try {
    groups.value = await listAllGroupModelPrices()
  } catch {
    groups.value = []
  } finally {
    loading.value = false
  }
}

function openDialog(groupId: number, m: GroupModelPriceEntry) {
  if (saving.value) return
  dialog.visible = true
  dialog.groupId = groupId
  dialog.models = [m.model]
  dialog.groupName = groups.value.find(group => group.id === groupId)?.name ?? ''
  dialog.batch = false
  dialog.error = ''
  // 回显当前覆盖倍率；未配置时留空，提示将回落分组默认倍率。
  dialog.value = m.rate_multiplier ?? undefined
}

function openBatchDialog(group: GroupModelPriceGroup) {
  if (saving.value) return
  const models = selectedModels(group)
  if (!models.length) return
  Object.assign(dialog, { visible: true, groupId: group.id, groupName: group.name,
    models: models.map(model => model.model), batch: true, error: '', value: undefined })
}

function closeDialog() {
  if (!saving.value) dialog.visible = false
}

async function saveRate() {
  if (saving.value || !dialog.groupId || !dialog.models.length) return
  const value = Number(dialog.value)
  if (!Number.isFinite(value) || value <= 0) {
    dialog.error = t('admin.modelPrice.invalidRate')
    return
  }
  saving.value = true
  dialog.error = ''
  feedback.value = ''
  progress.value = 0
  const failed: string[] = []
  const models = [...dialog.models]
  try {
    for (const model of models) {
      try {
        await setModelRateMultiplier(dialog.groupId, model, value)
        const entry = groups.value.find(group => group.id === dialog.groupId)?.models.find(entry => entry.model === model)
        if (entry) { entry.rate_multiplier = value; entry.custom_rate = true }
        selection.delete(selectionKey(dialog.groupId, model))
      } catch {
        failed.push(model)
      }
      progress.value++
    }
    feedback.value = t('admin.modelPrice.savedCount', { count: models.length - failed.length })
    if (failed.length) {
      dialog.models = failed
      dialog.error = t('admin.modelPrice.failedCount', { count: failed.length })
    } else {
      dialog.visible = false
    }
  } finally {
    saving.value = false
  }
}

async function clearRate(groupId: number, m: GroupModelPriceEntry) {
  if (saving.value) return
  saving.value = true
  try {
    await clearModelRateMultiplier(groupId, m.model)
    m.rate_multiplier = null
    m.custom_rate = false
    feedback.value = ''
  } catch {
    feedback.value = t('admin.modelPrice.resetFailed')
  } finally {
    saving.value = false
  }
}

onMounted(loadGroups)
</script>
