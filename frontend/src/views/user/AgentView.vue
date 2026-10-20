<template>
  <AppLayout>
    <section class="agent-workspace" :class="{ 'artifact-active': !!activeArtifact }" aria-label="Agent 对话工作区">
      <header class="agent-toolbar">
        <div class="toolbar-title"><button class="icon-button history-toggle" type="button" :aria-expanded="historyOpen" aria-controls="agent-history" @click="historyOpen = !historyOpen"><Icon name="menu" size="sm" /><span>历史对话</span></button><h2>{{ selected?.title || '新对话' }}</h2></div>
        <div class="toolbar-actions"><button type="button" class="quiet-button toolbar-new-chat" :disabled="sending || savingSettings" @click="newConversation"><span aria-hidden="true">＋</span> 新对话</button><button ref="settingsTrigger" type="button" class="quiet-button" aria-haspopup="dialog" :disabled="sending || opening || savingSettings" @click="openSettings"><Icon name="cog" size="sm" />设置</button><button type="button" class="workspace-entry icon-button" :class="{ active: !!activeArtifact }" :aria-label="activeArtifact?.kind === 'workspace' ? '关闭工作区' : '打开工作区'" :title="activeArtifact?.kind === 'workspace' ? '关闭工作区' : '工作区'" :aria-expanded="!!activeArtifact" aria-controls="agent-workspace-panel" @click="toggleWorkspace"><Icon name="panelRight" size="sm" /></button></div>
      </header>
      <p v-if="error" role="alert" class="agent-notice agent-error">{{ error }}<button type="button" aria-label="关闭错误提示" @click="error = ''">×</button></p>
      <p v-if="info && !info.enabled" class="agent-notice" role="status">Agent 尚未启用。管理员完成后台与存储配置后即可使用。</p>
      <p v-if="backgroundRun" class="agent-notice background-task" role="status"><span>{{ agentState(backgroundRun.status).label }}，当前任务仍在后台处理，结束后可发送新请求。</span><button type="button" :disabled="sending || savingSettings" @click="openConversation(backgroundRun.conversation_id)">查看任务 →</button></p>
      <p v-if="statusDisconnected" class="agent-notice" role="status">任务状态连接暂时中断，正在自动同步；仍可切换或新建对话。<button type="button" class="sync-task" @click="synchronizeActiveRun?.()">立即同步</button></p>
      <p v-if="preferenceNotice" class="agent-notice" role="status">{{ preferenceNotice }}</p>
      <div class="agent-grid" :class="{ 'artifact-open': !!activeArtifact, 'plan-open': !!activeArtifact && activeArtifact.kind !== 'asset' }">
        <aside id="agent-history" class="agent-conversations" :class="{ 'history-open': historyOpen }" aria-label="对话列表">
          <AgentHistory :key="authStore.user?.id || 'anonymous'" :items="conversations" :selected="selected?.id" :loading="loading" :disabled="sending || savingSettings" @open="openConversation" @changed="onHistoryChanged" @new="newConversation" />
        </aside>
        <div class="agent-canvas" :class="{ 'file-dragging': fileDragDepth > 0 }" @paste="onComposerPaste" @dragenter="onFileDragEnter" @dragover="onFileDragOver" @dragleave="onFileDragLeave" @drop="onFileDrop" :inert="(activeArtifact?.kind === 'asset' && artifactOverlay) || undefined" :aria-hidden="activeArtifact?.kind === 'asset' && artifactOverlay ? true : undefined">
          <div v-if="fileDragDepth > 0" class="file-drop-overlay" role="status"><Icon name="document" size="lg" /><strong>松开以添加文件</strong><span>图片、PDF、文本或 Office 文件 · 单个不超过 10 MiB</span></div>
          <div ref="scrollArea" class="agent-scroll" :aria-busy="loading || opening" @scroll="onScroll">
            <div v-if="loading || opening" class="conversation-loading" role="status"><span class="status-dot working" />正在加载对话…</div>
            <div v-else-if="!messages.length && !run && !workPlans.length" class="agent-intro">
              <div class="intro-symbol" aria-hidden="true"><Icon name="sparkles" size="lg" /></div>
              <h2>今天想一起做点什么？</h2><p>聊想法、理清问题，或把灵感变成图片。</p>
              <div class="agent-suggestions"><button v-for="example in examples" :key="example.title" type="button" @click="chooseExample(example.prompt)"><Icon :name="example.icon" size="md" /><span><strong>{{ example.title }}</strong><small>{{ example.description }}</small></span><span aria-hidden="true">↗</span></button></div>
              <button v-if="!configured && info?.enabled" type="button" class="setup-link" @click="openSettings">先选择对话模型 <span aria-hidden="true">→</span></button>
            </div>
            <div v-else class="conversation-content">
              <AgentChat :messages="displayMessages" :retry-run-id="run && terminal(run) ? run.id : undefined" :retry-disabled="busy || !!backgroundRun || sending || opening || savingSettings || !configured" :retry-error="isPausedRun ? '' : retryError" :hide-latest-retry="isPausedRun || !!run?.work" @retry="retryReply" :recover-disabled="busy || !!backgroundRun || sending || opening || savingSettings" @recover="recoverImage">
                <template #assets="{ message }">
                  <div v-if="message.asset_ids?.length" class="agent-attachments">
                    <AgentImageGallery @request="requestVisibleAsset" v-if="messageImages(message).length > 1" :assets="messageImages(message)" @preview="previewId = $event" @loaded="onAssetLoaded" @image-error="assetImageError" @retry="retryAsset" />
                    <AgentAttachmentCard @request="requestVisibleAsset" @download="downloadAsset(id)" v-for="id in messageAttachmentIds(message)" :key="id" :asset="assetById(id)" :can-reference="!!config.image_api_key_id" :disabled="busy || sending" @preview="message.role === 'user' || message.kind === 'question_answer' ? openUploadedPreview(id) : previewId = id" @loaded="onAssetLoaded" @image-error="assetImageError(id)" @retry="retryAsset(id)" @reference="useReference(id)" />
                  </div>
                </template>
                <template #work="{ message }"><AgentWorkPlan @request="requestVisibleAsset" @download="downloadAsset" v-for="plan in workPlans.filter(plan => message.role === 'assistant' && plan.source_run_id === message.run_id)" :key="plan.id" :plan="plan" :expanded="activeArtifact?.kind === 'plan' && activeArtifact.id === plan.id" :panel-target="artifactPlanHost" :upload-assets="files => uploadPlanAssets(plan, files)" @open="openPlanArtifact(plan.id)" @asset-error="assetImageError" @asset-retry="retryAsset" @draft-change="updateLocalDraft(plan, $event)" @upload-state="updatePlanUpload(plan.id, $event)" @reference-preview="openUploadedPreview" :grants="grants.filter(grant => grant.plan_id === plan.id)" :image-model="config.image_model" :config-version="selected?.config_version" :assets="assetViews" :results="run?.work?.plan.id === plan.id ? run.work.results : plan.results" :disabled="!canStartRun" :pending="workPending === plan.id" :error="workPending === plan.id || workError.startsWith(plan.id + ':') ? workError.split(':').slice(1).join(':') : ''" @save="saveWorkDraft(plan, $event)" @revoke="revokeWorkGrant($event)" @approve="approveWork(plan, $event)" @approve-item="(draft, itemId) => approveWork(plan, draft, [itemId])" @preview="previewId = $event" @settings="openSettings" /></template>
              </AgentChat>
              <div class="workspace-orphan-plans" hidden><AgentWorkPlan @request="requestVisibleAsset" @download="downloadAsset" v-for="plan in orphanWorkPlans" :key="plan.id" :plan="plan" :expanded="activeArtifact?.kind === 'plan' && activeArtifact.id === plan.id" :panel-target="artifactPlanHost" :upload-assets="files => uploadPlanAssets(plan, files)" @open="openPlanArtifact(plan.id)" @asset-error="assetImageError" @asset-retry="retryAsset" @draft-change="updateLocalDraft(plan, $event)" @upload-state="updatePlanUpload(plan.id, $event)" @reference-preview="openUploadedPreview" :grants="grants.filter(grant => grant.plan_id === plan.id)" :image-model="config.image_model" :config-version="selected?.config_version" :assets="assetViews" :results="run?.work?.plan.id === plan.id ? run.work.results : plan.results" :disabled="!canStartRun" :pending="workPending === plan.id" :error="workPending === plan.id || workError.startsWith(plan.id + ':') ? workError.split(':').slice(1).join(':') : ''" @save="saveWorkDraft(plan, $event)" @revoke="revokeWorkGrant($event)" @approve="approveWork(plan, $event)" @approve-item="(draft, itemId) => approveWork(plan, draft, [itemId])" @preview="previewId = $event" @settings="openSettings" /></div>
              <AgentQuestion v-if="run?.status === 'waiting_user' && run.question" :key="run.question.id" :question="run.question" :sending="answering || sending || !!run?.cancel_requested" :error="answerError" @answer="answerQuestion" @cancel="cancelRun" />
              <Transition :css="false" @enter="workspaceMotion.enter" @enter-cancelled="workspaceMotion.cancel"><div v-if="run && isPausedRun && !retryingSource" class="paused-region" :data-status="run.status">
                <div class="paused-toolbar" role="group" aria-label="暂停后的对话操作">
                  <span class="paused-state" role="status"><Icon name="pauseCircle" size="sm" aria-hidden="true" /><span>对话已暂停</span></span>
                  <button type="button" class="pause-continue" aria-label="继续对话" title="基于已有上下文继续处理" :disabled="!canStartRun" @click="continueRun"><Icon name="play" size="sm" aria-hidden="true" /><span>{{ continuing ? '继续中…' : '继续' }}</span></button>
                  <button v-if="!run?.work" type="button" class="pause-retry" aria-label="重试本轮回复" title="重新生成这一轮回复" :disabled="!canStartRun" @click="retryPausedRun"><Icon name="refresh" size="sm" aria-hidden="true" /><span>重试</span></button>
                </div>
                <p v-if="pausedError || retryError || run.error_code" class="paused-error" role="alert">{{ pausedError || retryError || agentError(run.error_code) }}</p>
              </div></Transition>
              <div v-if="run && showRunNotice" class="agent-run" :class="`tone-${state.tone}`" :data-status="run.status">
                <div class="run-summary" role="status" aria-live="polite"><span class="status-dot" :class="{ working: busy && run.status !== 'reconciling' }" /><strong>{{ run.cancel_requested && busy ? '正在停止…' : state.label }}</strong></div>
                <p v-if="state.detail && (busy || run.status !== 'completed')" class="run-description">{{ state.detail }}</p>
                <p v-if="run.status === 'reconciling' && run.reconcile_deadline" class="run-description">核对截止 {{ new Date(run.reconcile_deadline).toLocaleTimeString('zh-CN', { hour12: false }) }}，超时后自动结束。</p>
                <button v-if="run.status === 'reconciling'" type="button" class="sync-task" @click="synchronizeActiveRun?.()">同步任务状态</button>
                <p v-if="run.error_code" class="run-error" role="alert">{{ agentError(run.error_code) }}</p>

              </div>
            </div>
          </div>
          <button v-if="!atBottom && messages.length" type="button" class="jump-bottom" @click="scrollToBottom">回到最新消息 ↓</button>
          <div class="composer-region">
            <form class="agent-composer" @submit.prevent="send">
              <div v-if="referenceIds.length" class="draft-attachments" aria-label="待发送的附件"><AgentAttachmentCard @request="requestVisibleAsset" @download="downloadAsset(id)" v-for="id in referenceIds" :key="id" :asset="assetById(id)" compact removable :disabled="(busy && run?.status !== 'waiting_user') || sending" @preview="openUploadedPreview(id)" @remove="referenceIds = referenceIds.filter(value => value !== id)" @retry="retryAsset(id)" @image-error="assetImageError(id)" /></div>
              <label class="sr-only" for="agent-message">发送给 Agent 的消息</label>
              <textarea id="agent-message" ref="composer" v-model="prompt" placeholder="发送消息，或描述你想生成的图片…" :disabled="busy || sending || loading || opening || !info?.enabled" maxlength="16000" rows="2" @keydown="onComposerKeydown" />
              <div class="compose-actions"><label class="agent-upload" :class="{ disabled: attachmentUploadBlocked }" title="可多选图片、PDF、文本或 Office 文件，每条消息最多 8 个，单个不超过 10 MiB"><input type="file" multiple aria-label="添加文件" :accept="agentFileAccept" :disabled="attachmentUploadBlocked" @change="upload" /><Icon name="plus" size="sm" /><span>添加文件</span></label><AgentModelPicker :key="authStore.user?.id || 'anonymous'" :config="config" :text-models="pickerTextModels" :image-models="pickerImageModels" :capabilities="modelCapabilities" :capabilities-loading="capabilitiesLoading" :capabilities-error="capabilitiesError" @reload-capabilities="loadModelCapabilities" :disabled="loading || sending || opening" :busy="busy" :loading="savingSettings" @select="selectModel" @effort="changeReasoningEffort" @manage="openSettings" /><span class="compose-spacer" /><button v-if="busy && run?.status !== 'reconciling'" type="button" class="send-button stop-button" :disabled="run?.cancel_requested" aria-label="停止生成" @click="cancelRun"><span aria-hidden="true">■</span></button><button v-else type="submit" class="send-button" :disabled="!canSend" :aria-label="sending ? '正在发送' : '发送消息'"><span aria-hidden="true">{{ sending ? '…' : '↑' }}</span></button></div>
            </form>
            <div class="composer-hint"><span>{{ configured ? 'AI 可能出错，请核实重要信息。' : '在设置中选择对话 key 和模型后，即可开始。' }}</span><span class="keyboard-hint">Enter 发送 · Shift + Enter 换行</span></div>
          </div>
        </div>
        <AgentArtifactPanel id="agent-workspace-panel" :persistent="activeArtifact?.kind !== 'asset'" :show="!!activeArtifact" :title="activeArtifact?.kind === 'workspace' ? '工作区' : activeArtifact?.kind === 'plan' ? '方案配置' : activeAsset?.mime_type?.startsWith('image/') ? '图片预览' : '文件预览'" :subtitle="activeArtifact?.kind === 'plan' ? activeWorkPlan?.title : activeAsset?.filename" :can-back="!!returnArtifact" :back-label="returnArtifact?.kind === 'workspace' ? '返回工作区' : '返回配置卡'" @close="closeArtifact" @back="backToArtifact" @overlay="artifactOverlay = $event" @keydown="onPreviewKeydown">
          <template #actions><button v-if="run?.status === 'waiting_user'" type="button" class="artifact-tool" aria-label="返回对话回答问题" @click="closeArtifact">回答问题</button><button v-if="busy && run?.status !== 'reconciling'" type="button" class="artifact-tool" aria-label="停止当前任务" :disabled="run?.cancel_requested" @click="cancelRun">停止</button><template v-if="activeArtifact?.kind === 'asset' && activeAsset?.url"><button v-if="activeAsset.mime_type?.startsWith('image/')" type="button" class="artifact-tool" @click="naturalImageSize = !naturalImageSize">{{ naturalImageSize ? '适应宽度' : '原图尺寸' }}</button><a class="artifact-tool" :href="activeAsset.url" :download="activeAsset.filename || activeAsset.id">下载</a></template></template>
          <AgentWorkspaceResources @request="requestVisibleAsset" v-if="activeArtifact?.kind === 'workspace'" :plans="workPlans" :images="workspaceImages" @plan="openPlanArtifact" @image="previewId = $event" @image-error="assetImageError" />
          <div v-show="activeArtifact?.kind === 'plan'" ref="artifactPlanHost" class="artifact-plan-host" />
          <template v-if="activeArtifact?.kind === 'asset'"><div class="artifact-image-view" :class="{ 'has-image-navigation': previewImageIndex >= 0 && workspaceImages.length > 1 }"><AgentImageNavigation v-if="previewImageIndex >= 0" class="preview-navigation" :index="previewImageIndex" :total="workspaceImages.length" @select="selectPreviewImage" /><AgentImagePreview v-if="activeAsset?.url" embedded :natural-size="naturalImageSize" :url="activeAsset.url" :filename="activeAsset.filename" :mime="activeAsset.mime_type" :size="activeAsset.byte_size" :text="activeAsset.text" @image-error="originalImageError(activeAsset.id)" /><p v-else-if="activeAsset?.status === 'expired'" class="artifact-empty" role="status">文件内容已过期，请重新上传。历史记录仍然保留。</p><div v-else class="artifact-empty" role="status"><p>{{ activeAsset?.failed ? '文件暂时无法读取，请重新加载。' : '正在加载预览…' }}</p><button v-if="activeAsset?.failed" type="button" @click="retryAsset(activeArtifact.id)">重新加载</button></div></div></template>
        </AgentArtifactPanel>
      </div>

      <AgentImagePreview v-if="modalPreviewId && modalAsset?.url" :url="modalAsset.url" :filename="modalAsset.filename" :mime="modalAsset.mime_type" :size="modalAsset.byte_size" :text="modalAsset.text" @close="closeUploadedPreview" @image-error="originalImageError(modalPreviewId)" />
      <AgentSettings :show="settingsOpen" :config="config" :library="modelLibrary" :text-keys="textKeys" :image-keys="imageKeys" :busy="busy || sending || opening" :saving="savingSettings" :save-error="settingsError" @close="settingsOpen = false" @save="saveSettings" />
      <p v-if="settingsSaved" class="settings-saved" role="status">{{ settingsFeedback }}</p>
    </section>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAgentMotion } from '@/composables/useAgentMotion'
