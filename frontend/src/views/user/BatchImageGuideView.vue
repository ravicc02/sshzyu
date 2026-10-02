<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-3">
          <div class="flex flex-col gap-3 2xl:flex-row 2xl:items-center 2xl:justify-between">
            <div class="grid w-full grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-[260px_160px_144px_152px] 2xl:w-auto">
              <div class="min-w-0">
                <SearchInput
                  v-model="filters.taskName"
                  :placeholder="t('batchImage.filters.searchTaskName')"
                  class="w-full"
                  @search="applyFilters"
                />
              </div>
              <Select v-model="filters.apiKeyId" :options="apiKeyFilterOptions" class="w-full" @change="applyFilters" />
              <Select v-model="filters.status" :options="statusFilterOptions" class="w-full" @change="applyFilters" />
              <Select v-model="filters.downloaded" :options="downloadFilterOptions" class="w-full" @change="applyFilters" />
            </div>
            <div class="flex flex-wrap items-center justify-start gap-2 sm:justify-end 2xl:flex-shrink-0">
              <button type="button" class="btn btn-secondary" :disabled="loadingJobs" @click="resetFilters">
                {{ t('common.reset') }}
              </button>
              <button type="button" class="btn btn-secondary" :disabled="loadingKeys || loadingJobs" :title="t('common.refresh')" @click="refreshPage">
                <Icon name="refresh" size="md" :class="loadingKeys || loadingJobs ? 'animate-spin' : ''" />
              </button>
              <button type="button" class="btn btn-secondary" @click="showGuideModal = true">
                <Icon name="book" size="md" class="mr-2" />
                {{ t('batchImage.actions.usageGuide') }}
              </button>
              <button type="button" class="btn btn-primary" @click="openCreateModal">
                <Icon name="plus" size="md" class="mr-2" />
                {{ t('batchImage.actions.createJob') }}
              </button>
            </div>
          </div>

          <div v-if="downloadArtifact" class="flex flex-wrap items-center gap-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700" role="status" data-testid="download-artifact">
            <span class="text-sm">{{ t('batchImage.config.downloadReady') }}</span>
            <a :href="downloadArtifact.url" :download="downloadArtifact.filename" class="btn btn-secondary btn-sm" data-testid="save-zip-link">{{ t('batchImage.config.saveZip') }}</a>
            <button type="button" class="btn btn-secondary btn-sm" @click="clearDownloadArtifact">{{ t('common.close') }}</button>
          </div>

          <div
            v-if="selectedJobIds.size"
            class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-gray-200 bg-white px-3 py-2 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          >
            <i18n-t
              keypath="batchImage.list.selectedJobs"
              tag="span"
              scope="global"
              :plural="selectedJobIds.size"
              class="text-sm text-gray-600 dark:text-gray-300"
            >
              <template #count>
                <span class="font-medium text-gray-900 dark:text-white">{{ selectedJobIds.size }}</span>
              </template>
            </i18n-t>
            <div class="flex flex-wrap items-center gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="downloading || bulkDownloading || selectedDownloadableRows.length === 0"
                data-testid="download-selected-jobs"
                @click="downloadSelectedJobs"
              >
                <Icon :name="bulkDownloading || downloading ? 'refresh' : 'download'" size="sm" class="mr-1.5" :class="bulkDownloading || downloading ? 'animate-spin' : ''" />
                {{ t('batchImage.actions.downloadSelected') }}
              </button>
              <button
                type="button"
                class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                :disabled="bulkDeleting"
                @click="deleteSelectedJobs"
              >
                <Icon :name="bulkDeleting ? 'refresh' : 'trash'" size="sm" class="mr-1.5" :class="bulkDeleting ? 'animate-spin' : ''" />
                {{ t('batchImage.actions.deleteRecords') }}
              </button>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="visibleBatchJobs"
          :loading="loadingKeys || loadingJobs"
          :expandable-actions="false"
          row-key="id"
        >
          <template #header-select>
            <input
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="allVisibleSelected"
              :indeterminate="someVisibleSelected"
              @change="toggleAllVisible(($event.target as HTMLInputElement).checked)"
            />
          </template>

          <template #cell-select="{ row }">
            <input
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="selectedJobIds.has(row.id)"
              @change="toggleJobSelection(row.id, ($event.target as HTMLInputElement).checked)"
              @click.stop
            />
          </template>

          <template #cell-id="{ row }">
	            <div class="flex w-[220px] items-start gap-1">
	              <span class="w-2 flex-shrink-0" />
	              <button type="button" class="min-w-0 flex-1 rounded-lg py-1 text-left transition-colors hover:bg-gray-100 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:hover:bg-dark-700" @click="selectJob(row.id)">
	                <span
	                  class="flex min-w-0 items-center gap-2 text-sm font-medium"
	                  :class="row.task_name ? 'text-gray-900 dark:text-white' : 'text-gray-500 dark:text-gray-400'"
                >
                  <span class="min-w-0 truncate">{{ row.task_name || defaultTaskName(row.created_at) }}</span>
                  <span v-if="row.technical_batch_count > 1" class="flex-shrink-0 rounded-full bg-gray-100 px-2 py-0.5 text-xs font-normal text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                    {{ t('batchImage.list.executionBatchCount', { n: row.technical_batch_count }) }}
                  </span>
	                </span>
	                <span class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
	                  <span>{{ formatDate(row.created_at) }}</span>
	                </span>
	              </button>
	            </div>
	          </template>

          <template #cell-model="{ row }">
	            <div class="mx-auto max-w-[180px] text-center">
	              <p class="truncate text-sm text-gray-700 dark:text-gray-300" :title="row.model">{{ row.model }}</p>
	            </div>
	          </template>

          <template #cell-api_key_name="{ value }">
            <span class="block truncate text-center text-sm text-gray-700 dark:text-gray-300">
              {{ value || t('batchImage.list.keyNotRecorded') }}
            </span>
          </template>

          <template #cell-status="{ row }">
            <div class="flex justify-center">
              <span :class="statusBadgeClass(row)" class="badge">
                {{ statusLabel(row) }}
              </span>
            </div>
          </template>

          <template #cell-counts="{ row }">
            <div class="flex items-center justify-center gap-2 text-sm tabular-nums">
              <span class="text-emerald-600 dark:text-emerald-300">{{ row.success_count }}</span>
              <span class="text-gray-300 dark:text-dark-500">/</span>
              <span :class="row.fail_count > 0 ? 'text-red-600 dark:text-red-300' : 'text-gray-400 dark:text-gray-500'">{{ row.fail_count }}</span>
              <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('batchImage.list.totalCount', { n: row.item_count }) }}</span>
            </div>
          </template>

          <template #cell-cost="{ row }">
            <span class="block text-center text-sm text-gray-700 dark:text-gray-300">
              {{ costLabel(row) }}
            </span>
          </template>

          <template #cell-downloaded="{ row }">
            <span class="block text-center text-sm" :class="row.downloaded_at ? 'text-emerald-700 dark:text-emerald-300' : 'text-gray-500 dark:text-gray-400'">
              {{ row.downloaded_at ? formatDate(row.downloaded_at) : t('batchImage.list.notDownloaded') }}
            </span>
          </template>

	          <template #cell-actions="{ row }">
	            <div class="flex items-center justify-center gap-1">
              <button
                type="button"
                class="batch-row-action flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('batchImage.actions.viewDetail')"
                @click="selectJob(row.id)"
              >
                <Icon name="eye" size="sm" />
                <span class="text-xs">{{ t('common.view') }}</span>
              </button>
              <button
                type="button"
                class="batch-row-action flex flex-col items-center gap-0.5 rounded-lg p-1.5 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30"
                :class="canDownload(row) ? 'text-gray-500 hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400' : 'text-gray-300 dark:text-dark-500'"
                :disabled="!canDownload(row) || downloading"
                :title="t('batchImage.actions.downloadZip')"
                @click="downloadTask(row)"
              >
                <Icon
                  :name="isDownloadingJob(row.id) ? 'refresh' : 'download'"
	                  size="sm"
	                  :class="isDownloadingJob(row.id) ? 'animate-spin' : ''"
	                />
                <span class="text-xs">{{ t('batchImage.actions.download') }}</span>
	              </button>
              <div v-if="canRetry(row) || canDeleteRecord(row)">
                <button
                  type="button"
                  class="batch-row-action flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:hover:bg-dark-700 dark:hover:text-white"
                  :class="{ 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-white': openMoreJobId === row.id }"
                  :title="t('batchImage.actions.moreActions')"
                  @click.stop="toggleMoreMenu(row, $event)"
                >
                  <Icon name="more" size="sm" />
                  <span class="text-xs">{{ t('common.more') }}</span>
                </button>
              </div>
	            </div>
	          </template>

          <template #empty>
            <div class="flex min-h-[260px] flex-col items-center justify-center py-6 md:min-h-[300px]">
              <Icon name="sparkles" size="xl" class="mb-4 h-12 w-12 text-gray-400 dark:text-dark-500" />
              <p class="text-lg font-medium text-gray-900 dark:text-gray-100">{{ t('batchImage.list.empty') }}</p>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('batchImage.list.emptyHint') }}
              </p>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <div
          v-if="visibleBatchJobs.length > 0 || pagination.page > 1"
          class="flex flex-col gap-3 border-t border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800 sm:flex-row sm:items-center sm:justify-between sm:px-6"
        >
          <div class="flex flex-wrap items-center gap-3 text-sm text-gray-700 dark:text-gray-300">
            <i18n-t keypath="batchImage.pagination.pageNumber" tag="span" scope="global">
              <template #page>
                <span class="font-medium">{{ pagination.page }}</span>
              </template>
            </i18n-t>
            <i18n-t keypath="batchImage.pagination.pageItems" tag="span" scope="global">
              <template #count>
                <span class="font-medium">{{ visibleBatchJobs.length }}</span>
              </template>
            </i18n-t>
            <div class="flex items-center gap-2">
              <span>{{ t('pagination.perPage') }}</span>
              <Select
                v-model="pagination.page_size"
                :options="batchPageSizeOptions"
                class="w-24"
                @change="handlePageSizeChange"
              />
            </div>
          </div>
          <div class="flex items-center justify-end gap-2">
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="pagination.page <= 1 || loadingJobs"
              @click="handlePageChange(pagination.page - 1)"
            >
              <Icon name="chevronLeft" size="sm" class="mr-1" />
              {{ t('pagination.previous') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="!pagination.has_more || loadingJobs"
              @click="handlePageChange(pagination.page + 1)"
            >
              {{ t('pagination.next') }}
              <Icon name="chevronRight" size="sm" class="ml-1" />
            </button>
          </div>
        </div>
      </template>
    </TablePageLayout>

    <Teleport to="body">
      <div
        v-if="openMoreJobId"
        class="fixed z-[9999] w-44 overflow-hidden rounded-xl bg-white py-1 text-sm shadow-lg ring-1 ring-black/5 dark:bg-dark-800 dark:ring-white/10"
        :style="moreMenuStyle"
        @click.stop
      >
        <template v-for="job in visibleBatchJobs" :key="job.id">
          <template v-if="job.id === openMoreJobId">
            <button
              v-if="canRetry(job)"
              type="button"
              class="flex w-full items-center gap-2 px-3 py-2 text-left text-gray-700 transition-colors hover:bg-amber-50 hover:text-amber-700 disabled:opacity-60 dark:text-gray-200 dark:hover:bg-amber-900/20 dark:hover:text-amber-300"
              :disabled="retryingBatchId === job.id"
              @click="retryTask(job)"
            >
              <Icon name="refresh" size="sm" :class="retryingBatchId === job.id ? 'animate-spin' : ''" />
              {{ t('batchImage.actions.retryFailedItems') }}
            </button>
            <button
              v-if="canDeleteRecord(job)"
              type="button"
              class="flex w-full items-center gap-2 px-3 py-2 text-left text-red-600 transition-colors hover:bg-red-50 disabled:opacity-60 dark:text-red-400 dark:hover:bg-red-900/20"
              :disabled="deletingBatchId === job.id"
              @click="deleteTask(job)"
            >
              <Icon :name="deletingBatchId === job.id ? 'refresh' : 'trash'" size="sm" :class="deletingBatchId === job.id ? 'animate-spin' : ''" />
              {{ t('batchImage.actions.deleteRecords') }}
            </button>
          </template>
        </template>
      </div>
    </Teleport>

    <Teleport to="body">
      <div
        v-if="promptPopover.visible"
        class="batch-prompt-popover fixed z-[9999] rounded-lg border border-gray-200 bg-white p-3 text-sm text-gray-800 shadow-xl ring-1 ring-black/5 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-100 dark:ring-white/10"
        :style="promptPopover.style"
        @mouseenter="cancelPromptPopoverClose"
        @mouseleave="schedulePromptPopoverClose"
      >
        <div class="mb-2 flex items-center justify-between gap-3">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('batchImage.promptPopover.title') }}</span>
          <button
            type="button"
            class="rounded-md px-2 py-1 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-primary-300 dark:hover:bg-primary-900/20"
            @click="copyPromptPopover"
          >
            {{ t('common.copy') }}
          </button>
        </div>
        <p class="max-h-48 overflow-y-auto whitespace-pre-wrap break-words leading-6 selection:bg-primary-100 selection:text-primary-900 dark:selection:bg-primary-900/60 dark:selection:text-primary-100">
          {{ promptPopover.text }}
        </p>
      </div>
    </Teleport>

    <BaseDialog :show="!!currentJob" :title="t('batchImage.detail.title')" width="extra-wide" @close="closeDetail">
      <div v-if="currentJob" class="space-y-4">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ currentTask?.task_name || currentJob.task_name }}</h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ currentTask?.provider || currentJob.provider }} · {{ currentTask?.model || currentJob.model }} · {{ t('batchImage.list.executionBatchCount', { n: currentTask?.technical_batch_count || 1 }) }}
          </p>
        </div>
        <div class="rounded-lg border border-gray-200 bg-gray-50/70 px-4 py-3 dark:border-dark-700 dark:bg-dark-900/40">
          <div class="grid gap-x-6 gap-y-3 sm:grid-cols-2 lg:grid-cols-4">
            <div class="min-w-0 text-center">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('common.status') }}</p>
              <div class="mt-1 flex justify-center">
                <span :class="statusBadgeClass(currentDisplayJob || currentJob)" class="badge whitespace-nowrap">
                  {{ statusLabel(currentDisplayJob || currentJob) }}
                </span>
              </div>
            </div>
            <div class="min-w-0 text-center">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ hasChildJobs(currentJob.id) ? t('batchImage.detail.aggregatedResult') : t('batchImage.detail.result') }}</p>
              <p class="mt-1 flex items-center justify-center gap-2 font-medium tabular-nums">
              <span class="text-emerald-600 dark:text-emerald-300">{{ (currentDisplayJob || currentJob).success_count }}</span>
              <span class="text-gray-300 dark:text-dark-500">/</span>
              <span :class="(currentDisplayJob || currentJob).fail_count > 0 ? 'text-red-600 dark:text-red-300' : 'text-gray-400 dark:text-gray-500'">{{ (currentDisplayJob || currentJob).fail_count }}</span>
            </p>
            </div>
            <div class="min-w-0 text-center">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('batchImage.detail.cost') }}</p>
              <p class="mt-1 truncate font-medium text-gray-900 dark:text-white">{{ costLabel(currentDisplayJob || currentJob) }}</p>
            </div>
            <div class="min-w-0 text-center">
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('batchImage.detail.downloadStatus') }}</p>
              <p class="mt-1 truncate font-medium text-gray-900 dark:text-white">
              {{ (currentTask || currentJob).downloaded_at ? formatDate((currentTask || currentJob).downloaded_at || 0) : t('batchImage.list.notDownloaded') }}
            </p>
            </div>
          </div>
        </div>

        <div class="flex border-b border-gray-200 dark:border-dark-700" role="tablist">
          <button type="button" class="border-b-2 px-4 py-2 text-sm font-medium" :class="detailTab === 'results' ? 'border-primary-600 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'" role="tab" :aria-selected="detailTab === 'results'" @click="detailTab = 'results'">
            {{ t('batchImage.detail.resultsTab', { n: (currentDisplayJob || currentJob).item_count }) }}
          </button>
          <button type="button" class="border-b-2 px-4 py-2 text-sm font-medium" :class="detailTab === 'batches' ? 'border-primary-600 text-primary-600' : 'border-transparent text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-100'" role="tab" :aria-selected="detailTab === 'batches'" @click="detailTab = 'batches'">
            {{ t('batchImage.detail.batchesTab', { n: currentTask?.technical_batch_count || 1 }) }}
          </button>
        </div>

        <template v-if="detailTab === 'results'">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('batchImage.detail.items') }}</h3>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="refreshing || loadingItems" @click="refreshDetail">
            <Icon name="refresh" size="sm" class="mr-1.5" :class="refreshing || loadingItems ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
        </div>

        <div v-if="items.length" class="overflow-x-auto rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
          <table class="w-full min-w-[860px] table-fixed divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <colgroup>
              <col class="w-[18%]" />
              <col class="w-[34%]" />
              <col class="w-[12%]" />
              <col class="w-[10%]" />
              <col class="w-[26%]" />
            </colgroup>
            <thead class="bg-gray-50 dark:bg-dark-800/80">
              <tr>
                <th class="px-3 py-3 text-center text-sm font-medium text-gray-500 dark:text-gray-400">Custom ID</th>
                <th class="px-3 py-3 text-left text-sm font-medium text-gray-500 dark:text-gray-400">Prompt</th>
                <th class="px-3 py-3 text-center text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('common.status') }}</th>
                <th class="px-3 py-3 text-center text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('batchImage.detail.preview') }}</th>
                <th class="px-3 py-3 text-center text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('batchImage.detail.result') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr
                v-for="item in items"
                :key="itemPreviewKey(item)"
                class="align-middle"
                :class="detailItemRowClass(item)"
              >
                <td class="px-3 py-2.5 text-center">
                  <span
                    class="block min-w-0 truncate font-mono text-sm"
                    :class="isRecoveredOriginalFailure(item) ? 'text-gray-400 dark:text-gray-500' : 'text-gray-900 dark:text-white'"
                    :title="item.custom_id"
                  >
                    {{ item.custom_id }}
                  </span>
                </td>
                <td class="px-3 py-2.5 text-left" :class="isRecoveredOriginalFailure(item) ? 'text-gray-400 dark:text-gray-500' : 'text-gray-700 dark:text-gray-300'">
                  <div
                    class="batch-prompt-trigger cursor-default truncate rounded px-1 text-sm leading-6 focus:outline-none"
                    tabindex="0"
                    @pointerenter="schedulePromptPopoverOpen($event, item.prompt_preview || '-')"
                    @pointerleave="schedulePromptPopoverClose"
                    @mouseenter="schedulePromptPopoverOpen($event, item.prompt_preview || '-')"
                    @mouseleave="schedulePromptPopoverClose"
                    @click="showPromptPopover($event, item.prompt_preview || '-')"
                    @focus="showPromptPopover($event, item.prompt_preview || '-')"
                    @focusin="showPromptPopover($event, item.prompt_preview || '-')"
                    @blur="schedulePromptPopoverClose"
                  >
                    {{ item.prompt_preview || '-' }}
                  </div>
                </td>
                <td class="px-3 py-2.5 text-center">
                  <span :class="itemDisplayStatusBadgeClass(item)" class="badge max-w-full truncate whitespace-nowrap" :title="itemDisplayStatusLabel(item)">
                    {{ itemDisplayStatusLabel(item) }}
                  </span>
                </td>
                <td class="px-3 py-2.5 text-center">
                  <div class="mx-auto h-12 w-12 overflow-hidden rounded-md border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                    <button
                      v-if="itemPreviewUrls[itemPreviewKey(item)] && !previewErrorIds.has(itemPreviewKey(item))"
                      type="button"
                      class="block h-full w-full overflow-hidden"
                      :title="t('batchImage.detail.previewZoom', { id: item.custom_id })"
                      @click="openImagePreview(item)"
                    >
                      <img
                        :src="itemPreviewUrls[itemPreviewKey(item)]"
                        class="h-full w-full object-cover"
                        alt=""
                        @error="handlePreviewError(itemPreviewKey(item))"
                      />
                    </button>
                    <button
                      v-else-if="canLoadItemPreview(item)"
                      type="button"
                      class="flex h-full w-full items-center justify-center text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 disabled:cursor-wait disabled:opacity-70 dark:text-gray-400 dark:hover:bg-dark-700"
                      :disabled="previewLoadingIds.has(itemPreviewKey(item))"
                      :title="previewErrorIds.has(itemPreviewKey(item)) ? t('batchImage.detail.previewReload') : t('batchImage.detail.previewLoad')"
                      @click="loadItemPreview(item)"
                    >
                      <Icon :name="previewLoadingIds.has(itemPreviewKey(item)) ? 'refresh' : 'eye'" size="sm" :class="previewLoadingIds.has(itemPreviewKey(item)) ? 'animate-spin' : ''" />
                    </button>
                    <div v-else class="flex h-full w-full items-center justify-center text-gray-400" :title="item.image_count > 0 ? t('batchImage.detail.previewUnavailable') : t('batchImage.detail.noImage')">
                      <Icon name="document" size="sm" />
                    </div>
                  </div>
                </td>
                <td class="px-3 py-2.5 text-center">
                  <span
                    class="inline-flex max-w-full items-center justify-center truncate rounded-md px-2.5 py-1 text-xs font-medium leading-5 ring-1 ring-inset"
                    :class="itemResultClass(item)"
                    :title="itemResultLabel(item)"
                  >
                    {{ itemResultLabel(item) }}
                  </span>
                  <details class="mt-1 text-left" data-testid="item-full-result">
                    <summary class="cursor-pointer rounded text-center text-xs text-primary-600 hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:text-primary-400">
                      {{ t('batchImage.detail.viewFullResult') }}
                    </summary>
                    <div
                      class="mt-2 max-h-64 overflow-y-auto whitespace-pre-wrap break-words rounded bg-gray-50 p-3 text-left text-xs leading-5 text-gray-700 select-text [overflow-wrap:anywhere] focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:bg-dark-800 dark:text-gray-300"
                      tabindex="0"
                      role="region"
                      :aria-label="t('batchImage.detail.fullResult', { id: item.custom_id })"
                    >{{ itemFullResult(item) }}</div>
                  </details>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="rounded-lg border border-dashed border-gray-200 py-10 text-center dark:border-dark-700">
          <Icon name="refresh" size="lg" class="mx-auto mb-3 text-gray-400" :class="loadingItems ? 'animate-spin' : ''" />
          <p class="text-sm font-medium text-gray-700 dark:text-gray-200">
            {{ loadingItems ? t('batchImage.detail.loadingItems') : t('batchImage.detail.noItems') }}
          </p>
          <p v-if="!loadingItems" class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('batchImage.detail.noItemsHint') }}
          </p>
        </div>
        </template>

        <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[760px] divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 dark:bg-dark-800/80">
              <tr>
                <th class="px-3 py-3 text-left font-medium text-gray-500">{{ t('batchImage.detail.batchId') }}</th>
                <th class="px-3 py-3 text-left font-medium text-gray-500">{{ t('batchImage.detail.specification') }}</th>
                <th class="px-3 py-3 text-center font-medium text-gray-500">{{ t('common.status') }}</th>
                <th class="px-3 py-3 text-center font-medium text-gray-500">{{ t('batchImage.detail.result') }}</th>
                <th class="px-3 py-3 text-right font-medium text-gray-500">{{ t('batchImage.detail.cost') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="job in currentTask?.collection_jobs || []" :key="job.id">
                <td class="px-3 py-3 font-mono text-xs text-gray-700 dark:text-gray-300">{{ job.id }}</td>
                <td class="px-3 py-3 text-gray-700 dark:text-gray-300">{{ job.image_size || '1K' }} · {{ job.aspect_ratio || '1:1' }} · {{ job.model }}</td>
                <td class="px-3 py-3 text-center"><span :class="statusBadgeClass(job)" class="badge">{{ statusLabel(job) }}</span></td>
                <td class="px-3 py-3 text-center tabular-nums">{{ job.success_count }} / {{ job.fail_count }}</td>
                <td class="px-3 py-3 text-right">{{ costLabel(job) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
	          <button type="button" class="btn btn-secondary" :disabled="!currentTask || !currentTask.collection_jobs.some(canCancel) || cancelling" @click="cancelSelected">
	            <Icon v-if="cancelling" name="refresh" size="sm" class="mr-2 animate-spin" />
	            {{ t('batchImage.actions.cancelJob') }}
	          </button>
	          <button
	            v-if="currentJob && currentDisplayJob && canRetry(currentDisplayJob)"
	            type="button"
	            class="btn btn-secondary inline-flex min-w-[116px] items-center justify-center"
	            :disabled="retryingBatchId === currentJob.id"
	            @click="retrySelected"
	          >
	            <Icon name="refresh" size="sm" class="mr-2" :class="currentJob && retryingBatchId === currentJob.id ? 'animate-spin' : ''" />
	            {{ t('batchImage.actions.retryFailedItems') }}
	          </button>
	          <button
            type="button"
            class="btn btn-primary inline-flex min-w-[112px] items-center justify-center"
            :disabled="!currentTask || !canDownload(currentTask) || downloading"
            @click="currentTask && downloadTask(currentTask)"
          >
            <Icon
              :name="currentJob && isDownloadingJob(currentJob.id) ? 'refresh' : 'download'"
              size="sm"
              class="mr-2"
              :class="currentJob && isDownloadingJob(currentJob.id) ? 'animate-spin' : ''"
            />
            {{ t('batchImage.actions.downloadZip') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="!!previewImageItem" :title="previewImageItem?.custom_id || t('batchImage.imagePreview.title')" width="extra-wide" :z-index="60" @close="closeImagePreview">
      <div class="space-y-3">
        <div class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-100">
          {{ t('batchImage.imagePreview.notice') }}
        </div>
        <div class="flex min-h-[420px] items-center justify-center rounded-lg bg-gray-50 p-4 dark:bg-dark-900">
          <img
            v-if="previewImageUrl"
            :src="previewImageUrl"
            class="max-h-[70vh] max-w-full rounded-md object-contain"
            :alt="previewImageItem?.custom_id || ''"
          />
        </div>
      </div>
    </BaseDialog>

    <BaseDialog :show="showCreateModal" :title="t('batchImage.create.title')" width="extra-wide" @close="closeCreateModal">
      <form class="space-y-5" @submit.prevent="submitConfigJob">
        <div v-if="pendingConfigAttempts.length" class="space-y-2 rounded-lg border border-amber-200 p-3" data-testid="config-recovery">
          <p class="text-sm">{{ t('batchImage.config.pending', { count: pendingConfigAttempts.length }) }}</p>
          <div v-for="attempt in pendingConfigAttempts" :key="attempt.fingerprint" class="flex items-center justify-between gap-3">
            <span class="truncate text-sm">{{ attempt.config?.task_name || t('batchImage.config.untitled') }}</span>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="submitting" @click="restorePendingConfig(attempt)">{{ t('batchImage.config.restore') }}</button>
          </div>
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <div class="md:col-span-2">
            <label class="input-label">{{ t('batchImage.create.taskName') }}</label>
            <input
              v-model="form.taskName"
              type="text"
              maxlength="255"
              class="input"
              :disabled="submitting"
              :placeholder="t('batchImage.create.taskNamePlaceholder')"
            />
          </div>

          <div class="md:col-span-2">
            <label class="input-label">API Key</label>
            <select v-model.number="form.apiKeyId" class="input" :disabled="loadingKeys || submitting" data-testid="config-key">
              <option :value="0">{{ loadingKeys ? t('batchImage.create.loadingKeys') : t('batchImage.create.selectKeyPlaceholder') }}</option>
              <option v-for="key in batchImageApiKeys" :key="key.id" :value="key.id">
                [{{ key.group?.platform === 'gemini' ? 'Gemini' : 'OpenAI' }}] {{ key.name }} · {{ key.group?.name || key.group?.platform }}
              </option>
            </select>
            <p v-if="!loadingKeys && batchImageApiKeys.length === 0" class="input-hint text-amber-600 dark:text-amber-400">{{ t('batchImage.create.noKeysHint') }}</p>
          </div>

          <template v-if="selectedApiKey">
          <details class="md:col-span-2 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/50" data-testid="config-matrix">
            <summary class="flex cursor-pointer items-center justify-between gap-3">
              <div>
                <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t(isGeminiPlatform ? 'batchImage.config.geminiMatrix' : 'batchImage.config.matrix') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('batchImage.config.matrixHint') }}</p>
              </div>
              <span class="text-xs text-gray-500 dark:text-gray-400">1K / 2K / 4K × 8 种比例</span>
            </summary>
            <div class="mt-3 grid gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-3">
              <div v-for="size in configSizes" :key="size" class="rounded-md border border-gray-200 bg-white p-2 dark:border-dark-700 dark:bg-dark-800">
                <p class="font-semibold text-gray-900 dark:text-white">{{ size }}</p>
                <dl class="mt-1 space-y-1">
                  <div v-for="ratio in configRatios" :key="ratio" class="flex justify-between gap-2">
                    <dt>{{ ratio }}</dt><dd>{{ configMatrix[size]?.[ratio] }}</dd>
                  </div>
                </dl>
              </div>
            </div>
          </details>

          <div class="md:col-span-2 grid items-stretch gap-6 md:grid-cols-2" data-testid="config-workspace">
            <div class="flex min-w-0 flex-col">
              <label for="batch-config-input" class="input-label">{{ t('batchImage.config.inputLabel') }}</label>
              <textarea
                id="batch-config-input"
                v-model="configInput"
                rows="8"
                class="min-h-[190px] w-full flex-1 resize-y rounded-md border border-gray-300 px-3 py-3 text-sm leading-6 outline-none focus:border-primary-500 focus:ring-2 focus:ring-primary-100 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-100 dark:focus:border-primary-500 dark:focus:ring-primary-900/40"
                :disabled="submitting"
                :placeholder="t('batchImage.config.inputPlaceholder')"
                data-testid="config-input"
              />
              <p class="mt-2 text-xs text-gray-600 dark:text-gray-300">{{ t('batchImage.config.inputHint') }}</p>
              <div class="mt-4 rounded-lg border border-gray-200 bg-gray-50/70 p-3 dark:border-dark-700 dark:bg-dark-900/40" data-testid="reference-settings">
                <div class="flex items-center justify-between gap-4">
                  <div class="min-w-0">
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('batchImage.config.consistency') }}</p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ configConsistency ? t(isGeminiPlatform ? 'batchImage.config.sharedReferencesHint' : 'batchImage.config.sharedReferenceHint') : t('batchImage.config.itemReferenceHint') }}</p>
                  </div>
                  <Toggle v-model="configConsistency" :disabled="submitting" data-testid="config-consistency" :aria-label="t('batchImage.config.consistency')" />
                </div>
                <div v-if="configConsistency" class="mt-3 space-y-2">
                  <p v-if="hasRecommendedConsistencyModel" class="text-xs text-gray-600 dark:text-gray-300">{{ t('batchImage.config.recommendation') }}</p>
                  <BatchImageReferenceUpload
                    :images="configReferenceImages" :names="configReferenceNames"
                    :label="t('batchImage.config.uploadReference')" :limit="sharedReferenceLimit"
                    :multiple="isGeminiPlatform" :disabled="submitting || loadingModels"
                    :loading="configReferenceLoading" :error="configReferenceError" required
                    test-id="config-reference" remove-test-id="remove-shared-reference"
                    @files="loadConfigReferenceFiles" @remove="removeConfigReference" @clear="clearConfigReference"
                  />
                </div>
              </div>
              <button type="button" class="btn btn-secondary mt-2" :disabled="submitting || configConverting || !configInput.trim()" data-testid="convert-config" @click="convertConfig">
                {{ configConverting ? t('batchImage.config.converting') : t('batchImage.config.convert') }}
              </button>
              <p v-if="configError" role="alert" class="mt-2 text-sm text-red-600 dark:text-red-400">{{ configError }}</p>
              <p v-else-if="configStatus === 'stale'" role="status" class="mt-2 text-xs text-amber-700 dark:text-amber-300">{{ t('batchImage.config.stale') }}</p>
              <p v-if="configCards.length && modelLoadError" role="alert" class="mt-2 text-xs text-red-600">{{ modelLoadError }}</p>
              <p v-else-if="configCards.length && !loadingModels && availableBatchImageModels.length === 0" role="alert" class="mt-2 text-xs text-red-600">{{ t('batchImage.config.noModels') }}</p>
            </div>

            <div class="flex min-h-[380px] min-w-0 flex-col rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800" data-testid="config-cards">
              <div class="flex items-center justify-between">
                <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ configCards.length ? t('batchImage.config.preview', { count: configCards.length }) : t('batchImage.config.draftTitle') }}</p>
                <span class="text-xs text-gray-500 dark:text-gray-400" data-testid="config-status">{{ configStatusLabel }}</span>
              </div>
              <template v-if="configCards.length && (configInput.trim() || configRecovered)">
              <div v-for="(card, index) in configCards" :key="card.localId" class="rounded-lg border border-gray-200 dark:border-dark-700">
                <button type="button" class="flex w-full items-center justify-between px-3 py-2 text-left text-sm" :aria-expanded="expandedConfigCard === card.localId" data-testid="config-card-toggle" @click="toggleConfigCard(card.localId)">
                  <span class="truncate font-medium">{{ expandedConfigCard === card.localId ? '▼' : '▶' }} {{ t('batchImage.config.image', { index: String(index + 1).padStart(2, '0') }) }}：{{ card.prompt }}</span>
                  <span class="ml-2 flex-shrink-0 text-xs text-gray-500">{{ card.image_size }} / {{ card.aspect_ratio }}</span>
                </button>
                <div v-if="expandedConfigCard === card.localId" class="space-y-2 border-t border-gray-100 p-3 dark:border-dark-700">
                  <label class="block text-sm">{{ t('batchImage.config.prompt') }}
                    <textarea v-model="card.prompt" rows="5" class="input mt-1 resize-y text-sm leading-6" aria-label="prompt" :disabled="submitting" @input="markConfigEdited" />
                  </label>
                  <div class="grid gap-3" :class="isGeminiPlatform ? 'sm:grid-cols-2' : 'sm:grid-cols-3'">
                    <label class="block text-sm">{{ t('batchImage.config.resolution') }}
                      <select v-model="card.image_size" class="input mt-1 text-sm" aria-label="image_size" :disabled="submitting" @change="markConfigEdited">
                        <option v-for="size in configSizes" :key="size" :value="size" :disabled="!configModelSupportsSize(card, size)">{{ size }}</option>
                      </select>
                    </label>
                    <label class="block text-sm">{{ t('batchImage.config.ratio') }}
                      <select v-model="card.aspect_ratio" class="input mt-1 text-sm" aria-label="aspect_ratio" :disabled="submitting" @change="markConfigEdited">
                        <option v-for="ratio in configRatios" :key="ratio" :value="ratio">{{ ratio }}</option>
                      </select>
                    </label>
                    <label class="block text-sm" :class="isGeminiPlatform ? 'sm:col-span-2' : ''">{{ t('batchImage.config.model') }}
                      <select v-model="card.model" class="input mt-1 text-sm" aria-label="model" :disabled="submitting" @change="markConfigEdited">
                        <option :value="BATCH_IMAGE_AUTO_MODEL">{{ t('batchImage.config.auto') }}</option>
                        <option v-for="model in availableBatchImageModels" :key="model.value" :value="model.value">{{ model.label }}</option>
                      </select>
                    </label>
                  </div>
                  <label class="block text-sm">{{ t('batchImage.config.count') }}
                    <select v-model.number="card.output_count" class="input mt-1 text-sm" aria-label="output_count" :disabled="submitting" @change="markConfigEdited">
                      <option v-for="count in outputCountOptions" :key="count" :value="count">{{ count }}</option>
                    </select>
                  </label>
                  <div v-if="configConsistency && configReferenceImages.length" class="flex min-w-0 items-center gap-2 bg-gray-50 px-2 py-1.5 text-xs text-gray-600 dark:bg-dark-900 dark:text-gray-300">
                    <span class="min-w-0 break-all">{{ t('batchImage.config.usesSharedReference') }} · {{ configReferenceNames.join(', ') }}</span>
                  </div>
                  <div v-else-if="!configConsistency" class="space-y-2">
                    <BatchImageReferenceUpload
                      :images="card.reference_images || []" :names="card.reference_names || (card.reference_name ? [card.reference_name] : [])"
                      :label="t('batchImage.config.uploadItemReference')" :limit="configCardReferenceLimit(card)"
                      :multiple="isGeminiPlatform" :disabled="submitting || loadingModels"
                      :loading="!!card.reference_loading" :error="card.reference_error" compact
                      test-id="config-item-reference" remove-test-id="remove-item-reference" clear-test-id="clear-item-reference"
                      @files="loadCardReferenceFiles(card, $event)" @remove="removeCardReference(card, $event)" @clear="clearCardReference(card)"
                    />
                  </div>
                  <p class="text-xs text-gray-600 dark:text-gray-300">{{ t(isGeminiPlatform ? 'batchImage.config.geminiPixels' : 'batchImage.config.pixels', { size: batchImageConfigPixels(card, configPlatform) || t('batchImage.config.invalidCombination'), model: resolveBatchImageConfigModel(card, availableBatchImageModels) || t('batchImage.config.notSelected') }) }}</p>
                </div>
                <p v-if="configPreview.errors[card.localId]?.length" class="px-3 pb-2 text-xs text-red-600 dark:text-red-400" role="alert">{{ t('batchImage.config.image', { index: index + 1 }) }}：{{ configPreview.errors[card.localId].join('；') }}</p>
              </div>
              <details v-if="configPreview.groups.length" class="rounded border border-gray-200 p-2 text-xs dark:border-dark-700" data-testid="config-payload-preview">
                <summary class="cursor-pointer">{{ t('batchImage.config.payload') }}</summary>
                <pre class="mt-2 max-h-52 overflow-auto whitespace-pre-wrap break-all">{{ JSON.stringify(configPayloadDisplay, null, 2) }}</pre>
              </details>
              </template>
              <div v-else class="flex flex-1 flex-col items-center justify-center px-6 text-center" data-testid="config-empty-state">
                <span class="flex h-12 w-12 items-center justify-center rounded-lg bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-gray-300"><Icon name="document" size="lg" /></span>
                <p class="mt-3 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('batchImage.config.emptyDraft') }}</p>
              </div>
            </div>
          </div>

          <div class="md:col-span-2">
            <label class="input-label">{{ t('batchImage.create.outputFormat') }}</label>
            <select v-model="form.responseMimeType" class="input" :disabled="submitting" data-testid="config-mime">
              <option v-for="mime in batchImageMimeTypes" :key="mime" :value="mime">{{ mime === 'image/webp' ? 'WebP' : mime.slice(6).toUpperCase() }}</option>
            </select>
          </div>
          </template>

          <div v-else class="md:col-span-2 rounded-lg border border-dashed border-gray-200 py-14 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">
            {{ t('batchImage.create.selectKeyPlaceholder') }}
          </div>
        </div>

      </form>

      <template #footer>
        <div class="flex w-full items-center justify-end gap-3" data-testid="config-footer">
          <button type="button" class="btn btn-secondary flex-shrink-0" :disabled="submitting" @click="closeCreateModal">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary inline-flex min-w-[156px] justify-center" :disabled="!canSubmitConfig" data-testid="submit-config" @click="submitConfigJob">
            <Icon v-if="submitting" name="refresh" size="sm" class="mr-2 animate-spin" />
            {{ submitting ? t('batchImage.config.submitting') : t('batchImage.config.submit') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog :show="showGuideModal" :title="t('batchImage.guide.title')" width="wide" @close="showGuideModal = false">
	      <div class="space-y-5">
	        <section class="space-y-3">
	          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('batchImage.guide.uiTitle') }}</h3>
	          <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm leading-6 text-gray-700 dark:border-dark-700 dark:bg-dark-900/50 dark:text-gray-200">
	            <p>{{ t('batchImage.guide.step1') }}</p>
	            <p>{{ t('batchImage.guide.step2') }}</p>
	            <p>{{ t('batchImage.guide.step3') }}</p>
	            <p>{{ t('batchImage.guide.step4') }}</p>
	          </div>
	        </section>
	        <section class="space-y-3">
	          <div class="flex flex-wrap items-center justify-between gap-3">
	            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('batchImage.guide.skillTitle') }}</h3>
	            <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('batchImage.guide.skillDesc') }}</p>
	          </div>
	        <textarea
	          :value="agentInstruction"
	          readonly
	          class="min-h-[420px] w-full resize-y rounded-md border border-gray-200 bg-gray-50 p-4 font-mono text-sm leading-6 text-gray-800 outline-none focus:border-primary-400 focus:ring-2 focus:ring-primary-100 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-100 dark:focus:border-primary-500 dark:focus:ring-primary-900/40"
	        />
	        </section>
	      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="showGuideModal = false">{{ t('common.close') }}</button>
          <button type="button" class="btn btn-primary" @click="copyInstruction">
            <Icon name="copy" size="sm" class="mr-2" />
            {{ t('batchImage.actions.copyInstruction') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import BatchImageReferenceUpload from '@/components/batch-image/BatchImageReferenceUpload.vue'
import { batchImageMimeTypes, keyAllowsBatchImage } from '@/utils/batchImage'
import {
  BATCH_IMAGE_AUTO_MODEL,
  batchImageConfigMatrix,
  batchImageConfigPixels,
  batchImageConfigReferenceLimit,
  buildBatchImageConfigPreview,
  parseBatchImageConfigInput,
  resolveBatchImageConfigModel,
  type BatchImageConfigCard,
  type BatchImageConfigPlatform,
  type BatchImageConfigStatus,
} from '@/utils/batchImageConfig'
import { useClipboard } from '@/composables/useClipboard'
import { getPersistedPageSize, setPersistedPageSize } from '@/composables/usePersistedPageSize'
import { useAppStore } from '@/stores/app'
import { keysAPI } from '@/api'
import {
  cancelBatchImageJob,
  deleteBatchImageJobRecord,
  downloadBatchImageZip,
  getBatchImageItemContent,
  getBatchImageJob,
  getBatchImageRetryInput,
  listBatchImageJobs,
  listBatchImageItems,
  listBatchImageModels,
  saveBlob,
  submitBatchImageJob,
  type BatchImageItem,
  type BatchImageJob,
  type BatchImageJobsListOptions,
  type BatchImageReferenceImage,
  type BatchImageStatus,
  type BatchImageSubmitRequest,
} from '@/api/batchImage'
import type { ApiKey } from '@/types'
import { batchImageZipMergeLimits, buildBatchImageConfigByCustomId, mergeBatchImageZips, type BatchImageZipInput } from '@/utils/mergeBatchImageZips'
import type { Column } from '@/components/common/types'

type BatchImageJobRow = Pick<BatchImageJob, 'id' | 'task_name' | 'collection_id' | 'parent_batch_id' | 'status' | 'model' | 'provider' | 'item_count' | 'success_count' | 'fail_count' | 'estimated_cost' | 'hold_amount' | 'actual_cost' | 'created_at' | 'downloaded_at' | 'image_size' | 'aspect_ratio' | 'response_mime_type'> & {
  api_key_id: number
  api_key_name: string
  child_count: number
  is_child?: boolean
}

type BatchImageTaskRow = BatchImageJobRow & {
  collection_jobs: BatchImageJobRow[]
  technical_batch_count: number
}

type BatchImageDetailItem = BatchImageItem & {
  batch_id: string
  source_task_name: string
}

type PreviewImageSource = ImageBitmap | HTMLImageElement

type PreviewCacheRecord = {
  key: string
  blob: Blob
  size: number
  createdAt: number
  lastAccessedAt: number
}

type ConfigGroupSubmission = {
  fingerprint: string
  collectionFingerprints?: string[]
  idempotencyKey: string
  job: BatchImageJob
  config: BatchImageSubmitRequest
}

type PersistedConfigAttempt = {
  fingerprint: string
  collectionFingerprints?: string[]
  referenceMode?: 'shared' | 'perItem'
  idempotencyKey: string
  apiKeyId: number
  config: BatchImageSubmitRequest
  job?: BatchImageJob
  status: 'pending' | 'succeeded'
  updatedAt: number
}

const TERMINAL_STATUSES = new Set(['completed', 'partial_success', 'failed', 'cancelled', 'output_deleted'])
const PREVIEW_CACHE_DB_NAME = 'sub2api-batch-image-preview-cache'
const PREVIEW_CACHE_STORE_NAME = 'thumbnails'
const PREVIEW_THUMBNAIL_MAX_EDGE = 360
const PREVIEW_THUMBNAIL_QUALITY = 0.72
const PREVIEW_CACHE_MAX_AGE_MS = 3 * 24 * 60 * 60 * 1000
const PREVIEW_CACHE_MAX_ENTRIES = 120
const PREVIEW_CACHE_MAX_BYTES = 48 * 1024 * 1024
const BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM = 4
const outputCountOptions = Array.from({ length: BATCH_IMAGE_MAX_OUTPUTS_PER_ITEM }, (_, index) => index + 1)
const batchPageSizeOptions: SelectOption[] = [20, 50, 100].map(size => ({ value: size, label: String(size) }))
const CONFIG_ATTEMPT_STORAGE_PREFIX = 'sub2api-batch-image-config-attempts-v1'
const CONFIG_HISTORY_STORAGE_PREFIX = 'sub2api-batch-image-config-history-v1'
const CONFIG_STORAGE_TTL_MS = 30 * 24 * 60 * 60 * 1000
const CONFIG_HISTORY_MAX_ENTRIES = 200

function configStorageKey(prefix: string) {
  if (typeof window === 'undefined') return ''
  try {
    const user = JSON.parse(window.localStorage.getItem('auth_user') || 'null') as { id?: unknown } | null
    const userID = Number(user?.id || 0)
    return Number.isSafeInteger(userID) && userID > 0 ? `${prefix}:${userID}` : ''
  } catch {
    return ''
  }
}

function jsonClone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

function configForStorage(config: BatchImageSubmitRequest): BatchImageSubmitRequest {
  const copy = jsonClone(config)
  copy.items = copy.items.map(item => ({
    ...item,
    ...(item.reference_images ? {
      reference_images: item.reference_images.map(reference => {
        const { data: _data, file_uri: _fileURI, ...metadata } = reference
        return metadata
      }),
    } : {}),
  }))
  return copy
}

function loadPersistedConfigAttempts(): PersistedConfigAttempt[] {
  const key = configStorageKey(CONFIG_ATTEMPT_STORAGE_PREFIX)
  if (!key) return []
  try {
    const raw = JSON.parse(window.localStorage.getItem(key) || '[]')
    if (!Array.isArray(raw)) return []
    const cutoff = Date.now() - CONFIG_STORAGE_TTL_MS
    return raw.filter((entry): entry is PersistedConfigAttempt => Boolean(entry)
      && typeof entry.fingerprint === 'string'
      && typeof entry.idempotencyKey === 'string'
      && Number.isSafeInteger(entry.apiKeyId)
      && (entry.status === 'pending' || entry.status === 'succeeded')
      && Number.isFinite(entry.updatedAt)
      && entry.updatedAt >= cutoff)
  } catch {
    return []
  }
}

function persistConfigAttempts(attempts: PersistedConfigAttempt[]): boolean {
  const key = configStorageKey(CONFIG_ATTEMPT_STORAGE_PREFIX)
  if (!key || attempts.length > CONFIG_HISTORY_MAX_ENTRIES) return false
  try {
    window.localStorage.setItem(key, JSON.stringify(attempts))
    return true
  } catch {
    // Never send a billable request without a durable idempotency key.
    return false
  }
}

function loadConfigHistory(): ConfigGroupSubmission[] {
  const key = configStorageKey(CONFIG_HISTORY_STORAGE_PREFIX)
  if (!key) return []
  try {
    const raw = JSON.parse(window.localStorage.getItem(key) || '[]')
    if (!Array.isArray(raw)) return []
    const cutoff = Date.now() - CONFIG_STORAGE_TTL_MS
    return raw.filter((entry): entry is ConfigGroupSubmission & { updatedAt: number } => Boolean(entry)
      && typeof entry.fingerprint === 'string'
      && typeof entry.idempotencyKey === 'string'
      && entry.job && typeof entry.job.id === 'string'
      && entry.config && Number.isFinite(entry.updatedAt)
      && entry.updatedAt >= cutoff)
      .map(({ updatedAt: _updatedAt, ...entry }) => entry)
  } catch {
    return []
  }
}

function persistConfigHistory(history: ConfigGroupSubmission[]) {
  const key = configStorageKey(CONFIG_HISTORY_STORAGE_PREFIX)
  if (!key) return
  try {
    window.localStorage.setItem(key, JSON.stringify(history.slice(-CONFIG_HISTORY_MAX_ENTRIES).map(entry => ({
      ...entry,
      config: configForStorage(entry.config),
      updatedAt: Date.now(),
    }))))
  } catch {
    // 本地存储不可用或空间不足时，保留内存态。
  }
}

function restoreConfigSubmissionState() {
  configSubmissions.value = loadConfigHistory()
  pendingConfigAttempts.value = loadPersistedConfigAttempts().filter(entry => entry.status === 'pending')
}

function persistConfigAttempt(attempt: PersistedConfigAttempt): boolean {
  const attempts = loadPersistedConfigAttempts().filter(entry => entry.fingerprint !== attempt.fingerprint)
  // 只淘汰已确认记录，未确认的计费请求不能被静默丢弃。
  while (attempts.length >= CONFIG_HISTORY_MAX_ENTRIES) {
    const confirmed = attempts.findIndex(entry => entry.status === 'succeeded')
    if (confirmed < 0) return false
    attempts.splice(confirmed, 1)
  }
  attempts.push({ ...attempt, config: configForStorage(attempt.config) })
  const saved = persistConfigAttempts(attempts)
  if (saved) pendingConfigAttempts.value = attempts.filter(entry => entry.status === 'pending')
  return saved
}

async function configGroupFingerprint(group: BatchImageSubmitRequest, apiKeyId: number): Promise<string> {
  if (!globalThis.crypto?.subtle) throw new Error('此浏览器环境不支持安全的提交恢复记录；请使用 HTTPS 或本机安全环境。')
  const { collection_id: _collectionID, ...stableGroup } = group
  const bytes = new TextEncoder().encode(JSON.stringify([apiKeyId, stableGroup]))
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return `${apiKeyId}:${Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')}`
}

async function collectionIDForFingerprints(fingerprints: string[]) {
  const bytes = new TextEncoder().encode([...fingerprints].sort().join('|'))
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return `imgcol_${Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('').slice(0, 32)}`
}

const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const { t, locale } = useI18n()

const columns = computed<Column[]>(() => [
  { key: 'select', label: '', sortable: false, class: 'w-12 text-center' },
  { key: 'id', label: t('batchImage.columns.taskName'), sortable: false, class: 'w-[240px] max-w-[240px]' },
  { key: 'model', label: t('batchImage.columns.model'), sortable: false, class: 'w-[180px] max-w-[180px] text-center' },
  { key: 'api_key_name', label: t('batchImage.columns.apiKey'), sortable: false, class: 'w-40 max-w-40 text-center' },
  { key: 'status', label: t('common.status'), sortable: false, class: 'w-28 text-center' },
  { key: 'counts', label: t('batchImage.columns.result'), sortable: false, class: 'w-32 text-center' },
  { key: 'cost', label: t('batchImage.columns.cost'), sortable: false, class: 'w-36 text-center' },
  { key: 'downloaded', label: t('batchImage.columns.downloadStatus'), sortable: false, class: 'w-40 text-center' },
  { key: 'actions', label: t('common.actions'), sortable: false, class: 'w-40 text-center' },
])

const statusFilterOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('batchImage.filters.allStatuses') },
  { value: 'queued', label: t('batchImage.status.queued') },
  { value: 'running', label: t('batchImage.status.running') },
  { value: 'processing_results', label: t('batchImage.status.processingResults') },
  { value: 'settling', label: t('batchImage.status.settling') },
  { value: 'completed', label: t('batchImage.status.completed') },
  { value: 'failed', label: t('batchImage.status.failed') },
  { value: 'cancelled', label: t('batchImage.status.cancelled') },
  { value: 'output_deleted', label: t('batchImage.status.outputDeleted') },
])

const downloadFilterOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('batchImage.filters.allDownloadStates') },
  { value: 'true', label: t('batchImage.filters.downloaded') },
  { value: 'false', label: t('batchImage.filters.notDownloaded') },
])

const form = reactive({
  apiKeyId: 0,
  taskName: '',
  responseMimeType: 'image/png',
})

const filters = reactive({
  taskName: '',
  apiKeyId: '',
  status: '',
  downloaded: '',
})

const pagination = reactive({
  page: 1,
  page_size: Math.min(getPersistedPageSize(20), 100),
  has_more: false,
})

const apiKeys = ref<ApiKey[]>([])
const loadingKeys = ref(false)
const loadingJobs = ref(false)
const submitting = ref(false)
const refreshing = ref(false)
const cancelling = ref(false)
const downloading = ref(false)
const downloadingBatchId = ref('')
const retryingBatchId = ref('')
const bulkDownloading = ref(false)
const bulkDeleting = ref(false)
const deletingBatchId = ref('')
const loadingItems = ref(false)
const loadingModels = ref(false)
const showCreateModal = ref(false)
const showGuideModal = ref(false)
const currentJob = ref<BatchImageJob | null>(null)
const selectedCollectionId = ref('')
const detailTab = ref<'results' | 'batches'>('results')
const selectedBatchId = ref('')
const selectedBatchApiKeyId = ref(0)
const items = ref<BatchImageDetailItem[]>([])
const batchJobs = ref<BatchImageJobRow[]>([])
const selectedJobIds = ref(new Set<string>())
const configInput = ref('')
const configCards = ref<BatchImageConfigCard[]>([])
const configError = ref('')
const configConverting = ref(false)
const configStatus = ref<BatchImageConfigStatus>('empty')
const expandedConfigCard = ref('')
const configEdited = ref(false)
const configRecovered = ref(false)
const configConsistency = ref(false)
const configReferenceImages = ref<BatchImageReferenceImage[]>([])
const configReferenceNames = ref<string[]>([])
const configReferenceError = ref('')
const configReferenceLoading = ref(false)
const configSubmissions = ref<ConfigGroupSubmission[]>([])
const configCollectionId = ref('')
const downloadArtifact = ref<{ url: string; filename: string } | null>(null)
const pendingConfigAttempts = ref<PersistedConfigAttempt[]>([])
let configReferenceRequest = 0
const itemPreviewUrls = reactive<Record<string, string>>({})
const previewLoadingIds = ref(new Set<string>())
const previewErrorIds = ref(new Set<string>())
const previewImageItem = ref<BatchImageItem | null>(null)
const availableBatchImageModels = ref<Array<{
  value: string; label: string; supported_image_sizes?: string[]; supported_mime_types?: string[]
}>>([])
const modelLoadError = ref('')
const modelsApiKeyId = ref(0)
const openMoreJobId = ref('')
const moreMenuStyle = ref<Record<string, string>>({})
const promptPopover = reactive({
  visible: false,
  text: '',
  style: {} as Record<string, string>,
})
let modelRequestSeq = 0
let revertingApiKey = false
let restoringConfig = false
let pollTimer: ReturnType<typeof setInterval> | null = null
let previewCacheDBPromise: Promise<IDBDatabase | null> | null = null
let previewCacheCleanupTimer: ReturnType<typeof setInterval> | null = null
let promptPopoverCloseTimer: ReturnType<typeof setTimeout> | null = null
let promptPopoverOpenTimer: ReturnType<typeof setTimeout> | null = null
let activePromptPopoverTarget: HTMLElement | null = null

