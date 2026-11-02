<template>
  <AppLayout>
    <main class="hosted-page">
      <header class="page-head">
        <div>
          <h1>托管账号</h1>
          <p>{{ activeCount }} 个运行中</p>
        </div>
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

      <section v-else class="account-grid">
        <article v-for="account in accounts" :key="account.id" class="account-card">
          <header class="card-head">
            <div class="identity">
              <div class="account-avatar">{{ initials(account) }}</div>
              <div class="identity-copy">
                <div class="title-line">
                  <h2>{{ account.account_name || account.account_email || account.user_name }}</h2>
                  <span class="status-chip" :class="statusClass(account)">{{ statusLabel(account) }}</span>
                </div>
                <p>{{ account.account_email }} · {{ account.quota.account_type }}</p>
              </div>
            </div>
            <button type="button" class="icon-button" title="刷新额度" :disabled="refreshingID === account.id" @click="refreshQuota(account)">
              <Icon name="refresh" size="sm" :class="{ spinning: refreshingID === account.id }" />
            </button>
          </header>

          <dl class="account-meta">
            <div>
              <dt>模型</dt>
              <dd :title="account.models.join(', ')">{{ account.models.join(' · ') }}</dd>
            </div>
            <div>
              <dt>分组</dt>
              <dd :title="account.group_name">{{ account.group_name }}</dd>
            </div>
            <div>
              <dt>代理</dt>
              <dd>{{ account.proxy_name || '直连' }}</dd>
            </div>
            <div>
              <dt>Messages</dt>
              <dd>{{ account.supports_messages ? '已启用' : '关闭' }}</dd>
            </div>
          </dl>

          <div class="quota-zone">
            <div class="quota-window-grid">
              <div class="quota-window">
                <UsageProgressBar
                  v-if="account.quota.five_hour"
                  label="5h"
                  :utilization="account.quota.five_hour.used_percent"
                  :resets-at="account.quota.five_hour.reset_at"
                  :show-now-when-idle="true"
                  color="indigo"
                />
                <div v-else class="quota-empty-row"><span>5h</span><em>暂无数据</em></div>
              </div>
              <div class="quota-window">
                <UsageProgressBar
                  v-if="account.quota.weekly"
                  label="7d"
                  :utilization="account.quota.weekly.used_percent"
                  :resets-at="account.quota.weekly.reset_at"
                  :show-now-when-idle="true"
                  color="emerald"
                />
                <div v-else class="quota-empty-row"><span>7d</span><em>暂无数据</em></div>
              </div>
            </div>
            <p v-if="account.quota.error" class="quota-error">{{ account.quota.error }}</p>
            <p v-else-if="!account.quota.fetched_at" class="quota-caption">刷新后读取实时窗口</p>
            <p v-else class="quota-caption">更新于 {{ formatTime(account.quota.fetched_at) }}</p>
          </div>

          <footer class="card-actions">
            <button type="button" class="btn btn-secondary" @click="openEdit(account)">
              <Icon name="edit" size="sm" />
              配置
            </button>
            <button type="button" class="btn btn-secondary" @click="openReauth(account)">
              <Icon name="key" size="sm" />
              重新授权
            </button>
          </footer>
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
import UsageProgressBar from '@/components/account/UsageProgressBar.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
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
  if (account.account_status !== 'active') return '需检查'
  return '运行中'
}

function statusClass(account: HostedOpenAIAccount) {
  if (account.status !== 'active' || account.account_status !== 'active') return 'is-muted'
  return 'is-active'
}

