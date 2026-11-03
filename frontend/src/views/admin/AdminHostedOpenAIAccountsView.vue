<template>
  <AppLayout>
    <main class="admin-hosted-page">
      <section class="filter-bar">
        <label class="search-box">
          <Icon name="search" size="sm" />
          <input v-model.trim="search" type="search" placeholder="用户、邮箱、分组" />
        </label>
        <select v-model="statusFilter" class="input status-select">
          <option value="">全部状态</option>
          <option value="active">运行中</option>
          <option value="suspended">已停用</option>
          <option value="archived">已归档</option>
        </select>
        <span class="filter-meta">{{ filteredAccounts.length }} 条记录</span>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
          <Icon name="refresh" size="sm" :class="{ spinning: loading }" />
          刷新
        </button>
      </section>

      <section v-if="loading && accounts.length === 0" class="hosted-empty">
        <span class="loading-ring" />
      </section>
      <section v-else-if="filteredAccounts.length === 0" class="hosted-empty">
        <Icon name="globe" size="xl" />
        <h2>暂无托管账号</h2>
      </section>

      <section v-else class="hosted-list">
        <div class="list-head" aria-hidden="true">
          <span>账号</span>
          <span>ID / 容量</span>
          <span>平台 / 类型</span>
          <span>分组 / 模型</span>
          <span>代理</span>
          <span>用量窗口</span>
          <span>状态</span>
          <span>调度</span>
          <span class="actions-col">操作</span>
        </div>
        <article v-for="account in filteredAccounts" :key="account.id" class="hosted-row">
          <div class="identity-cell">
            <div class="avatar">{{ initials(account) }}</div>
            <div class="identity-copy">
              <strong :title="account.account_name || account.account_email">{{ account.account_name || account.account_email }}</strong>
              <span :title="account.account_email">{{ account.account_email }}</span>
              <small :title="account.user_email">{{ account.user_name }} · {{ account.user_email }}</small>
            </div>
          </div>

          <div class="account-id-cell">
            <code>#{{ account.account_id }}</code>
            <HostedAccountCapacity :concurrency="account.concurrency" />
          </div>

          <HostedAccountMeta :account="account" />

          <div class="group-cell">
            <strong class="ellipsis" :title="account.group_name">{{ account.group_name }}</strong>
            <div class="model-chips" :title="(account.models || [account.model]).join(', ')">
              <code v-for="model in (account.models || [account.model]).slice(0, 3)" :key="model">{{ model }}</code>
              <span v-if="(account.models || [account.model]).length > 3">+{{ (account.models || [account.model]).length - 3 }}</span>
            </div>
          </div>

          <div class="proxy-cell">
            <strong>{{ account.proxy_name || '直连' }}</strong>
            <span class="messages-state" :class="{ enabled: account.supports_messages }">
              {{ account.supports_messages ? '/v1/messages' : 'Messages 关闭' }}
            </span>
          </div>

          <HostedAccountUsage :account="account" />

          <div class="status-stack">
            <span class="status-chip" :class="statusClass(account)">{{ statusLabel(account) }}</span>
            <small>{{ schedulingLabel(account) }}</small>
          </div>

          <div class="schedule-cell">
            <button
              type="button"
              role="switch"
              class="switch-control"
              :class="{ checked: account.schedulable }"
              :aria-checked="account.schedulable"
              :disabled="busyID === account.id || account.status === 'archived'"
              :title="account.schedulable ? '关闭账号调度' : '开启账号调度'"
              @click="toggleStatus(account)"
            >
              <span />
            </button>
          </div>

          <div class="row-actions">
            <button type="button" class="icon-button" title="刷新额度" :disabled="busyID === account.id" @click="refreshQuota(account)">
              <Icon name="refresh" size="sm" />
            </button>
            <button type="button" class="icon-button" title="编辑" @click="openEdit(account)">
              <Icon name="edit" size="sm" />
            </button>
            <button
              v-if="account.status !== 'archived'"
              type="button"
              class="icon-button danger"
              title="归档"
              :disabled="busyID === account.id"
              @click="archive(account)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </article>
      </section>
    </main>

    <div v-if="editing" class="modal-overlay" @click.self="editing = null">
      <div class="modal-content hosted-modal w-full">
        <header class="modal-head">
          <div>
            <h2>托管配置</h2>
            <p>{{ editing.user_name }} · {{ editing.account_email }}</p>
          </div>
          <button type="button" class="icon-button" title="关闭" @click="editing = null">
            <Icon name="x" size="sm" />
          </button>
        </header>
        <div class="modal-body form-grid">
          <label class="field">
            <span>账号名称</span>
            <input v-model.trim="editForm.account_name" class="input" />
          </label>
          <label class="field">
            <span>代理</span>
            <select v-model="editForm.proxy_id" class="input">
              <option :value="null">直连</option>
              <option v-for="proxy in options.proxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }}</option>
            </select>
          </label>
          <section class="settings-section">
            <header class="settings-section-head">
              <span class="section-icon"><Icon name="users" size="sm" /></span>
              <div>
                <strong>自动分配订阅分组</strong>
                <p>这些配置直接作用于该托管账号对应的专属分组。</p>
              </div>
            </header>
            <label class="field">
              <span>模型白名单</span>
              <ModelWhitelistSelector
                v-model="editForm.models"
                platform="openai"
                :show-sync-button="false"
              />
            </label>
            <label class="switch-row group-switch">
              <span>
                <strong>支持 /v1/messages</strong>
                <small>允许 Claude 协议请求路由到该订阅分组</small>
              </span>
              <input v-model="editForm.supports_messages" type="checkbox" class="switch-input" />
            </label>
          </section>
        </div>
        <footer class="modal-foot">
          <button type="button" class="btn btn-secondary" @click="editing = null">取消</button>
          <button type="button" class="btn btn-primary" :disabled="saving || !editForm.account_name || editForm.models.length === 0" @click="saveEdit">
            {{ saving ? '保存中' : '保存' }}
          </button>
        </footer>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import HostedAccountCapacity from '@/components/hosted/HostedAccountCapacity.vue'