const batchImageApiKeys = computed(() =>
  apiKeys.value.filter(keyAllowsBatchImage),
)

const selectedApiKey = computed(() =>
  batchImageApiKeys.value.find((key) => key.id === Number(form.apiKeyId)) || null,
)

const isGeminiPlatform = computed(() => selectedApiKey.value?.group?.platform === 'gemini')
const configPlatform = computed<BatchImageConfigPlatform>(() => isGeminiPlatform.value ? 'gemini' : 'openai')
const configMatrix = computed(() => batchImageConfigMatrix(configPlatform.value))
const configSizes = computed(() => Object.keys(configMatrix.value))
const configRatios = computed(() => Object.keys(configMatrix.value['1K'] || {}))

function configCardReferenceLimit(card: BatchImageConfigCard) {
  if (!isGeminiPlatform.value) return 1
  return batchImageConfigReferenceLimit(resolveBatchImageConfigModel(card, availableBatchImageModels.value), configPlatform.value)
}

function configModelSupportsSize(card: BatchImageConfigCard, size: string) {
  const model = availableBatchImageModels.value.find(option => option.value === resolveBatchImageConfigModel(card, availableBatchImageModels.value))
  return !model?.supported_image_sizes || model.supported_image_sizes.includes(size)
}

const sharedReferenceLimit = computed(() => {
  if (!isGeminiPlatform.value) return 1
  const limits = configCards.value.length
    ? configCards.value.map(configCardReferenceLimit)
    : [batchImageConfigReferenceLimit(availableBatchImageModels.value[0]?.value || '', configPlatform.value)]
  return Math.min(...limits)
})
const configStatusLabel = computed(() => ({
  empty: t('batchImage.config.waiting'),
  converting: t('batchImage.config.converting'),
  converted: configEdited.value ? t('batchImage.config.edited') : t('batchImage.config.converted'),
  stale: t('batchImage.config.staleStatus'),
  invalid: t('batchImage.config.invalid'),
  submitting: t('batchImage.config.submitting'),
}[configStatus.value]))
const hasRecommendedConsistencyModel = computed(() => !isGeminiPlatform.value && availableBatchImageModels.value.some(model => model.value === 'gpt-image-2.5-sunburst'))
const configPreview = computed(() => buildBatchImageConfigPreview(
  configCards.value, availableBatchImageModels.value, form.taskName, form.responseMimeType,
  configConsistency.value ? configReferenceImages.value : undefined, configPlatform.value,
))
const canSubmitConfig = computed(() => configStatus.value === 'converted' &&
  !submitting.value &&
  (!configConsistency.value || (configReferenceImages.value.length > 0 && !configReferenceLoading.value && !configReferenceError.value)) &&
  !!selectedApiKey.value && !loadingModels.value && modelsApiKeyId.value === form.apiKeyId &&
  !modelLoadError.value && configCards.value.length > 0 &&
  Object.keys(configPreview.value.errors).length === 0)
