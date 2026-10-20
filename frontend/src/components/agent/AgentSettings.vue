<template>
  <dialog ref="dialog" class="agent-settings-dialog" aria-labelledby="agent-settings-title" @keydown.tab="trapFocus" @cancel="onCancel" @close="emit('close')" @click="onBackdrop">
    <form @submit.prevent="save">
      <header><div><h2 id="agent-settings-title">Agent 设置</h2><p>管理对话、生图连接与用量。</p></div><button type="button" class="close-settings" aria-label="关闭设置" :disabled="saving" @click="emit('close')">×</button></header>
      <div class="settings-layout">
        <nav ref="navigation" class="settings-navigation" role="tablist" aria-label="Agent 设置分类">
          <button v-for="(tab, index) in tabs" :id="`agent-settings-tab-${tab.id}`" :key="tab.id" type="button" role="tab" aria-controls="agent-settings-content" :aria-selected="activeTab === tab.id" :tabindex="activeTab === tab.id ? 0 : -1" :disabled="saving || validating" @click="selectTab(tab.id)" @keydown="onTabKeydown($event, index)"><Icon :name="tab.icon" size="sm" /><span>{{ tab.label }}</span></button>
        </nav>
        <div id="agent-settings-content" class="settings-body" role="tabpanel" :aria-labelledby="`agent-settings-tab-${activeTab}`">
        <p v-if="busy && activeTab !== 'usage'" class="settings-notice" role="status">任务进行中，结束后可以修改连接和常用模型。</p>
        <p v-if="activeTab !== 'usage' && (loadError || saveError)" class="settings-error" role="alert">{{ loadError || saveError }}</p>
        <fieldset v-show="activeTab !== 'usage'" class="connections" :disabled="busy || saving || validating">
          <section v-for="purpose in purposes" v-show="activeTab === purpose" :key="purpose" class="connection-section">
            <h3>{{ purpose === 'text' ? '对话配置' : '生图配置' }}</h3>
            <label>{{ purpose === 'text' ? '对话 API key' : '生图 API key' }}<select :value="keyFor(purpose)" :aria-label="purpose === 'text' ? '文本 API key' : '生图 API key'" @change="changeKey(purpose, Number(($event.target as HTMLSelectElement).value))"><option :value="0">{{ purpose === 'text' ? '选择已创建的 key' : '不启用图片生成' }}</option><option v-for="key in purpose === 'text' ? textKeys : imageKeys" :key="key.id" :value="key.id" :disabled="!!key.disabled_reason || (purpose === 'image' ? key.id === draft.model_api_key_id : key.id === draft.image_api_key_id)" :title="key.disabled_reason">{{ key.name }} · {{ key.group_name }}{{ key.disabled_reason ? `（${key.disabled_reason}）` : '' }}</option></select></label>
            <template v-if="keyFor(purpose)">
              <button type="button" class="fetch-models" :data-purpose="purpose" :disabled="fetching[purpose]" @click="openImport(purpose)"><Icon name="refresh" size="xs" />获取全部模型</button>
              <div class="models-heading"><span>已添加模型</span><span>{{ added(purpose).length }}</span></div>
              <ul class="added-models"><li v-for="model in added(purpose)" :key="model"><span>{{ model }}</span><button type="button" :aria-label="`移除 ${purpose === 'text' ? '对话' : '生图'}模型 ${model}`" @click="removeModel(purpose, model)">×</button></li></ul>
              <p v-if="!added(purpose).length" class="field-description">获取全部模型后，勾选需要使用的模型。</p>
            </template>
          </section>
          <RouterLink to="/keys" class="manage-keys" @click="emit('close')">管理我的 API key ↗</RouterLink>
        </fieldset>
        <AgentUsage v-if="show && activeTab === 'usage'" />
        </div>
      </div>
      <footer v-if="activeTab === 'usage'"><button type="button" class="settings-button secondary" :disabled="saving" @click="emit('close')">关闭</button></footer>
      <footer v-else><button type="button" class="settings-button secondary" :disabled="saving" @click="emit('close')">取消</button><button type="submit" class="settings-button primary" :disabled="!valid || busy || saving || validating">{{ saving || validating ? '正在保存…' : '保存设置' }}</button></footer>
    </form>
  </dialog>
  <dialog ref="importDialog" class="agent-settings-dialog agent-model-import" aria-labelledby="agent-import-title" @cancel.prevent="closeImport">
    <div class="import-content">
      <header><div><h2 id="agent-import-title">导入模型</h2><p>从服务商拉到 {{ available[activePurpose].length }} 个模型，已添加模型默认勾选；取消勾选会从当前列表移除（最多 20 个）。</p></div><button type="button" class="close-settings" aria-label="关闭导入模型" @click="closeImport">×</button></header>
      <div class="settings-body import-body">
        <label class="sr-only" for="agent-model-search">搜索模型 ID</label><input id="agent-model-search" v-model="search" type="search" placeholder="搜索模型 ID" />
        <div class="selection-summary"><span>已选 {{ checked.length }} 个</span><div><button type="button" @click="checked = []">清空</button><button type="button" :disabled="fetching[activePurpose]" @click="selectAll">全选</button></div></div>
        <p v-if="loadError || importError" class="settings-error" role="alert">{{ loadError || importError }}</p>
        <p v-if="fetching[activePurpose]" class="discovery-empty" role="status">正在读取此 key 可用的模型…</p>
        <ul v-else-if="filteredModels.length" class="available-models"><li v-for="model in filteredModels" :key="model"><label><input type="checkbox" :aria-label="`选择 ${model}`" :checked="checked.includes(model)" @change="toggleModel(model, $event)" /><span>{{ model }}</span><small v-if="added(activePurpose).includes(model)">已添加</small></label></li></ul>
        <p v-else class="discovery-empty">{{ search ? '没有匹配的模型。' : '此 key 没有返回可用模型。' }}</p>
      </div>
      <footer><button type="button" class="settings-button secondary" @click="closeImport">取消</button><button type="button" class="settings-button primary apply-import" :disabled="fetching[activePurpose] || loadedKey[activePurpose] !== keyFor(activePurpose) || checked.length > 20" @click="applyImport">更新列表（{{ checked.length }}/20）</button></footer>
    </div>
  </dialog>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import AgentUsage from './AgentUsage.vue'