import AgentHistory from '@/components/agent/AgentHistory.vue'
import AgentQuestion from '@/components/agent/AgentQuestion.vue'
import AgentImagePreview from '@/components/agent/AgentImagePreview.vue'
import AgentArtifactPanel from '@/components/agent/AgentArtifactPanel.vue'
import AgentWorkspaceResources from '@/components/agent/AgentWorkspaceResources.vue'
import AgentAttachmentCard from '@/components/agent/AgentAttachmentCard.vue'
import AgentImageGallery from '@/components/agent/AgentImageGallery.vue'
import AgentImageNavigation from '@/components/agent/AgentImageNavigation.vue'
import { agentFileAccept, supportedAgentFile, readAttachmentPreview, agentFilesFromTransfer, agentTransferHasFiles, agentTransferHasDirectory, type AgentAttachmentView } from '@/utils/agentAttachments'
import AgentWorkPlan from '@/components/agent/AgentWorkPlan.vue'
import type { AgentWorkDraft, AgentWorkPlan as WorkPlan } from '@/api/agent'
import AgentChat from '@/components/agent/AgentChat.vue'
import AgentSettings from '@/components/agent/AgentSettings.vue'
import AgentModelPicker from '@/components/agent/AgentModelPicker.vue'
import { useAuthStore } from '@/stores/auth'
import { agentModelsForKey, emptyAgentConfig, emptyAgentModelLibrary, normalizeAgentModelLibrary, readAgentModelLibrary, readAgentPreferences, saveAgentPreferences, storedAgentConfig, withCurrentAgentModels, type AgentModelLibrary, type AgentModelPurpose } from '@/utils/agentPreferences'
import { agentState, agentError } from '@/utils/agentPresentation'
import { agentAPI, agentReasoningEfforts, terminal, watchAgentRun, type AgentModelCapability, type AgentConfig, type AgentConversation, type AgentMessage, type AgentRun, type AgentKey, type AgentReasoningEffort } from '@/api/agent'
const info = ref<{ enabled: boolean; history_available: boolean; conversation_memory?: boolean; asset_thumbnail_supported?: boolean;  }>()
const authStore = useAuthStore()
const defaultConfig = ref<AgentConfig>(emptyAgentConfig())
const modelLibrary = ref<AgentModelLibrary>(emptyAgentModelLibrary()), settingsFeedback = ref('设置已保存')
const config = reactive<AgentConfig>(emptyAgentConfig())
const preferenceNotice = ref('')
const workspaceMotion = useAgentMotion()
const textKeys = ref<AgentKey[]>([]), imageKeys = ref<AgentKey[]>([])
const conversations = ref<AgentConversation[]>([]), selected = ref<AgentConversation>(), messages = ref<AgentMessage[]>([]), run = ref<AgentRun>()
const grants = ref<import('@/api/agent').AgentGrant[]>([])
const workPlans = ref<WorkPlan[]>([]), workPending = ref(''), workError = ref('')
const activeRun = ref<AgentRun>(), statusDisconnected = ref(false), reconciliationOverdue = ref(false)
let synchronizeActiveRun: (() => Promise<void>) | undefined
const settingsOpen = ref(false), historyOpen = ref(false), settingsSaved = ref(false), savingSettings = ref(false), settingsError = ref('')
const retryError = ref(''), retryingSource = ref(''), pausedError = ref(''), continuing = ref(false)
const prompt = ref(''), error = ref(''), sending = ref(false), opening = ref(false), loading = ref(true), referenceIds = ref<string[]>([])
const composer = ref<HTMLTextAreaElement>(), scrollArea = ref<HTMLElement>(), settingsTrigger = ref<HTMLButtonElement>(), atBottom = ref(true)
type AssetView = AgentAttachmentView
const assetViews = ref<AssetView[]>([])
type ArtifactSelection = { kind: 'plan' | 'asset' | 'workspace'; id: string }
const activeArtifact = ref<ArtifactSelection>(), returnArtifact = ref<ArtifactSelection>(), artifactPlanHost = ref<HTMLElement>(), artifactOverlay = ref(false), naturalImageSize = ref(false)
const activeAsset = computed(() => originalAsset(activeArtifact.value?.kind === 'asset' ? activeArtifact.value.id : ''))
const activeWorkPlan = computed(() => activeArtifact.value?.kind === 'plan' ? workPlans.value.find(plan => plan.id === activeArtifact.value!.id) : undefined)
const generatedAssetIds = computed(() => [...new Set([
  ...messages.value.filter(message => message.role === 'assistant').flatMap(message => message.asset_ids || []),
  ...(run.value?.asset_ids || []),
  ...workPlans.value.flatMap(plan => [...(plan.results || []), ...(plan.inherited_results || [])].flatMap(result => result.asset_id ? [result.asset_id] : [])),
  ...(run.value?.work?.results || []).flatMap(result => result.asset_id ? [result.asset_id] : []),
])])
const workspaceImages = computed(() => generatedAssetIds.value.map(id => assetById(id) || { id }).filter(asset => !asset.mime_type || asset.mime_type.startsWith('image/')))
watch(generatedAssetIds, ids => { if (activeArtifact.value?.kind === 'workspace') void loadAssets(ids, generation) })

