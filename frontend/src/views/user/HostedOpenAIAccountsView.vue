<template>
  <AppLayout>
    <main class="hosted-page">
      <header class="hosted-toolbar">
        <span class="toolbar-meta">{{ accounts.length }} 个账号 · {{ activeCount }} 个运行中</span>
        <div class="head-actions">
          <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
            <Icon name="refresh" size="sm" :class="{ spinning: loading }" />
            刷新
          </button>
          <button type="button" class="btn btn-primary" @click="openCreate">
            <Icon name="plus" size="sm" />
            添加账号
          </button>
        </div>
      </header>

      <section v-if="loading && accounts.length === 0" class="empty-panel">
        <span class="loading-ring" />
      </section>

      <section v-else-if="accounts.length === 0" class="empty-panel">
        <div class="empty-mark"><Icon name="globe" size="xl" /></div>
        <h2>还没有托管账号</h2>
        <button type="button" class="btn btn-primary" @click="openCreate">
          <Icon name="plus" size="sm" />
          添加
        </button>
      </section>

      <section v-else class="hosted-list">
        <div class="list-head" aria-hidden="true">
          <span>账号</span>
          <span>平台 / 类型</span>
          <span>分组 / 模型</span>
          <span>代理</span>
          <span>用量窗口</span>
          <span>状态</span>
          <span class="actions-col">操作</span>
        </div>
        <article v-for="account in accounts" :key="account.id" class="hosted-row">
          <div class="identity-cell">
            <div class="account-avatar">{{ initials(account) }}</div>
            <div class="identity-copy">
              <strong :title="account.account_name || account.account_email">{{ account.account_name || account.account_email }}</strong>
              <span :title="account.account_email">{{ account.account_email }}</span>
              <small>#{{ account.account_id }} · {{ account.quota.account_type }}</small>
            </div>
          </div>

          <HostedAccountMeta :account="account" />

          <div class="group-cell">
            <strong :title="account.group_name">{{ account.group_name }}</strong>
            <div class="model-chips" :title="account.models.join(', ')">
              <code v-for="model in account.models.slice(0, 3)" :key="model">{{ model }}</code>
              <span v-if="account.models.length > 3">+{{ account.models.length - 3 }}</span>
            </div>
          </div>

          <div class="proxy-cell">
            <strong>{{ account.proxy_name || '直连' }}</strong>
            <span class="messages-state" :class="{ enabled: account.supports_messages }">
              {{ account.supports_messages ? '/v1/messages' : 'Messages 关闭' }}
            </span>
          </div>

          <HostedAccountUsage :account="account" />

          <div class="status-cell">
            <span class="status-chip" :class="statusClass(account)">{{ statusLabel(account) }}</span>
          </div>

          <div class="row-actions">
            <button type="button" class="icon-button" title="刷新额度" :disabled="refreshingID === account.id" @click="refreshQuota(account)">
              <Icon name="refresh" size="sm" :class="{ spinning: refreshingID === account.id }" />
            </button>
            <button type="button" class="icon-button" title="配置" @click="openEdit(account)">
              <Icon name="edit" size="sm" />
            </button>
            <button type="button" class="icon-button" title="重新授权" @click="openReauth(account)">
              <Icon name="key" size="sm" />
            </button>
          </div>
        </article>
      </section>
    </main>

    <div v-if="createOpen" class="modal-overlay" @click.self="closeCreate">
      <div class="modal-content hosted-modal w-full">
        <header class="modal-head">
          <div>
            <h2>添加 OpenAI 托管</h2>
            <p>登录后自动生成专属分组与订阅</p>
          </div>
          <button type="button" class="icon-button" title="关闭" @click="closeCreate">
            <Icon name="x" size="sm" />
          </button>
        </header>

        <div class="modal-body hosted-form">
          <label class="field">
            <span>账号名称</span>
            <input v-model.trim="createForm.account_name" class="input" placeholder="例如 主账号" />
          </label>

          <label class="field">
            <span>代理</span>
            <select v-model="createForm.proxy_id" class="input">
              <option :value="null">直连</option>
              <option v-for="proxy in options.proxies" :key="proxy.id" :value="proxy.id">{{ proxy.name }}</option>
            </select>
          </label>

          <section class="settings-section">
            <header class="settings-section-head">
              <span class="section-icon"><Icon name="users" size="sm" /></span>
              <div>
                <strong>自动分配订阅分组</strong>
                <p>创建后自动生成专属私密分组与订阅，以下选项作用于该分组。</p>
              </div>
            </header>
            <label class="field">
              <span>模型白名单</span>
              <ModelWhitelistSelector
                v-model="createForm.models"
                platform="openai"
                :show-sync-button="false"
              />
            </label>
            <label class="switch-row group-switch">
              <span>
                <strong>支持 /v1/messages</strong>
                <small>允许 Claude 协议请求路由到该订阅分组</small>
              </span>
              <input v-model="createForm.supports_messages" type="checkbox" class="switch-input" />
            </label>
          </section>

          <section class="settings-section auth-section">
            <header class="settings-section-head">
              <span class="section-icon"><Icon name="key" size="sm" /></span>
              <div>
                <strong>OpenAI 授权</strong>
                <p>生成登录链接并完成授权，再粘贴回调地址或 code。</p>
              </div>
            </header>
            <button type="button" class="btn btn-secondary justify-self-start" :disabled="authLoading || !canCreateConfig" @click="generateAuth">
              <Icon :name="authURL ? 'refresh' : 'externalLink'" size="sm" />
              {{ authURL ? '重新生成登录链接' : '生成登录链接' }}
            </button>
            <div v-if="authURL" class="auth-url-panel">
              <input :value="authURL" readonly class="input auth-url-input" aria-label="OpenAI 登录地址" />
              <button type="button" class="icon-button copy-button" :title="copied ? '已复制' : '复制登录地址'" @click="copyAuthURL">
                <Icon :name="copied ? 'check' : 'copy'" size="sm" />
              </button>
              <a :href="authURL" target="_blank" rel="noopener noreferrer" class="auth-link">
                打开登录
                <Icon name="externalLink" size="sm" />
              </a>
            </div>
            <label v-if="sessionID" class="field">
              <span>回调地址或授权码</span>
              <textarea
                v-model.trim="createForm.callback_url"
                class="input callback-input"
                placeholder="粘贴完整回调地址，或直接粘贴 code"
              />
              <small v-if="createCallback.code" class="callback-recognized">
                <Icon name="checkCircle" size="xs" />
                已识别授权码{{ createCallback.state ? '与 state' : '' }}
              </small>
            </label>
          </section>
        </div>

        <footer class="modal-foot">
          <button type="button" class="btn btn-secondary" @click="closeCreate">取消</button>
          <button type="button" class="btn btn-primary" :disabled="creating || !canCreate" @click="createAccount">
            {{ creating ? '创建中' : '完成托管' }}
          </button>
        </footer>
      </div>
    </div>

    <div v-if="editAccount" class="modal-overlay" @click.self="editAccount = null">
      <div class="modal-content hosted-modal w-full">
        <header class="modal-head">
          <div>
            <h2>账号配置</h2>
            <p>{{ editAccount.account_email }}</p>
          </div>
          <button type="button" class="icon-button" title="关闭" @click="editAccount = null">
            <Icon name="x" size="sm" />
          </button>
        </header>
        <div class="modal-body hosted-form">
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
                <p>这些设置直接作用于该托管账号对应的专属分组。</p>
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
          <button type="button" class="btn btn-secondary" @click="editAccount = null">取消</button>
          <button type="button" class="btn btn-primary" :disabled="savingEdit || !editForm.account_name || editForm.models.length === 0" @click="saveEdit">
            {{ savingEdit ? '保存中' : '保存' }}
          </button>
        </footer>
      </div>
    </div>

    <div v-if="reauthAccount" class="modal-overlay" @click.self="closeReauth">
      <div class="modal-content hosted-modal w-full">
        <header class="modal-head">
          <div>
            <h2>重新授权</h2>
            <p>{{ reauthAccount.account_email }}</p>
          </div>
          <button type="button" class="icon-button" title="关闭" @click="closeReauth">
            <Icon name="x" size="sm" />
          </button>
        </header>
        <div class="modal-body hosted-form">
          <section class="settings-section auth-section">
            <header class="settings-section-head">
              <span class="section-icon"><Icon name="key" size="sm" /></span>
              <div>
                <strong>重新授权 OpenAI</strong>
                <p>生成新的登录链接，授权后粘贴回调地址或 code。</p>
              </div>
            </header>
            <button type="button" class="btn btn-secondary justify-self-start" :disabled="authLoading" @click="generateReauth">
              <Icon :name="reauthURL ? 'refresh' : 'externalLink'" size="sm" />
              {{ reauthURL ? '重新生成登录链接' : '生成登录链接' }}
            </button>
            <div v-if="reauthURL" class="auth-url-panel">
              <input :value="reauthURL" readonly class="input auth-url-input" aria-label="OpenAI 重新授权地址" />
              <button type="button" class="icon-button copy-button" :title="copied ? '已复制' : '复制登录地址'" @click="copyReauthURL">
                <Icon :name="copied ? 'check' : 'copy'" size="sm" />
              </button>
              <a :href="reauthURL" target="_blank" rel="noopener noreferrer" class="auth-link">
                打开登录
                <Icon name="externalLink" size="sm" />
              </a>
            </div>
            <label v-if="reauthSessionID" class="field">
              <span>回调地址或授权码</span>
              <textarea
                v-model.trim="reauthCallback"
                class="input callback-input"
                placeholder="粘贴完整回调地址，或直接粘贴 code"
              />
              <small v-if="reauthCallbackParsed.code" class="callback-recognized">
                <Icon name="checkCircle" size="xs" />
                已识别授权码{{ reauthCallbackParsed.state ? '与 state' : '' }}
              </small>
            </label>
          </section>
        </div>
        <footer class="modal-foot">
          <button type="button" class="btn btn-secondary" @click="closeReauth">取消</button>
          <button type="button" class="btn btn-primary" :disabled="reauthing || !canReauth" @click="saveReauth">
            {{ reauthing ? '验证中' : '更新授权' }}
          </button>
        </footer>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  createHostedOpenAIAccount,
  generateHostedOpenAIAuthURL,
  getHostedOpenAIOptions,
  listHostedOpenAIAccounts,
  reauthorizeHostedOpenAIAccount,
  refreshHostedOpenAIQuota,
  updateHostedOpenAIAccount,
  type HostedOpenAIAccount,
  type HostedOpenAIOptions
} from '@/api/hostedOpenAIAccounts'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import HostedAccountMeta from '@/components/hosted/HostedAccountMeta.vue'
import HostedAccountUsage from '@/components/hosted/HostedAccountUsage.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useClipboard } from '@/composables/useClipboard'
import { parseOAuthCallbackInput } from '@/utils/oauthCallback'