import { useAgentMotion } from '@/composables/useAgentMotion'
import { agentAPI, type AgentConfig, type AgentKey } from '@/api/agent'
import { agentModelsForKey, emptyAgentModelLibrary, normalizeAgentModelLibrary, withAgentModels, withCurrentAgentModels, type AgentModelLibrary, type AgentModelPurpose } from '@/utils/agentPreferences'
const props = defineProps<{ show: boolean; config: AgentConfig; library?: AgentModelLibrary; textKeys: AgentKey[]; imageKeys: AgentKey[]; busy: boolean; saving: boolean; saveError?: string }>()
const emit = defineEmits<{ close: []; save: [config: AgentConfig, library: AgentModelLibrary] }>()
const motion = useAgentMotion()
const purposes = ['text', 'image'] as const
type SettingsTab = 'text' | 'image' | 'usage'
const tabs = [{ id: 'text', label: '对话配置', icon: 'chat' }, { id: 'image', label: '生图配置', icon: 'photograph' }, { id: 'usage', label: '用量统计', icon: 'chart' }] as const
const activeTab = ref<SettingsTab>('text'), navigation = ref<HTMLElement>()
function selectTab(tab: SettingsTab) { activeTab.value = tab; loadError.value = '' }
function onTabKeydown(event: KeyboardEvent, index: number) {
  if (!['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const target = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (index + (event.key === 'ArrowUp' || event.key === 'ArrowLeft' ? -1 : 1) + tabs.length) % tabs.length
  selectTab(tabs[target]!.id)
  void nextTick(() => navigation.value?.querySelectorAll<HTMLButtonElement>('[role=tab]')[target]?.focus())
}

const dialog = ref<HTMLDialogElement>(), draft = reactive<AgentConfig>({ ...props.config })
const draftLibrary = ref(emptyAgentModelLibrary()), activePurpose = ref<AgentModelPurpose>('text'), search = ref(''), loadError = ref(''), validating = ref(false)
const available = reactive<{ text: string[]; image: string[] }>({ text: [], image: [] }), fetching = reactive({ text: false, image: false }), loadedKey = reactive({ text: 0, image: 0 })
const importDialog = ref<HTMLDialogElement>(), importing = ref(false), checked = ref<string[]>([])
const importError = ref('')
async function openImport(purpose: AgentModelPurpose) {
  activePurpose.value = purpose; search.value = ''; checked.value = [...added(purpose)]; importError.value = ''
  importing.value = true
  await nextTick(); importDialog.value?.showModal(); motion.reveal(importDialog.value)
  await fetchModels(purpose)
}
function closeImport() { motion.cancel(importDialog.value); importing.value = false; importDialog.value?.close() }
function toggleModel(model: string, event: Event) {
  const input = event.target as HTMLInputElement, enabled = input.checked
  if (!enabled) checked.value = checked.value.filter(item => item !== model)
  else if (checked.value.length < 20) checked.value = [...checked.value, model]
  else { input.checked = false; importError.value = '每个 key 最多添加 20 个模型。' }
}
function selectAll() { checked.value = available[activePurpose.value].slice(0, 20); importError.value = available[activePurpose.value].length > 20 ? '已选前 20 个模型，可按需调整。' : '' }
function applyImport() {
  const purpose = activePurpose.value
  if (fetching[purpose] || loadedKey[purpose] !== keyFor(purpose) || checked.value.length > 20) return
  const models = checked.value.filter(model => available[purpose].includes(model))
  draftLibrary.value = withAgentModels(draftLibrary.value, purpose, keyFor(purpose), models)
  if (purpose === 'text' && !models.includes(draft.model)) draft.model = models[0] || ''
  if (purpose === 'image' && !models.includes(draft.image_model || '')) draft.image_model = models[0] || ''
  closeImport()
}
const requests: Partial<Record<AgentModelPurpose, AbortController>> = {}
let generation = 0
function keyFor(purpose: AgentModelPurpose) { return purpose === 'text' ? draft.model_api_key_id : draft.image_api_key_id || 0 }
function added(purpose: AgentModelPurpose) { return agentModelsForKey(draftLibrary.value, purpose, keyFor(purpose)) }
watch(activePurpose, () => { search.value = '' })
const filteredModels = computed(() => available[activePurpose.value].filter(model => model.toLowerCase().includes(search.value.trim().toLowerCase())))
const valid = computed(() => props.textKeys.some(key => key.id === draft.model_api_key_id && !key.disabled_reason) && added('text').length > 0 && (!draft.image_api_key_id || (draft.image_api_key_id !== draft.model_api_key_id && props.imageKeys.some(key => key.id === draft.image_api_key_id) && added('image').length > 0)))
function changeKey(purpose: AgentModelPurpose, key: number) {
  requests[purpose]?.abort(); fetching[purpose] = false; loadedKey[purpose] = 0; available[purpose] = []; loadError.value = ''; activePurpose.value = purpose
  if (purpose === 'text') { draft.model_api_key_id = key; draft.model = added(purpose)[0] || '' }
  else { draft.image_api_key_id = key; draft.image_model = added(purpose)[0] || '' }
}
function removeModel(purpose: AgentModelPurpose, model: string) {
  draftLibrary.value = withAgentModels(draftLibrary.value, purpose, keyFor(purpose), added(purpose).filter(item => item !== model))
  if (purpose === 'text' && draft.model === model) draft.model = added(purpose)[0] || ''
  if (purpose === 'image' && draft.image_model === model) draft.image_model = added(purpose)[0] || ''
}
async function fetchModels(purpose: AgentModelPurpose): Promise<boolean> {
  const key = keyFor(purpose), token = generation
  if (!key) return false
  requests[purpose]?.abort()
  const controller = new AbortController(); requests[purpose] = controller; fetching[purpose] = true; loadError.value = ''
  try {
    const models = await agentAPI.models(key, purpose, controller.signal)
    if (token !== generation || !props.show || controller.signal.aborted || key !== keyFor(purpose)) return false
    available[purpose] = [...new Set(models)]; loadedKey[purpose] = key
    if (importing.value && activePurpose.value === purpose) checked.value = checked.value.filter(model => available[purpose].includes(model)).slice(0, 20)
    return true
  } catch {
    if (token === generation && !controller.signal.aborted) loadError.value = '模型列表读取失败，请重新获取。'
    return false
  } finally { if (requests[purpose] === controller) fetching[purpose] = false }
}
async function save() {
  if (!valid.value || props.busy || props.saving || validating.value) return
  const token = generation
  validating.value = true; loadError.value = ''
  try {
    for (const purpose of purposes) {
      if (!keyFor(purpose)) continue
      if (loadedKey[purpose] !== keyFor(purpose) && !await fetchModels(purpose)) { activeTab.value = purpose; return }
      if (token !== generation || !props.show) return
      const missing = added(purpose).filter(model => !available[purpose].includes(model))
      if (missing.length) { activeTab.value = purpose; activePurpose.value = purpose; loadError.value = `此 key 不支持已添加的模型：${missing.slice(0, 3).join('、')}。请移除或重新获取。`; return }
    }
    if (token !== generation || !props.show) return
    const value = { ...draft, model: added('text').includes(draft.model) ? draft.model : added('text')[0] || '', image_api_key_id: draft.image_api_key_id || 0, image_model: draft.image_api_key_id ? (added('image').includes(draft.image_model || '') ? draft.image_model : added('image')[0]) : '' }
    emit('save', value, normalizeAgentModelLibrary(draftLibrary.value))
  } finally { if (token === generation) validating.value = false }
}
watch(() => props.show, async show => {
  ++generation; requests.text?.abort(); requests.image?.abort(); validating.value = false
  if (!show) { motion.cancel(dialog.value?.querySelector('form')); closeImport(); dialog.value?.close(); return }
  Object.assign(draft, props.config, { image_api_key_id: props.config.image_api_key_id || 0, image_model: props.config.image_model || '' })
  draftLibrary.value = withCurrentAgentModels(normalizeAgentModelLibrary(props.library), draft)
  activeTab.value = 'text'; activePurpose.value = 'text'; search.value = ''; loadError.value = ''; available.text = []; available.image = []; loadedKey.text = 0; loadedKey.image = 0; fetching.text = false; fetching.image = false
  await nextTick(); dialog.value?.showModal(); motion.reveal(dialog.value?.querySelector('form'))
})
function trapFocus(event: KeyboardEvent) {
  const controls = Array.from(dialog.value?.querySelectorAll<HTMLElement>('button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), [tabindex="0"]') || []).filter(el => el.getClientRects().length > 0)
  const first = controls[0], last = controls[controls.length - 1]
  if (!first) { event.preventDefault(); dialog.value?.focus(); return }
  if (event.shiftKey && (document.activeElement === first || document.activeElement === dialog.value)) { event.preventDefault(); last?.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}
function onCancel(event: Event) { event.preventDefault(); if (!props.saving) emit('close') }
function onBackdrop(event: MouseEvent) { if (event.target !== dialog.value || props.saving) return; const b = dialog.value.getBoundingClientRect(); if (event.clientX < b.left || event.clientX > b.right || event.clientY < b.top || event.clientY > b.bottom) emit('close') }
onBeforeUnmount(() => { ++generation; requests.text?.abort(); requests.image?.abort(); closeImport(); dialog.value?.close() })
</script>
<style scoped>
/* finesse · component: AgentSettings · register=product · tokens: inherited */
.agent-settings-dialog { --settings-bg:#fff; --settings-ink:#202124; --settings-muted:#64646d; --settings-line:#dedee4; --settings-soft:#f4f5f7; --settings-accent:#0674c8; color-scheme:light; width:min(600px,calc(100vw - 32px)); max-height:calc(100dvh - 40px); padding:0; margin:auto; border:1px solid var(--settings-line); border-radius:16px; color:var(--settings-ink); background:var(--settings-bg); box-shadow:0 24px 90px #0003; }
:global(.dark .agent-settings-dialog) { --settings-bg:#232326; --settings-ink:#f5f5f7; --settings-muted:#b0b0b9; --settings-line:#424249; --settings-soft:#303034; --settings-accent:#69aeff; color-scheme:dark; }
.agent-settings-dialog::backdrop { background:#0007; }form,.import-content { display:flex; flex-direction:column; max-height:calc(100dvh - 42px); }header { display:flex; justify-content:space-between; gap:16px; padding:22px 24px 18px; border-bottom:1px solid var(--settings-line); }h2 { font-size:18px; font-weight:600; }header p { font-size:12px; color:var(--settings-muted); line-height:1.7; margin-top:6px; }.close-settings { font-size:24px; min-width:32px; height:32px; border-radius:6px; }@media (hover: hover) {.close-settings:hover { background:var(--settings-soft); } }
.settings-body { padding:0 24px 20px; overflow-y:auto; min-height:0; }.connections { padding:20px 0; min-width:0; }.connection-section + .connection-section { border-top:1px solid var(--settings-line); padding-top:20px; margin-top:24px; }h3 { font-size:14px; font-weight:600; }label { display:block; font-size:12px; margin-top:16px; }select,input[type=search] { display:block; width:100%; min-height:40px; padding:9px 10px; border:1px solid var(--settings-line); border-radius:8px; background:var(--settings-bg); color:var(--settings-ink); font-size:12px; }select { margin-top:8px; text-overflow:ellipsis; }.models-heading { display:flex; justify-content:space-between; margin:16px 0 4px; font-size:11px; color:var(--settings-muted); }.added-models li { display:flex; align-items:center; gap:10px; padding:6px 0; font-size:12px; }.added-models li span { flex:1; overflow-wrap:anywhere; }.added-models button { width:28px; min-height:28px; color:var(--settings-muted); border-radius:5px; }@media (hover: hover) {.added-models button:hover { background:var(--settings-soft); } }.fetch-models { display:inline-flex; align-items:center; gap:6px; margin-top:12px; min-height:36px; padding:7px 12px; border:1px solid var(--settings-line); border-radius:8px; color:var(--settings-accent); font-size:12px; }.field-description,.settings-notice { color:var(--settings-muted); font-size:12px; line-height:1.8; margin-top:12px; }.manage-keys { display:inline-block; margin-top:20px; font-size:12px; color:var(--settings-accent); }.settings-error { color:#bc3d35; font-size:12px; line-height:1.8; margin:12px 0; }:global(.dark .agent-settings-dialog .settings-error) { color:#ffaaa3; }
footer { display:flex; align-items:center; justify-content:flex-end; gap:10px; padding:16px 24px; border-top:1px solid var(--settings-line); }.settings-button { min-height:40px; border-radius:8px; padding:9px 16px; font-size:13px; font-weight:500; }.secondary { border:1px solid var(--settings-line); }.primary { background:var(--settings-accent); color:#fff; }button:disabled,fieldset:disabled { opacity:.5; }button:disabled { cursor:not-allowed; }:where(button,a,input,select):focus-visible { outline:2px solid var(--settings-accent); outline-offset:3px; }
.agent-model-import { width:min(680px,calc(100vw - 24px)); }.import-body { padding-top:20px; }.selection-summary { display:flex; align-items:center; justify-content:space-between; color:var(--settings-muted); font-size:12px; margin:16px 0; }.selection-summary button { color:var(--settings-accent); min-height:32px; margin-left:16px; }.available-models { background:var(--settings-soft); padding:8px; border-radius:12px; max-height:380px; overflow-y:auto; }.available-models label { display:flex; align-items:center; gap:10px; padding:12px; margin:0; cursor:pointer; border-radius:8px; }.available-models label:has(input:checked) { background:color-mix(in srgb,var(--settings-accent) 12%,transparent); }.available-models input { accent-color:var(--settings-accent); width:18px; height:18px; flex-shrink:0; }.available-models span { overflow-wrap:anywhere; }.available-models small { margin-left:auto; white-space:nowrap; color:var(--settings-muted); }.discovery-empty { padding:20px 0; color:var(--settings-muted); font-size:12px; }
@media(max-width:640px) { .agent-settings-dialog { width:calc(100vw - 20px); }header,footer { padding:16px; }.settings-body { padding-left:16px; padding-right:16px; }.available-models { max-height:45dvh; } }

.agent-settings-dialog { box-shadow:0 20px 64px color-mix(in srgb,var(--settings-ink) 16%,transparent); }
header { padding:22px 24px 20px; }
h2 { font-size:17px; }
.connection-section h3 { display:flex; align-items:center; gap:8px; }
.added-models li { padding:8px 0; }
.fetch-models { background:var(--settings-soft); border-color:transparent; }
.settings-button:active:not(:disabled), .fetch-models:active:not(:disabled) { transform:translateY(1px); }
.settings-button { min-width:76px; }
select:focus-visible, input:focus-visible { outline:2px solid var(--settings-accent); outline-offset:2px; }
.settings-notice { padding:10px 12px; background:var(--settings-soft); border-radius:8px; }


.agent-settings-dialog:not(.agent-model-import) { width:min(880px,calc(100vw - 32px)); }
.settings-layout { display:grid; grid-template-columns:160px minmax(0,1fr); min-height:0; overflow:hidden; }
.settings-navigation { display:flex; flex-direction:column; gap:5px; padding:20px 12px; border-right:1px solid var(--settings-line); background:var(--settings-soft); }
.settings-navigation button { display:flex; align-items:center; gap:9px; min-height:42px; padding:10px 12px; border-radius:8px; color:var(--settings-muted); font-size:12px; text-align:left; white-space:nowrap; }
.settings-navigation button[aria-selected=true] { color:var(--settings-ink); background:var(--settings-bg); box-shadow:0 1px 3px color-mix(in srgb,var(--settings-ink) 6%,transparent); font-weight:550; }
.settings-navigation button:active:not(:disabled) { transform:translateY(1px); }
.settings-layout > .settings-body { padding:24px; min-height:0; }
.settings-layout .connections { padding:0; min-height:380px; }
.settings-layout .connection-section + .connection-section { border-top:0; padding-top:0; margin-top:0; }
@media(hover:hover) { .settings-navigation button:hover:not(:disabled):not([aria-selected=true]) { color:var(--settings-ink); background:color-mix(in srgb,var(--settings-bg) 55%,transparent); } }
@media(max-width:640px) {
  .agent-settings-dialog:not(.agent-model-import) { width:calc(100vw - 20px); }
  .settings-layout { grid-template-columns:minmax(0,1fr); }
  .settings-navigation { flex-direction:row; padding:8px; border-right:0; border-bottom:1px solid var(--settings-line); gap:4px; }
  .settings-navigation button { flex:1; justify-content:center; gap:5px; min-height:42px; padding:9px 7px; font-size:12px; }
  .settings-navigation button :deep(svg) { display:none; }
  .settings-layout > .settings-body { padding:20px 16px; min-height:0; }
  .settings-layout .connections { min-height:310px; }
}
</style>