const configPayloadDisplay = computed(() => configPreview.value.groups.map(group => ({
  ...group,
  items: group.items.map(item => ({ ...item, ...(item.reference_images ? {
    reference_images: item.reference_images.map(image => ({ ...image, data: configConsistency.value ? '[统一参考图 base64 已省略]' : '[参考图 base64 已省略]' })),
  } : {}) })),
})))
watch(configConsistency, () => {
  if (configCards.value.length && configStatus.value !== 'stale' && !submitting.value) markConfigEdited()
})
watch(configInput, () => {
  configRecovered.value = false
  if (configCards.value.length && !configConverting.value) configStatus.value = 'stale'
  if (configError.value) {
    configError.value = ''
    if (!configCards.value.length) configStatus.value = 'empty'
  }
})
watch(configPreview, preview => {
  if (!configCards.value.length || configStatus.value === 'stale' || submitting.value) return
  configStatus.value = Object.keys(preview.errors).length ? 'invalid' : 'converted'
})
const filteredApiKeys = computed(() => {
  const selectedFilterID = Number(filters.apiKeyId || 0)
  if (!selectedFilterID) return batchImageApiKeys.value
  return batchImageApiKeys.value.filter(key => key.id === selectedFilterID)
})

const apiKeyFilterOptions = computed<SelectOption[]>(() => [
  { value: '', label: t('batchImage.filters.allApiKeys') },
  ...batchImageApiKeys.value.map(key => ({
    value: String(key.id),
    label: key.name || `API Key #${key.id}`,
  })),
])

const childrenByParent = computed(() => {
  const groups = new Map<string, BatchImageJobRow[]>()
  for (const job of batchJobs.value) {
    if (!job.parent_batch_id) continue
    const rows = groups.get(job.parent_batch_id) || []
    rows.push(job)
    groups.set(job.parent_batch_id, rows)
  }
  for (const rows of groups.values()) {
    rows.sort((a, b) => a.created_at - b.created_at)
  }
  return groups
})

const visibleBatchJobs = computed<BatchImageTaskRow[]>(() => {
  const collections = new Map<string, BatchImageJobRow[]>()
  for (const job of batchJobs.value.filter(item => !item.parent_batch_id)) {
    const collectionID = job.collection_id || job.id
    const rows = collections.get(collectionID) || []
    rows.push(job)
    collections.set(collectionID, rows)
  }
  return [...collections.entries()].map(([collectionID, roots]) => {
    roots.sort((left, right) => right.created_at - left.created_at)
    const jobs = roots.flatMap(root => [root, ...(childrenByParent.value.get(root.id) || [])])
    const displayed = roots.map(root => displayJob(root))
    const representative = roots[0]!
    const models = new Set(roots.map(job => job.model))
    const providers = new Set(roots.map(job => job.provider))
    const allActualCostsReady = displayed.every(job => job.actual_cost !== null)
    const downloadedJobs = roots.filter(job => job.status === 'completed' && job.success_count > 0)
    return {
      ...representative,
      collection_id: collectionID,
      model: models.size === 1 ? representative.model : t('batchImage.list.multipleModels'),
      provider: providers.size === 1 ? representative.provider : 'mixed',
      status: aggregateTaskStatus(displayed),
      item_count: displayed.reduce((sum, job) => sum + job.item_count, 0),
      success_count: displayed.reduce((sum, job) => sum + job.success_count, 0),
      fail_count: displayed.reduce((sum, job) => sum + job.fail_count, 0),
      estimated_cost: displayed.reduce((sum, job) => sum + job.estimated_cost, 0),
      hold_amount: displayed.reduce((sum, job) => sum + job.hold_amount, 0),
      actual_cost: allActualCostsReady ? displayed.reduce((sum, job) => sum + (job.actual_cost || 0), 0) : null,
      downloaded_at: downloadedJobs.length > 0 && downloadedJobs.every(job => !!job.downloaded_at)
        ? Math.max(...downloadedJobs.map(job => job.downloaded_at || 0))
        : null,
      collection_jobs: jobs,
      technical_batch_count: roots.length,
      child_count: jobs.length - roots.length,
    }
  }).sort((left, right) => right.created_at - left.created_at)
})

const selectedTaskRows = computed(() => visibleBatchJobs.value.filter(task => selectedJobIds.value.has(task.id)))
const selectedRows = computed(() => {
  const seen = new Set<string>()
  return selectedTaskRows.value.flatMap(task => task.collection_jobs).filter(job => !seen.has(job.id) && seen.add(job.id))
})

const selectedDownloadableRows = computed(() =>
  selectedRows.value.filter(job => canDownload(job)),
)

const allVisibleSelected = computed(() =>
  visibleBatchJobs.value.length > 0 && visibleBatchJobs.value.every(job => selectedJobIds.value.has(job.id)),
)

const someVisibleSelected = computed(() =>
  visibleBatchJobs.value.some(job => selectedJobIds.value.has(job.id)) && !allVisibleSelected.value,
)

const previewImageUrl = computed(() => {
  const item = previewImageItem.value
  if (!item) return ''
  return itemPreviewUrls[itemPreviewKey(item)] || ''
})

const recoveredOriginalCustomIds = computed(() => {
  const rootBatchId = detailRootBatchId()
  if (!rootBatchId) return new Set<string>()
  const ids = new Set<string>()
  for (const item of items.value) {
    if (!isChildDetailItem(item) || !isSuccessfulImageItem(item)) continue
    const sourceCustomID = retrySourceCustomID(item.custom_id)
    if (sourceCustomID) ids.add(sourceCustomID)
  }
  return ids
})

const currentDisplayJob = computed(() => {
  if (!currentJob.value) return null
  const task = visibleBatchJobs.value.find(row => row.collection_id === selectedCollectionId.value)
  if (task) return task
  return displayJob(currentJob.value)
})
const currentTask = computed(() => visibleBatchJobs.value.find(row => row.collection_id === selectedCollectionId.value) || null)

function aggregateTaskStatus(jobs: Array<Pick<BatchImageJob, 'status' | 'success_count' | 'fail_count'>>) {
  if (jobs.some(job => !TERMINAL_STATUSES.has(job.status))) {
    if (jobs.some(job => ['running', 'indexing', 'processing_results', 'settling'].includes(job.status))) return 'running'
    return 'queued'
  }
  const success = jobs.reduce((sum, job) => sum + job.success_count, 0)
  const failed = jobs.reduce((sum, job) => sum + job.fail_count, 0)
  if (success > 0 && failed > 0) return 'partial_success'
  if (success > 0) return 'completed'
  if (jobs.every(job => job.status === 'cancelled')) return 'cancelled'
  if (jobs.every(job => job.status === 'output_deleted')) return 'output_deleted'
  return 'failed'
}

const endpointBase = computed(() => {
  const configured = appStore.apiBaseUrl?.trim()
  if (configured) return configured.replace(/\/+$/, '')
  if (typeof window !== 'undefined') return window.location.origin.replace(/\/+$/, '')
  return '<你的 Sub2API API 端点>'
})

const agentInstruction = computed(() => `---
name: sub2api-batch-image
description: 当用户希望用 OpenAI 或 Gemini/Vertex 批量生成图片、批量跑提示词、下载批量生图结果、重试失败图片时使用。
---

你是 Codex 中的批量生图执行 Agent。用户不需要手动填写页面表单；你应从当前聊天、用户给的文件、目录或上下文中整理任务名称、prompt 列表和输出目录，只有缺少关键决策时才向用户提问。

默认端点：
${endpointBase.value}

你需要自己完成：
1. 从用户聊天或附件中提取 prompt。每条 prompt 保留完整文本，按顺序生成稳定 custom_id，例如 img_001、img_002。
2. 从用户要求或上下文推断任务名称；没有明确名称时用当前时间生成任务名。
3. 从用户要求或上下文推断输出目录；如果用户没有说保存到哪里，才询问用户。
4. 提交前必须先计算 expected_output_count = 所有 item 的 output_count 之和。单个批量任务硬性最多 200 张输出图；超过 200 张必须拆成多组任务，不能提交一个超大任务，也不能把参考图附件上限当成生成张数上限。
5. 如果用户提供参考图，把参考图按用途绑定到具体 item。参考图只是输入附件，不是输出图数量。模型单条限制必须按模型执行：Gemini 2.5 Flash Image 每条最多 3 张参考图；Gemini 3 Pro Image 每条最多 14 张参考图。不要把后端附件风控理解成 Pro 单条能力：按 output_count 展开后，所有 item 的参考图附件总数还有内部保护阈值 1000 个，inline base64 参考图解码后总量最多 128MB。这个 1000 只是服务器拒绝异常请求的保护阈值，不是推荐规模；参考图很多或总请求体较大时应主动拆分任务。
6. 参考图会按 output_count 重复消耗输入 token；大量任务、重复复用同一张参考图或参考图总体积较大时，优先使用 gs:// file_uri 或拆分成多组任务。
7. 选择 API Key 和模型：先获取当前可用的批量生图 Key/模型；如果用户指定模型且该 Key 支持，则使用用户指定模型；否则使用该 Key 可用模型中的默认/第一个。不要展示或询问内部 provider 名称。
8. 调用批量生图 API 提交、轮询、下载，不要求用户去页面里手填。

API 调用规范：
- 模型：GET ${joinEndpointPath(endpointBase.value, '/v1/images/batches/models')}
- 提交：POST ${joinEndpointPath(endpointBase.value, '/v1/images/batches')}
- 查询：GET ${joinEndpointPath(endpointBase.value, '/v1/images/batches/{id}')}
- 明细：GET ${joinEndpointPath(endpointBase.value, '/v1/images/batches/{id}/items')}
- 下载：GET ${joinEndpointPath(endpointBase.value, '/v1/images/batches/{id}/download')}
- 取消：POST ${joinEndpointPath(endpointBase.value, '/v1/images/batches/{id}/cancel')}

提交请求体：
{
  "model": "<按所选 Key 可用模型填写>",
  "task_name": "<从聊天推断；为空则用当前时间>",
  "image_size": "1K",
  "response_mime_type": "image/png",
  "items": [
    {
      "custom_id": "img_001",
      "prompt": "<第一条完整 prompt>",
      "output_count": 1,
      "reference_images": [
        {
          "id": "face",
          "type": "subject",
          "mime_type": "image/png",
          "data": "<base64，不含 data:image/png;base64, 前缀>"
        }
      ]
    }
  ]
}

必须遵守：
- 不要把 API Key 写入仓库、日志、提交记录或最终回复。
- 不要把参考图 base64 写入最终回复、日志或公开文件。恢复记录中只保存参考图文件名、用途、数量和请求 JSON 文件路径；若请求 JSON 文件包含 base64，应保存在用户指定输出目录且不要提交到仓库。
- output_count 表示同一 prompt 和参考图重复生成几张，默认 1，每条最多 4；这不是依赖 Gemini 单次请求返回多图，而是系统展开成多个真实任务项。提交前必须确认预计输出图总数不超过 200，超过就拆分成多组任务。绝不能因为参考图附件有更高的内部保护阈值，就提交会生成超过 200 张图的任务。
- 当前对用户的批量生图计费仍按成功输出图片数量结算，不单独对参考图加价。可以向用户说明：参考图会产生少量上游输入 token 和临时存储成本，且会随 output_count 重复计算；页面显示的冻结/结算金额按输出图片数量计算。
- 提交成功后，必须立刻在输出目录写入本地恢复记录，例如 batch-image-resume.json。不要在恢复记录里保存 API Key。
- 恢复记录至少包含：endpoint、task_name、batch_id、model、output_dir、request_file、submitted_at、last_status、status_url、items_url、download_url、prompt_count、expected_output_count，以及可用于失败重试的 custom_id 到 prompt 映射或请求 JSON 文件路径。
- 每次查询状态后更新恢复记录，写入 last_checked_at、last_status、成功数、失败数、实际扣费和失败摘要。会话中断或暂停后，下次必须能凭该文件继续查询、下载或重试。
- 不要高频轮询。首次查询等待约 20 到 30 秒；queued 状态每 60 到 120 秒查询一次；如果连续 3 次仍是 queued，就先停止主动查询，告诉用户任务仍在排队，并保留恢复记录，之后可继续其他任务或等待用户稍后让你恢复。
- running 状态每约 60 秒查询一次，服务器压力大或大批量任务时可以更久；processing_results 等接近完成的状态可每 20 到 45 秒查询一次。
- 任务完成后报告任务名、任务 id、成功数、失败数、实际扣费和保存路径。
- 只下载成功图片。部分失败时，先展示失败 custom_id、错误码、错误来源和简要原因。
- 重试只能重试失败项，不能重复提交已成功项。必须使用完整原始 prompt、规格和参考图，不能把 prompt_preview 当作完整输入。可通过 GET /v1/images/batches/{id}/items?retry_input=true 读取受权限和保留期限制的失败原输入（当前仅支持 OpenAI 本地输入）。若不可恢复，必须要求用户重新提供原 prompt 和参考图，禁止静默改为文生图，也不能宣称完整重放。
- 取消任务前必须提醒：已被系统索引为成功的图片仍会按成功项结算扣费，其余冻结金额会释放。
- 图片预览按需加载；不要为了查看列表自动批量加载图片内容。`)

