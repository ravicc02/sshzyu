<template>
  <AppLayout>
    <main class="admin-hosted-page">
      <header class="admin-head">
        <div>
          <h1>托管管理</h1>
          <p>{{ filteredAccounts.length }} 条记录</p>
        </div>
        <div class="head-actions">
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
            <Icon name="refresh" size="sm" :class="{ spinning: loading }" />
            刷新
          </button>
        </div>
      </header>

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
      </section>

      <section v-if="loading && accounts.length === 0" class="hosted-empty">
        <span class="loading-ring" />
      </section>
      <section v-else-if="filteredAccounts.length === 0" class="hosted-empty">
        <Icon name="globe" size="xl" />
        <h2>暂无托管账号</h2>
      </section>

      <section v-else class="table-shell">
        <table>
          <thead>
            <tr>
              <th>用户 / 账号</th>
              <th>分组 / 模型</th>
              <th>代理</th>
              <th>额度窗口</th>
              <th>状态</th>
              <th class="actions-col">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="account in filteredAccounts" :key="account.id">
              <td>
                <div class="user-cell">
                  <div class="avatar">{{ initials(account) }}</div>
                  <div>
                    <strong>{{ account.user_name }}</strong>
                    <span>{{ account.account_name || account.account_email || account.user_email }}</span>
                    <span>{{ account.account_email || account.user_email }}</span>
                    <small>#{{ account.user_id }} · {{ account.quota.account_type }}</small>
                  </div>
                </div>
              </td>
              <td>
                <div class="stack-cell">
                  <strong :title="account.group_name">{{ account.group_name }}</strong>
                  <code :title="account.models.join(', ')">{{ account.models.join(' · ') }}</code>
                  <span>{{ account.supports_messages ? '/v1/messages' : 'Messages 关闭' }}</span>
                </div>
              </td>
              <td>
                <div class="stack-cell">
                  <strong>{{ account.proxy_name || '直连' }}</strong>
                  <span>账号 #{{ account.account_id }}</span>
                </div>
              </td>
              <td>
                <div class="admin-quota">
                  <QuotaBar label="5h" :window="account.quota.five_hour" />
                  <QuotaBar label="7d" :window="account.quota.weekly" />
                </div>
              </td>
              <td>
                <div class="status-stack">
                  <span class="status-chip" :class="statusClass(account)">{{ statusLabel(account) }}</span>
                  <small>{{ account.account_status }}</small>
                </div>
              </td>
              <td>
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
                    class="icon-button"
                    :title="account.status === 'active' ? '停用' : '恢复'"
                    :disabled="busyID === account.id"
                    @click="toggleStatus(account)"
                  >
                    <Icon :name="account.status === 'active' ? 'pauseCircle' : 'play'" size="sm" />
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
              </td>
            </tr>
          </tbody>
        </table>
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
            <span>模型白名单</span>
            <ModelWhitelistSelector
              v-model="editForm.models"
              platform="openai"
              :show-sync-button="false"
            />
          </label>
          <label class="field">
            <span>代理</span>
            <select v-model="editForm.proxy_id" class="input">
              <option :value="null">直连</option>
              <option v-for="proxy in options.proxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }}</option>
            </select>
          </label>
          <label class="switch-row">
            <span><strong>/v1/messages</strong></span>
            <input v-model="editForm.supports_messages" type="checkbox" class="switch-input" />
          </label>
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
import QuotaBar from '@/components/hosted/QuotaBar.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  getHostedOpenAIOptions,
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
    const [items, hostedOptions] = await Promise.all([listAdminHostedOpenAIAccounts(), getHostedOpenAIOptions()])
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
  if (account.account_status !== 'active') return '需检查'
  return '运行中'
}

function statusClass(account: HostedOpenAIAccount) {
  if (account.status === 'active' && account.account_status === 'active') return 'is-active'
  return 'is-muted'
}

onMounted(load)
</script>