let artifactTrigger: HTMLElement | undefined, planUploadController: AbortController | undefined, composerUploadController: AbortController | undefined
const modalPreviewId = ref('')
const modalAsset = computed(() => originalAsset(modalPreviewId.value))
let modalTrigger: HTMLElement | undefined
const localPlanDrafts = new Map<string, { revision: number; draft: AgentWorkDraft }>()
const pendingWorkApprovals = new Map<string, { signature: string; reviewed: WorkPlan }>()
const uploadingPlans = ref(new Set<string>())
const previewId = ref(''), answering = ref(false), answerError = ref('')
let controller: AbortController | undefined, conversationController: AbortController | undefined, generation = 0, initialization = 0, toastTimer: ReturnType<typeof setTimeout> | undefined
let pending: { key: string; conversation: string; content: string; assets: string[]; version: number } | undefined
let pendingRetry: { source: string; key: string; version: number } | undefined
let pendingContinuation: { source: string; key: string; version: number } | undefined
const examples = [
  { icon: 'chat', title: '一起想一想', description: '讨论想法，整理思路', prompt: '我想和你讨论一个想法，请先问我几个问题，帮我把思路理清楚。' },
  { icon: 'document', title: '帮我写点东西', description: '把零散的内容变得清楚', prompt: '帮我把一段零散的想法整理成清楚的表达，请先问我主题和受众。' },
  { icon: 'photograph', title: '把灵感画出来', description: '描述画面，生成图片', prompt: '请帮我生成一张图片：' },
] as const
const busy = computed(() => !!run.value && !terminal(run.value))
const backgroundRun = computed(() => activeRun.value && !terminal(activeRun.value) && activeRun.value.conversation_id !== selected.value?.id ? activeRun.value : undefined)
const configured = computed(() => config.model_api_key_id > 0 && !!config.model)
const canStartRun = computed(() => !!info.value?.enabled && !busy.value && !backgroundRun.value && !sending.value && !opening.value && !loading.value && !savingSettings.value && !workPending.value && uploadingPlans.value.size === 0 && configured.value && (!config.image_api_key_id || (!!config.image_model && config.image_api_key_id !== config.model_api_key_id)))
const fileDragDepth = ref(0)
const attachmentUploadBlocked = computed(() => !info.value?.enabled || (busy.value && run.value?.status !== 'waiting_user') || sending.value || loading.value || opening.value || savingSettings.value || !!workPending.value)
const canSend = computed(() => canStartRun.value && (!!prompt.value.trim() || referenceIds.value.length > 0))
const modelCapabilities = ref<AgentModelCapability[]>([]), capabilitiesLoading = ref(false), capabilitiesError = ref('')
let capabilityController: AbortController | undefined
let capabilityGeneration = 0
async function loadModelCapabilities() {
  const user = authStore.user?.id, key = config.model_api_key_id, token = ++capabilityGeneration
  capabilityController?.abort(); modelCapabilities.value = []; capabilitiesError.value = ''
  capabilitiesLoading.value = false
  if (!user || !key) return
  const controller = new AbortController(); capabilityController = controller; capabilitiesLoading.value = true
  try {
    const response = await agentAPI.capabilities(key, 'text', controller.signal)
    if (token === capabilityGeneration && !controller.signal.aborted && authStore.user?.id === user && config.model_api_key_id === key) modelCapabilities.value = Array.isArray(response.model_capabilities) ? response.model_capabilities : []
  } catch {
    if (token === capabilityGeneration && !controller.signal.aborted) capabilitiesError.value = '思考档位读取失败'
  } finally { if (token === capabilityGeneration) capabilitiesLoading.value = false }
}
const pickerTextModels = computed(() => config.model_api_key_id ? [...new Set([...agentModelsForKey(modelLibrary.value, 'text', config.model_api_key_id), ...(config.model ? [config.model] : [])])] : [])
const pickerImageModels = computed(() => config.image_api_key_id ? [...new Set([...agentModelsForKey(modelLibrary.value, 'image', config.image_api_key_id), ...(config.image_model ? [config.image_model] : [])])] : [])
const state = computed(() => run.value?.recovery_only && !terminal(run.value) && !reconciliationOverdue.value ? { label: '正在恢复图片', detail: '正在从已保存的上游响应恢复，不会重新生成图片。', tone: 'active' } : run.value?.status === 'reconciling' && reconciliationOverdue.value ? { label: '核对期限已到，正在同步最终状态', detail: '正在向服务器获取任务的结束状态；你仍可切换对话。', tone: 'warning' } : agentState(run.value?.status, run.value?.error_code))
const displayMessages = computed<AgentMessage[]>(() => {
  const current = run.value
  const replacing = retryingSource.value || (current?.retry_of && !['failed', 'cancelled', 'expired'].includes(current.status) ? current.retry_of : '')
  let visible = replacing ? messages.value.filter(message => message.role !== 'assistant' || message.run_id !== replacing) : messages.value
  if (current?.recovery_only && !terminal(current)) visible = visible.map(message => message.role === 'assistant' && message.run_id === current.id ? { ...message, text: current.text, trace: current.trace, asset_ids: current.asset_ids, error_code: current.error_code, run_status: current.status, can_recover_image: false } : message)
  if (retryingSource.value) return [...visible, { id: `retry-pending-${retryingSource.value}`, role: 'assistant', text: '', pending: true }]
  if (!current || visible.some(message => message.run_id === current.id && message.role === 'assistant')) return visible
  const hasContent = !!(current.text || current.asset_ids?.length || current.trace?.length || workPlans.value.some(plan => plan.source_run_id === current.id))
  const pending = !!replacing && !terminal(current) && !hasContent
  if (!hasContent && !pending) return visible
  return [...visible, { id: current.id, role: 'assistant', text: current.text || '', asset_ids: current.asset_ids, run_id: current.id, trace: current.trace, metrics: current.metrics, run_status: current.status, error_code: current.error_code, pending }]
})
const orphanWorkPlans = computed(() => workPlans.value.filter(plan => !displayMessages.value.some(message => message.role === 'assistant' && message.run_id === plan.source_run_id)))
const isPausedRun = computed(() => run.value?.status === 'cancelled' || (run.value?.status === 'partial_success' && run.value.cancel_requested && !run.value.error_code))
const showRunNotice = computed(() => {
  const current = run.value
  if (!current || current.status === 'completed' || isPausedRun.value) return false
  // Errors and uncertain results need explicit actions outside the folded trace.
  if (current.error_code || current.cancel_requested || ['reconciling', 'partial_success', 'failed', 'expired', 'cancelled'].includes(current.status)) return true
  if (current.status === 'waiting_user' && current.question) return false
  // Normal execution is already represented by the current reply's process
  // disclosure (or the retry placeholder). Keep only a pre-trace fallback.
  return !displayMessages.value.some(message => message.role === 'assistant' && (message.pending || (message.run_id === current.id && message.trace?.length)))
})

function onHistoryChanged(items: AgentConversation[]) {
  conversations.value = items
  if (selected.value) {
    const updated = items.find(item => item.id === selected.value?.id)
    if (!updated || updated.archived_at) newConversation()
    else selected.value = updated
  }
}
async function answerQuestion(answers: Record<string, string>) {
  if (answering.value || sending.value || !run.value?.question || run.value.status !== 'waiting_user') return
  const token = generation, id = run.value.id, question = run.value.question
  answering.value = true; answerError.value = ''
  try {
    const snapshot = await (referenceIds.value.length ? agentAPI.answer(id, question.id, answers, referenceIds.value) : agentAPI.answer(id, question.id, answers))
    if (token !== generation || run.value?.id !== id) return
    run.value = snapshot
    const answeredAssets = [...referenceIds.value]; referenceIds.value = []
    const acknowledgement: AgentMessage = { id: `answer-${question.id}`, kind: 'question_answer', question_id: question.id, asset_ids: answeredAssets, role: 'user', text: '', run_id: id, answers: question.fields.filter(field => answers[field.id]?.trim()).map(field => ({ label: field.label, value: answers[field.id]!.trim() })) }
    if (!messages.value.some(message => message.question_id === question.id)) messages.value.push(acknowledgement)
    void follow(id, token)
    // The task resumes as soon as the answer is accepted. A history refresh
    // must not delay its stream, or overwrite a later terminal refresh.
    void agentAPI.detail(snapshot.conversation_id).then(detail => {
      if (token === generation && run.value?.id === id && busy.value) messages.value = detail.messages || []
    }).catch(() => undefined)
  } catch (e) { if (token === generation) answerError.value = errorMessage(e) }
  finally { if (token === generation) answering.value = false }
}
function errorMessage(e: unknown) { if ((e as { status?: number })?.status === 404) return '当前后端尚未接入 Agent，请启动支持 Agent 的开发版本。'; return e instanceof Error ? e.message : (e as { message?: string })?.message || '请求未完成，请稍后重试。' }
function showError(e: unknown) { error.value = errorMessage(e) }
function normalizeConfig(c: AgentConfig) { return { ...c, image_api_key_id: c.image_api_key_id || 0, image_model: c.image_model || '' } }
function openSettings() { if (sending.value || opening.value || savingSettings.value) return; settingsError.value = ''; settingsOpen.value = true }
async function saveSettings(value: AgentConfig, library: AgentModelLibrary = modelLibrary.value, source: 'settings' | 'picker' | 'effort' = 'settings') {
  if (busy.value || sending.value || opening.value || savingSettings.value) return
  const userId = authStore.user?.id, token = generation
  if (!userId) { settingsError.value = '登录状态已变化，请重新进入 Agent。'; return }
  const next = storedAgentConfig(value)
  if (!next) { settingsError.value = '请检查所选模型和 API key。'; return }
  savingSettings.value = true
  settingsError.value = ''
  try {
    if (source === 'picker') {
      error.value = ''
      const purpose = next.model !== config.model ? 'text' : 'image'
      const key = purpose === 'text' ? next.model_api_key_id : next.image_api_key_id
      const model = purpose === 'text' ? next.model : next.image_model
      const response = key && purpose === 'text' ? await agentAPI.capabilities(key, purpose) : undefined
      const allowed = response?.models || (key ? await agentAPI.models(key, purpose) : [])
      if (response) {
        const capability = response.model_capabilities?.find(item => item.id === model)
        if (capability?.reasoning_levels.length && !capability.reasoning_levels.includes(next.reasoning_effort || 'medium')) next.reasoning_effort = capability.default_reasoning_effort || capability.reasoning_levels[0]
      }
      if (token !== generation || authStore.user?.id !== userId) return
      if (!model || !allowed.includes(model)) throw new Error('此 key 当前不支持所选模型，请在设置中重新获取并添加。')
    }
    if (selected.value) {
      const d = await agentAPI.configure(selected.value.id, next, selected.value.config_version)
      if (authStore.user?.id !== userId || token !== generation) return
      selected.value = d.conversation
    }
    if (authStore.user?.id !== userId || token !== generation) return
    Object.assign(config, next)
    defaultConfig.value = { ...next }
    modelLibrary.value = withCurrentAgentModels(normalizeAgentModelLibrary(library), next)
    preferenceNotice.value = saveAgentPreferences(userId, next, modelLibrary.value) ? '' : '设置已应用，但浏览器无法保存默认设置，刷新后可能无法恢复。'
    if (source === 'settings') settingsOpen.value = false
    settingsFeedback.value = source === 'picker' ? '模型已切换' : source === 'effort' ? '思考强度已更新' : '设置已保存'
    settingsSaved.value = !preferenceNotice.value
    clearTimeout(toastTimer)
    toastTimer = setTimeout(() => { settingsSaved.value = false }, 2500)
    if (source === 'settings') { await nextTick(); composer.value?.focus() }
  } catch (e) { if (authStore.user?.id === userId && token === generation) { if (source !== 'settings') showError(e); else settingsError.value = errorMessage(e) } }
  finally { if (token === generation) savingSettings.value = false }
}
async function changeReasoningEffort(value: AgentReasoningEffort) {
  if (!agentReasoningEfforts.includes(value) || value === (config.reasoning_effort || 'medium')) return
  await saveSettings({ ...config, reasoning_effort: value }, modelLibrary.value, 'effort')
}
async function selectModel(purpose: AgentModelPurpose, model: string) {
  if (busy.value || sending.value || opening.value || savingSettings.value) return
  const added = purpose === 'text' ? pickerTextModels.value : pickerImageModels.value
  const selectedModel = purpose === 'text' ? config.model : config.image_model
  if (!added.includes(model) || model === selectedModel) return
  const next = { ...config, ...(purpose === 'text' ? { model } : { image_model: model }) }
  await saveSettings(next, modelLibrary.value, 'picker')
}
async function chooseExample(value: string) { prompt.value = value; if (value.includes('生成一张图片') && !config.image_api_key_id) openSettings(); else { await nextTick(); composer.value?.focus() } }
const shownLiveArtifacts = new Set<string>()
let planSyncRequest = 0
function presentLiveArtifact(kind: 'plan' | 'asset', id: string, runId: string) {
  const key = `${kind}:${runId}`
  if (shownLiveArtifacts.has(key)) return
  shownLiveArtifacts.add(key)
  if (activeArtifact.value && !(kind === 'plan' && activeArtifact.value.kind === 'plan')) return
  if (kind === 'plan') openPlanArtifact(id)
  else previewId.value = id
}
function mergeLivePlans(plans: WorkPlan[]) {
  const merged = new Map(workPlans.value.map(plan => [plan.id, plan]))
  for (const plan of plans) {
    const current = merged.get(plan.id)
    if (current && (current.revision > plan.revision || (current.revision === plan.revision && current.status === 'submitted' && plan.status !== 'submitted'))) continue
    merged.set(plan.id, plan)
  }
  workPlans.value = [...merged.values()]
}
async function syncLivePlans(id: string, token: number, signal: AbortSignal) {
  const conversation = selected.value?.id, request = ++planSyncRequest
  if (!conversation) return
  try {
    const detail = await agentAPI.detail(conversation, signal)
    if (signal.aborted || token !== generation || request !== planSyncRequest || run.value?.id !== id || selected.value?.id !== conversation) return
    mergeLivePlans(detail.work_plans || []); grants.value = detail.execution_grants || []
    void loadWorkAssets(token)
    const plan = [...(detail.work_plans || [])].reverse().find(item => item.source_run_id === id)
    if (plan) presentLiveArtifact('plan', plan.id, id)
  } catch { /* A final snapshot or the next persisted event can refresh the card. */ }
}
function openPlanArtifact(id: string) {
  if (!activeArtifact.value) artifactTrigger = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  const back = activeArtifact.value?.kind === 'workspace' ? { ...activeArtifact.value } : undefined
  activeArtifact.value = { kind: 'plan', id }; returnArtifact.value = back; previewId.value = ''; historyOpen.value = false
}
function toggleWorkspace() {
  if (activeArtifact.value?.kind === 'workspace') { closeArtifact(); return }
  if (!activeArtifact.value) artifactTrigger = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  activeArtifact.value = { kind: 'workspace', id: 'workspace' }; returnArtifact.value = undefined; previewId.value = ''; historyOpen.value = false
  void loadAssets(generatedAssetIds.value, generation)
}
function closeArtifact() { activeArtifact.value = undefined; returnArtifact.value = undefined; previewId.value = ''; naturalImageSize.value = false; if (artifactTrigger?.isConnected) artifactTrigger.focus({ preventScroll: true }); artifactTrigger = undefined }
function backToArtifact() { const previous = returnArtifact.value; if (!previous) return; activeArtifact.value = previous; returnArtifact.value = undefined; previewId.value = ''; naturalImageSize.value = false }
watch(previewId, id => {
  if (!id) { if (activeArtifact.value?.kind === 'asset') closeArtifact(); return }
  if (!activeArtifact.value) artifactTrigger = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
  if (activeArtifact.value && activeArtifact.value.kind !== 'asset') returnArtifact.value = { ...activeArtifact.value }
  activeArtifact.value = { kind: 'asset', id }; naturalImageSize.value = false; historyOpen.value = false
  void loadOriginalAsset(id, generation)
})
function revokeAssets() { assetViews.value.forEach(a => { if (a.url) URL.revokeObjectURL(a.url); if (a.original_url) URL.revokeObjectURL(a.original_url) }); assetViews.value = []; assetLoads.clear(); metadataLoads.clear(); originalLoads.clear(); originalControllers.forEach(controller => controller.abort()); originalControllers.clear() }
function assetById(id: string) { return assetViews.value.find(a => a.id === id) }
function messageImages(message: AgentMessage): AssetView[] {
  if (message.role !== 'assistant' || message.kind === 'question_answer') return []
  return (message.asset_ids || []).map(id => assetById(id) || { id }).filter(asset => !asset.mime_type || asset.mime_type.startsWith('image/'))
}
function messageAttachmentIds(message: AgentMessage) {
  const images = messageImages(message)
  const grouped = images.length > 1 ? new Set(images.map(asset => asset.id)) : new Set<string>()
  return (message.asset_ids || []).filter(id => !grouped.has(id))
}
const previewImageIndex = computed(() => workspaceImages.value.findIndex(asset => asset.id === previewId.value))
function selectPreviewImage(index: number) {
  const asset = workspaceImages.value[index]
  if (asset) previewId.value = asset.id
}
function onPreviewKeydown(event: KeyboardEvent) {
  if (previewImageIndex.value < 0 || event.altKey || event.ctrlKey || event.metaKey || !['ArrowLeft', 'ArrowRight'].includes(event.key)) return
  event.preventDefault()
  selectPreviewImage(previewImageIndex.value + (event.key === 'ArrowLeft' ? -1 : 1))
}