const appStore = useAppStore()
const { copied, copyToClipboard } = useClipboard()
const loading = ref(true)
const accounts = ref<HostedOpenAIAccount[]>([])
const options = ref<HostedOpenAIOptions>({ proxies: [], suggested_models: [] })
const refreshingID = ref<number | null>(null)

const createOpen = ref(false)
const creating = ref(false)
const authLoading = ref(false)
const authURL = ref('')
const createAuthState = ref('')
const sessionID = ref('')
const createForm = reactive({
  account_name: '',
  models: [] as string[],
  proxy_id: null as number | null,
  supports_messages: false,
  callback_url: ''
})

const editAccount = ref<HostedOpenAIAccount | null>(null)
const savingEdit = ref(false)
const editForm = reactive({
  account_name: '',
  models: [] as string[],
  proxy_id: null as number | null,
  supports_messages: false
})

const reauthAccount = ref<HostedOpenAIAccount | null>(null)
const reauthURL = ref('')
const reauthAuthState = ref('')
const reauthSessionID = ref('')
const reauthCallback = ref('')
const reauthing = ref(false)

const activeCount = computed(() => accounts.value.filter((item) => item.status === 'active').length)
const canCreateConfig = computed(() => Boolean(createForm.account_name && createForm.models.length > 0))
const createCallback = computed(() => parseOAuthCallbackInput(createForm.callback_url))
const reauthCallbackParsed = computed(() => parseOAuthCallbackInput(reauthCallback.value))
const createCode = computed(() => createCallback.value.code)
const createState = computed(() => createCallback.value.state || createAuthState.value)
const reauthCode = computed(() => reauthCallbackParsed.value.code)
const reauthState = computed(() => reauthCallbackParsed.value.state || reauthAuthState.value)
const canCreate = computed(() =>
  Boolean(sessionID.value && createCode.value && createState.value && canCreateConfig.value)
)
const canReauth = computed(() =>
  Boolean(reauthSessionID.value && reauthCode.value && reauthState.value)
)