function formatTime(value?: string | null) {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

onMounted(load)
</script>

<style scoped>
.hosted-page { width: min(1180px, 100%); margin: 0 auto; padding: 28px 24px 72px; color: var(--text-primary); }
.page-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; margin-bottom: 22px; }
.page-head h1 { font-size: 24px; line-height: 1.1; font-weight: 650; letter-spacing: 0; }
.page-head p { margin-top: 7px; color: var(--text-tertiary); font-size: 12px; }
.head-actions { display: flex; gap: 8px; }
.empty-panel { min-height: 330px; display: grid; place-items: center; align-content: center; gap: 16px; border: 1px dashed var(--border-color); border-radius: 12px; background: var(--surface); }
.empty-mark { display: grid; place-items: center; width: 54px; height: 54px; border-radius: 50%; color: var(--primary-600); background: color-mix(in srgb, var(--primary-500) 10%, transparent); }
.empty-panel h2 { font-size: 15px; font-weight: 600; }
.loading-ring { width: 30px; height: 30px; border: 2px solid var(--border-color); border-top-color: var(--primary-500); border-radius: 50%; animation: spin 0.8s linear infinite; }
.account-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.account-card { min-width: 0; overflow: hidden; border: 1px solid var(--border-color); border-radius: 10px; background: var(--surface); box-shadow: 0 1px 2px rgb(0 0 0 / 0.03); }
.card-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px; border-bottom: 1px solid var(--border-color); }
.identity { display: flex; align-items: center; min-width: 0; gap: 12px; }
.account-avatar { display: grid; place-items: center; width: 38px; height: 38px; flex: 0 0 auto; border: 1px solid color-mix(in srgb, var(--primary-500) 25%, var(--border-color)); border-radius: 9px; color: var(--primary-700); background: color-mix(in srgb, var(--primary-500) 9%, var(--surface)); font-size: 15px; font-weight: 700; }
.identity-copy { min-width: 0; }
.title-line { display: flex; align-items: center; min-width: 0; gap: 8px; }
.title-line h2 { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; font-weight: 650; }
.identity-copy p { margin-top: 3px; color: var(--text-tertiary); font-size: 11px; }
.status-chip { flex: 0 0 auto; padding: 2px 7px; border-radius: 999px; font-size: 10px; font-weight: 600; }
.status-chip.is-active { color: #087a55; background: #e7f8ef; }
.status-chip.is-muted { color: #8a5a00; background: #fff3d8; }
.icon-button { display: grid; place-items: center; width: 32px; height: 32px; flex: 0 0 auto; border: 1px solid var(--border-color); border-radius: 7px; color: var(--text-secondary); background: var(--surface); }
.icon-button:disabled { opacity: 0.55; }
.account-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); margin: 0; padding: 0 18px; }
.account-meta > div { min-width: 0; padding: 12px 0; border-bottom: 1px solid var(--border-color); }
.account-meta > div:nth-child(odd) { padding-right: 14px; }
.account-meta > div:nth-child(even) { padding-left: 14px; border-left: 1px solid var(--border-color); }
.account-meta dt { margin-bottom: 4px; color: var(--text-tertiary); font-size: 10px; }
.account-meta dd { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; font-weight: 550; }
.quota-zone { display: grid; gap: 10px; padding: 14px 18px 16px; background: color-mix(in srgb, var(--surface-secondary) 70%, var(--surface)); }
.quota-window-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.quota-window { min-width: 0; padding: 10px 11px; border: 1px solid var(--border-color); border-radius: 8px; background: var(--surface); }
.quota-empty-row { display: flex; align-items: center; gap: 8px; color: var(--text-tertiary); font-size: 10px; }
.quota-empty-row span { display: inline-flex; min-width: 32px; height: 18px; align-items: center; justify-content: center; border-radius: 4px; color: var(--text-secondary); background: var(--surface-secondary); font-weight: 600; }
.quota-empty-row em { font-style: normal; }
.quota-error { color: var(--error-600); font-size: 11px; }
.quota-caption { color: var(--text-tertiary); font-size: 10px; }
.card-actions { display: flex; justify-content: flex-end; gap: 8px; padding: 12px 18px; border-top: 1px solid var(--border-color); }
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
@media (max-width: 860px) {
  .hosted-page { padding: 18px 14px 56px; }
  .account-grid { grid-template-columns: 1fr; }
  .page-head { align-items: flex-start; }
}
@media (max-width: 560px) {
  .page-head { flex-direction: column; }
  .head-actions { width: 100%; }
  .head-actions .btn { flex: 1; justify-content: center; }
  .account-meta { grid-template-columns: 1fr; }
  .account-meta > div:nth-child(odd), .account-meta > div:nth-child(even) { padding: 10px 0; border-left: 0; }
  .quota-window-grid { grid-template-columns: 1fr; }
  .card-actions .btn { flex: 1; justify-content: center; }
  .modal-content { margin: 10px; max-height: calc(100vh - 20px); }
}
</style>
