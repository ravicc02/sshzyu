<template>
  <div ref="historyRoot" class="history-manager">
    <header><h3>对话</h3><button type="button" class="history-search-toggle" aria-label="打开对话搜索" :aria-expanded="searching" @click="toggleSearch"><Icon name="search" size="sm" /></button></header>
    <div class="history-filters"><input v-show="searching || search" ref="searchInput" v-model="search" aria-label="搜索对话" placeholder="搜索对话" @keydown.esc="search = ''; searching = false" /></div>
    <button type="button" class="new-conversation" :disabled="disabled || working" @click="startConversation"><Icon name="edit" size="sm" /><span>新建对话</span></button>
    <p v-if="failure" class="history-error" role="alert">{{ failure }}</p>
    <div v-if="loading" role="status" class="history-empty">正在加载对话…</div>
    <section v-for="section in sections" :key="section.id" class="history-section" :data-history-section="section.id">
      <div class="history-section-heading">
        <button type="button" class="history-section-toggle" :aria-expanded="expanded[section.id]" :aria-controls="`${instanceId}-${section.id}`" @click="toggleSection(section.id)">
          <Icon name="chevronRight" size="xs" class="section-chevron" :class="{ expanded: expanded[section.id] }" />
          <Icon :name="section.icon" size="sm" /><span>{{ section.title }}</span><span class="section-count">{{ section.filtered.length }}</span>
        </button>
        <button v-if="expanded[section.id]" type="button" class="bulk-mode" :aria-label="selecting === section.id ? '结束多选' : '多选对话'" :aria-pressed="selecting === section.id" @click="toggleSelection(section.id)">{{ selecting === section.id ? '完成' : '多选' }}</button>
      </div>
      <Transition :css="false" @enter="motion.enter" @enter-cancelled="motion.cancel"><div v-if="expanded[section.id]" :id="`${instanceId}-${section.id}`" class="history-section-content">
        <div v-if="selecting === section.id" class="bulk-actions"><label><input type="checkbox" :checked="allChecked(section.id)" @change="selectAll(section.id)" />全选</label><span>{{ checked.length }} 项</span><button type="button" :disabled="!checked.length || working" @click="manage(checked, section.id === 'archived' ? 'restore' : 'archive')">{{ section.id === 'archived' ? '取消归档' : '归档' }}</button><button type="button" :disabled="!checked.length || working" @click="askDelete(checked)">删除</button></div>
        <div v-for="item in section.visible" :key="item.id" class="history-row" :class="{ selected: selected === item.id }">
          <input v-if="selecting === section.id" v-model="checked" type="checkbox" :value="item.id" :aria-label="`选择对话 ${item.title}`" />
          <button type="button" class="conversation-open agent-conversation" :title="`${item.title} · ${formatDate(item.updated_at)}`" :aria-current="selected === item.id ? 'true' : undefined" :disabled="disabled" @click="openConversation(item.id)"><span :title="item.title">{{ item.title }}</span><time class="sr-only" :datetime="item.updated_at">{{ formatDate(item.updated_at) }}</time></button>
          <details v-if="selecting !== section.id" class="conversation-menu" @toggle="menuToggled"><summary :aria-label="`管理对话 ${item.title}`">···</summary><div class="menu-actions"><button type="button" @click="rename(item)"><Icon name="edit" size="sm" aria-hidden="true" />重命名</button><button type="button" :disabled="working" @click="manage([item.id], item.archived_at ? 'restore' : 'archive')"><Icon name="document" size="sm" aria-hidden="true" />{{ item.archived_at ? '取消归档' : '归档' }}</button><button type="button" :disabled="working" @click="askDelete([item.id])"><Icon name="trash" size="sm" aria-hidden="true" />删除对话</button></div></details>
        </div>
        <button v-if="section.filtered.length > section.visible.length" type="button" class="load-more" @click="limits[section.id] += 20">加载更多</button>
        <p v-if="!loading && !section.visible.length" class="history-empty">{{ search ? '没有匹配的对话' : section.id === 'archived' ? '暂无归档对话' : '从一个问题开始，对话会保存在这里。' }}</p>
      </div></Transition>
    </section>
    <dialog ref="renameDialog" class="history-dialog" aria-labelledby="rename-title" @cancel="closeRename">
      <form @submit.prevent="submitRename"><h3 id="rename-title">重命名对话</h3><input v-model="title" aria-label="对话名称" maxlength="100" required autofocus /><p v-if="dialogError" role="alert" class="history-error">{{ dialogError }}</p><footer><button type="button" :disabled="working" @click="closeRename">取消</button><button type="submit" :disabled="!title.trim() || working">保存</button></footer></form>
    </dialog>
    <dialog ref="deleteDialog" role="alertdialog" class="history-dialog" aria-labelledby="delete-title" aria-describedby="delete-description" @cancel="closeDelete">
      <h3 id="delete-title">删除 {{ deleteIds.length }} 个对话？</h3><p id="delete-description">删除后将从对话列表移除。已有用量和费用记录会保留。</p><p v-if="dialogError" role="alert" class="history-error">{{ dialogError }}</p><footer><button type="button" :disabled="working" autofocus @click="closeDelete">取消</button><button type="button" class="delete-confirm" :disabled="working" @click="confirmDelete">{{ working ? '正在删除…' : '删除对话' }}</button></footer>
    </dialog>
  </div>