async function load() {
  loading.value = true
  try {
    const [items, hostedOptions] = await Promise.all([listHostedOpenAIAccounts(), getHostedOpenAIOptions()])
    accounts.value = items
    options.value = hostedOptions
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '加载托管账号失败'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  createOpen.value = true
  createForm.account_name = ''
  createForm.models = []
  createForm.proxy_id = null
  createForm.supports_messages = false
  createForm.callback_url = ''
  authURL.value = ''
  createAuthState.value = ''
  sessionID.value = ''
}

function closeCreate() {
  if (creating.value) return
  createOpen.value = false
}

async function generateAuth() {
  authLoading.value = true
  try {
    const result = await generateHostedOpenAIAuthURL({ proxy_id: createForm.proxy_id })
    authURL.value = result.auth_url
    createAuthState.value = oauthStateFromURL(result.auth_url)
    sessionID.value = result.session_id
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '生成登录链接失败'))
  } finally {
    authLoading.value = false
  }
}

async function createAccount() {
  creating.value = true
  try {
    const created = await createHostedOpenAIAccount({
      session_id: sessionID.value,
      callback_url: createForm.callback_url,
      code: createCode.value,
      state: createState.value,
      account_name: createForm.account_name,
      models: createForm.models,
      proxy_id: createForm.proxy_id,
      supports_messages: createForm.supports_messages
    })
    accounts.value = [created, ...accounts.value]
    createOpen.value = false
    appStore.showSuccess?.('托管账号已创建')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '创建托管账号失败'))
  } finally {
    creating.value = false
  }
}