function joinEndpointPath(base: string, path: string): string {
  return `${base.replace(/\/+$/, '')}/${path.replace(/^\/+/, '')}`
}

function clearConfigReference() {
  configReferenceRequest += 1
  configReferenceImages.value = []
  configReferenceNames.value = []
  configReferenceError.value = ''
  configReferenceLoading.value = false
}

const cardReferenceRequests = new WeakMap<BatchImageConfigCard, number>()

function clearCardReference(card: BatchImageConfigCard) {
  cardReferenceRequests.set(card, (cardReferenceRequests.get(card) || 0) + 1)
  card.reference_images = []
  card.reference_name = ''
  card.reference_names = []
  card.reference_loading = false
  card.reference_error = ''
  markConfigEdited()
}

function removeCardReference(card: BatchImageConfigCard, index: number) {
  if (submitting.value || card.reference_loading) return
  card.reference_images?.splice(index, 1)
  card.reference_names?.splice(index, 1)
  card.reference_error = ''
  markConfigEdited()
}

function validateReferenceFiles(files: File[], existingCount: number, limit: number) {
  if (files.length + existingCount > limit) return t('batchImage.config.referenceLimitError', { limit })
  if (files.some(file => !batchImageMimeTypes.includes(file.type))) return t('batchImage.config.referenceTypeError')
  if (files.some(file => !file.size || file.size > 10 * 1024 * 1024)) return t('batchImage.config.referenceSizeError')
  return ''
}

async function readReferenceFiles(files: File[]): Promise<BatchImageReferenceImage[]> {
  return Promise.all(files.map(file => new Promise<BatchImageReferenceImage>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const data = String(reader.result || '').split(',')[1]
      if (!data) reject(new Error('empty reference'))
      else resolve({ mime_type: file.type, data })
    }
    reader.onerror = () => reject(new Error('read failed'))
    reader.onabort = () => reject(new Error('read aborted'))
    reader.readAsDataURL(file)
  })))
}

async function loadCardReferenceFiles(card: BatchImageConfigCard, files: File[]) {
  if (!files.length || submitting.value || configConsistency.value || card.reference_loading) return
  const existing = isGeminiPlatform.value && card.reference_images?.every(image => image.data) ? card.reference_images : []
  const names = existing.length ? card.reference_names || [] : []
  card.reference_error = validateReferenceFiles(files, existing.length, configCardReferenceLimit(card))
  if (card.reference_error) return
  const request = (cardReferenceRequests.get(card) || 0) + 1
  cardReferenceRequests.set(card, request)
  card.reference_loading = true
  try {
    const images = await readReferenceFiles(files)
    if (request !== cardReferenceRequests.get(card) || !configCards.value.includes(card)) return
    card.reference_images = [...existing, ...images]
    card.reference_names = [...names, ...files.map(file => file.name)]
    card.reference_name = card.reference_names[0] || ''
    markConfigEdited()
  } catch {
    if (request === cardReferenceRequests.get(card)) card.reference_error = t('batchImage.config.referenceReadError')
  } finally {
    if (request === cardReferenceRequests.get(card)) card.reference_loading = false
  }
}

function removeConfigReference(index: number) {
  if (submitting.value || configReferenceLoading.value) return
  configReferenceImages.value.splice(index, 1)
  configReferenceNames.value.splice(index, 1)
  configReferenceError.value = ''
  if (configCards.value.length) markConfigEdited()
}

async function loadConfigReferenceFiles(files: File[]) {
  if (!files.length || submitting.value || configReferenceLoading.value) return
  const existing = isGeminiPlatform.value ? configReferenceImages.value : []
  const names = existing.length ? configReferenceNames.value : []
  configReferenceError.value = validateReferenceFiles(files, existing.length, sharedReferenceLimit.value)
  if (configReferenceError.value) return
  const request = ++configReferenceRequest
  configReferenceLoading.value = true
  try {
    const images = await readReferenceFiles(files)
    if (request !== configReferenceRequest) return
    configReferenceImages.value = [...existing, ...images]
    configReferenceNames.value = [...names, ...files.map(file => file.name)]
    if (configCards.value.length) markConfigEdited()
  } catch {
    if (request === configReferenceRequest) configReferenceError.value = t('batchImage.config.referenceReadError')
  } finally {
    if (request === configReferenceRequest) configReferenceLoading.value = false
  }
}

function convertConfig() {
  if (configConverting.value || submitting.value) return
  if (configEdited.value && configCards.value.length && !window.confirm(t('batchImage.config.overwrite'))) return
  configConverting.value = true
  configStatus.value = 'converting'
  const result = parseBatchImageConfigInput(configInput.value, configPlatform.value)
  configConverting.value = false
  if (!result.ok) {
    configCards.value = []
    expandedConfigCard.value = ''
    configEdited.value = false
    configStatus.value = 'invalid'
    configError.value = result.error
    return
  }
  configError.value = ''
  configCards.value = result.cards
  expandedConfigCard.value = result.cards[0]?.localId || ''
  configEdited.value = false
  configStatus.value = Object.keys(configPreview.value.errors).length ? 'invalid' : 'converted'
}

function toggleConfigCard(localId: string) {
  expandedConfigCard.value = expandedConfigCard.value === localId ? '' : localId
}

function markConfigEdited() {
  configEdited.value = true
  if (configStatus.value !== 'stale') configStatus.value = Object.keys(configPreview.value.errors).length ? 'invalid' : 'converted'
}

function newCollectionID() {
  if (globalThis.crypto?.randomUUID) return `imgcol_${globalThis.crypto.randomUUID().replace(/-/g, '')}`
  return `imgcol_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 14)}`
}

async function submitConfigJob() {
  if (!canSubmitConfig.value || submitting.value) return
  const key = selectedApiKey.value
  if (!key) {
    appStore.showError(batchImageText('selectApiKey'))
    return
  }

  // Lock before the first asynchronous digest so rapid clicks cannot create
  // independent requests for the same billable configuration.
  submitting.value = true
  configStatus.value = 'submitting'
  const groups = configPreview.value.groups.map(group => jsonClone(group))
  const submitted = new Map(configSubmissions.value.map(entry => [entry.fingerprint, entry]))
  const pendingGroups: Array<{ group: BatchImageSubmitRequest; fingerprint: string }> = []
  const errors: unknown[] = []
  try {
    const currentFingerprints = new Set<string>()
    for (const group of groups) {
      if (!group.task_name?.trim()) {
        // Server-side default names depend on the current second and would
        // change the normalized hash when a lost response is replayed.
        const digest = await configGroupFingerprint({ ...group, task_name: '' }, key.id)
        group.task_name = `批量生图-${digest.slice(-12)}`
      }
      const fingerprint = await configGroupFingerprint(group, key.id)
      currentFingerprints.add(fingerprint)
    }
    configCollectionId.value = configCollectionId.value || await collectionIDForFingerprints([...currentFingerprints])
    for (const group of groups) group.collection_id = configCollectionId.value
    for (const group of groups) {
      const fingerprint = await configGroupFingerprint(group, key.id)
      if (!submitted.has(fingerprint)) pendingGroups.push({ group, fingerprint })
    }
    if (!pendingGroups.length) {
      appStore.showSuccess('所有当前配置组均已提交。')
      return
    }
    for (const { group, fingerprint } of pendingGroups) {
      const previous = loadPersistedConfigAttempts().find(entry => entry.fingerprint === fingerprint)
      if (previous?.status === 'succeeded') {
        if (!previous.job) {
          errors.push(new Error('已提交任务缺少可恢复的任务编号，请先在历史列表核对，勿重复提交。'))
          continue
        }
        const record: ConfigGroupSubmission = { fingerprint, collectionFingerprints: [...currentFingerprints], idempotencyKey: previous.idempotencyKey, job: previous.job, config: previous.config }
        configSubmissions.value = [...configSubmissions.value, record]
        submitted.set(fingerprint, record)
        persistConfigHistory(configSubmissions.value)
        continue
      }
      if (previous && (!previous.config || !canReuseConfigAttempt(previous.config, group))) {
        errors.push(new Error('已有未确认的提交记录，请恢复完全相同的配置和参考图后重试，或先在历史列表核对。'))
        continue
      }
      const idempotencyKey = previous?.idempotencyKey || `sub2api-ui-config-${fingerprint}`
      const attempt: PersistedConfigAttempt = {
        fingerprint, collectionFingerprints: [...currentFingerprints], referenceMode: configConsistency.value ? 'shared' : 'perItem', idempotencyKey, apiKeyId: key.id,
        config: group, status: 'pending', updatedAt: Date.now(),
      }
      if (!persistConfigAttempt(attempt)) {
        errors.push(new Error('浏览器无法保存提交恢复记录；为避免重复生成和扣费，本组未发送请求。请检查浏览器存储空间。'))
        continue
      }
      try {
        const submittedJob = await submitBatchImageJob(key.key, group, idempotencyKey)
        const job = { ...submittedJob, collection_id: submittedJob.collection_id || group.collection_id }
        const record: ConfigGroupSubmission = { fingerprint, collectionFingerprints: [...currentFingerprints], idempotencyKey, job, config: jsonClone(group) }
        configSubmissions.value = [...configSubmissions.value, record]
        submitted.set(fingerprint, record)
        if (!persistConfigAttempt({ ...attempt, config: group, job, status: 'succeeded', updatedAt: Date.now() })) {
          errors.push(new Error(`任务 ${job.id} 已提交，但浏览器无法保存结果。请在历史任务列表核对后再操作。`))
        }
        persistConfigHistory(configSubmissions.value)
        upsertJob(job, key)
      } catch (error) {
        // 只有服务端明确确认冻结余额失败（尚未调用 provider）才开启新意图。
        // 超时、断网及其它不确定结果始终复用原键。
        if ((error as { code?: unknown })?.code === 'INSUFFICIENT_BALANCE') {
          const round = Number(idempotencyKey.match(/-balance-(\d+)$/)?.[1] || 0) + 1
          persistConfigAttempt({ ...attempt, idempotencyKey: `sub2api-ui-config-${fingerprint}-balance-${round}` })
        }
        errors.push(error)
      }
    }

    const completedSubmissions = [...submitted.values()].filter(entry => currentFingerprints.has(entry.fingerprint))
    const firstSubmitted = completedSubmissions[0]?.job
    if (firstSubmitted) {
      currentJob.value = firstSubmitted
      selectedCollectionId.value = firstSubmitted.collection_id || firstSubmitted.id
      selectedBatchId.value = firstSubmitted.id
      selectedBatchApiKeyId.value = key.id
      items.value = []
      if (!errors.length) showCreateModal.value = false
      void loadItems()
      void loadBatchJobs()
      startPolling()
    }

    if (errors.length) {
      const message = batchImageErrorMessage(errors[0], batchImageText('submitFailed'))
      const submittedCount = completedSubmissions.length
      appStore.showError(submittedCount
        ? `已成功提交 ${submittedCount}/${configPreview.value.groups.length} 组；${errors.length} 组提交失败。再次提交会跳过已成功组。${message}`
        : message)
      configStatus.value = 'converted'
      return
    }

    if (!completedSubmissions.length) return
    configStatus.value = 'converted'
    appStore.showSuccess(`已真实提交 ${completedSubmissions.length} 组批量任务。`)
    resetCreateDraft()
  } catch (error) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('submitFailed')))
  } finally {
    submitting.value = false
    if (configStatus.value === 'submitting') configStatus.value = 'converted'
  }
}

function restorePendingConfig(attempt: PersistedConfigAttempt) {
  if (submitting.value || !attempt.config || !Array.isArray(attempt.config.items)) return
  if (!batchImageApiKeys.value.some(key => key.id === attempt.apiKeyId)) {
    appStore.showError(t('batchImage.config.originalKeyUnavailable'))
    return
  }
  if (configCards.value.length && !window.confirm('恢复将覆盖当前未提交的配置，是否继续？')) return
  const group = attempt.config
  configCollectionId.value = group.collection_id || newCollectionID()
  restoringConfig = true
  form.apiKeyId = attempt.apiKeyId
  form.taskName = group.task_name || ''
  form.responseMimeType = group.response_mime_type || 'image/png'
  clearConfigReference()
  configInput.value = ''
  configConsistency.value = attempt.referenceMode !== 'perItem' && group.items.some(item => !!item.reference_images?.length)
  configCards.value = group.items.map((item, index) => ({
    localId: `restored-${index + 1}`,
    custom_id: item.custom_id,
    prompt: item.prompt,
    image_size: group.image_size || '1K',
    aspect_ratio: group.aspect_ratio || '1:1',
    model: group.model,
    output_count: item.output_count || 1,
    ...(!configConsistency.value && item.reference_images?.length ? {
      reference_images: item.reference_images.map(image => ({ ...image })),
    } : {}),
  }))
  expandedConfigCard.value = configCards.value[0]?.localId || ''
  configEdited.value = false
  configError.value = ''
  // 等 v-model 的原文侦听器执行后，恢复草稿才进入可编辑状态。
  void nextTick(() => {
    restoringConfig = false
    configRecovered.value = true
    configStatus.value = Object.keys(configPreview.value.errors).length ? 'invalid' : 'converted'
  })
}

function canReuseConfigAttempt(stored: BatchImageSubmitRequest, current: BatchImageSubmitRequest): boolean {
  return JSON.stringify(stored) === JSON.stringify(configForStorage(current))
}

async function loadApiKeys() {
  loadingKeys.value = true
  try {
    const response = await keysAPI.list(1, 100, { status: 'active', sort_by: 'created_at', sort_order: 'desc' })
    apiKeys.value = response.items || []
    if (!batchImageApiKeys.value.some(key => key.id === form.apiKeyId)) {
      form.apiKeyId = batchImageApiKeys.value[0]?.id || 0
    }
    if (filters.apiKeyId && !batchImageApiKeys.value.some(key => String(key.id) === filters.apiKeyId)) {
      filters.apiKeyId = ''
    }
    if (!selectedApiKey.value) {
      modelRequestSeq += 1
      modelsApiKeyId.value = 0
      availableBatchImageModels.value = []
    }
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('loadKeysFailed')))
  } finally {
    loadingKeys.value = false
  }
}

async function loadAvailableModels() {
  const key = selectedApiKey.value
  const requestID = ++modelRequestSeq
  modelLoadError.value = ''
  modelsApiKeyId.value = 0
  availableBatchImageModels.value = []
  if (!key) {
    loadingModels.value = false
    return
  }

  loadingModels.value = true
  try {
    const result = await listBatchImageModels(key.key)
    if (requestID !== modelRequestSeq) return
    const seen = new Set<string>()
    availableBatchImageModels.value = (result.data || [])
      .map(model => ({ ...model, value: String(model.id || '').trim(), label: String(model.id || '').trim() }))
      .filter((model) => {
        if (!model.value || seen.has(model.value)) return false
        seen.add(model.value)
        return true
      })
    modelsApiKeyId.value = key.id
  } catch (error: any) {
    if (requestID !== modelRequestSeq) return
    modelLoadError.value = batchImageErrorMessage(error, batchImageText('loadModelsFailed'))
  } finally {
    if (requestID === modelRequestSeq) {
      loadingModels.value = false
    }
  }
}

async function refreshPage() {
  await loadApiKeys()
  await loadBatchJobs()
}

function applyFilters() {
  pagination.page = 1
  selectedJobIds.value = new Set()
  void loadBatchJobs()
}

function resetFilters() {
  filters.taskName = ''
  filters.apiKeyId = ''
  filters.status = ''
  filters.downloaded = ''
  applyFilters()
}

function listOptions(): BatchImageJobsListOptions {
  const options: BatchImageJobsListOptions = {
    limit: pagination.page_size,
    cursor: String((pagination.page - 1) * pagination.page_size),
  }
  if (filters.taskName.trim()) options.taskName = filters.taskName.trim()
  if (filters.status) options.status = filters.status
  if (filters.downloaded) options.downloaded = filters.downloaded
  return options
}

function toJobRow(job: BatchImageJob, key = selectedApiKey.value): BatchImageJobRow {
  const storedSubmission = configSubmissions.value.find(entry => entry.job.id === job.id)
  const storedConfig = storedSubmission?.config
  const storedCollectionID = storedSubmission?.config.collection_id || (storedSubmission?.collectionFingerprints?.length
    ? `legacy_${[...storedSubmission.collectionFingerprints].sort().join('|')}`
    : '')
  return {
    id: job.id,
    task_name: job.task_name || defaultTaskName(job.created_at),
    collection_id: job.collection_id || storedCollectionID || job.id,
    parent_batch_id: job.parent_batch_id || null,
    status: job.status,
    model: job.model,
    provider: job.provider,
    image_size: job.image_size || storedConfig?.image_size,
    aspect_ratio: job.aspect_ratio || storedConfig?.aspect_ratio,
    response_mime_type: job.response_mime_type || storedConfig?.response_mime_type,
    item_count: job.item_count,
    success_count: job.success_count,
    fail_count: job.fail_count,
    estimated_cost: job.estimated_cost,
    hold_amount: job.hold_amount,
    actual_cost: job.actual_cost,
    created_at: job.created_at,
    downloaded_at: job.downloaded_at,
    api_key_id: key?.id || 0,
    api_key_name: key?.name || '',
    child_count: 0,
  }
}