const metadataLoads = new Map<string, Promise<AssetView | undefined>>()
const assetLoads = new Map<string, Promise<void>>(), originalLoads = new Map<string, Promise<void>>(), originalControllers = new Map<string, AbortController>()
watch(() => [previewId.value, modalPreviewId.value], () => { for (const [id, controller] of originalControllers) if (id !== previewId.value && id !== modalPreviewId.value) controller.abort(); for (const asset of assetViews.value) if (asset.original_url && asset.id !== previewId.value && asset.id !== modalPreviewId.value) { URL.revokeObjectURL(asset.original_url); asset.original_url = undefined; asset.text = undefined } })
function originalAsset(id: string) { const asset = assetById(id); return asset ? { ...asset, url: asset.original_url || (!asset.thumbnail ? asset.url : undefined), failed: asset.original_failed || asset.failed } : undefined }
function requestVisibleAsset(id: string) { void loadAssets([id], generation, true) }
// Register lightweight placeholders first. Fetch payloads only near the viewport.
async function loadAssetMetadata(id: string, token: number): Promise<AssetView | undefined> {
  const known = assetById(id)
  if (known?.mime_type && known.filename) return known
  const existing = metadataLoads.get(id); if (existing) return existing
  const task = (async () => {
    try {
      const meta = await agentAPI.assetMeta(id)
      if (generation !== token) return
      const asset = assetById(id); if (!asset) return
      Object.assign(asset, meta)
      if (meta.content_available === false || (meta.expires_at && Date.parse(meta.expires_at) <= Date.now())) Object.assign(asset, { status: 'expired', content_available: false })
      return asset
    } catch { if (generation === token) { const asset = assetById(id); if (asset) asset.failed = true } }
  })()
  metadataLoads.set(id, task)
  try { return await task } finally { if (metadataLoads.get(id) === task) metadataLoads.delete(id) }
}
async function loadAssets(ids: string[], token: number, content = false) {
  if (token !== generation) return
  const unique = [...new Set(ids)]
  for (const id of unique) if (!assetById(id)) assetViews.value.push({ id })
  if (!content) { for (const id of unique) { if (generation !== token) return; await loadAssetMetadata(id, token) } return }
  for (const id of unique) {
    if (token !== generation) return
    if (!assetById(id)) assetViews.value.push({ id })
    const current = assetById(id)!
    if (!content || current.url || current.failed || current.status === 'expired' || (current.filename && current.mime_type && !current.mime_type.startsWith('image/'))) continue
    const existing = assetLoads.get(id); if (existing) { await existing; continue }
    const load = (async () => {
      try {
        const meta = await loadAssetMetadata(id, token)
        if (generation !== token || !meta) return
        const asset = assetById(id); if (!asset) return
        Object.assign(asset, meta)
        if (meta.content_available === false || (meta.expires_at && Date.parse(meta.expires_at) <= Date.now())) { Object.assign(asset, { status: 'expired', content_available: false }); return }
        if (!meta.mime_type?.startsWith('image/')) return
        const thumbnail = info.value?.asset_thumbnail_supported === true
        const blob = thumbnail ? await agentAPI.assetThumbnail(id) : await agentAPI.asset(id)
        if (generation !== token) return
        const mime = meta.mime_type || blob.type
        const text = !thumbnail && (mime.startsWith('text/') || mime === 'application/json') ? await readAttachmentPreview(blob) : undefined
        if (generation !== token) return
        Object.assign(asset, { filename: meta.filename || `${id}.${mime.split('/').pop() || 'file'}`, mime_type: mime, byte_size: meta.byte_size ?? blob.size, url: URL.createObjectURL(blob), text, thumbnail, preview_bytes: blob.size })
        trimAssetPreviews(id)
      } catch { if (generation === token) { const asset = assetById(id); if (asset) asset.failed = true } }
    })()
    assetLoads.set(id, load)
    try { await load } finally { if (assetLoads.get(id) === load) assetLoads.delete(id) }
  }
}
function trimAssetPreviews(keep: string) {
  let bytes = assetViews.value.reduce((sum, asset) => sum + (asset.preview_bytes || 0), 0)
  for (const asset of assetViews.value) {
    if (bytes <= 16 * 1024 * 1024) break
    if (!asset.url || !asset.thumbnail || asset.id === keep || asset.id === previewId.value || asset.id === modalPreviewId.value) continue
    URL.revokeObjectURL(asset.url); bytes -= asset.preview_bytes || 0; Object.assign(asset, { url: undefined, preview_bytes: 0 })
  }
}
async function loadOriginalAsset(id: string, token: number) {
  if (!id) return
  await loadAssets([id], token, true)
  if (generation !== token) return
  const asset = assetById(id); if (!asset || asset.status === 'expired' || asset.original_url || (asset.url && !asset.thumbnail)) return
  if (originalLoads.has(id) && !originalControllers.get(id)?.signal.aborted) return originalLoads.get(id)
  if (originalControllers.get(id)?.signal.aborted) { originalLoads.delete(id); originalControllers.delete(id) }
  const transfer = new AbortController(); originalControllers.set(id, transfer)
  const load = (async () => {
    try {
      const blob = await agentAPI.asset(id, transfer.signal)
      if (generation !== token || transfer.signal.aborted || (id !== previewId.value && id !== modalPreviewId.value)) return
      for (const other of assetViews.value) if (other.original_url && other.id !== previewId.value && other.id !== modalPreviewId.value) { URL.revokeObjectURL(other.original_url); other.original_url = undefined }
      asset.original_url = URL.createObjectURL(blob); asset.original_failed = false
      if (asset.mime_type?.startsWith('text/') || asset.mime_type === 'application/json') { const text = await readAttachmentPreview(blob); if (generation === token && !transfer.signal.aborted && (id === previewId.value || id === modalPreviewId.value)) asset.text = text }
    } catch { if (generation === token && !transfer.signal.aborted) asset.original_failed = true }
  })()
  originalLoads.set(id, load)
  try { await load } finally { if (originalLoads.get(id) === load) originalLoads.delete(id); if (originalControllers.get(id) === transfer) originalControllers.delete(id) }
}
async function downloadAsset(id: string) {
  const token = generation
  try { const blob = await agentAPI.asset(id); if (generation !== token) return; const url = URL.createObjectURL(blob), link = document.createElement('a'); link.href = url; link.download = assetById(id)?.filename || id; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000) }
  catch (e) { if (generation === token) error.value = errorMessage(e) }
}
let expiryTimer: ReturnType<typeof setInterval> | undefined
function expireAssetViews() {
  for (const asset of assetViews.value) {
    if (asset.expires_at && Date.parse(asset.expires_at) <= Date.now()) {
      if (asset.url) URL.revokeObjectURL(asset.url)
      if (asset.original_url) URL.revokeObjectURL(asset.original_url)
      Object.assign(asset, { url: undefined, original_url: undefined, text: undefined, status: 'expired', content_available: false, failed: false })
      if (previewId.value === asset.id) previewId.value = ''
      if (modalPreviewId.value === asset.id) closeUploadedPreview()
      referenceIds.value = referenceIds.value.filter(id => id !== asset.id)
    }
  }
}
watch(() => assetViews.value.map(asset => asset.expires_at).join(), expireAssetViews)
onMounted(() => { expiryTimer = setInterval(expireAssetViews, 1000) })
function originalImageError(id: string) { const asset = assetById(id); if (!asset) return; if (asset.original_url) URL.revokeObjectURL(asset.original_url); asset.original_url = undefined; asset.original_failed = true; if (!asset.thumbnail && asset.url) { URL.revokeObjectURL(asset.url); asset.url = undefined } }
function assetImageError(id: string) { const asset = assetById(id); if (asset?.url) { URL.revokeObjectURL(asset.url); asset.url = undefined; asset.failed = true } }
function retryAsset(id: string) { const asset = assetById(id); if (asset?.url) URL.revokeObjectURL(asset.url); if (asset?.original_url) URL.revokeObjectURL(asset.original_url); assetViews.value = assetViews.value.filter(a => a.id !== id); if (previewId.value === id || modalPreviewId.value === id) void loadOriginalAsset(id, generation); else void loadAssets([id], generation, true) }
async function useReference(id: string) { referenceIds.value = [id]; await nextTick(); composer.value?.focus() }
function leaveConversation() {
  composerUploadController?.abort(); fileDragDepth.value = 0
  pendingWorkApprovals.clear(); localPlanDrafts.clear(); uploadingPlans.value = new Set(); closeUploadedPreview()
  planUploadController?.abort(); closeArtifact(); shownLiveArtifacts.clear(); ++planSyncRequest
  workPlans.value = []; grants.value = []
  workPending.value = ''; workError.value = ''
  if (run.value && !terminal(run.value)) activeRun.value = run.value
  controller?.abort()
  conversationController?.abort()
  ++generation
  pending = undefined
  pendingRetry = undefined
  pendingContinuation = undefined
  pausedError.value = ''; continuing.value = false
  retryError.value = ''
  retryingSource.value = ''
  referenceIds.value = []
  settingsOpen.value = false
  error.value = ''
  historyOpen.value = false
  previewId.value = ''; answering.value = false; answerError.value = ''
  revokeAssets()
}
async function openConversation(id: string) {
  if (sending.value || savingSettings.value) return
  leaveConversation()
  const token = generation
  conversationController = new AbortController()
  const signal = conversationController.signal
  opening.value = true
  try {
    const d = await agentAPI.detail(id, signal)
    if (token !== generation || signal.aborted) return
    selected.value = d.conversation
    Object.assign(config, normalizeConfig(d.conversation.config))
    messages.value = d.messages || []
    workPlans.value = d.work_plans || []; grants.value = d.execution_grants || []
    for (const plan of workPlans.value) shownLiveArtifacts.add(`plan:${plan.source_run_id}`)
    if (d.active_run?.asset_ids?.length) shownLiveArtifacts.add(`asset:${d.active_run.id}`)
    void loadWorkAssets(token)
    run.value = d.active_run
    if (!run.value) {
      const lastRunId = [...messages.value].reverse().find(m => m.run_id)?.run_id
      if (lastRunId) {
        try {
          const previous = await agentAPI.run(lastRunId, signal)
          if (token !== generation || signal.aborted) return
          run.value = previous
        } catch { /* History remains readable when task metadata is unavailable. */ }
      }
    }
    if (token !== generation || signal.aborted) return
    atBottom.value = true
    void loadAssets([...messages.value.flatMap(m => m.asset_ids || []), ...(run.value?.asset_ids || [])], token)
    if (run.value && !terminal(run.value)) void follow(run.value.id, token)
  } catch (e) { if (token === generation && !signal.aborted) showError(e) }
  finally { if (token === generation) { opening.value = false; await scrollToBottom() } }
}
function newConversation() {
  if (sending.value || savingSettings.value) return
  leaveConversation()
  opening.value = false
  selected.value = undefined
  Object.assign(config, defaultConfig.value)
  messages.value = []
  workPlans.value = []; grants.value = []; workPending.value = ''; workError.value = ''
  run.value = undefined
  prompt.value = ''
  atBottom.value = true
  void nextTick(() => composer.value?.focus())
}
async function ensureConversation() {
  if (!selected.value) {
    const token = generation
    const created = await agentAPI.create(prompt.value.slice(0, 30) || '新对话', { ...config })
    if (token !== generation) throw new Error('会话已切换。')
    selected.value = created
    conversations.value.unshift(created)
  }
  return selected.value
}
function onComposerKeydown(event: KeyboardEvent) { if (event.key === 'Enter' && !event.shiftKey && !event.isComposing && event.keyCode !== 229) { event.preventDefault(); void send() } }
function updateLocalDraft(plan: WorkPlan, draft?: AgentWorkDraft) {
  if (draft) localPlanDrafts.set(plan.id, { revision: plan.revision, draft })
  else localPlanDrafts.delete(plan.id)
}
function updatePlanUpload(id: string, uploading: boolean) { const next = new Set(uploadingPlans.value); if (uploading) next.add(id); else next.delete(id); uploadingPlans.value = next }
async function syncEditedPlans(token: number, only?: string) {
  for (const [id, entry] of [...localPlanDrafts]) {
    if (only && id !== only) continue
    const plan = workPlans.value.find(item => item.id === id)
    if (!plan || !['draft', 'ready'].includes(plan.status)) { localPlanDrafts.delete(id); continue }
    if (plan.revision !== entry.revision) throw new Error('方案已更新，请核对右侧配置后再发送。')
    workPending.value = id
    try {
      const updated = await agentAPI.saveDraft(id, entry.revision, entry.draft)
      if (token !== generation) throw new Error('会话已切换。')
      localPlanDrafts.delete(id)
      mergeLivePlans([updated])
    } catch (e) { if (token === generation) workError.value = id + ':' + errorMessage(e); throw e }
    finally { if (token === generation) workPending.value = '' }
  }
}
function openUploadedPreview(id: string) { modalTrigger = document.activeElement instanceof HTMLElement ? document.activeElement : undefined; modalPreviewId.value = id; void loadOriginalAsset(id, generation) }
function closeUploadedPreview() { modalPreviewId.value = ''; if (modalTrigger?.isConnected) modalTrigger.focus({ preventScroll: true }); modalTrigger = undefined }
async function send() {
  if (!canSend.value) return
  sending.value = true; error.value = ''; atBottom.value = true
  const token = generation
  try {
    const c = await ensureConversation(), text = prompt.value.trim() || '请查看我上传的附件。'
    if (token !== generation) return
    await syncEditedPlans(token)
    if (token !== generation) return
    const signature = JSON.stringify([c.id, text, referenceIds.value, c.config_version])
    if (!pending || JSON.stringify([pending.conversation, pending.content, pending.assets, pending.version]) !== signature) pending = { key: crypto.randomUUID(), conversation: c.id, content: text, assets: [...referenceIds.value], version: c.config_version }
    const p = pending; const submitted = await agentAPI.submit(p.conversation, p.content, p.assets, p.version, p.key)
    if (token !== generation) return
    run.value = submitted
    // Ordinary messages preserve the current project drafts and approvals.
    messages.value.push({ id: p.key, role: 'user', text, asset_ids: [...referenceIds.value], run_id: run.value.id }); void loadAssets(referenceIds.value, generation); prompt.value = ''; referenceIds.value = []; pending = undefined; void follow(run.value.id, generation)
  } catch (e) { if (token === generation) showError(e) }
  finally { if (token === generation) sending.value = false }
}
async function continueRun() {
  if (!isPausedRun.value || !run.value || !selected.value || !canStartRun.value) return
  const token = generation, c = selected.value, source = run.value.id
  if (!pendingContinuation || pendingContinuation.source !== source || pendingContinuation.version !== c.config_version) pendingContinuation = { source, key: crypto.randomUUID(), version: c.config_version }
  const p = pendingContinuation
  sending.value = true; continuing.value = true; pausedError.value = ''; retryError.value = ''; atBottom.value = true
  try {
    await syncEditedPlans(token)
    if (token !== generation) return
    const submitted = await agentAPI.submit(c.id, '继续', [], p.version, p.key)
    if (token !== generation) return
    run.value = submitted
    messages.value.push({ id: p.key, role: 'user', text: '继续', run_id: submitted.id })
    pendingContinuation = undefined
    void follow(submitted.id, token)
  } catch (e) { if (token === generation) pausedError.value = errorMessage(e) }
  finally { if (token === generation) { sending.value = false; continuing.value = false } }
}
function retryPausedRun() {
  if (!isPausedRun.value || !run.value || !canStartRun.value) return
  pausedError.value = ''
  void retryReply({ id: run.value.id, role: 'assistant', text: run.value.text || '', run_id: run.value.id })
}
async function recoverImage(message: AgentMessage) {
  if (!message.run_id || busy.value || backgroundRun.value || sending.value || opening.value || savingSettings.value) return
  const token = generation; sending.value = true; error.value = ''
  try { const snapshot = await agentAPI.recoverImage(message.run_id); if (token !== generation) return; run.value = snapshot; void follow(snapshot.id, token) }
  catch (e) { if (token === generation) showError(e) }
  finally { if (token === generation) sending.value = false }
}
async function retryReply(message: AgentMessage) {
  if (!message.run_id || !selected.value || message.run_id !== run.value?.id || !terminal(run.value) || busy.value || backgroundRun.value || sending.value || opening.value || savingSettings.value || !configured.value) return
  retryingSource.value = message.run_id
  sending.value = true; retryError.value = ''; error.value = ''; atBottom.value = true
  const token = generation, c = selected.value
  if (!pendingRetry || pendingRetry.source !== message.run_id || pendingRetry.version !== c.config_version) pendingRetry = { source: message.run_id, key: crypto.randomUUID(), version: c.config_version }
  try {
    const p = pendingRetry
    const submitted = await agentAPI.retry(p.source, p.version, p.key)
    if (token !== generation) return
    run.value = { ...submitted, retry_of: submitted.retry_of || p.source }; pendingRetry = undefined; retryingSource.value = ''
    void follow(submitted.id, token)
  } catch (e) { if (token === generation) retryError.value = errorMessage(e) }
  finally { if (token === generation) { sending.value = false; retryingSource.value = '' } }
}
async function applySnapshot(snapshot: AgentRun, token: number, signal?: AbortSignal) {
  if (token !== generation || selected.value?.id !== snapshot.conversation_id || run.value?.id !== snapshot.id) return true
  if (terminal(run.value) && !terminal(snapshot)) return true
  if ((snapshot.last_event_sequence || 0) < (run.value.last_event_sequence || 0)) return false
  run.value = { ...snapshot, retry_of: snapshot.retry_of || run.value.retry_of }
  void loadAssets(snapshot.asset_ids || [], token)
  if (terminal(snapshot)) {
    const d = await agentAPI.detail(snapshot.conversation_id, signal)
    if (token === generation && run.value?.id === snapshot.id) {
      messages.value = d.messages || []
      mergeLivePlans(d.work_plans || []); grants.value = d.execution_grants || []
      const freshPlan = [...(d.work_plans || [])].reverse().find(plan => plan.source_run_id === snapshot.id)
      if (freshPlan) presentLiveArtifact('plan', freshPlan.id, snapshot.id)
      else if (snapshot.asset_ids?.length) presentLiveArtifact('asset', snapshot.asset_ids[0]!, snapshot.id)
      void loadWorkAssets(token)
      void loadAssets(messages.value.flatMap(m => m.asset_ids || []), token)
      conversations.value = conversations.value.map(c => c.id === d.conversation.id ? d.conversation : c)
    }
    return true
  }
  return snapshot.status === 'waiting_user'
}
async function refreshRun(id: string, token: number, signal?: AbortSignal) {
  const snapshot = await agentAPI.run(id, signal)
  return applySnapshot(snapshot, token, signal)
}
// Admission belongs to the task, not the visible conversation. Keep one small
// observer independent of the selected view so a stalled SSE cannot lock it.
watch(run, current => {
  if (!current) return
  if (!terminal(current)) activeRun.value = current
  else if (activeRun.value?.id === current.id) activeRun.value = undefined
}, { deep: true, flush: 'sync' })
watch(() => activeRun.value?.id, (id, _previous, onCleanup) => {
  if (!id) { statusDisconnected.value = false; reconciliationOverdue.value = false; synchronizeActiveRun = undefined; return }
  const observer = new AbortController()
  let polling = false
  let deadlineTimer: ReturnType<typeof setTimeout> | undefined
  const synchronize = async () => {
    if (polling || observer.signal.aborted) return
    polling = true
    try {
      const snapshot = await agentAPI.run(id, observer.signal)
      if (observer.signal.aborted || activeRun.value?.id !== id) return
      statusDisconnected.value = false
      if (run.value?.id === id && !opening.value) await applySnapshot(snapshot, generation, conversationController?.signal)
      if (activeRun.value?.id === id) activeRun.value = terminal(snapshot) ? undefined : snapshot
    } catch {
      if (!observer.signal.aborted && activeRun.value?.id === id) statusDisconnected.value = true
    } finally { polling = false }
  }
  synchronizeActiveRun = synchronize
  const resume = (event: Event) => { if (event.type !== 'visibilitychange' || document.visibilityState !== 'hidden') void synchronize() }
  const timer = window.setInterval(synchronize, 5000)
  const stopDeadline = watch(() => activeRun.value?.reconcile_deadline, deadline => {
    clearTimeout(deadlineTimer)
    reconciliationOverdue.value = false
    if (!deadline) return
    const remaining = Date.parse(deadline) - Date.now()
    if (!Number.isFinite(remaining)) return
    deadlineTimer = setTimeout(() => { reconciliationOverdue.value = true; void synchronize() }, Math.max(0, remaining) + 100)
  }, { immediate: true })
  window.addEventListener('focus', resume)
  window.addEventListener('online', resume)
  window.addEventListener('pageshow', resume)
  document.addEventListener('visibilitychange', resume)
  onCleanup(() => {
    window.clearInterval(timer); clearTimeout(deadlineTimer); stopDeadline(); observer.abort()
    window.removeEventListener('focus', resume); window.removeEventListener('online', resume); window.removeEventListener('pageshow', resume); document.removeEventListener('visibilitychange', resume)
    if (synchronizeActiveRun === synchronize) synchronizeActiveRun = undefined
  })
})
async function follow(id: string, token: number) {
  controller?.abort(); controller = new AbortController(); const signal = controller.signal; let after = run.value?.id === id ? run.value.last_event_sequence || 0 : 0
  while (!signal.aborted && token === generation) {
    try {
      await watchAgentRun(id, after, signal, event => {
        if (token !== generation || run.value?.id !== id || terminal(run.value)) return
        after = Math.max(after, event.sequence)
        if (event.sequence <= (run.value.last_event_sequence || 0)) return
        const data = event.data as Partial<AgentRun> & { id?: string; run_id?: string }
        if (data?.run_id === id && Array.isArray(data.trace)) run.value.trace = data.trace
        if (data?.id === id && data.status) run.value = { ...run.value, ...data } as AgentRun
        else if (run.value) {
          if (event.type === 'model.started' && run.value.status !== 'resuming_model') run.value.status = 'running'
          if (event.type === 'tool.execution.started') run.value.status = 'executing_tool'
          if (event.type === 'tool.call.created') run.value.status = 'waiting_tool'
          if (event.type === 'asset.created' && data?.id) { run.value.status = 'resuming_model'; run.value.asset_ids ||= []; if (!run.value.asset_ids.includes(data.id)) run.value.asset_ids.push(data.id); void loadAssets([data.id], token) }
        }
        if (event.type === 'work.item.completed' && Array.isArray(data.asset_ids)) void loadAssets(data.asset_ids, token)
        if (['work.plan.created', 'work.plan.updated'].includes(event.type)) void syncLivePlans(id, token, signal)
        if (event.type === 'asset.created' && data?.id && data.id !== id) presentLiveArtifact('asset', data.id, id)
        if (event.type === 'work.item.completed' && data.asset_ids?.length) presentLiveArtifact('asset', data.asset_ids[0]!, id)
        run.value.last_event_sequence = Math.max(run.value.last_event_sequence || 0, event.sequence)
      })
      if (signal.aborted) return
      if (await refreshRun(id, token, signal)) return
    } catch {
      if (signal.aborted) return
      try { if (await refreshRun(id, token, signal)) return }
      catch (e) { if (!signal.aborted && token === generation) showError(e); return }
    }
    await new Promise<void>(resolve => { const done = () => { clearTimeout(timer); signal.removeEventListener('abort', done); resolve() }; const timer = window.setTimeout(done, 1500); signal.addEventListener('abort', done, { once: true }) })
  }
}
function workAssetIds(plan: WorkPlan): string[] { return [...new Set([...plan.shared_reference_asset_ids, ...plan.items.flatMap(item => item.reference_asset_ids), ...(plan.results || []).flatMap(item => item.asset_id ? [item.asset_id] : [])])] }
async function loadWorkAssets(token: number) { await loadAssets(workPlans.value.flatMap(workAssetIds), token) }
async function uploadPlanAssets(plan: WorkPlan, files: File[]): Promise<import('@/utils/agentAttachments').AgentReferenceUploadResult> {
  if (!canStartRun.value || workPending.value || selected.value?.id !== plan.conversation_id) throw new Error('当前无法上传素材')
  const token = generation, request = new AbortController()
  planUploadController?.abort(); planUploadController = request
  const uploaded: import('@/utils/agentAttachments').AgentReferenceUploadResult = { assets: [], errors: [] }
  sending.value = true
  try {
    for (const file of files) {
      if (token !== generation || request.signal.aborted) throw new Error('对话已切换')
      try {
        const asset = await agentAPI.upload(plan.conversation_id, file, request.signal)
        if (token !== generation || request.signal.aborted) throw new Error('对话已切换')
        const view = { ...asset, filename: asset.filename || file.name, mime_type: asset.mime_type || file.type, byte_size: file.size, url: !info.value?.asset_thumbnail_supported && (asset.mime_type || file.type).startsWith('image/') ? URL.createObjectURL(file) : undefined }
        assetViews.value.push(view); uploaded.assets.push(view)
      } catch (e) {
        if (token !== generation || request.signal.aborted) throw e
        uploaded.errors.push(`${file.name}：${errorMessage(e)}`)
      }
    }
    return uploaded
  } finally { if (token === generation) sending.value = false; if (planUploadController === request) planUploadController = undefined }
}
async function saveWorkDraft(plan: WorkPlan, draft: AgentWorkDraft) {
  if (!canStartRun.value || workPending.value) return
  const token = generation; workPending.value = plan.id; workError.value = ''
  try { const updated = await agentAPI.saveDraft(plan.id, plan.revision, draft); if (token === generation) { localPlanDrafts.delete(plan.id); mergeLivePlans([updated]) } }
  catch (e) { if (token === generation) workError.value = plan.id + ':' + errorMessage(e) }
  finally { if (token === generation) workPending.value = '' }
}
async function revokeWorkGrant(id: string) {
  const token = generation
  try { await agentAPI.revokeGrant(id); if (token === generation) grants.value = grants.value.map(grant => grant.id === id ? { ...grant, status: 'closed' } : grant) }
  catch (e) { if (token === generation) showError(e) }
}
async function approveWork(plan: WorkPlan, draft: AgentWorkDraft, itemIds?: string[]) {
  if (!canStartRun.value || workPending.value || selected.value?.id !== plan.conversation_id) return
  const token = generation; workPending.value = plan.id; workError.value = ''; sending.value = true
  const approvedDraft = JSON.parse(JSON.stringify(draft)) as AgentWorkDraft
  try {
    const signature = JSON.stringify([approvedDraft, itemIds || [], selected.value.config_version])
    const pendingApproval = pendingWorkApprovals.get(plan.id)
    const reviewed = pendingApproval?.signature === signature ? pendingApproval.reviewed : itemIds?.length ? await agentAPI.reviewWork(plan.id, plan.revision, approvedDraft, itemIds) : await agentAPI.reviewWork(plan.id, plan.revision, approvedDraft)
    if (token !== generation || selected.value?.id !== plan.conversation_id) return
    // A lost submit response must retry the same reviewed revision. Requoting
    // would turn a transport retry into a newly billed generation.
    pendingWorkApprovals.set(plan.id, { signature, reviewed })
    // Bind this human approval to the reviewed immutable revision, never the stale card.
    localPlanDrafts.delete(plan.id); mergeLivePlans([reviewed])
    const submitted = await agentAPI.submitWork(reviewed.id, reviewed.revision)
    if (token !== generation || selected.value?.id !== plan.conversation_id) return
    pendingWorkApprovals.delete(plan.id)
    run.value = submitted
    workPlans.value = workPlans.value.map(item => item.id === reviewed.id ? { ...item, status: 'submitted', run_id: submitted.id } : item)
    messages.value.push({ id: submitted.id + '-approval', role: 'user', text: itemIds?.length ? `批准单项生成：${approvedDraft.items.filter(item => itemIds.includes(item.id)).map(item => item.title).join("、")}` : `批准方案配置：${reviewed.title}`, run_id: submitted.id })
    void follow(submitted.id, token)
  } catch (e) { if (token === generation) {
    if (['AGENT_WORK_REVIEW_REQUIRED', 'AGENT_CONFLICT', 'AGENT_KEY_BINDING_CHANGED', 'AGENT_AUTHORIZATION_DENIED', 'AGENT_MODEL_NOT_ALLOWED', 'AGENT_RUN_BUSY', 'ASSET_EXPIRED'].includes((e as { code?: string }).code || '')) pendingWorkApprovals.delete(plan.id)
    workError.value = plan.id + ':' + errorMessage(e)
  } }
  finally { if (token === generation) { workPending.value = ''; sending.value = false } }
}
async function cancelRun() {
  if (!run.value) return
  const id = run.value.id, token = generation
  try {
    const result = await agentAPI.cancel(id)
    if (token === generation && run.value?.id === id) run.value = result
  } catch (e) { if (token === generation && run.value?.id === id) showError(e) }
}
async function upload(event: Event) {
  const input = event.target as HTMLInputElement, files = Array.from(input.files || [])
  input.value = ''
  await uploadComposerFiles(files)
}
function onComposerPaste(event: ClipboardEvent) {
  const files = agentFilesFromTransfer(event.clipboardData, true)
  if (!files.length) return
  event.preventDefault()
  void uploadComposerFiles(files)
}
function onFileDragEnter(event: DragEvent) {
  if (!agentTransferHasFiles(event.dataTransfer)) return
  event.preventDefault()
  if (!attachmentUploadBlocked.value) fileDragDepth.value++
}
function onFileDragOver(event: DragEvent) {
  if (!agentTransferHasFiles(event.dataTransfer)) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = attachmentUploadBlocked.value ? 'none' : 'copy'
}
function onFileDragLeave() { fileDragDepth.value = Math.max(0, fileDragDepth.value - 1) }
function onFileDrop(event: DragEvent) {
  if (!agentTransferHasFiles(event.dataTransfer)) return
  event.preventDefault(); event.stopPropagation(); fileDragDepth.value = 0
  if (agentTransferHasDirectory(event.dataTransfer)) { error.value = '暂不支持上传文件夹，请拖入具体文件。'; return }
  void uploadComposerFiles(agentFilesFromTransfer(event.dataTransfer))
}
function resetFileDrag() { fileDragDepth.value = 0 }
function preventFileNavigation(event: DragEvent) { if (agentTransferHasFiles(event.dataTransfer)) { event.preventDefault(); if (event.type === 'drop') resetFileDrag() } }
onMounted(() => {
  document.addEventListener('dragover', preventFileNavigation)
  document.addEventListener('drop', preventFileNavigation)
  window.addEventListener('dragend', resetFileDrag)
  window.addEventListener('blur', resetFileDrag)
})
onBeforeUnmount(() => {
  document.removeEventListener('dragover', preventFileNavigation)
  document.removeEventListener('drop', preventFileNavigation)
  window.removeEventListener('dragend', resetFileDrag)
  window.removeEventListener('blur', resetFileDrag)
})
async function uploadComposerFiles(files: File[]) {
  if (!files.length || attachmentUploadBlocked.value) return
  if (referenceIds.value.length + files.length > 8) { error.value = '每条消息最多添加 8 个文件，请减少本次选择的文件数量。'; return }
  if (!configured.value) { openSettings(); return }
  const invalid = files.find(file => file.size === 0 || file.size > 10 * 1024 * 1024 || !supportedAgentFile(file))
  if (invalid) { error.value = `${invalid.name}：请添加不超过 10 MiB 的图片、PDF、TXT、Markdown、CSV、JSON 或 DOCX/XLSX/PPTX 文件。`; return }
  sending.value = true; error.value = ''
  const token = generation, request = new AbortController()
  composerUploadController = request
  try {
    const c = await ensureConversation()
    if (token !== generation || request.signal.aborted) return
    const failures: string[] = []
    for (const file of files) {
      if (token !== generation || request.signal.aborted) return
      try {
        const asset = await agentAPI.upload(c.id, file, request.signal)
        if (token !== generation || request.signal.aborted) return
        const mime = asset.mime_type || file.type
        const text = mime.startsWith('text/') || mime === 'application/json' ? await readAttachmentPreview(file) : undefined
        if (token !== generation || request.signal.aborted) return
        referenceIds.value = [...referenceIds.value, asset.id]
        assetViews.value.push({ ...asset, filename: asset.filename || file.name, mime_type: mime, byte_size: file.size, url: !info.value?.asset_thumbnail_supported && mime.startsWith('image/') ? URL.createObjectURL(file) : undefined, text })
      } catch (e) {
        if (token !== generation || request.signal.aborted) return
        failures.push(`${file.name}：${errorMessage(e)}`)
      }
    }
    if (failures.length) error.value = failures.join('；')
  } catch (e) { if (token === generation) showError(e) }
  finally { if (token === generation) sending.value = false; if (composerUploadController === request) composerUploadController = undefined }
}