import HostedAccountMeta from '@/components/hosted/HostedAccountMeta.vue'
import HostedAccountUsage from '@/components/hosted/HostedAccountUsage.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  getAdminHostedOpenAIOptions,
  listAdminHostedOpenAIAccounts,
  refreshAdminHostedOpenAIQuota,
  setAdminHostedOpenAIStatus,
  updateAdminHostedOpenAIAccount,
  type HostedOpenAIAccount,
  type HostedOpenAIOptions
} from '@/api/hostedOpenAIAccounts'

const appStore = useAppStore()
const loading = ref(true)
const accounts = ref<HostedOpenAIAccount[]>([])
const options = ref<HostedOpenAIOptions>({ proxies: [], suggested_models: [] })
const search = ref('')
const statusFilter = ref('')
const busyID = ref<number | null>(null)
const editing = ref<HostedOpenAIAccount | null>(null)
const saving = ref(false)
const editForm = reactive({
  account_name: '',
  models: [] as string[],
  proxy_id: null as number | null,
  supports_messages: false
})

const filteredAccounts = computed(() => {
  const query = search.value.toLowerCase()
  return accounts.value.filter((account) => {
    if (statusFilter.value && account.status !== statusFilter.value) return false
    if (!query) return true
    return [
      account.user_name,
      account.user_email,
      account.account_email,
      account.account_name,
      account.group_name,
      ...(account.models || [account.model])
    ]
      .join(' ')
      .toLowerCase()
      .includes(query)
  })
})

async function load() {
  loading.value = true
  try {
    const [items, hostedOptions] = await Promise.all([listAdminHostedOpenAIAccounts(), getAdminHostedOpenAIOptions()])
    accounts.value = items
    options.value = hostedOptions
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '加载托管账号失败'))
  } finally {
    loading.value = false
  }
}