function applyChildCounts(rows: BatchImageJobRow[]) {
  const counts = new Map<string, number>()
  for (const row of rows) {
    if (!row.parent_batch_id) continue
    counts.set(row.parent_batch_id, (counts.get(row.parent_batch_id) || 0) + 1)
  }
  return rows.map(row => ({ ...row, child_count: counts.get(row.id) || 0 }))
}

function displayJob<T extends Pick<BatchImageJob, 'id' | 'parent_batch_id' | 'status' | 'item_count' | 'success_count' | 'fail_count' | 'estimated_cost' | 'hold_amount' | 'actual_cost'>>(job: T): T {
  if (job.parent_batch_id) return job
  const children = childrenByParent.value.get(job.id) || []
  if (!children.length) return job

  const childSuccess = children.reduce((sum, child) => sum + child.success_count, 0)
  const childEstimated = children.reduce((sum, child) => sum + child.estimated_cost, 0)
  const childHold = children.reduce((sum, child) => sum + child.hold_amount, 0)
  const childActual = children.reduce((sum, child) => sum + (child.actual_cost || 0), 0)
  const childActualReady = children.every(child => child.actual_cost !== null)
  const successCount = Math.min(job.item_count, job.success_count + childSuccess)
  const failCount = Math.max(0, job.item_count - successCount)
  const actualCost = job.actual_cost === null
    ? (childActualReady ? childActual : null)
    : job.actual_cost + childActual

  return {
    ...job,
    success_count: successCount,
    fail_count: failCount,
    status: failCount === 0 && TERMINAL_STATUSES.has(job.status) ? 'completed' : job.status,
    estimated_cost: job.estimated_cost + childEstimated,
    hold_amount: job.hold_amount + childHold,
    actual_cost: actualCost,
  }
}

function hasChildJobs(batchId: string) {
  return (childrenByParent.value.get(batchId) || []).length > 0
}

function closeMoreMenu() {
  openMoreJobId.value = ''
}

function toggleMoreMenu(job: BatchImageJobRow, event: MouseEvent) {
  if (openMoreJobId.value === job.id) {
    closeMoreMenu()
    return
  }
  const trigger = event.currentTarget as HTMLElement | null
  const rect = trigger?.getBoundingClientRect()
  if (!rect) return
  const menuWidth = 176
  const margin = 8
  const left = Math.max(margin, Math.min(rect.right - menuWidth, window.innerWidth - menuWidth - margin))
  const top = Math.min(rect.bottom + margin, window.innerHeight - 96)
  moreMenuStyle.value = {
    left: `${left}px`,
    top: `${Math.max(margin, top)}px`,
  }
  openMoreJobId.value = job.id
}

function cancelPromptPopoverClose() {
  if (!promptPopoverCloseTimer) return
  clearTimeout(promptPopoverCloseTimer)
  promptPopoverCloseTimer = null
}

function cancelPromptPopoverOpen() {
  if (!promptPopoverOpenTimer) return
  clearTimeout(promptPopoverOpenTimer)
  promptPopoverOpenTimer = null
}

function closePromptPopover() {
  cancelPromptPopoverOpen()
  cancelPromptPopoverClose()
  promptPopover.visible = false
  promptPopover.text = ''
  promptPopover.style = {}
  activePromptPopoverTarget = null
}

function schedulePromptPopoverClose() {
  cancelPromptPopoverOpen()
  cancelPromptPopoverClose()
  promptPopoverCloseTimer = setTimeout(() => {
    closePromptPopover()
  }, 180)
}

function schedulePromptPopoverOpen(event: MouseEvent | PointerEvent, text: string) {
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  const value = String(text || '').trim()
  if (!value || value === '-') return
  activePromptPopoverTarget = target
  cancelPromptPopoverOpen()
  cancelPromptPopoverClose()
  promptPopoverOpenTimer = setTimeout(() => {
    if (activePromptPopoverTarget !== target || !document.body.contains(target)) return
    openPromptPopover(target, value)
  }, 520)
}

function showPromptPopover(event: MouseEvent | FocusEvent, text: string) {
  const value = String(text || '').trim()
  if (!value || value === '-') return
  const target = event.currentTarget as HTMLElement | null
  cancelPromptPopoverClose()
  cancelPromptPopoverOpen()
  if (!target) return
  activePromptPopoverTarget = target
  openPromptPopover(target, value)
}

function openPromptPopover(target: HTMLElement, value: string) {
  const rect = target.getBoundingClientRect()
  if (!rect) return
  const viewportWidth = window.innerWidth || 1280
  const viewportHeight = window.innerHeight || 720
  const width = Math.min(440, Math.max(320, viewportWidth - 32))
  const left = Math.max(16, Math.min(rect.left, viewportWidth - width - 16))
  const estimatedHeight = 178
  const preferredTop = rect.bottom + 8
  const top = preferredTop + estimatedHeight > viewportHeight
    ? Math.max(16, rect.top - estimatedHeight - 8)
    : preferredTop
  promptPopover.text = value
  promptPopover.style = {
    left: `${left}px`,
    top: `${top}px`,
    width: `${width}px`,
  }
  promptPopover.visible = true
}

function copyPromptPopover() {
  if (!promptPopover.text) return
  void copyToClipboard(promptPopover.text, t('batchImage.promptPopover.copied'))
}

async function loadBatchJobs() {
  const keys = filteredApiKeys.value
  if (!keys.length) {
    batchJobs.value = []
    pagination.has_more = false
    return
  }
  loadingJobs.value = true
  closeMoreMenu()
  try {
    const options = listOptions()
    const results = await Promise.all(keys.map(async (key) => {
      const result = await listBatchImageJobs(key.key, options)
      return {
        hasMore: Boolean(result.has_more),
        rows: (result.data || []).map(job => toJobRow(job, key)),
      }
    }))
    batchJobs.value = applyChildCounts(results
      .flatMap(result => result.rows)
      .sort((a, b) => b.created_at - a.created_at)
      .slice(0, pagination.page_size))
    pagination.has_more = results.some(result => result.hasMore)
    selectedJobIds.value = new Set([...selectedJobIds.value].filter(id => visibleBatchJobs.value.some(job => job.id === id)))
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('loadJobsFailed')))
  } finally {
    loadingJobs.value = false
  }
}

function upsertJob(job: BatchImageJob, key = selectedApiKey.value) {
  const submission = configSubmissions.value.find(entry => entry.job.id === job.id)
  if (submission) {
    submission.job = job
    persistConfigHistory(configSubmissions.value)
  }
  const next = toJobRow(job, key)
  const index = batchJobs.value.findIndex(item => item.id === job.id)
  if (index >= 0) {
    const rows = [...batchJobs.value]
    rows[index] = { ...next, is_child: rows[index].is_child }
    batchJobs.value = applyChildCounts(rows)
    return
  }
  batchJobs.value = applyChildCounts([next, ...batchJobs.value].slice(0, pagination.page_size))
}

function handlePageChange(page: number) {
  if (page < 1 || page === pagination.page) return
  pagination.page = page
  selectedJobIds.value = new Set()
  void loadBatchJobs()
}

function handlePageSizeChange(value: string | number | boolean | null) {
  if (value === null || typeof value === 'boolean') return
  const nextSize = Math.min(Math.max(Number(value) || 20, 1), 100)
  pagination.page_size = nextSize
  pagination.page = 1
  setPersistedPageSize(nextSize)
  selectedJobIds.value = new Set()
  void loadBatchJobs()
}

function openCreateModal() {
  showCreateModal.value = true
  if (!apiKeys.value.length) {
    void loadApiKeys()
  }
}

function closeCreateModal() {
  if (submitting.value) return
  showCreateModal.value = false
  resetCreateDraft()
}

function resetCreateDraft() {
  configRecovered.value = false
  configCollectionId.value = ''
  submitting.value = false
  form.taskName = ''
  form.responseMimeType = 'image/png'
  configConsistency.value = false
  clearConfigReference()
  configInput.value = ''
  configCards.value = []
  configError.value = ''
  configConverting.value = false
  configStatus.value = 'empty'
  expandedConfigCard.value = ''
  configEdited.value = false
}

function clearProviderDrafts() {
  configRecovered.value = false
  configCollectionId.value = ''
  configConsistency.value = false
  clearConfigReference()
  configInput.value = ''
  configCards.value = []
  configError.value = ''
  configStatus.value = 'empty'
  expandedConfigCard.value = ''
  configEdited.value = false
}

function closeDetail() {
  closePromptPopover()
  currentJob.value = null
  selectedCollectionId.value = ''
  detailTab.value = 'results'
  selectedBatchId.value = ''
  selectedBatchApiKeyId.value = 0
  items.value = []
  clearItemPreviews()
}

function keyForSelectedBatch(): ApiKey | null {
  if (selectedBatchApiKeyId.value) {
    const key = batchImageApiKeys.value.find(item => item.id === selectedBatchApiKeyId.value)
    if (key) return key
  }
  return selectedApiKey.value
}

function requireApiKey(): ApiKey | null {
  if (!selectedApiKey.value) {
    appStore.showError(batchImageText('selectApiKey'))
    return null
  }
  return selectedApiKey.value
}

async function refreshSelected() {
  if (!selectedBatchId.value) return
  const key = keyForSelectedBatch() || requireApiKey()
  if (!key) return
  refreshing.value = true
  try {
    const job = await getBatchImageJob(key.key, selectedBatchId.value)
    currentJob.value = job
    upsertJob(job)
    if (TERMINAL_STATUSES.has(job.status) && !configSubmissions.value.some(entry => !TERMINAL_STATUSES.has(entry.job.status))) stopPolling()
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('refreshFailed')))
  } finally {
    refreshing.value = false
  }
}

async function refreshDetail() {
  await Promise.all([
    refreshSelected(),
    loadItems(),
  ])
}

function selectJob(batchId: string) {
  const row = visibleBatchJobs.value.find(job => job.id === batchId)
  if (row?.api_key_id && batchImageApiKeys.value.some(key => key.id === row.api_key_id)) {
    form.apiKeyId = row.api_key_id
    selectedBatchApiKeyId.value = row.api_key_id
  } else {
    selectedBatchApiKeyId.value = 0
  }
  selectedCollectionId.value = row?.collection_id || row?.id || batchId
  detailTab.value = 'results'
  selectedBatchId.value = batchId
  currentJob.value = null
  items.value = []
  void refreshSelected()
  void loadItems()
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(() => {
    const active = configSubmissions.value.some(entry => !TERMINAL_STATUSES.has(entry.job.status))
    if ((!currentJob.value || TERMINAL_STATUSES.has(currentJob.value.status)) && !active) {
      stopPolling()
      return
    }
    if (currentJob.value && !TERMINAL_STATUSES.has(currentJob.value.status)) void refreshSelected()
    void refreshConfigSubmissions()
  }, 8000)
}

let refreshingConfigSubmissions = false
async function refreshConfigSubmissions() {
  if (refreshingConfigSubmissions) return
  refreshingConfigSubmissions = true
  try {
    for (const entry of configSubmissions.value) {
      if (entry.job.id === selectedBatchId.value || TERMINAL_STATUSES.has(entry.job.status)) continue
      const key = batchImageApiKeys.value.find(key => key.id === Number(entry.fingerprint.split(':')[0]))
      if (!key) continue
      try {
        upsertJob(await getBatchImageJob(key.key, entry.job.id), key)
      } catch {
        continue
      }
    }
  } finally {
    refreshingConfigSubmissions = false
  }
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function canCancel(job: Pick<BatchImageJob, 'status'>) {
  return !TERMINAL_STATUSES.has(job.status)
}

function canDownload(job: Pick<BatchImageJob, 'status' | 'success_count'>) {
  return (job.status === 'completed' || job.status === 'partial_success') && job.success_count > 0
}

function canRetry(job: Pick<BatchImageJob, 'status' | 'fail_count'>) {
  const display = 'id' in job ? displayJob(job as BatchImageJob) : job
  return TERMINAL_STATUSES.has(display.status) && display.fail_count > 0
}

function isDownloadingJob(batchId: string) {
  return downloading.value && downloadingBatchId.value === batchId
}

function applyJobApiKey(job: BatchImageJobRow | Pick<BatchImageJob, 'id'>) {
  if ('api_key_id' in job && job.api_key_id && batchImageApiKeys.value.some(key => key.id === job.api_key_id)) {
    form.apiKeyId = job.api_key_id
  }
}

function apiKeyForJob(job: BatchImageJobRow | Pick<BatchImageJob, 'id'>): ApiKey | null {
  if ('api_key_id' in job && job.api_key_id) {
    return batchImageApiKeys.value.find(key => key.id === job.api_key_id) || null
  }
  return selectedApiKey.value
}

function toggleJobSelection(batchId: string, checked: boolean) {
  const next = new Set(selectedJobIds.value)
  if (checked) next.add(batchId)
  else next.delete(batchId)
  selectedJobIds.value = next
}

function toggleAllVisible(checked: boolean) {
  const next = new Set(selectedJobIds.value)
  for (const job of visibleBatchJobs.value) {
    if (checked) next.add(job.id)
    else next.delete(job.id)
  }
  selectedJobIds.value = next
}

function canDeleteRecord(job: Pick<BatchImageJob, 'status'>) {
  return TERMINAL_STATUSES.has(job.status)
}

async function cancelSelected() {
  if (!currentTask.value) return
  const activeJobs = currentTask.value.collection_jobs.filter(job => !job.parent_batch_id && canCancel(job))
  if (!activeJobs.length) return
  if (!window.confirm(batchImageText('cancelConfirm'))) return
  cancelling.value = true
  try {
    for (const activeJob of activeJobs) {
      const key = apiKeyForJob(activeJob)
      if (!key) continue
      const job = await cancelBatchImageJob(key.key, activeJob.id)
      if (currentJob.value?.id === job.id) currentJob.value = job
      upsertJob(job, key)
    }
    appStore.showSuccess(batchImageText('cancelled'))
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('cancelFailed')))
  } finally {
    cancelling.value = false
  }
}

async function retrySelected() {
  if (!currentTask.value) return
  await retryTask(currentTask.value)
}

async function retryTask(task: BatchImageTaskRow) {
  const failedRoots = task.collection_jobs.filter(job => !job.parent_batch_id && canRetry(displayJob(job)))
  for (const job of failedRoots) await retryFailedJob(job)
}

async function retryFailedJob(job: BatchImageJobRow | BatchImageJob) {
  if (!canRetry(job) || retryingBatchId.value) return
  closeMoreMenu()
  const key = apiKeyForJob(job) || keyForSelectedBatch() || requireApiKey()
  if (!key) return
  retryingBatchId.value = job.id
  try {
    const original = await getBatchImageRetryInput(key.key, job.id)
    const retryDigest = await configGroupFingerprint({ ...original, parent_batch_id: job.id }, key.id)
    const failedItems = original.items.map(item => ({ ...item, custom_id: retryCustomID(item.custom_id, retryDigest.slice(-12)) }))
    if (failedItems.length === 0) {
      appStore.showError(batchImageText('retryMissingPrompts'))
      return
    }
    const retryJob = await submitBatchImageJob(
      key.key,
      {
        ...original,
        collection_id: job.collection_id || original.collection_id || job.id,
        task_name: `${original.task_name || job.id} 重试失败项`,
        parent_batch_id: rootBatchIdForRetry(job),
        items: failedItems,
      },
      `sub2api-ui-retry-${retryDigest}`,
    )
    currentJob.value = retryJob
    selectedBatchId.value = retryJob.id
    selectedBatchApiKeyId.value = key.id
    items.value = []
    upsertJob(retryJob)
    appStore.showSuccess(batchImageText('retrySubmitted'))
    void loadItems()
    startPolling()
  } catch (error: any) {
    appStore.showError(error?.code === 'BATCH_IMAGE_RETRY_INPUT_UNAVAILABLE'
      ? (locale.value.startsWith('zh')
        ? '无法恢复原始输入（可能已过期清理或该模型暂不支持恢复）。重试已阻止，不会改为文生图。请新建任务，重新上传原参考图并填写原提示词。'
        : 'Original input cannot be restored (expired, cleaned up, or unsupported provider). Retry is blocked, not converted to text-to-image. Create a new task and re-upload the original reference images and prompts.')
      : batchImageErrorMessage(error, batchImageText('retryFailed')))
  } finally {
    retryingBatchId.value = ''
  }
}

function retryCustomID(customID: string, digest: string) {
  const base = String(customID || 'item').replace(/(?:_retry_[a-z0-9]+)+$/i, '')
  return `${base}_retry_${digest}`
}

function rootBatchIdForRetry(job: BatchImageJobRow | BatchImageJob) {
  return job.parent_batch_id || job.id
}

async function downloadJob(job: (BatchImageJobRow | Pick<BatchImageJob, 'id'>)) {
  if (downloading.value) return
  closeMoreMenu()
  applyJobApiKey(job)
  const key = apiKeyForJob(job) || requireApiKey()
  if (!key) return
  downloading.value = true
  downloadingBatchId.value = job.id
  try {
    const blob = await downloadBatchImageZip(key.key, job.id)
    saveBatchResult(blob, `${job.id}.zip`)
    markJobDownloaded(job.id)
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('downloadFailed')))
  } finally {
    downloading.value = false
    downloadingBatchId.value = ''
  }
}

async function downloadTask(task: BatchImageTaskRow) {
  await downloadBatchImageRows([...task.collection_jobs])
}

async function downloadSelectedJobs() {
  await downloadBatchImageRows([...selectedRows.value])
}

function configForSubmittedJob(batchId: string): BatchImageSubmitRequest | undefined {
  const entry = configSubmissions.value.find(item => item.job.id === batchId)
  return entry?.config
}

function clearDownloadArtifact() {
  if (downloadArtifact.value) URL.revokeObjectURL(downloadArtifact.value.url)
  downloadArtifact.value = null
}

function saveBatchResult(blob: Blob, filename: string) {
  clearDownloadArtifact()
  downloadArtifact.value = { url: URL.createObjectURL(blob), filename }
  saveBlob(blob, filename)
}

