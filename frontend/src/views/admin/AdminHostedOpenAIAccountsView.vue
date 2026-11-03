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
          <span>平台 / 类型</span>
          <span>专属分组 / 接入</span>
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
              <small v-if="account.user_name !== account.account_email || account.user_email !== account.account_email" :title="account.user_email">
                {{ account.user_name }}<template v-if="account.user_email && account.user_email !== account.account_email"> · {{ account.user_email }}</template>
              </small>
              <div class="identity-meta">
                <code>#{{ account.account_id }}</code>
                <HostedAccountCapacity :concurrency="account.concurrency" />
              </div>
            </div>
          </div>

          <HostedAccountMeta data-label="平台 / 类型" :account="account" />

          <div class="group-cell" data-label="专属分组设置">
            <strong class="ellipsis" :title="account.group_name">{{ account.group_name }}</strong>
            <div class="model-chips" :title="(account.models || [account.model]).join(', ')">
              <code v-for="model in (account.models || [account.model]).slice(0, 2)" :key="model">{{ model }}</code>
              <span v-if="(account.models || [account.model]).length > 2">+{{ (account.models || [account.model]).length - 2 }} 个模型</span>
            </div>
            <div class="connection-meta">
              <span :title="`代理：${account.proxy_name || '直连'}`"><Icon name="globe" size="xs" />{{ account.proxy_name || '直连' }}</span>
              <span class="messages-state" :class="{ enabled: account.supports_messages }">
                {{ account.supports_messages ? '/v1/messages' : 'Messages 关闭' }}
              </span>
            </div>
          </div>

          <HostedAccountUsage data-label="用量窗口" :account="account" />

          <div class="status-stack" data-label="状态">
            <span class="status-chip" :class="statusClass(account)">{{ statusLabel(account) }}</span>
          </div>

          <div class="schedule-cell" data-label="参与账号调度">
            <button
              type="button"
              role="switch"
              class="switch-control"
              :class="{ checked: account.schedulable }"
              :aria-checked="account.schedulable"
              :aria-label="account.schedulable ? '关闭账号调度' : '开启账号调度'"
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
            <h2>编辑托管账号</h2>
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
                <strong>专属分组设置</strong>
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
    replaceAccount(await setAdminHostedOpenAIStatus(account.id, account.schedulable ? 'suspended' : 'active'))
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

onMounted(load)
</script>

<style scoped src="../../styles/hosted-accounts.css"></style>
<style scoped>
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
.spinning { animation: spin .8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (max-width: 560px) {
  .modal-content { margin: 12px; max-height: calc(100dvh - 24px); }
}
</style>