function openEdit(account: HostedOpenAIAccount) {
  editing.value = account
  editForm.account_name = account.account_name || account.account_email
  editForm.models = [...(account.models?.length ? account.models : [account.model])]
  editForm.proxy_id = account.proxy_id ?? null
  editForm.supports_messages = account.supports_messages
}

async function saveEdit() {
  if (!editing.value) return
  saving.value = true
  try {
    replaceAccount(
      await updateAdminHostedOpenAIAccount(editing.value.id, {
        account_name: editForm.account_name,
        models: editForm.models,
        proxy_id: editForm.proxy_id,
        supports_messages: editForm.supports_messages
      })
    )
    editing.value = null
    appStore.showSuccess?.('配置已保存')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '保存配置失败'))
  } finally {
    saving.value = false
  }
}

async function refreshQuota(account: HostedOpenAIAccount) {
  busyID.value = account.id
  try {
    replaceAccount(await refreshAdminHostedOpenAIQuota(account.id))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '刷新额度失败'))
  } finally {
    busyID.value = null
  }
}

async function toggleStatus(account: HostedOpenAIAccount) {
  busyID.value = account.id
  try {
    replaceAccount(await setAdminHostedOpenAIStatus(account.id, account.status === 'active' ? 'suspended' : 'active'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '更新状态失败'))
  } finally {
    busyID.value = null
  }
}

async function archive(account: HostedOpenAIAccount) {
  busyID.value = account.id
  try {
    replaceAccount(await setAdminHostedOpenAIStatus(account.id, 'archived'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '归档失败'))
  } finally {
    busyID.value = null
  }
}

function replaceAccount(updated: HostedOpenAIAccount) {
  const index = accounts.value.findIndex((item) => item.id === updated.id)
  if (index >= 0) accounts.value.splice(index, 1, updated)
}

function initials(account: HostedOpenAIAccount) {
  return (account.user_name || account.user_email || 'U').slice(0, 1).toUpperCase()
}

function statusLabel(account: HostedOpenAIAccount) {
  if (account.status === 'archived') return '已归档'
  if (account.status === 'suspended') return '已停用'
  if (account.account_status !== 'active') return '错误'
  return '正常'
}

function statusClass(account: HostedOpenAIAccount) {
  if (account.status === 'active' && account.account_status === 'active') return 'is-active'
  if (account.account_status !== 'active') return 'is-error'
  return 'is-muted'
}

function schedulingLabel(account: HostedOpenAIAccount) {
  if (account.status === 'archived') return '已归档'
  return account.schedulable ? '可调度' : '已关闭'
}

onMounted(load)
</script>