async function downloadBatchImageRows(rows: BatchImageJobRow[]) {
  if (downloading.value || bulkDownloading.value || !rows.some(row => canDownload(row))) return
  if (rows.length === 1) {
    await downloadJob(rows[0]!)
    return
  }
  if (rows.length > batchImageZipMergeLimits.maxInputs) {
    appStore.showError(`一次最多合并 ${batchImageZipMergeLimits.maxInputs} 个任务，请减少选择。`)
    return
  }
  bulkDownloading.value = true
  downloading.value = true
  const inputs: BatchImageZipInput[] = []
  let totalBytes = 0
  try {
    for (const row of rows) {
      const key = apiKeyForJob(row)
      downloading.value = true
      downloadingBatchId.value = row.id
      const budget = Math.min(batchImageZipMergeLimits.maxInputZipBytes, batchImageZipMergeLimits.maxTotalInputBytes - totalBytes)
      if (budget <= 0) throw new Error('ZIP 总大小超过合并限制，请减少选择或分开下载。')
      let zip: Blob | undefined
      let unavailable: BatchImageZipInput['unavailable']
      if (!key || !canDownload(row)) {
        unavailable = { code: !key ? 'API_KEY_UNAVAILABLE' : row.status.toUpperCase(), message: `Batch ${row.id}: ${!key ? 'API key unavailable' : row.status}`, model: row.model, itemCount: row.item_count }
      } else {
        try {
          zip = await downloadBatchImageZip(key.key, row.id, budget)
        } catch {
          unavailable = { code: 'DOWNLOAD_FAILED', message: `Download failed for batch ${row.id}; retry its download.`, model: row.model, itemCount: row.item_count }
        }
      }
      if (zip) {
        totalBytes += zip.size
        if (zip.size > budget) throw new Error('ZIP 超过合并下载大小限制。')
      }
      const config = configForSubmittedJob(row.id)
      inputs.push({
        batchId: row.id,
        zip,
        unavailable,
        ...(config ? {
          config: {
            model: config.model,
            ...(config.image_size ? { image_size: config.image_size } : {}),
            ...(config.aspect_ratio ? { aspect_ratio: config.aspect_ratio } : {}),
            ...(config.response_mime_type ? { response_mime_type: config.response_mime_type } : {}),
            ...(config.task_name ? { task_name: config.task_name } : {}),
          },
          configByCustomId: buildBatchImageConfigByCustomId(config),
        } : {}),
      })
      if (zip) markJobDownloaded(row.id)
    }
    const merged = await mergeBatchImageZips(inputs)
    saveBatchResult(merged, `batch-image-merged-${new Date().toISOString().replace(/[:.]/g, '-')}.zip`)
    appStore.showSuccess(`已合并 ${inputs.length} 个真实批量任务的 ZIP 结果。`)
  } catch (error: any) {
    appStore.showError(error?.message || batchImageErrorMessage(error, batchImageText('downloadFailed')))
  } finally {
    bulkDownloading.value = false
    downloading.value = false
    downloadingBatchId.value = ''
  }
}

async function deleteTask(task: BatchImageTaskRow) {
  const rows = task.collection_jobs.filter(job => canDeleteRecord(job))
  if (!rows.length || deletingBatchId.value) return
  closeMoreMenu()
  if (!window.confirm(batchImageText('deleteConfirm'))) return
  for (const row of rows) {
    const key = apiKeyForJob(row)
    if (!key) continue
    deletingBatchId.value = row.id
    try {
      await deleteBatchImageJobRecord(key.key, row.id)
      removeJobFromList(row.id)
    } catch (error: any) {
      appStore.showError(batchImageErrorMessage(error, batchImageText('deleteFailed')))
      break
    }
  }
  deletingBatchId.value = ''
  appStore.showSuccess(batchImageText('deleted'))
}

async function deleteSelectedJobs() {
  const rows = selectedRows.value.filter(job => canDeleteRecord(job))
  if (bulkDeleting.value || rows.length === 0) return
  if (!window.confirm(batchImageText('deleteSelectedConfirm'))) return
  bulkDeleting.value = true
  try {
    for (const row of rows) {
      const key = apiKeyForJob(row)
      if (!key) continue
      deletingBatchId.value = row.id
      await deleteBatchImageJobRecord(key.key, row.id)
      removeJobFromList(row.id)
    }
    appStore.showSuccess(batchImageText('deleted'))
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('deleteFailed')))
  } finally {
    bulkDeleting.value = false
    deletingBatchId.value = ''
  }
}

function markJobDownloaded(batchId: string) {
  const downloadedAt = Math.floor(Date.now() / 1000)
  batchJobs.value = batchJobs.value.map(job => job.id === batchId ? { ...job, downloaded_at: job.downloaded_at || downloadedAt } : job)
  if (currentJob.value?.id === batchId && !currentJob.value.downloaded_at) {
    currentJob.value = { ...currentJob.value, downloaded_at: downloadedAt }
  }
}

function removeJobFromList(batchId: string) {
  batchJobs.value = batchJobs.value.filter(job => job.id !== batchId)
  toggleJobSelection(batchId, false)
  if (currentJob.value?.id === batchId) closeDetail()
}

function canLoadItemPreview(item: BatchImageItem) {
  return (item.status === 'succeeded' || item.status === 'success') && item.image_count > 0
}

function isSuccessfulImageItem(item: Pick<BatchImageItem, 'status' | 'image_count'>) {
  return (item.status === 'succeeded' || item.status === 'success') && item.image_count > 0
}

function detailRootBatchId() {
  return currentJob.value?.parent_batch_id || selectedBatchId.value || currentJob.value?.id || ''
}

function isChildDetailItem(item: Pick<BatchImageDetailItem, 'batch_id'>) {
  const rootBatchId = detailRootBatchId()
  return Boolean(rootBatchId && item.batch_id && item.batch_id !== rootBatchId)
}

function retrySourceCustomID(customID: string) {
  return String(customID || '').replace(/(?:_retry_[a-z0-9]+)+$/i, '')
}

function isRecoveredOriginalFailure(item: BatchImageDetailItem) {
  const rootBatchId = detailRootBatchId()
  return Boolean(
    rootBatchId
    && item.batch_id === rootBatchId
    && item.status === 'failed'
    && recoveredOriginalCustomIds.value.has(item.custom_id),
  )
}

function detailItemRowClass(item: BatchImageDetailItem) {
  if (isRecoveredOriginalFailure(item)) {
    return 'bg-gray-50/80 text-gray-400 hover:bg-gray-100/80 dark:bg-dark-900/60 dark:text-gray-500 dark:hover:bg-dark-800/70'
  }
  return 'hover:bg-gray-50/70 dark:hover:bg-dark-800/60'
}

function previewCacheSupported() {
  return typeof window !== 'undefined' && 'indexedDB' in window
}

function previewCacheKey(batchId: string, customID: string, imageIndex = 0) {
  return [batchId, customID, imageIndex].map(part => encodeURIComponent(String(part))).join(':')
}

function itemPreviewKey(item: Pick<BatchImageItem, 'batch_id' | 'custom_id'>) {
  return previewCacheKey(item.batch_id || selectedBatchId.value || currentJob.value?.id || '', item.custom_id, 0)
}

function idbRequest<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function openPreviewCacheDB(): Promise<IDBDatabase | null> {
  if (!previewCacheSupported()) return Promise.resolve(null)
  if (previewCacheDBPromise) return previewCacheDBPromise

  previewCacheDBPromise = new Promise((resolve) => {
    const request = window.indexedDB.open(PREVIEW_CACHE_DB_NAME, 1)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(PREVIEW_CACHE_STORE_NAME)) {
        const store = db.createObjectStore(PREVIEW_CACHE_STORE_NAME, { keyPath: 'key' })
        store.createIndex('lastAccessedAt', 'lastAccessedAt', { unique: false })
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => resolve(null)
    request.onblocked = () => resolve(null)
  })
  return previewCacheDBPromise
}

async function getCachedPreviewBlob(cacheKey: string): Promise<Blob | null> {
  const db = await openPreviewCacheDB()
  if (!db) return null
  const record = await idbRequest<PreviewCacheRecord | undefined>(
    db.transaction(PREVIEW_CACHE_STORE_NAME, 'readonly').objectStore(PREVIEW_CACHE_STORE_NAME).get(cacheKey),
  ).catch(() => undefined)
  if (!record?.blob) return null

  const now = Date.now()
  if (now - record.createdAt > PREVIEW_CACHE_MAX_AGE_MS) {
    void deleteCachedPreview(cacheKey)
    return null
  }
  void touchCachedPreview(cacheKey, now)
  return record.blob
}

async function hydrateCachedItemPreviews(detailItems: BatchImageDetailItem[]) {
  const previewableItems = detailItems.filter(item => canLoadItemPreview(item))
  if (!previewableItems.length || !previewCacheSupported()) return

  await Promise.all(previewableItems.map(async (item) => {
    const batchId = item.batch_id || selectedBatchId.value || currentJob.value?.id || ''
    const previewKey = itemPreviewKey(item)
    if (!batchId || itemPreviewUrls[previewKey] || previewErrorIds.value.has(previewKey)) return
    const cached = await getCachedPreviewBlob(previewCacheKey(batchId, item.custom_id, 0)).catch(() => null)
    if (!cached || itemPreviewUrls[previewKey]) return
    itemPreviewUrls[previewKey] = URL.createObjectURL(cached)
  }))
}

async function putCachedPreviewBlob(cacheKey: string, blob: Blob) {
  const db = await openPreviewCacheDB()
  if (!db) return
  const now = Date.now()
  const record: PreviewCacheRecord = {
    key: cacheKey,
    blob,
    size: blob.size,
    createdAt: now,
    lastAccessedAt: now,
  }
  await idbRequest(db.transaction(PREVIEW_CACHE_STORE_NAME, 'readwrite').objectStore(PREVIEW_CACHE_STORE_NAME).put(record)).catch(() => null)
  void cleanupPreviewCache()
}

async function touchCachedPreview(cacheKey: string, lastAccessedAt: number) {
  const db = await openPreviewCacheDB()
  if (!db) return
  const record = await idbRequest<PreviewCacheRecord | undefined>(
    db.transaction(PREVIEW_CACHE_STORE_NAME, 'readonly').objectStore(PREVIEW_CACHE_STORE_NAME).get(cacheKey),
  ).catch(() => undefined)
  if (!record) return
  record.lastAccessedAt = lastAccessedAt
  await idbRequest(db.transaction(PREVIEW_CACHE_STORE_NAME, 'readwrite').objectStore(PREVIEW_CACHE_STORE_NAME).put(record)).catch(() => null)
}

async function deleteCachedPreview(cacheKey: string) {
  const db = await openPreviewCacheDB()
  if (!db) return
  await idbRequest(db.transaction(PREVIEW_CACHE_STORE_NAME, 'readwrite').objectStore(PREVIEW_CACHE_STORE_NAME).delete(cacheKey)).catch(() => null)
}

async function cleanupPreviewCache() {
  const db = await openPreviewCacheDB()
  if (!db) return
  const records = await idbRequest<PreviewCacheRecord[]>(
    db.transaction(PREVIEW_CACHE_STORE_NAME, 'readonly').objectStore(PREVIEW_CACHE_STORE_NAME).getAll(),
  ).catch(() => [])
  if (!records.length) return

  const now = Date.now()
  const sorted = [...records].sort((a, b) => a.lastAccessedAt - b.lastAccessedAt)
  const deleteKeys = new Set<string>()
  let totalBytes = 0
  let keptCount = 0

  for (const record of sorted) {
    if (now - record.createdAt > PREVIEW_CACHE_MAX_AGE_MS) {
      deleteKeys.add(record.key)
      continue
    }
    totalBytes += record.size || record.blob?.size || 0
    keptCount += 1
  }

  for (const record of sorted) {
    if (deleteKeys.has(record.key)) continue
    if (keptCount <= PREVIEW_CACHE_MAX_ENTRIES && totalBytes <= PREVIEW_CACHE_MAX_BYTES) break
    deleteKeys.add(record.key)
    totalBytes -= record.size || record.blob?.size || 0
    keptCount -= 1
  }

  if (!deleteKeys.size) return
  const store = db.transaction(PREVIEW_CACHE_STORE_NAME, 'readwrite').objectStore(PREVIEW_CACHE_STORE_NAME)
  for (const key of deleteKeys) {
    store.delete(key)
  }
}

async function createThumbnailBlob(blob: Blob): Promise<Blob> {
  const source = await loadPreviewImageSource(blob)
  const width = source.width
  const height = source.height
  const scale = Math.min(1, PREVIEW_THUMBNAIL_MAX_EDGE / Math.max(width, height))
  const targetWidth = Math.max(1, Math.round(width * scale))
  const targetHeight = Math.max(1, Math.round(height * scale))
  const canvas = document.createElement('canvas')
  canvas.width = targetWidth
  canvas.height = targetHeight
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas unavailable')
  ctx.drawImage(source.image, 0, 0, targetWidth, targetHeight)
  source.close()
  return await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((thumbnail) => {
      if (thumbnail) resolve(thumbnail)
      else reject(new Error('thumbnail unavailable'))
    }, 'image/webp', PREVIEW_THUMBNAIL_QUALITY)
  })
}

async function loadPreviewImageSource(blob: Blob): Promise<{ image: PreviewImageSource, width: number, height: number, close: () => void }> {
  if ('createImageBitmap' in window) {
    const bitmap = await window.createImageBitmap(blob)
    return {
      image: bitmap,
      width: bitmap.width,
      height: bitmap.height,
      close: () => bitmap.close(),
    }
  }

  const url = URL.createObjectURL(blob)
  try {
    const image = await new Promise<HTMLImageElement>((resolve, reject) => {
      const img = new Image()
      img.onload = () => resolve(img)
      img.onerror = () => reject(new Error('image unavailable'))
      img.src = url
    })
    return {
      image,
      width: image.naturalWidth || image.width,
      height: image.naturalHeight || image.height,
      close: () => URL.revokeObjectURL(url),
    }
  } catch (error) {
    URL.revokeObjectURL(url)
    throw error
  }
}

async function loadItems() {
  const batchId = selectedBatchId.value || currentJob.value?.id || ''
  if (!batchId) return
  const key = keyForSelectedBatch() || requireApiKey()
  if (!key) return
  loadingItems.value = true
  try {
    clearItemPreviews()
    const jobs = detailJobsForBatch(batchId)
    const results = await Promise.all(jobs.map(async (job) => {
      const result = await listBatchImageItems(key.key, job.id)
      return (result.data || []).map(item => ({
        ...item,
        batch_id: job.id,
        source_task_name: detailSourceName(job, batchId),
      }))
    }))
    const detailItems = results.flat().sort((left, right) => {
      const leftID = retrySourceCustomID(left.custom_id)
      const rightID = retrySourceCustomID(right.custom_id)
      const idOrder = leftID.localeCompare(rightID, undefined, { numeric: true })
      if (idOrder !== 0) return idOrder
      return left.batch_id.localeCompare(right.batch_id)
    })
    items.value = detailItems
    void hydrateCachedItemPreviews(detailItems)
  } catch (error: any) {
    appStore.showError(batchImageErrorMessage(error, batchImageText('loadItemsFailed')))
  } finally {
    loadingItems.value = false
  }
}

function detailJobsForBatch(batchId: string): BatchImageJobRow[] {
  const task = visibleBatchJobs.value.find(job => job.collection_id === selectedCollectionId.value || job.id === batchId)
  if (task) return task.collection_jobs
  const row = batchJobs.value.find(job => job.id === batchId)
  const base = row || (currentJob.value && currentJob.value.id === batchId ? toJobRow(currentJob.value, keyForSelectedBatch() || selectedApiKey.value) : null)
  if (!base) return []
  if (base.parent_batch_id) return [base]
  return [base, ...(childrenByParent.value.get(base.id) || [])]
}

function detailSourceName(job: Pick<BatchImageJobRow, 'id' | 'task_name' | 'parent_batch_id'>, rootBatchId: string) {
  const name = job.task_name || job.id
  if (job.id === rootBatchId) return t('batchImage.detail.mainTask', { name })
  return t('batchImage.detail.childTask', { name })
}

async function loadItemPreview(item: BatchImageItem) {
  const batchId = item.batch_id || selectedBatchId.value || currentJob.value?.id || ''
  const previewKey = itemPreviewKey(item)
  if (!batchId || !canLoadItemPreview(item) || (itemPreviewUrls[previewKey] && !previewErrorIds.value.has(previewKey))) return
  const key = keyForSelectedBatch() || requireApiKey()
  if (!key) return
  const cacheKey = previewCacheKey(batchId, item.custom_id, 0)
  previewLoadingIds.value = new Set([...previewLoadingIds.value, previewKey])
  try {
    previewErrorIds.value = new Set([...previewErrorIds.value].filter(id => id !== previewKey))
    if (itemPreviewUrls[previewKey]) {
      URL.revokeObjectURL(itemPreviewUrls[previewKey])
      delete itemPreviewUrls[previewKey]
    }
    const cached = await getCachedPreviewBlob(cacheKey)
    if (cached) {
      itemPreviewUrls[previewKey] = URL.createObjectURL(cached)
      return
    }
    const blob = await getBatchImageItemContent(key.key, batchId, item.custom_id, 0)
    const thumbnail = await createThumbnailBlob(blob).catch(() => blob)
    itemPreviewUrls[previewKey] = URL.createObjectURL(thumbnail)
    if (thumbnail !== blob || thumbnail.size <= 1024 * 1024) {
      void putCachedPreviewBlob(cacheKey, thumbnail)
    }
  } catch (error: any) {
    previewErrorIds.value = new Set([...previewErrorIds.value, previewKey])
    appStore.showError(batchImageErrorMessage(error, batchImageText('loadPreviewFailed')))
  } finally {
    const next = new Set(previewLoadingIds.value)
    next.delete(previewKey)
    previewLoadingIds.value = next
  }
}