</template>
<script setup lang="ts">
import { computed, nextTick, reactive, ref, shallowRef, useId, watch } from 'vue'
import { onClickOutside, onKeyStroke } from '@vueuse/core'
import Icon from '@/components/icons/Icon.vue'
import { useAgentMotion } from '@/composables/useAgentMotion'
import { agentAPI, type AgentConversation } from '@/api/agent'
const props = defineProps<{ items: AgentConversation[]; selected?: string; disabled?: boolean; loading?: boolean }>()
const emit = defineEmits<{ open: [id: string]; changed: [items: AgentConversation[]]; new: [] }>()
const motion = useAgentMotion()
type SectionID = 'recent' | 'archived'
const instanceId = `agent-history-${useId()}`
const historyRoot = ref<HTMLElement>(), activeMenu = shallowRef<HTMLDetailsElement>()
const searching = ref(false), searchInput = ref<HTMLInputElement>(), search = ref('')
const expanded = reactive<Record<SectionID, boolean>>({ recent: true, archived: false })
const limits = reactive<Record<SectionID, number>>({ recent: 20, archived: 20 })
const selecting = ref<SectionID>(), checked = ref<string[]>([])
const working = ref(false), failure = ref(''), dialogError = ref(''), title = ref(''), renameId = ref(''), deleteIds = ref<string[]>([])
const renameDialog = ref<HTMLDialogElement>(), deleteDialog = ref<HTMLDialogElement>()
const sections = computed(() => (['recent', 'archived'] as const).map(id => {
  const filtered = props.items.filter(item => !!item.archived_at === (id === 'archived') && item.title.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase()))
  return { id, title: id === 'recent' ? '最近对话' : '已归档', icon: id === 'recent' ? 'chatBubble' as const : 'document' as const, filtered, visible: filtered.slice(0, limits[id]) }
}))
function closeMenus(except?: HTMLDetailsElement) {
  historyRoot.value?.querySelectorAll<HTMLDetailsElement>('.conversation-menu[open]').forEach(menu => { if (menu !== except) { motion.cancel(menu.querySelector('.menu-actions')); menu.open = false } })
  if (!except) activeMenu.value = undefined
}
function menuToggled(event: Event) {
  const menu = event.target as HTMLDetailsElement
  if (menu.open) { closeMenus(menu); activeMenu.value = menu; motion.reveal(menu.querySelector('.menu-actions')) }
  else { motion.cancel(menu.querySelector('.menu-actions')); if (activeMenu.value === menu) activeMenu.value = undefined }
}
onClickOutside(activeMenu, () => closeMenus())
onKeyStroke('Escape', event => {
  const menu = activeMenu.value
  if (!menu?.open) return
  event.preventDefault(); closeMenus(); menu.querySelector('summary')?.focus()
})
watch([() => props.selected, () => props.disabled], () => closeMenus())
watch(search, () => {
  closeMenus(); limits.recent = 20; limits.archived = 20; checked.value = []
  if (search.value.trim()) for (const section of sections.value) if (section.filtered.length) expanded[section.id] = true
})
async function toggleSearch() { closeMenus(); searching.value = !searching.value; if (searching.value) { await nextTick(); searchInput.value?.focus() } else search.value = '' }
function toggleSection(id: SectionID) { closeMenus(); expanded[id] = !expanded[id]; if (!expanded[id] && selecting.value === id) { selecting.value = undefined; checked.value = [] } }
function toggleSelection(id: SectionID) { closeMenus(); selecting.value = selecting.value === id ? undefined : id; checked.value = [] }
function allChecked(id: SectionID) { const visible = sections.value.find(section => section.id === id)!.visible; return visible.length > 0 && visible.every(item => checked.value.includes(item.id)) }
function selectAll(id: SectionID) { checked.value = allChecked(id) ? [] : sections.value.find(section => section.id === id)!.visible.map(item => item.id) }
function startConversation() { closeMenus(); emit('new') }
function openConversation(id: string) { closeMenus(); emit('open', id) }
function formatDate(value: string) { return new Date(value).toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' }) }
function rename(item: AgentConversation) { closeMenus(); renameId.value = item.id; title.value = item.title; dialogError.value = ''; renameDialog.value?.showModal() }
function closeRename() { if (!working.value) renameDialog.value?.close() }
function askDelete(ids: string[]) { closeMenus(); deleteIds.value = [...ids]; dialogError.value = ''; deleteDialog.value?.showModal() }
function closeDelete() { if (!working.value) deleteDialog.value?.close() }
async function manage(ids: string[], action: 'archive' | 'restore' | 'delete' | 'rename', nextTitle = '', dialog = false) {
  if (working.value || props.disabled) return false
  working.value = true; failure.value = ''; dialogError.value = ''; closeMenus()
  try { const items = await agentAPI.manage([...ids], action, nextTitle); emit('changed', items); checked.value = []; return true }
  catch (e) { const message = e instanceof Error ? e.message : '操作失败，请稍后重试。'; if (dialog) dialogError.value = message; else failure.value = message; return false }
  finally { working.value = false }
}
async function submitRename() { if (title.value.trim() && await manage([renameId.value], 'rename', title.value.trim(), true)) closeRename() }
async function confirmDelete() { if (await manage(deleteIds.value, 'delete', '', true)) closeDelete() }
</script>
<style scoped>
/* finesse · component: AgentHistory · register=product · tokens: inherited */
.history-manager { display:flex; flex-direction:column; min-height:100%; color:var(--workspace-ink); }
header { display:flex; align-items:center; justify-content:space-between; padding:0 8px 14px; }
h3 { font-size:15px; font-weight:600; }
.history-search-toggle { display:flex; align-items:center; justify-content:center; width:30px; height:30px; color:var(--workspace-muted); border-radius:6px; }
@media (hover: hover) {
.history-search-toggle:hover { background:var(--workspace-soft); color:var(--workspace-ink); } }
.history-filters input { width:calc(100% - 12px); margin:0 6px 10px; min-height:36px; padding:8px 10px; color:var(--workspace-ink); background:var(--workspace-surface); border:1px solid var(--workspace-divider); border-radius:8px; font-size:12px; }
.new-conversation { display:flex; align-items:center; gap:11px; min-height:44px; padding:10px 12px; margin-bottom:8px; border-radius:9px; text-align:left; font-size:13px; }
@media (hover: hover) {
.new-conversation:hover { background:var(--workspace-soft); } }
.new-conversation svg { color:var(--workspace-accent); }
.history-section + .history-section { margin-top:10px; }
.history-section-heading { display:flex; align-items:center; gap:4px; min-height:42px; padding:0 6px; color:var(--workspace-muted); }
.history-section-toggle { display:flex; align-items:center; gap:8px; flex:1; min-width:0; min-height:36px; padding:6px 3px; text-align:left; font-size:12px; border-radius:6px; }
@media (hover: hover) {
.history-section-toggle:hover { color:var(--workspace-ink); } }
.section-chevron { flex-shrink:0; }
.section-chevron.expanded { transform:rotate(90deg); }
.section-count { margin-left:auto; font-size:10px; font-variant-numeric:tabular-nums; }
.bulk-mode { min-height:28px; padding:4px; font-size:10px; }
.bulk-actions { display:flex; align-items:center; flex-wrap:wrap; gap:8px; padding:8px; font-size:11px; }
.bulk-actions label { display:flex; align-items:center; gap:5px; }
.bulk-actions span { margin-right:auto; color:var(--workspace-muted); }
.bulk-actions button { min-height:28px; padding:4px; }
.history-row { display:flex; align-items:center; gap:4px; padding:0 6px 0 10px; margin:2px 0; border:1px solid transparent; border-radius:11px; }
@media (hover: hover) {
.history-row:hover { background:var(--workspace-soft); } }
.history-row.selected { background:var(--workspace-surface); border-color:var(--workspace-divider); box-shadow:0 1px 2px #00000005; }
.conversation-open { flex:1; min-width:0; min-height:40px; padding:10px 3px; text-align:left; }
.conversation-open > span { display:block; overflow:hidden; white-space:nowrap; text-overflow:ellipsis; font-size:12px; line-height:1.6; }
.conversation-menu { position:relative; flex-shrink:0; }
.conversation-menu summary { display:flex; align-items:center; justify-content:center; min-width:25px; min-height:30px; cursor:pointer; list-style:none; color:var(--workspace-muted); font-size:16px; border-radius:5px; }
.conversation-menu summary::-webkit-details-marker { display:none; }
.menu-actions { position:absolute; z-index:20; top:32px; right:0; min-width:118px; padding:4px; border:1px solid var(--workspace-divider); border-radius:8px; background:var(--workspace-surface, #fff); color:var(--workspace-ink); box-shadow:0 4px 12px #0002; }
.menu-actions button { display:block; width:100%; min-height:34px; padding:8px; text-align:left; font-size:12px; border-radius:5px; }
@media (hover: hover) {
.menu-actions button:hover { background:var(--workspace-soft); } }
.load-more { min-height:36px; margin:8px 12px; font-size:12px; color:var(--workspace-muted); }
.history-empty { padding:18px 12px; font-size:12px; line-height:1.8; color:var(--workspace-muted); }
.history-error { color:#bc3d35; padding:8px; font-size:12px; }
.history-dialog { width:min(400px,90vw); margin:auto; padding:24px; color:var(--workspace-ink); background:var(--workspace-surface); border:1px solid var(--workspace-divider); border-radius:12px; }
.history-dialog::backdrop { background:#0008; }
.history-dialog h3 { margin-bottom:16px; font-size:16px; }
.history-dialog p { font-size:13px; line-height:1.7; }
.history-dialog input { width:100%; padding:10px; border:1px solid var(--workspace-divider); border-radius:6px; background:transparent; font-size:14px; }
.history-dialog footer { display:flex; justify-content:flex-end; gap:12px; margin-top:24px; }
.history-dialog footer button { padding:8px 14px; border:1px solid var(--workspace-divider); border-radius:6px; font-size:13px; }
.delete-confirm { color:#bc3d35; }
button:disabled { opacity:.45; cursor:not-allowed; }
button:focus-visible, summary:focus-visible, input:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:2px; }
@media(hover:hover) { .conversation-menu > summary { opacity:0; }.history-row:hover .conversation-menu > summary,.history-row:focus-within .conversation-menu > summary,.conversation-menu[open] > summary { opacity:1; } }

.history-section-toggle { padding:6px 5px; }
.section-chevron { transition:transform 160ms ease-out; }
.history-row { border-radius:9px; }
.history-row.selected { background:var(--workspace-selected); border-color:transparent; box-shadow:none; }
.history-row.selected .conversation-open > span { font-weight:500; }
.menu-actions { min-width:136px; padding:5px; border-radius:10px; box-shadow:0 8px 24px color-mix(in srgb,var(--workspace-ink) 10%,transparent); }
.menu-actions button { display:flex; align-items:center; gap:9px; border-radius:6px; }
.menu-actions button svg { color:var(--workspace-muted); flex-shrink:0; }
.history-search-toggle:active, .new-conversation:active, .menu-actions button:active { transform:translateY(1px); }
.bulk-mode { border-radius:5px; }
@media (hover: hover) {
.bulk-mode:hover { color:var(--workspace-ink); background:var(--workspace-soft); } }
@media(prefers-reduced-motion:reduce) { .section-chevron { transition:none; } }

</style>