<style scoped>
.admin-hosted-page { width: min(1480px, 100%); margin: 0 auto; padding: 24px 24px 64px; color: var(--text-primary); }
.admin-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 18px; margin-bottom: 16px; }
.admin-head h1 { font-size: 23px; font-weight: 650; line-height: 1.1; }
.admin-head p { margin-top: 6px; color: var(--text-tertiary); font-size: 11px; }
.head-actions { display: flex; gap: 8px; }
.filter-bar { display: flex; gap: 10px; padding: 12px; border: 1px solid var(--border-color); border-bottom: 0; border-radius: 10px 10px 0 0; background: var(--surface); }
.search-box { display: flex; align-items: center; width: min(360px, 100%); gap: 8px; padding: 0 10px; border: 1px solid var(--border-color); border-radius: 7px; background: var(--surface); color: var(--text-tertiary); }
.search-box input { width: 100%; height: 34px; border: 0; outline: 0; background: transparent; color: var(--text-primary); font-size: 12px; }
.status-select { width: 132px; height: 36px; }
.hosted-empty { min-height: 300px; display: grid; place-items: center; align-content: center; gap: 14px; border: 1px solid var(--border-color); border-radius: 0 0 10px 10px; background: var(--surface); color: var(--text-tertiary); }
.hosted-empty h2 { font-size: 14px; font-weight: 600; }
.loading-ring { width: 28px; height: 28px; border: 2px solid var(--border-color); border-top-color: var(--primary-500); border-radius: 50%; animation: spin 0.8s linear infinite; }
.table-shell { overflow: auto; border: 1px solid var(--border-color); border-radius: 0 0 10px 10px; background: var(--surface); }
table { width: 100%; border-collapse: collapse; min-width: 1120px; }
th { padding: 10px 14px; border-bottom: 1px solid var(--border-color); color: var(--text-tertiary); background: var(--surface-secondary); font-size: 10px; font-weight: 600; text-align: left; white-space: nowrap; }
td { padding: 13px 14px; border-bottom: 1px solid var(--border-color); vertical-align: middle; }
tbody tr:last-child td { border-bottom: 0; }
tbody tr:hover { background: color-mix(in srgb, var(--primary-500) 3%, var(--surface)); }
.user-cell { display: flex; align-items: center; min-width: 220px; gap: 10px; }
.avatar { display: grid; place-items: center; width: 32px; height: 32px; flex: 0 0 auto; border-radius: 8px; color: var(--primary-700); background: color-mix(in srgb, var(--primary-500) 10%, var(--surface)); font-size: 12px; font-weight: 700; }
.user-cell > div:last-child, .stack-cell { display: grid; min-width: 0; gap: 3px; }
.user-cell strong, .stack-cell strong { overflow: hidden; max-width: 250px; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; font-weight: 600; }
.user-cell span, .stack-cell span, .user-cell small, .status-stack small { color: var(--text-tertiary); font-size: 10px; }
.stack-cell code { width: fit-content; max-width: 250px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 1px 5px; border-radius: 4px; color: var(--text-secondary); background: var(--surface-secondary); font-size: 10px; }
.admin-quota { display: grid; width: 220px; gap: 10px; }
.status-stack { display: grid; gap: 4px; }
.status-chip { width: fit-content; padding: 2px 7px; border-radius: 999px; font-size: 10px; font-weight: 600; }
.status-chip.is-active { color: #087a55; background: #e7f8ef; }
.status-chip.is-muted { color: #8a5a00; background: #fff3d8; }
.row-actions { display: flex; justify-content: flex-end; gap: 6px; min-width: 144px; }
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
.switch-row { display: flex; align-items: center; justify-content: space-between; padding: 12px; border: 1px solid var(--border-color); border-radius: 8px; background: var(--surface-secondary); }
.switch-row strong { font-size: 12px; }
.switch-input { width: 34px; height: 19px; accent-color: var(--primary-600); }
.modal-foot { display: flex; justify-content: flex-end; gap: 8px; padding: 14px 22px; border-top: 1px solid var(--border-color); }
.spinning { animation: spin 0.8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
:global(.dark) .status-chip.is-active { color: #9df1ce; background: rgb(4 120 87 / 0.26); }
:global(.dark) .status-chip.is-muted { color: #ffd38a; background: rgb(146 94 0 / 0.25); }
@media (max-width: 700px) {
  .admin-hosted-page { padding: 16px 12px 48px; }
  .filter-bar { flex-direction: column; }
  .search-box, .status-select { width: 100%; }
}
</style>