function openImagePreview(item: BatchImageItem) {
  const previewKey = itemPreviewKey(item)
  if (!itemPreviewUrls[previewKey] || previewErrorIds.value.has(previewKey)) return
  previewImageItem.value = item
}

function closeImagePreview() {
  previewImageItem.value = null
}

function handlePreviewError(customID: string) {
  if (itemPreviewUrls[customID]) {
    URL.revokeObjectURL(itemPreviewUrls[customID])
    delete itemPreviewUrls[customID]
  }
  previewErrorIds.value = new Set([...previewErrorIds.value, customID])
}

function clearItemPreviews() {
  closePromptPopover()
  for (const url of Object.values(itemPreviewUrls)) {
    if (url) URL.revokeObjectURL(url)
  }
  for (const key of Object.keys(itemPreviewUrls)) {
    delete itemPreviewUrls[key]
  }
  previewLoadingIds.value = new Set()
  previewErrorIds.value = new Set()
  previewImageItem.value = null
}

function copyInstruction() {
  void copyToClipboard(agentInstruction.value, batchImageText('copiedInstruction'))
}

function statusLabel(jobOrStatus: BatchImageStatus | Pick<BatchImageJob, 'status' | 'success_count' | 'fail_count'>) {
  const status = typeof jobOrStatus === 'string' ? jobOrStatus : jobOrStatus.status
  if (typeof jobOrStatus !== 'string' && status === 'completed' && jobOrStatus.fail_count > 0) {
    if (jobOrStatus.success_count > 0) return t('batchImage.status.partialSuccess')
    return t('batchImage.status.allFailed')
  }
  const statusKeys: Record<string, string> = {
    queued: 'queued',
    running: 'running',
    indexing: 'processingResults',
    processing_results: 'processingResults',
    settling: 'settling',
    partial_success: 'partialSuccess',
    completed: 'completed',
    failed: 'failed',
    cancelled: 'cancelled',
    output_deleted: 'outputDeleted',
  }
  const key = statusKeys[status]
  return key ? t(`batchImage.status.${key}`) : status
}

function statusBadgeClass(jobOrStatus: BatchImageStatus | Pick<BatchImageJob, 'status' | 'success_count' | 'fail_count'>) {
  const status = typeof jobOrStatus === 'string' ? jobOrStatus : jobOrStatus.status
  if (typeof jobOrStatus !== 'string' && status === 'completed' && jobOrStatus.fail_count > 0) {
    if (jobOrStatus.success_count > 0) return 'badge-warning'
    return 'badge-danger'
  }
  if (status === 'completed') return 'badge-success'
  if (status === 'partial_success') return 'badge-warning'
  if (status === 'failed' || status === 'cancelled') return 'badge-danger'
  if (status === 'output_deleted') return 'badge-gray'
  return 'badge-primary'
}

function itemStatusLabel(status: string) {
  const statusKeys: Record<string, string> = {
    pending: 'pending',
    succeeded: 'succeeded',
    success: 'succeeded',
    failed: 'failed',
    cancelled: 'cancelled',
  }
  const key = statusKeys[status]
  return key ? t(`batchImage.itemStatus.${key}`) : status
}

function itemDisplayStatusLabel(item: BatchImageDetailItem) {
  if (isRecoveredOriginalFailure(item)) return t('batchImage.itemStatus.recovered')
  return itemStatusLabel(item.status)
}

function itemStatusBadgeClass(status: string) {
  if (status === 'succeeded' || status === 'success') return 'badge-success'
  if (status === 'failed' || status === 'cancelled') return 'badge-danger'
  return 'badge-primary'
}

function itemDisplayStatusBadgeClass(item: BatchImageDetailItem) {
  if (isRecoveredOriginalFailure(item)) return 'badge-gray'
  return itemStatusBadgeClass(item.status)
}

function itemResultLabel(item: BatchImageDetailItem) {
  if (isRecoveredOriginalFailure(item)) return t('batchImage.itemResult.recoveredByRetry')
  if (item.error) return friendlyItemError(item.error)
  if (item.status === 'succeeded' || item.status === 'success') {
    return itemPreviewUrls[itemPreviewKey(item)] ? t('batchImage.itemResult.readyPreview') : t('batchImage.itemResult.readyDownload')
  }
  if (item.status === 'failed') return t('batchImage.itemResult.noUsableImage')
  if (item.status === 'cancelled') return t('batchImage.itemResult.cancelled')
  return t('batchImage.itemResult.waiting')
}

function itemFullResult(item: BatchImageDetailItem) {
  return [itemResultLabel(item), item.error?.code, item.error?.message]
    .filter((value, index, values) => value && values.indexOf(value) === index)
    .join('\n')
}

function itemResultClass(item: BatchImageDetailItem) {
  if (isRecoveredOriginalFailure(item)) return 'bg-gray-100 text-gray-500 ring-gray-200 dark:bg-dark-800 dark:text-gray-400 dark:ring-dark-700'
  if (item.error || item.status === 'failed' || item.status === 'cancelled') return 'bg-red-50 text-red-700 ring-red-100 dark:bg-red-950/30 dark:text-red-300 dark:ring-red-900/50'
  if (item.status === 'succeeded' || item.status === 'success') return 'bg-emerald-50 text-emerald-700 ring-emerald-100 dark:bg-emerald-950/30 dark:text-emerald-300 dark:ring-emerald-900/50'
  return 'bg-gray-50 text-gray-500 ring-gray-200 dark:bg-dark-800 dark:text-gray-400 dark:ring-dark-700'
}

function friendlyItemError(error: BatchImageItem['error']) {
  if (!error) return '-'
  if (error.code === 'EMPTY_IMAGE_OUTPUT') return t('batchImage.itemResult.emptyImageOutput')
  if (error.code === 'PROVIDER_ITEM_FAILED') return t('batchImage.itemResult.providerItemFailed')
  return error.message || error.code || '-'
}

function formatMoney(value: number | null | undefined) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '$0.00'
  return `$${Number(value).toFixed(2)}`
}

function terminalZeroCost(job: Pick<BatchImageJob, 'status' | 'actual_cost'>) {
  return job.actual_cost === null && (job.status === 'failed' || job.status === 'cancelled')
}

function costLabel(job: Pick<BatchImageJob, 'status' | 'hold_amount' | 'actual_cost'>) {
  if (job.actual_cost !== null) return formatMoney(job.actual_cost)
  if (terminalZeroCost(job)) return formatMoney(0)
  return t('batchImage.detail.holdCost', { amount: formatMoney(job.hold_amount) })
}

type BatchImageTextKey =
  | 'loadKeysFailed'
  | 'loadModelsFailed'
  | 'loadJobsFailed'
  | 'selectApiKey'
  | 'noModelsForKey'
  | 'selectModel'
  | 'promptRequired'
  | 'submitted'
  | 'submitFailed'
  | 'refreshFailed'
  | 'cancelConfirm'
  | 'cancelled'
  | 'cancelFailed'
  | 'batchDownloadStarted'
	  | 'downloadFailed'
	  | 'retrySubmitted'
	  | 'retryFailed'
	  | 'retryMissingPrompts'
  | 'deleteConfirm'
  | 'deleteSelectedConfirm'
  | 'deleted'
  | 'deleteFailed'
	  | 'loadItemsFailed'
	  | 'loadPreviewFailed'
  | 'copiedInstruction'
  | 'loadingModels'
  | 'noModels'
  | 'noModelsHint'
  | 'noCompatibleAccount'
  | 'unsupportedProvider'
  | 'providerSubmitFailed'
  | 'vertexGcsBucketMissing'
  | 'queueFailed'
  | 'billingHoldFailed'
  | 'groupDisabled'
  | 'pricingMissing'
  | 'insufficientBalance'
  | 'invalidModel'
  | 'invalidItems'
  | 'duplicateCustomId'
  | 'promptTooLong'
  | 'invalidReferenceImage'
  | 'tooManyReferenceImages'
  | 'referenceImagesTooLarge'
  | 'tooManyOutputImages'
  | 'idempotencyConflict'
  | 'notReady'
  | 'outputDeleted'
  | 'resultMissing'
  | 'itemFailed'
  | 'itemImageIndexOutOfRange'
  | 'downloadLimited'
  | 'downloadTooLarge'
  | 'deleteNotReady'
  | 'disabled'
  | 'authRequired'
  | 'adminReference'
  | 'errorReference'

function isZhLocale() {
  return String(locale.value || '').toLowerCase().startsWith('zh')
}

function batchImageText(key: BatchImageTextKey) {
  return t(`batchImage.messages.${key}`)
}

function batchImageErrorReference(error: any) {
  const parts: string[] = []
  const code = String(error?.code || '').trim()
  const requestId = String(error?.requestId || '').trim()
  const status = String(error?.status || '').trim()
  if (code) parts.push(t('batchImage.messages.errorCodeRef', { code }))
  if (requestId) parts.push(t('batchImage.messages.requestIdRef', { id: requestId }))
  if (!code && status) parts.push(t('batchImage.messages.httpStatusRef', { status }))
  return parts.length ? `（${parts.join(isZhLocale() ? '，' : ', ')}）` : ''
}

function batchImageAdminError(base: string, error: any) {
  const reference = batchImageErrorReference(error)
  return `${base}${reference ? ` ${reference}` : ''} ${batchImageText('adminReference')}`
}

function batchImagePlainError(base: string) {
  return base
}

function batchImageErrorMessage(error: any, fallback: string) {
  const code = String(error?.code || '').trim()
  const message = String(error?.message || '').trim()
  if (code === 'API_KEY_REQUIRED' || code === '401') {
    return batchImagePlainError(batchImageText('authRequired'))
  }
  if (code === 'BATCH_IMAGE_NO_ACCOUNT_AVAILABLE' || /no compatible batch image account/i.test(message)) {
    return batchImageAdminError(batchImageText('noCompatibleAccount'), error)
  }
  if (code === 'BATCH_IMAGE_UNSUPPORTED_PROVIDER' || /unsupported batch image provider/i.test(message)) {
    return batchImageAdminError(batchImageText('unsupportedProvider'), error)
  }
  if (code === 'BATCH_IMAGE_VERTEX_GCS_BUCKET_MISSING' || code === 'VERTEX_MANAGED_GCS_BUCKET_MISSING') {
    return batchImageAdminError(batchImageText('vertexGcsBucketMissing'), error)
  }
  if (
    code === 'BATCH_IMAGE_PROVIDER_SUBMIT_FAILED' ||
    code === 'BATCH_IMAGE_PROVIDER_MISSING_API_KEY' ||
    code === 'BATCH_IMAGE_PROVIDER_MISSING_SERVICE_ACCOUNT' ||
    code === 'BATCH_IMAGE_PROVIDER_UNSUPPORTED_ACCOUNT'
  ) {
    return batchImageAdminError(batchImageText('providerSubmitFailed'), error)
  }
  if (code === 'BATCH_IMAGE_QUEUE_FAILED' || code === 'BATCH_IMAGE_QUEUE_NOT_CONFIGURED') {
    return batchImageAdminError(batchImageText('queueFailed'), error)
  }
  if (code === 'BATCH_IMAGE_BILLING_HOLD_FAILED') {
    return batchImageAdminError(batchImageText('billingHoldFailed'), error)
  }
  if (code === 'BATCH_IMAGE_GROUP_DISABLED') {
    return batchImagePlainError(batchImageText('groupDisabled'))
  }
  if (code === 'BATCH_IMAGE_SETTLEMENT_PRICING_MISSING') {
    return batchImageAdminError(batchImageText('pricingMissing'), error)
  }
  if (code === 'BATCH_IMAGE_INSUFFICIENT_BALANCE') {
    return batchImagePlainError(batchImageText('insufficientBalance'))
  }
  if (code === 'BATCH_IMAGE_INVALID_MODEL') {
    return batchImageText('invalidModel')
  }
  if (code === 'BATCH_IMAGE_INVALID_ITEMS') {
    return batchImageText('invalidItems')
  }
  if (code === 'BATCH_IMAGE_DUPLICATE_CUSTOM_ID') {
    return batchImageText('duplicateCustomId')
  }
  if (code === 'BATCH_IMAGE_PROMPT_TOO_LONG') {
    return batchImageText('promptTooLong')
  }
  if (code === 'BATCH_IMAGE_INVALID_REFERENCE_IMAGE') {
    return batchImageText('invalidReferenceImage')
  }
  if (code === 'BATCH_IMAGE_TOO_MANY_REFERENCE_IMAGES') {
    return batchImageText('tooManyReferenceImages')
  }
  if (code === 'BATCH_IMAGE_REFERENCE_IMAGES_TOO_LARGE') {
    return batchImageText('referenceImagesTooLarge')
  }
  if (code === 'BATCH_IMAGE_TOO_MANY_OUTPUT_IMAGES') {
    return batchImageText('tooManyOutputImages')
  }
  if (code === 'BATCH_IMAGE_IDEMPOTENCY_CONFLICT') {
    return batchImagePlainError(batchImageText('idempotencyConflict'))
  }
  if (code === 'BATCH_IMAGE_NOT_READY') {
    return batchImageText('notReady')
  }
  if (code === 'BATCH_IMAGE_OUTPUT_DELETED') {
    return batchImageText('outputDeleted')
  }
  if (code === 'BATCH_IMAGE_RESULT_MISSING') {
    return batchImageAdminError(batchImageText('resultMissing'), error)
  }
  if (code === 'BATCH_IMAGE_ITEM_FAILED') {
    return batchImagePlainError(batchImageText('itemFailed'))
  }
  if (code === 'BATCH_IMAGE_ITEM_IMAGE_INDEX_OUT_OF_RANGE') {
    return batchImagePlainError(batchImageText('itemImageIndexOutOfRange'))
  }
  if (code === 'BATCH_IMAGE_DOWNLOAD_LIMITED') {
    return batchImageText('downloadLimited')
  }
  if (code === 'BATCH_IMAGE_DOWNLOAD_TOO_LARGE') {
    return batchImageText('downloadTooLarge')
  }
  if (code === 'BATCH_IMAGE_RECORD_DELETE_NOT_READY') {
    return batchImagePlainError(batchImageText('deleteNotReady'))
  }
  if (code === 'BATCH_IMAGE_DISABLED') {
    return batchImageAdminError(batchImageText('disabled'), error)
  }
  if (code === 'INTERNAL_ERROR' || code === '500') {
    return batchImageAdminError(fallback, error)
  }
  if (isZhLocale()) {
    const detail = message ? `${batchImageText('errorReference')}：${message}` : batchImageText('adminReference')
    return `${fallback}。${detail} ${batchImageErrorReference(error)}`
  }
  return message || fallback
}

function formatDate(timestamp: number) {
  if (!timestamp) return ''
  return new Date(timestamp * 1000).toLocaleString()
}

function defaultTaskName(timestamp?: number) {
  const date = timestamp ? new Date(timestamp * 1000) : new Date()
  return date.toLocaleString()
}

onMounted(() => {
  restoreConfigSubmissionState()
  if (configSubmissions.value.some(entry => !TERMINAL_STATUSES.has(entry.job.status))) startPolling()
  void appStore.fetchPublicSettings()
  void refreshPage()
  void cleanupPreviewCache()
  previewCacheCleanupTimer = setInterval(() => {
    void cleanupPreviewCache()
  }, 60 * 60 * 1000)
  document.addEventListener('click', closeMoreMenu)
  window.addEventListener('resize', closeMoreMenu)
  window.addEventListener('scroll', closeMoreMenu, true)
  window.addEventListener('resize', closePromptPopover)
  window.addEventListener('scroll', closePromptPopover, true)
})

watch(
  () => form.apiKeyId,
  async (nextID, previousID) => {
    if (revertingApiKey) return
    if (restoringConfig) {
      void loadAvailableModels()
      return
    }
    const nextPlatform = batchImageApiKeys.value.find(key => key.id === nextID)?.group?.platform
    const previousPlatform = batchImageApiKeys.value.find(key => key.id === previousID)?.group?.platform
    const hasDraft = configCards.value.length > 0 || !!configInput.value.trim() || configReferenceImages.value.length > 0
    if (previousID && nextPlatform && previousPlatform && nextPlatform !== previousPlatform && hasDraft) {
      if (!window.confirm(t('batchImage.config.switchProviderConfirm'))) {
        revertingApiKey = true
        await nextTick()
        form.apiKeyId = previousID
        await nextTick()
        revertingApiKey = false
        return
      }
      clearProviderDrafts()
    }
    void loadAvailableModels()
  },
)

onBeforeUnmount(() => {
  clearDownloadArtifact()
  stopPolling()
  if (previewCacheCleanupTimer) {
    clearInterval(previewCacheCleanupTimer)
    previewCacheCleanupTimer = null
  }
  clearItemPreviews()
  document.removeEventListener('click', closeMoreMenu)
  window.removeEventListener('resize', closeMoreMenu)
  window.removeEventListener('scroll', closeMoreMenu, true)
  window.removeEventListener('resize', closePromptPopover)
  window.removeEventListener('scroll', closePromptPopover, true)
})
</script>

<style scoped>
.batch-row-action {
  display: flex !important;
  flex-direction: column !important;
  align-items: center !important;
  justify-content: center !important;
  min-width: 42px;
  line-height: 1;
  outline: none;
}

.batch-row-action:focus {
  outline: none;
}

.batch-row-action :deep(svg) {
  margin-right: 0 !important;
}

.batch-prompt-trigger:focus {
  outline: none;
  box-shadow: none;
}

.batch-prompt-popover {
  user-select: text;
}

.batch-prompt-popover p {
  scrollbar-width: thin;
}

.batch-output-count-select {
  height: 36px;
  min-height: 36px;
  padding-top: 0;
  padding-bottom: 0;
  padding-left: 14px;
  padding-right: 34px;
  line-height: 36px;
}
</style>