<style scoped>
.admin-hosted-page { width: min(1480px, 100%); margin: 0 auto; padding: 24px 24px 64px; color: var(--text-primary); }
.filter-bar { display: flex; align-items: center; gap: 10px; padding: 12px 14px; border: 1px solid var(--border-color); border-bottom: 0; border-radius: 10px 10px 0 0; background: var(--surface); }
.search-box { display: flex; align-items: center; width: min(440px, 100%); gap: 8px; padding: 0 10px; border: 1px solid var(--border-color); border-radius: 7px; background: var(--surface); color: var(--text-tertiary); }
.search-box input { width: 100%; height: 34px; border: 0; outline: 0; background: transparent; color: var(--text-primary); font-size: 12px; }
.status-select { width: 132px; height: 36px; }
.filter-meta { margin-left: auto; color: var(--text-tertiary); font-size: 10px; white-space: nowrap; }
.hosted-empty { min-height: 300px; display: grid; place-items: center; align-content: center; gap: 14px; border: 1px solid var(--border-color); border-radius: 0 0 10px 10px; background: var(--surface); color: var(--text-tertiary); }
.hosted-empty h2 { font-size: 14px; font-weight: 600; }
.loading-ring { width: 28px; height: 28px; border: 2px solid var(--border-color); border-top-color: var(--primary-500); border-radius: 50%; animation: spin 0.8s linear infinite; }
.hosted-list { overflow: hidden; border: 1px solid var(--border-color); border-radius: 0 0 10px 10px; background: var(--surface); }
.list-head, .hosted-row { display: grid; grid-template-columns: minmax(210px, 1.2fr) 82px minmax(128px, .72fr) minmax(190px, 1.15fr) minmax(92px, .55fr) minmax(190px, 1fr) 76px 54px 112px; align-items: center; gap: 12px; padding: 0 14px; }
.list-head { min-height: 38px; border-bottom: 1px solid var(--border-color); color: var(--text-tertiary); background: var(--surface-secondary); font-size: 10px; font-weight: 650; letter-spacing: .01em; }
.hosted-row { min-height: 116px; border-bottom: 1px solid var(--border-color); transition: background-color .15s ease; }
.hosted-row:last-child { border-bottom: 0; }
.hosted-row:hover { background: color-mix(in srgb, var(--primary-500) 3%, var(--surface)); }
.identity-cell { display: flex; align-items: center; min-width: 0; gap: 10px; }
.avatar { display: grid; place-items: center; width: 32px; height: 32px; flex: 0 0 auto; border-radius: 8px; color: var(--primary-700); background: color-mix(in srgb, var(--primary-500) 10%, var(--surface)); font-size: 12px; font-weight: 700; }
.identity-copy, .group-cell, .proxy-cell, .account-id-cell { display: grid; min-width: 0; gap: 4px; }
.identity-copy strong, .identity-copy span, .identity-copy small, .group-cell > strong, .proxy-cell strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.identity-copy strong { font-size: 12px; font-weight: 650; }
.identity-copy span { color: var(--text-secondary); font-size: 10px; }
.identity-copy small, .status-stack small, .proxy-cell span { color: var(--text-tertiary); font-size: 9px; }
.account-id-cell code { width: fit-content; color: var(--text-secondary); font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 10px; }
.group-cell > strong { max-width: 100%; }
.group-cell > strong, .proxy-cell strong { font-size: 11px; font-weight: 600; }
.model-chips { display: flex; min-width: 0; align-items: center; gap: 3px; overflow: hidden; }
.model-chips code { max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 1px 5px; border-radius: 4px; color: var(--text-secondary); background: var(--surface-secondary); font-size: 9px; }
.model-chips > span { flex: 0 0 auto; color: var(--text-tertiary); font-size: 9px; }
.messages-state { width: fit-content; padding: 1px 5px; border-radius: 4px; color: var(--text-tertiary); background: var(--surface-secondary); font-size: 9px; }
.messages-state.enabled { color: #087a55; background: #e7f8ef; }
.status-stack { display: grid; gap: 4px; }
.status-chip { width: fit-content; padding: 2px 7px; border-radius: 999px; font-size: 10px; font-weight: 600; }
.status-chip.is-active { color: #087a55; background: #e7f8ef; }
.status-chip.is-muted { color: #8a5a00; background: #fff3d8; }
.status-chip.is-error { color: #be123c; background: #ffe4e6; }
.schedule-cell { display: flex; justify-content: center; }
.switch-control { position: relative; width: 34px; height: 20px; padding: 0; border: 0; border-radius: 999px; background: #cbd5e1; transition: background-color .16s ease; }
.switch-control span { position: absolute; top: 2px; left: 2px; width: 16px; height: 16px; border-radius: 50%; background: #fff; box-shadow: 0 1px 3px rgb(15 23 42 / .28); transition: transform .16s ease; }
.switch-control.checked { background: var(--primary-600); }
.switch-control.checked span { transform: translateX(14px); }
.switch-control:disabled { cursor: not-allowed; opacity: .48; }
.row-actions { display: flex; justify-content: flex-end; gap: 5px; }
.actions-col { text-align: right; }
.icon-button { display: grid; place-items: center; width: 30px; height: 30px; border: 1px solid var(--border-color); border-radius: 6px; color: var(--text-secondary); background: var(--surface); }
.icon-button.danger { color: var(--error-600); }
.icon-button:disabled { opacity: 0.45; }
.hosted-modal { max-width: 580px; }
.modal-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; padding: 20px 22px; border-bottom: 1px solid var(--border-color); }
.modal-head h2 { font-size: 17px; font-weight: 650; }
.modal-head p { margin-top: 4px; color: var(--text-tertiary); font-size: 11px; }
.form-grid { display: grid; gap: 15px; padding: 20px 22px; }
.field { display: grid; gap: 7px; }
.field > span { color: var(--text-secondary); font-size: 11px; font-weight: 600; }
.settings-section { display: grid; gap: 13px; padding: 14px; border: 1px solid var(--border-color); border-radius: 9px; background: var(--surface-secondary); }
.settings-section-head { display: flex; align-items: flex-start; gap: 10px; }
.settings-section-head > div { display: grid; gap: 3px; }
.settings-section-head strong { font-size: 12px; font-weight: 650; }
.settings-section-head p { color: var(--text-tertiary); font-size: 10px; line-height: 1.45; }
.section-icon { display: grid; place-items: center; width: 28px; height: 28px; flex: 0 0 auto; border: 1px solid color-mix(in srgb, var(--primary-500) 18%, var(--border-color)); border-radius: 7px; color: var(--primary-600); background: color-mix(in srgb, var(--primary-500) 8%, var(--surface)); }
.switch-row { display: flex; align-items: center; justify-content: space-between; padding: 12px; border: 1px solid var(--border-color); border-radius: 8px; background: var(--surface-secondary); }
.switch-row span { display: grid; gap: 3px; }
.switch-row strong { font-size: 12px; }
.switch-row small { color: var(--text-tertiary); font-size: 10px; }
.group-switch { background: var(--surface); }
.switch-input { width: 34px; height: 19px; accent-color: var(--primary-600); }
.modal-foot { display: flex; justify-content: flex-end; gap: 8px; padding: 14px 22px; border-top: 1px solid var(--border-color); }
.spinning { animation: spin 0.8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
:global(.dark) .status-chip.is-active { color: #9df1ce; background: rgb(4 120 87 / 0.26); }
:global(.dark) .status-chip.is-muted { color: #ffd38a; background: rgb(146 94 0 / 0.25); }
:global(.dark) .status-chip.is-error { color: #fecdd3; background: rgb(159 18 57 / 0.28); }
:global(.dark) .messages-state.enabled { color: #9df1ce; background: rgb(4 120 87 / 0.26); }
@media (max-width: 1200px) and (min-width: 901px) {
  .hosted-list { overflow-x: auto; }
  .list-head, .hosted-row { min-width: 1180px; }
}
@media (max-width: 900px) {
  .hosted-list { overflow: visible; border: 0; border-radius: 0; background: transparent; }
  .list-head { display: none; }
  .hosted-row { min-width: 0; grid-template-columns: minmax(0, 1fr) auto; gap: 14px 18px; margin-bottom: 10px; padding: 16px; border: 1px solid var(--border-color); border-radius: 9px; background: var(--surface); }
  .hosted-row:last-child { border-bottom: 1px solid var(--border-color); }
  .identity-cell { grid-column: 1 / -1; }
  .account-id-cell, .status-stack { align-self: center; }
  .hosted-account-meta, .group-cell, .proxy-cell { min-width: 0; }
  .hosted-row > :nth-child(6) { grid-column: 1 / -1; }
  .schedule-cell { justify-content: flex-end; }
  .row-actions { grid-column: 1 / -1; justify-content: flex-start; }
}
@media (max-width: 700px) {
  .admin-hosted-page { padding: 16px 12px 48px; }
  .filter-bar { flex-wrap: wrap; }
  .search-box, .status-select { width: 100%; }
  .filter-meta { margin-left: 0; }
}
</style>