function openEdit(account: HostedOpenAIAccount) {
  editAccount.value = account
  editForm.account_name = account.account_name || account.account_email
  editForm.models = [...(account.models?.length ? account.models : [account.model])]
  editForm.proxy_id = account.proxy_id ?? null
  editForm.supports_messages = account.supports_messages
}

async function saveEdit() {
  if (!editAccount.value) return
  savingEdit.value = true
  try {
    const updated = await updateHostedOpenAIAccount(editAccount.value.id, {
      account_name: editForm.account_name,
      models: editForm.models,
      proxy_id: editForm.proxy_id,
      supports_messages: editForm.supports_messages
    })
    replaceAccount(updated)
    editAccount.value = null
    appStore.showSuccess?.('配置已保存')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '保存配置失败'))
  } finally {
    savingEdit.value = false
  }
}

async function refreshQuota(account: HostedOpenAIAccount) {
  refreshingID.value = account.id
  try {
    replaceAccount(await refreshHostedOpenAIQuota(account.id))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '刷新额度失败'))
  } finally {
    refreshingID.value = null
  }
}

function openReauth(account: HostedOpenAIAccount) {
  reauthAccount.value = account
  reauthURL.value = ''
  reauthAuthState.value = ''
  reauthSessionID.value = ''
  reauthCallback.value = ''
}