function onAssetLoaded() { if (atBottom.value) void scrollToBottom() }
function onScroll() { const el = scrollArea.value; if (el) atBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 100 }
async function scrollToBottom() { await nextTick(); const el = scrollArea.value; if (el) el.scrollTop = el.scrollHeight; atBottom.value = true }
watch(() => [displayMessages.value, run.value?.status, assetViews.value.map(a => a.url).join()], () => { if (atBottom.value) void scrollToBottom() }, { deep: true })
async function initializeWorkspace() {
  const userId = authStore.user?.id, token = ++initialization
  const current = () => token === initialization && authStore.user?.id === userId
  const saved = readAgentPreferences(userId)
  modelLibrary.value = readAgentModelLibrary(userId)
  defaultConfig.value = saved || emptyAgentConfig()
  Object.assign(config, defaultConfig.value)
  loading.value = true
  try {
    const available = await agentAPI.info()
    if (!current()) return
    info.value = available
    if (!available.enabled && !available.history_available) return
    const [text, image, items] = await Promise.all([available.enabled ? agentAPI.keys('text') : Promise.resolve([]), available.enabled ? agentAPI.keys('image') : Promise.resolve([]), agentAPI.list()])
    if (!current()) return
    textKeys.value = text || []
    imageKeys.value = image || []
    conversations.value = items || []
    const restored = saved || conversations.value.map(item => storedAgentConfig(item.config)).find(Boolean)
    if (restored) {
      const value = { ...restored }
      const notices: string[] = []
      if (available.enabled) {
        if (!textKeys.value.some(key => key.id === value.model_api_key_id)) {
          value.model_api_key_id = 0
          value.model = ''
          notices.push('已保存的对话 key 已不可用，请在设置中重新选择。')
        }
        if (value.image_api_key_id && !imageKeys.value.some(key => key.id === value.image_api_key_id)) {
          value.image_api_key_id = 0
          value.image_model = ''
          notices.push('已保存的生图 key 已不可用，请在设置中重新选择。')
        }
      }
      defaultConfig.value = value
      Object.assign(config, value)
      preferenceNotice.value = notices.join(' ')
      // Recover pre-fix settings from the user's own most recent server-side
      // conversation. Visiting history later never overwrites this default.
      modelLibrary.value = withCurrentAgentModels(modelLibrary.value, restored)
      if (!saved) saveAgentPreferences(userId, restored, modelLibrary.value)
    }
  } catch (e) { if (current()) showError(e) }
  finally { if (current()) loading.value = false }
}
watch(() => [authStore.user?.id, config.model_api_key_id], () => { void loadModelCapabilities() })
onMounted(initializeWorkspace)
watch(() => authStore.user?.id, () => {
  leaveConversation()
  ++initialization
  activeRun.value = undefined
  run.value = undefined
  selected.value = undefined
  messages.value = []
  conversations.value = []
  textKeys.value = []
  imageKeys.value = []
  prompt.value = ''
  opening.value = false
  sending.value = false
  savingSettings.value = false
  defaultConfig.value = emptyAgentConfig()
  modelLibrary.value = emptyAgentModelLibrary()
  info.value = undefined
  loading.value = !!authStore.user?.id
  preferenceNotice.value = ''
  settingsSaved.value = false
  clearTimeout(toastTimer)
  Object.assign(config, emptyAgentConfig())
  if (authStore.user?.id) void initializeWorkspace()
})
onBeforeUnmount(() => { composerUploadController?.abort(); planUploadController?.abort(); clearInterval(expiryTimer); ++capabilityGeneration; capabilityController?.abort(); controller?.abort(); conversationController?.abort(); ++generation; ++initialization; clearTimeout(toastTimer); revokeAssets() })
</script>
<style scoped>
.artifact-image-view { position:relative; display:flex; flex-direction:column; min-height:100%; }.artifact-image-view.has-image-navigation { padding:0 50px 28px; }.artifact-image-view > .image-preview { flex:1; min-height:0; }.preview-navigation { bottom:28px; }
/* finesse · component: AgentWorkspace · register=product · tokens: inherited */
.agent-workspace { --agent-muted: #66666f; color: var(--workspace-ink); max-width: 1600px; margin: 0 auto; }
:global(.dark .agent-workspace) { --agent-muted: #b0b0b9; }
.agent-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-height: 52px; border-bottom: 1px solid var(--workspace-divider); padding-bottom: 12px; }
.toolbar-title, .toolbar-actions { display: flex; align-items: center; gap: 8px; min-width: 0; }
.toolbar-title h2 { font-size: 14px; font-weight: 500; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; max-width: 440px; }
.quiet-button, .icon-button { min-height: 36px; padding: 8px 10px; display: inline-flex; align-items: center; justify-content: center; gap: 6px; font-size: 12px; border-radius: 7px; white-space: nowrap; }
@media (hover: hover) {
.quiet-button:hover, .icon-button:hover { background: var(--workspace-soft); } }
.history-toggle { display: none; }
.agent-grid { display: grid; grid-template-columns: 238px minmax(0, 1fr); height: calc(100dvh - 220px); min-height: 520px; }
.agent-conversations { border-right: 1px solid var(--workspace-divider); padding: 18px 12px 12px 0; display: flex; flex-direction: column; overflow-y: auto; }
.history-heading { display: flex; justify-content: space-between; align-items: center; padding: 0 10px 16px; font-size: 11px; color: var(--agent-muted); }
.history-heading h3 { font-weight: 500; }
.agent-conversation { flex-shrink: 0; text-align: left; padding: 12px 10px; border-radius: 7px; margin-bottom: 4px; }
@media (hover: hover) {
.agent-conversation:hover { background: var(--workspace-soft); } }
.agent-conversation.selected { background: var(--workspace-soft); }
.conversation-title { display: block; font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.agent-conversation time { display: block; color: var(--agent-muted); margin-top: 6px; font-size: 10px; }
.history-empty { padding: 0 10px; color: var(--agent-muted); font-size: 12px; line-height: 1.9; }
.history-footer { margin-top: auto; padding: 36px 10px 0; color: var(--agent-muted); font-size: 10px; line-height: 1.8; }
.agent-canvas { min-width: 0; min-height: 0; position: relative; display: flex; flex-direction: column; }
.agent-scroll { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding: 32px 36px 16px; scrollbar-gutter: stable; }
.agent-intro { width: 100%; max-width: 660px; margin: auto; min-height: 100%; display: flex; flex-direction: column; justify-content: center; padding: 20px 0 28px; }
.intro-symbol { display: flex; align-items: center; justify-content: center; width: 40px; height: 40px; border-radius: 12px; background: var(--workspace-soft); color: var(--workspace-ink); margin-bottom: 22px; }
.agent-intro h2 { font-size: 28px; font-weight: 550; letter-spacing: -.025em; line-height: 1.45; }
.agent-intro > p { font-size: 14px; line-height: 1.8; margin-top: 12px; color: var(--agent-muted); }
.agent-suggestions { margin-top: 30px; border-top: 1px solid var(--workspace-divider); }
.agent-suggestions button { display: flex; align-items: center; gap: 14px; width: 100%; text-align: left; padding: 15px 3px; border-bottom: 1px solid var(--workspace-divider); font-size: 13px; }
@media (hover: hover) {
.agent-suggestions button:hover { color: var(--workspace-accent); } }
.agent-suggestions button > span:last-child { margin-left: auto; }
.agent-suggestions strong { font-weight: 500; }
.agent-suggestions small { display: block; color: var(--agent-muted); font-size: 11px; margin-top: 4px; }
.setup-link { margin-top: 20px; text-align: left; font-size: 12px; color: var(--workspace-accent); width: fit-content; }
.conversation-content { max-width: 760px; margin: 0 auto; }
.conversation-loading { display: flex; align-items: center; justify-content: center; gap: 10px; min-height: 280px; color: var(--agent-muted); font-size: 13px; }
.composer-region { padding: 12px 36px 0; width: 100%; max-width: 850px; margin: 0 auto; }
.agent-composer { border: 1px solid var(--workspace-divider); border-radius: 16px; background: var(--workspace-surface); box-shadow: 0 3px 12px #00000005; }
.agent-composer:focus-within { border-color: var(--workspace-accent); }
.agent-composer textarea { display: block; width: 100%; border: 0; background: transparent; resize: vertical; min-height: 78px; max-height: 220px; padding: 16px 18px 8px; font-size: 14px; line-height: 1.8; color: var(--workspace-ink); }
.agent-composer textarea::placeholder { color: var(--agent-muted); }
.agent-composer textarea:focus { outline: none; }
.compose-actions { display: flex; align-items: center; gap: 8px; padding: 8px 12px 12px; }
.agent-upload, .model-summary { display: inline-flex; align-items: center; gap: 6px; font-size: 11px; color: var(--agent-muted); min-height: 32px; padding: 6px; border-radius: 6px; cursor: pointer; }
@media (hover: hover) {
.agent-upload:hover, .model-summary:hover { background: var(--workspace-soft); color: var(--workspace-ink); } }
.agent-upload input { position: absolute; width: 1px; height: 1px; opacity: 0; overflow: hidden; }
.agent-upload:focus-within { outline: 2px solid var(--workspace-accent); }
.model-summary { min-width: 0; max-width: 240px; }
.model-summary > span:first-child { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.reasoning-effort-control { display: inline-flex; align-items: center; gap: 5px; min-height: 32px; padding: 4px 6px; border: 1px solid var(--workspace-divider); border-radius: 6px; color: var(--agent-muted); font-size: 10px; white-space: nowrap; }
.reasoning-effort-control select { border: 0; background: transparent; color: var(--workspace-ink); font-size: 11px; outline: none; cursor: pointer; }
.reasoning-effort-control:focus-within { outline: 2px solid var(--workspace-accent); outline-offset: 2px; }
.compose-spacer { flex: 1; }
.send-button { flex: 0 0 28px; width: 28px; height: 28px; border-radius: 8px; background: var(--workspace-ink); color: var(--workspace-surface); display: flex; align-items: center; justify-content: center; font-size: 18px; line-height: 1; }
.stop-button { font-size: 11px; }
.send-button:disabled { opacity: 1; background: var(--workspace-soft); color: var(--agent-muted); }
.composer-hint { display: flex; justify-content: space-between; gap: 10px; color: var(--agent-muted); font-size: 10px; line-height: 1.7; padding: 10px 4px 0; }
.draft-attachments { display:flex; flex-wrap:wrap; gap:10px; padding:12px 14px 0; }
.agent-reference { margin: 12px 12px 0; border-radius: 7px; background: var(--workspace-soft); padding: 9px 12px; display: flex; justify-content: space-between; font-size: 12px; }
.agent-reference button { color: var(--agent-muted); }
.paused-region { display:flex; flex-direction:column; align-items:center; margin:16px 0 24px; }
.paused-toolbar { display:flex; align-items:center; justify-content:center; flex-wrap:wrap; gap:8px; max-width:100%; padding:8px 10px 8px 16px; border:1px solid var(--workspace-divider); border-radius:999px; background:var(--workspace-surface); box-shadow:0 3px 12px #0000000d; }
.paused-state { display:inline-flex; align-items:center; gap:8px; padding-right:4px; color:var(--workspace-ink); font-size:13px; font-weight:500; white-space:nowrap; }
.paused-state svg { color:var(--workspace-accent); }
.paused-toolbar button { display:inline-flex; align-items:center; justify-content:center; gap:6px; min-height:34px; padding:7px 12px; border-radius:999px; font-size:12px; white-space:nowrap; }
.pause-continue { background:var(--workspace-soft); color:var(--workspace-ink); }
.pause-retry { color:var(--workspace-accent); }
@media (hover: hover) {
.paused-toolbar button:hover:not(:disabled) { background:var(--workspace-soft); } }
.paused-toolbar button:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:2px; }
.paused-toolbar button:disabled { opacity:.5; cursor:not-allowed; }
.paused-error { max-width:100%; margin-top:8px; color:var(--workspace-ink); font-size:12px; line-height:1.7; overflow-wrap:anywhere; }
.agent-run { padding: 4px 0 12px; margin-bottom: 8px; }
.run-summary { display: flex; gap: 8px; align-items: center; font-size: 12px; color: var(--agent-muted); flex-wrap: wrap; }
.run-summary strong { color: var(--workspace-ink); font-weight: 500; }
.status-dot { display: inline-block; width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.status-dot.working { animation: agent-pulse 1.8s ease-in-out infinite; }
.tone-success .status-dot { color: #287052; }
.tone-warning .status-dot { color: #936400; }
.tone-error .status-dot { color: #b13730; }
:global(.dark) .tone-success .status-dot { color: #7ed9ac; }
:global(.dark) .tone-warning .status-dot { color: #f2ce7b; }
:global(.dark) .tone-error .status-dot { color: #ffaaa3; }
.run-cost { margin-left: auto; font-variant-numeric: tabular-nums; font-size: 11px; }
.run-details-toggle { font-size: 11px; padding: 5px 0 5px 5px; }
@media (hover: hover) {
.run-details-toggle:hover { color: var(--workspace-ink); } }
.run-description { color: var(--agent-muted); font-size: 12px; line-height: 1.8; margin-top: 10px; }
.run-error { font-size: 12px; color: #b13730; line-height: 1.8; margin-top: 10px; }
:global(.dark) .run-error, :global(.dark) .agent-error { color: #ffaaa3; }
.run-details { display: flex; gap: 8px 18px; flex-wrap: wrap; padding: 12px; margin-top: 10px; border-radius: 6px; background: var(--workspace-soft); color: var(--agent-muted); font-size: 11px; }
.run-details code { flex-basis: 100%; overflow-wrap: anywhere; font-size: 10px; }
.agent-assets { margin: 16px 0 8px; }
.agent-assets figure { width: min(320px, 100%); max-width: 100%; margin-bottom: 16px; border: 1px solid var(--workspace-divider); border-radius: 10px; overflow: hidden; }
.asset-thumbnail { display: block; width: 100%; cursor: zoom-in; }
.asset-thumbnail:focus-visible { outline: 2px solid var(--workspace-accent); outline-offset: -3px; }
.agent-assets img { display: block; width: 100%; max-height: 240px; object-fit: contain; background: var(--workspace-soft); }
.agent-assets figcaption { display: flex; align-items: center; gap: 16px; padding: 12px 14px; font-size: 11px; }
.agent-assets figcaption > span { color: var(--agent-muted); margin-right: auto; }
.agent-assets figcaption a, .agent-assets figcaption button { color: var(--workspace-accent); }
.asset-placeholder { padding: 40px 16px; display: flex; flex-direction: column; gap: 16px; align-items: center; font-size: 12px; color: var(--agent-muted); background: var(--workspace-soft); text-align: center; }
.asset-placeholder button { text-decoration: underline; }
.agent-notice { margin: 12px 0; border: 1px solid var(--workspace-divider); background: var(--workspace-soft); border-radius: 8px; padding: 12px 16px; font-size: 12px; line-height: 1.8; }
.sync-task { color: var(--workspace-accent); font-size: 11px; padding: 6px 4px; }
.background-task { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.background-task button { color: var(--workspace-accent); white-space: nowrap; }
.agent-error { display: flex; justify-content: space-between; gap: 16px; color: #b13730; }
.agent-error button { font-size: 18px; }
.jump-bottom { position: absolute; bottom: 172px; left: 50%; transform: translateX(-50%); background: var(--workspace-surface); border: 1px solid var(--workspace-divider); border-radius: 18px; padding: 7px 14px; font-size: 12px; box-shadow: 0 2px 10px #0001; }
.history-skeleton i { display: block; height: 38px; background: var(--workspace-soft); border-radius: 6px; margin: 8px; }
.settings-saved { position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%); background: var(--workspace-ink); color: var(--workspace-surface); padding: 10px 18px; border-radius: 8px; font-size: 12px; z-index: 40; }
button:disabled, .agent-upload.disabled { cursor: not-allowed; opacity: .5; }
:where(button, a):focus-visible { outline: 2px solid var(--workspace-accent); outline-offset: 3px; }
@keyframes agent-pulse { 50% { opacity: .25; } }
@media(prefers-reduced-motion: reduce) { .status-dot.working { animation: none; } }
@media(max-width: 1100px) { .agent-grid { grid-template-columns: 204px minmax(0, 1fr); } .agent-scroll { padding: 28px 24px 12px; } .composer-region { padding: 12px 24px 0; } .keyboard-hint { display: none; } }
@media(min-width: 761px) { .toolbar-new-chat { display:none; } }
@media(max-width: 760px) {
  .agent-toolbar { gap: 4px; } .toolbar-title h2 { display: none; } .history-toggle { display: inline-flex; } .toolbar-actions { gap: 0; } .quiet-button { padding: 8px; }
  .agent-grid { display: flex; flex-direction: column; height: calc(100dvh - 186px); min-height: 430px; }
  .agent-conversations { display: none; } .agent-conversations.history-open { display: block; border-right: 0; border-bottom: 1px solid var(--workspace-divider); max-height: 200px; padding: 12px 0; }
  .history-footer { display: none; } .history-heading { padding-bottom: 4px; } .agent-conversation { display: block; width: 100%; } .agent-canvas { flex: 1; }
  .agent-scroll { padding: 26px 4px 14px; } .agent-intro { padding: 12px 8px 24px; } .agent-intro h2 { font-size: 23px; } .agent-intro > p { font-size: 13px; }
  .agent-suggestions { margin-top: 24px; } .agent-suggestions button { padding: 12px 0; } .intro-symbol { margin-bottom: 18px; }
  .composer-region { padding: 10px 0 0; } .agent-composer textarea { padding: 14px 14px 6px; min-height: 70px; } .compose-actions { gap: 3px; } .model-summary { max-width: 155px; }
  .agent-upload span, .image-capability { display: none; } .agent-upload { min-width: 32px; justify-content: center; } .run-details-toggle { margin-left: auto; } .run-cost { margin-left: 4px; }
  .agent-assets figcaption { gap: 12px; padding: 10px; } .composer-hint { font-size: 10px; } .agent-conversation time { display: none; }
}

.agent-composer { border-radius:15px; box-shadow:0 2px 8px color-mix(in srgb,var(--workspace-ink) 4%,transparent); }
.agent-composer:focus-within { box-shadow:0 0 0 3px color-mix(in srgb,var(--workspace-accent) 10%,transparent); }
.agent-suggestions button { border-radius:8px; padding-left:8px; padding-right:8px; }
@media (hover: hover) {
.agent-suggestions button:hover { background:var(--workspace-soft); } }
.agent-suggestions button:active { transform:translateY(1px); }
.quiet-button:active:not(:disabled), .icon-button:active:not(:disabled) { transform:translateY(1px); }
.send-button { position:relative; }
.send-button:active:not(:disabled) { transform:scale(.94); }
.paused-toolbar { box-shadow:0 6px 18px color-mix(in srgb,var(--workspace-ink) 7%,transparent); }
.paused-toolbar button:active:not(:disabled) { transform:translateY(1px); }
@media(pointer:coarse) { .send-button::after { content:''; position:absolute; inset:-8px; } }
@media(prefers-reduced-motion:reduce) { .agent-workspace :deep(*), .agent-workspace :deep(*::before), .agent-workspace :deep(*::after) { animation-duration:.01ms!important; animation-iteration-count:1!important; transition-duration:.01ms!important; } }

.agent-grid { position:relative; }.agent-grid.artifact-open { grid-template-columns:minmax(0,1fr) clamp(360px,46%,620px); }
.artifact-open > .agent-conversations { display:none; }.artifact-active .history-toggle { display:inline-flex; }.artifact-open > .agent-conversations.history-open { display:flex; position:absolute; z-index:30; inset:0 auto 0 0; width:250px; background:var(--workspace-surface); padding:16px; box-shadow:12px 0 30px color-mix(in srgb,var(--workspace-ink) 10%,transparent); }
.artifact-open .agent-scroll { padding-left:22px; padding-right:22px; }.artifact-open .composer-region { padding-left:22px; padding-right:22px; }.artifact-plan-host { height:100%; min-height:0; }.artifact-tool { display:inline-flex; align-items:center; justify-content:center; padding:6px; min-height:30px; font-size:10px; border-radius:6px; color:var(--workspace-muted); white-space:nowrap; }.artifact-tool:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:2px; }.artifact-empty { padding:22px 6px; color:var(--workspace-muted); font-size:12px; line-height:1.8; }.artifact-empty button { margin-top:14px; color:var(--workspace-accent); }
@media(hover:hover) { .artifact-tool:hover { color:var(--workspace-ink); background:var(--workspace-soft); } }
@media(max-width:1100px) { .agent-grid.artifact-open { grid-template-columns:minmax(0,1fr); }.artifact-open .agent-scroll { padding-left:20px; padding-right:20px; } }

.agent-grid.artifact-open.plan-open { grid-template-columns:minmax(0,1fr) minmax(310px,46%); }
@media(max-width:1100px) and (min-width:761px) { .plan-open .agent-scroll,.plan-open .composer-region { padding-left:14px; padding-right:14px; }.plan-open .compose-actions { flex-wrap:wrap; } }
@media(max-width:760px) { .agent-grid.artifact-open.plan-open { grid-template-columns:minmax(0,1fr); grid-template-rows:minmax(380px,1fr) minmax(380px,1fr); height:auto; min-height:calc(100dvh - 160px); }.plan-open > .agent-canvas { height:60dvh; min-height:380px; } }

.workspace-entry { display:flex; align-items:center; justify-content:center; width:34px; height:34px; color:var(--workspace-muted); border:0; background:transparent; }
.workspace-entry.active { color:var(--workspace-ink); }.workspace-entry:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:3px; }.workspace-entry:active { transform:translateY(1px); }
@media(hover:hover) { .workspace-entry:hover { color:var(--workspace-ink); background:transparent; } }

.file-drop-overlay { position:absolute; inset:10px; z-index:35; pointer-events:none; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:12px; padding:24px; border:1px dashed var(--workspace-accent); border-radius:12px; background:color-mix(in srgb,var(--workspace-surface) 96%,transparent); color:var(--workspace-ink); text-align:center; }
.file-drop-overlay > svg { color:var(--workspace-accent); }.file-drop-overlay strong { font-size:15px; font-weight:550; }.file-drop-overlay span { font-size:11px; color:var(--workspace-muted); line-height:1.8; }.agent-composer:focus-within { border-color:var(--workspace-accent); }
</style>