function closeReauth() {
  if (reauthing.value) return
  reauthAccount.value = null
}

async function generateReauth() {
  if (!reauthAccount.value) return
  authLoading.value = true
  try {
    const result = await generateHostedOpenAIAuthURL({ proxy_id: reauthAccount.value.proxy_id ?? null })
    reauthURL.value = result.auth_url
    reauthAuthState.value = oauthStateFromURL(result.auth_url)
    reauthSessionID.value = result.session_id
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '生成登录链接失败'))
  } finally {
    authLoading.value = false
  }
}

async function saveReauth() {
  if (!reauthAccount.value) return
  reauthing.value = true
  try {
    const updated = await reauthorizeHostedOpenAIAccount(reauthAccount.value.id, {
      session_id: reauthSessionID.value,
      callback_url: reauthCallback.value,
      code: reauthCode.value,
      state: reauthState.value
    })
    replaceAccount(updated)
    reauthAccount.value = null
    appStore.showSuccess?.('授权已更新')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '更新授权失败'))
  } finally {
    reauthing.value = false
  }
}

function replaceAccount(updated: HostedOpenAIAccount) {
  const index = accounts.value.findIndex((item) => item.id === updated.id)
  if (index >= 0) accounts.value.splice(index, 1, updated)
  else accounts.value.unshift(updated)
}

function oauthStateFromURL(value: string) {
  try {
    return new URL(value).searchParams.get('state')?.trim() || ''
  } catch {
    return ''
  }
}

function copyAuthURL() {
  if (authURL.value) {
    void copyToClipboard(authURL.value, '登录地址已复制')
  }
}

function copyReauthURL() {
  if (reauthURL.value) {
    void copyToClipboard(reauthURL.value, '登录地址已复制')
  }
}

function initials(account: HostedOpenAIAccount) {
  const text = account.account_name || account.account_email || account.user_name || 'O'
  return text.slice(0, 1).toUpperCase()
}

function statusLabel(account: HostedOpenAIAccount) {
  if (account.status === 'suspended') return '已停用'
  if (account.status === 'archived') return '已归档'
  if (account.account_status !== 'active') return '错误'
  return '正常'
}

function statusClass(account: HostedOpenAIAccount) {
  if (account.account_status !== 'active') return 'is-error'
  if (account.status !== 'active') return 'is-muted'
  return 'is-active'
}

onMounted(load)
</script>

<style scoped>
.hosted-page { width: min(1180px, 100%); margin: 0 auto; padding: 28px 24px 72px; color: var(--text-primary); }
.hosted-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; padding: 10px 12px; border: 1px solid var(--border-color); border-radius: 9px; background: var(--surface); }
.toolbar-meta { color: var(--text-tertiary); font-size: 11px; }
.head-actions { display: flex; gap: 8px; }
.empty-panel { min-height: 330px; display: grid; place-items: center; align-content: center; gap: 16px; border: 1px dashed var(--border-color); border-radius: 12px; background: var(--surface); }
.empty-mark { display: grid; place-items: center; width: 54px; height: 54px; border-radius: 50%; color: var(--primary-600); background: color-mix(in srgb, var(--primary-500) 10%, transparent); }
.empty-panel h2 { font-size: 15px; font-weight: 600; }
.loading-ring { width: 30px; height: 30px; border: 2px solid var(--border-color); border-top-color: var(--primary-500); border-radius: 50%; animation: spin 0.8s linear infinite; }
.hosted-list { overflow: hidden; border: 1px solid var(--border-color); border-radius: 10px; background: var(--surface); }
.list-head, .hosted-row { display: grid; grid-template-columns: minmax(220px, 1.2fr) minmax(140px, .78fr) minmax(190px, 1.1fr) minmax(96px, .55fr) minmax(190px, 1fr) 76px 112px; align-items: center; gap: 14px; padding: 0 16px; }
.list-head { min-height: 40px; border-bottom: 1px solid var(--border-color); color: var(--text-tertiary); background: var(--surface-secondary); font-size: 10px; font-weight: 650; }
.hosted-row { min-height: 116px; border-bottom: 1px solid var(--border-color); transition: background-color .15s ease; }
.hosted-row:last-child { border-bottom: 0; }
.hosted-row:hover { background: color-mix(in srgb, var(--primary-500) 3%, var(--surface)); }
.identity-cell { display: flex; align-items: center; min-width: 0; gap: 11px; }
.account-avatar { display: grid; place-items: center; width: 34px; height: 34px; flex: 0 0 auto; border: 1px solid color-mix(in srgb, var(--primary-500) 25%, var(--border-color)); border-radius: 8px; color: var(--primary-700); background: color-mix(in srgb, var(--primary-500) 9%, var(--surface)); font-size: 13px; font-weight: 700; }
.identity-copy, .group-cell, .proxy-cell { display: grid; min-width: 0; gap: 4px; }
.identity-copy strong, .identity-copy span, .identity-copy small, .group-cell > strong, .proxy-cell strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.identity-copy strong { font-size: 12px; font-weight: 650; }
.identity-copy span { color: var(--text-secondary); font-size: 10px; }
.identity-copy small, .proxy-cell span { color: var(--text-tertiary); font-size: 9px; }
.group-cell > strong, .proxy-cell strong { font-size: 11px; font-weight: 600; }
.model-chips { display: flex; min-width: 0; align-items: center; gap: 3px; overflow: hidden; }
.model-chips code { max-width: 140px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 1px 5px; border-radius: 4px; color: var(--text-secondary); background: var(--surface-secondary); font-size: 9px; }
.model-chips > span { flex: 0 0 auto; color: var(--text-tertiary); font-size: 9px; }
.messages-state { width: fit-content; padding: 1px 5px; border-radius: 4px; color: var(--text-tertiary); background: var(--surface-secondary); font-size: 9px; }
.messages-state.enabled { color: #087a55; background: #e7f8ef; }
.status-cell { display: flex; align-items: center; }
.status-chip { flex: 0 0 auto; width: fit-content; padding: 2px 7px; border-radius: 999px; font-size: 10px; font-weight: 600; }
.status-chip.is-active { color: #087a55; background: #e7f8ef; }
.status-chip.is-muted { color: #8a5a00; background: #fff3d8; }
.status-chip.is-error { color: #be123c; background: #ffe4e6; }
.icon-button { display: grid; place-items: center; width: 32px; height: 32px; flex: 0 0 auto; border: 1px solid var(--border-color); border-radius: 7px; color: var(--text-secondary); background: var(--surface); }
.icon-button:disabled { opacity: 0.55; }
.row-actions { display: flex; justify-content: flex-end; gap: 5px; }
.actions-col { text-align: right; }
.hosted-modal { max-width: 580px; }
.modal-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; padding: 20px 22px; border-bottom: 1px solid var(--border-color); }
.modal-head h2 { font-size: 17px; font-weight: 650; }
.modal-head p { margin-top: 4px; color: var(--text-tertiary); font-size: 11px; }
.hosted-form { display: grid; gap: 15px; padding: 20px 22px; }
.field { display: grid; gap: 7px; }
.field > span { color: var(--text-secondary); font-size: 11px; font-weight: 600; }
.settings-section { display: grid; gap: 13px; padding: 14px; border: 1px solid var(--border-color); border-radius: 9px; background: var(--surface-secondary); }
.settings-section-head { display: flex; align-items: flex-start; gap: 10px; }
.settings-section-head > div { display: grid; gap: 3px; }
.settings-section-head strong { font-size: 12px; font-weight: 650; }
.settings-section-head p { color: var(--text-tertiary); font-size: 10px; line-height: 1.45; }
.section-icon { display: grid; place-items: center; width: 28px; height: 28px; flex: 0 0 auto; border: 1px solid color-mix(in srgb, var(--primary-500) 18%, var(--border-color)); border-radius: 7px; color: var(--primary-600); background: color-mix(in srgb, var(--primary-500) 8%, var(--surface)); }
.switch-row { display: flex; align-items: center; justify-content: space-between; gap: 18px; min-height: 54px; padding: 10px 12px; border: 1px solid var(--border-color); border-radius: 8px; background: var(--surface-secondary); }
.group-switch { background: var(--surface); }
.switch-row span { display: grid; gap: 3px; }
.switch-row strong { font-size: 12px; font-weight: 600; }
.switch-row small { color: var(--text-tertiary); font-size: 10px; }
.switch-input { width: 34px; height: 19px; accent-color: var(--primary-600); }
.auth-section { background: color-mix(in srgb, var(--primary-500) 3%, var(--surface)); }
.auth-url-panel { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 8px; }
.auth-url-input { min-width: 0; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 10px; }
.copy-button { background: var(--surface); }
.auth-url-panel .auth-link { grid-column: 1 / -1; justify-self: start; }
.auth-link { display: inline-flex; align-items: center; gap: 5px; color: var(--primary-600); font-size: 12px; font-weight: 600; }
.callback-input { min-height: 82px; resize: vertical; line-height: 1.45; }
.callback-recognized { display: inline-flex; align-items: center; gap: 5px; color: #087a55; font-size: 10px; font-weight: 600; }
.modal-foot { display: flex; justify-content: flex-end; gap: 8px; padding: 14px 22px; border-top: 1px solid var(--border-color); }
.spin, .spinning { animation: spin 0.8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
:global(.dark) .status-chip.is-active { color: #9df1ce; background: rgb(4 120 87 / 0.26); }
:global(.dark) .status-chip.is-muted { color: #ffd38a; background: rgb(146 94 0 / 0.25); }
:global(.dark) .status-chip.is-error { color: #fecdd3; background: rgb(159 18 57 / 0.28); }
:global(.dark) .messages-state.enabled { color: #9df1ce; background: rgb(4 120 87 / 0.26); }
@media (max-width: 1080px) and (min-width: 861px) {
  .hosted-list { overflow-x: auto; }
  .list-head, .hosted-row { min-width: 1040px; }
}
@media (max-width: 860px) {
  .hosted-page { padding: 18px 14px 56px; }
  .hosted-toolbar { align-items: flex-start; }
  .hosted-list { overflow: visible; border: 0; background: transparent; }
  .list-head { display: none; }
  .hosted-row { min-width: 0; grid-template-columns: minmax(0, 1fr) auto; gap: 14px 18px; margin-bottom: 10px; padding: 16px; border: 1px solid var(--border-color); border-radius: 9px; background: var(--surface); }
  .hosted-row:last-child { border-bottom: 1px solid var(--border-color); }
  .identity-cell { grid-column: 1 / -1; }
  .hosted-row > :nth-child(3) { grid-column: 1 / -1; }
  .hosted-row > :nth-child(5) { grid-column: 1 / -1; }
  .status-cell { justify-content: flex-end; }
  .row-actions { grid-column: 1 / -1; justify-content: flex-start; }
}
@media (max-width: 560px) {
  .hosted-toolbar { flex-direction: column; }
  .head-actions { width: 100%; }
  .head-actions .btn { flex: 1; justify-content: center; }
  .modal-content { margin: 10px; max-height: calc(100vh - 20px); }
}
</style>
