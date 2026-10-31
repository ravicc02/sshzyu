<template>
  <AppLayout>
    <main class="scheduler-page">
      <header class="page-heading">
        <div>
          <h1>账号调度</h1>
          <p>OAuth 优先，同层账号按可用性、负载、错误率与降智率评分排序。</p>
        </div>
        <div class="heading-actions">
          <button type="button" class="button-secondary" :disabled="loading" @click="load">
            {{ loading ? '刷新中' : '刷新' }}
          </button>
          <button type="button" class="button-primary" :disabled="!selectedGroup || saving" @click="saveSettings">
            {{ saving ? '保存中' : '保存配置' }}
          </button>
        </div>
      </header>

      <section class="status-strip" aria-label="调度状态">
        <div>
          <span>评分调度</span>
          <strong>{{ overview?.advanced_enabled === false ? '被显式关闭' : '已生效' }}</strong>
        </div>
        <div>
          <span>OAuth 优先</span>
          <strong>开启</strong>
        </div>
        <div>
          <span>账号可用</span>
          <strong>{{ overview?.available_accounts ?? 0 }} / {{ overview?.total_accounts ?? 0 }}</strong>
        </div>
        <div>
          <span>降智测试分组</span>
          <strong>{{ overview?.enabled_groups ?? 0 }}</strong>
        </div>
        <div>
          <span>降智率权重</span>
          <strong>{{ formatWeight(overview?.degradation_weight) }}</strong>
        </div>
      </section>

      <div v-if="overview?.advanced_enabled === false" class="inline-warning">
        高级评分调度被系统设置显式关闭；当前页面仍可测试，但请求会走旧调度路径。
      </div>

      <section v-if="loading && !overview" class="empty-state">正在加载账号与探测记录...</section>
      <section v-else-if="!overview || overview.groups.length === 0" class="empty-state">暂无可调度分组。</section>

      <div v-else class="scheduler-workspace">
        <aside class="group-rail">
          <div class="rail-heading">
            <span>分组</span>
            <small>{{ overview.groups.length }}</small>
          </div>
          <button
            v-for="group in overview.groups"
            :key="group.id"
            type="button"
            class="group-row"
            :class="{ active: selectedGroupID === group.id }"
            @click="selectGroup(group.id)"
          >
            <span class="group-main">
              <strong>{{ group.name }}</strong>
              <span>
                {{ platformLabel(group.platform) }} · {{ group.accounts.length }} 个账号
              </span>
            </span>
            <span class="group-flags">
              <span v-if="group.oauth_first" class="flag oauth">OAuth</span>
              <span v-if="group.degradation_enabled" class="flag test">测试</span>
              <span v-if="group.lowest_account_id" class="rank-mark" title="当前分组降智率最低">#1</span>
            </span>
          </button>
        </aside>

        <div v-if="selectedGroup" class="main-panel">
          <section class="group-toolbar">
            <div class="group-title">
              <div>
                <h2>{{ selectedGroup.name }}</h2>
                <span>{{ platformLabel(selectedGroup.platform) }} · {{ selectedGroup.accounts.length }} 个账号</span>
              </div>
              <span class="mode-chip">{{ selectedGroup.oauth_first ? 'OAuth 优先' : '统一评分池' }}</span>
            </div>

            <div class="settings-grid">
              <label class="setting-field toggle-field">
                <span>降智商测试</span>
                <button
                  type="button"
                  class="switch"
                  :class="{ on: draft.enabled }"
                  :aria-pressed="draft.enabled"
                  @click="draft.enabled = !draft.enabled"
                >
                  <span />
                </button>
              </label>

              <label class="setting-field">
                <span>测试间隔</span>
                <select v-model.number="draft.intervalSeconds">
                  <option :value="300">5 分钟</option>
                  <option :value="600">10 分钟</option>
                  <option :value="1800">30 分钟</option>
                  <option :value="3600">1 小时</option>
                  <option :value="21600">6 小时</option>
                </select>
              </label>

              <label class="setting-field model-field">
                <span>测试模型</span>
                <select v-if="selectedGroup.model_candidates.length" v-model="draft.testModel">
                  <option value="">自动选择</option>
                  <option v-for="model in selectedGroup.model_candidates" :key="model" :value="model">
                    {{ model }}
                  </option>
                </select>
                <input v-else v-model.trim="draft.testModel" placeholder="填写分组可用模型" />
              </label>

              <label class="setting-field">
                <span>降智率权重</span>
                <input v-model.number="draft.degradationWeight" type="number" min="0" max="100" step="0.1" />
              </label>

              <button
                type="button"
                class="button-secondary run-button"
                :disabled="running"
                @click="runGroup"
              >
                {{ running ? '测试中' : '立即测试本组' }}
              </button>
            </div>
          </section>

          <section class="accounts-section">
            <div class="section-heading">
              <div>
                <h3>调度排名</h3>
                <p>降智率只统计有效的通过/降智样本，请求失败单列，不污染降智率。</p>
              </div>
              <span>评分调度 · Priority 已忽略</span>
            </div>

            <div class="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>排名</th>
                    <th>账号</th>
                    <th>凭证</th>
                    <th>可用性</th>
                    <th>降智率</th>
                    <th>样本</th>
                    <th>最近结果</th>
                    <th>评分</th>
                    <th>权重</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="account in selectedGroup.accounts"
                    :key="account.id"
                    :class="{ 'lowest-row': account.lowest_degradation }"
                  >
                    <td>
                      <span class="rank-number">{{ account.rank }}</span>
                    </td>
                    <td class="account-cell">
                      <div>
                        <strong>{{ account.name }}</strong>
                        <span>#{{ account.id }} · {{ platformLabel(account.platform) }}</span>
                      </div>
                      <span v-if="account.lowest_degradation" class="lowest-badge">最低降智</span>
                    </td>
                    <td>
                      <span class="credential-chip" :class="{ oauth: account.oauth }">
                        {{ account.oauth ? 'OAuth' : 'API Key' }}
                      </span>
                    </td>
                    <td>
                      <span class="availability" :class="{ down: !account.available }">
                        <i />
                        {{ account.available ? '可用' : account.availability_reason || '不可用' }}
                      </span>
                    </td>
                    <td>
                      <div class="rate-cell">
                        <strong :class="rateClass(account.degradation_rate)">{{ formatRate(account.degradation_rate) }}</strong>
                        <span>{{ account.degraded_count }} / {{ account.sample_count }}</span>
                      </div>
                    </td>
                    <td>
                      <div class="sample-cell">
                        <strong>{{ account.sample_count }}</strong>
                        <span>失败 {{ account.error_count }}</span>
                      </div>
                    </td>
                    <td>
                      <div class="probe-cell">
                        <span class="probe-status" :class="account.latest_status">
                          {{ probeStatusLabel(account.latest_status) }}
                        </span>
                        <small>{{ formatTime(account.latest_checked_at) }}</small>
                      </div>
                    </td>
                    <td>
                      <strong class="score-value">{{ formatScore(account.score) }}</strong>
                    </td>
                    <td>
                      <div class="factor-cell">
                        <span>{{ formatWeight(account.score_weight) }}</span>
                        <small>因子 {{ account.degradation_factor.toFixed(2) }}</small>
                      </div>
                    </td>
                    <td>
                      <button
                        type="button"
                        class="row-action"
                        :disabled="runningAccountID === account.id"
                        title="测试此账号"
                        @click="runAccount(account.id)"
                      >
                        {{ runningAccountID === account.id ? '…' : '测试' }}
                      </button>
                    </td>
                  </tr>
                  <tr v-if="selectedGroup.accounts.length === 0">
                    <td colspan="10" class="table-empty">该分组没有关联账号。</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <section class="probe-section">
            <div class="section-heading">
              <div>
                <h3>最近探测</h3>
                <p>固定推理题：答 21 为通过，非 21 记为降智，请求错误不计入降智率。</p>
              </div>
              <span>最近 {{ selectedProbes.length }} 条</span>
            </div>
            <div class="probe-list">
              <article v-for="probe in selectedProbes" :key="probe.id" class="probe-row">
                <span class="probe-status" :class="probe.status">{{ probeStatusLabel(probe.status) }}</span>
                <strong>{{ probe.account_name || `账号 #${probe.account_id}` }}</strong>
                <span>{{ probe.model || '自动模型' }}</span>
                <span>{{ formatLatency(probe.latency_ms) }}</span>
                <small>{{ formatTime(probe.checked_at) }}</small>
                <span class="probe-message">
                  {{ probe.status === 'error' ? probe.error_message : probe.response_text }}
                </span>
              </article>
              <div v-if="selectedProbes.length === 0" class="probe-empty">该分组暂无探测记录。</div>
            </div>
          </section>
        </div>
      </div>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { adminAPI } from '@/api/admin'
import type {
  AccountSchedulerGroup,
  AccountSchedulerOverview,
} from '@/api/admin/accountScheduler'
import { useAppStore } from '@/stores'

const appStore = useAppStore()
const overview = ref<AccountSchedulerOverview | null>(null)
const selectedGroupID = ref<number | null>(null)
const loading = ref(false)
const saving = ref(false)
const running = ref(false)
const runningAccountID = ref<number | null>(null)
const draft = reactive({
  enabled: false,
  intervalSeconds: 600,
  testModel: '',
  degradationWeight: 1.5,
})

const selectedGroup = computed<AccountSchedulerGroup | null>(() => {
  if (!overview.value || selectedGroupID.value === null) return null
  return overview.value.groups.find((group) => group.id === selectedGroupID.value) ?? null
})

const selectedProbes = computed(() => {
  if (!overview.value || !selectedGroup.value) return []
  return overview.value.recent_probes
    .filter((probe) => probe.group_id === selectedGroup.value?.id)
    .slice(0, 20)
})

watch(selectedGroup, (group) => {
  if (!group) return
  draft.enabled = group.degradation_enabled
  draft.intervalSeconds = group.interval_seconds || 600
  draft.testModel = group.test_model || ''
}, { immediate: true })

watch(() => overview.value?.degradation_weight, (weight) => {
  if (typeof weight === 'number') draft.degradationWeight = weight
}, { immediate: true })

async function load() {
  loading.value = true
  try {
    const data = await adminAPI.accountScheduler.getOverview()
    overview.value = data
    const preferred = selectedGroupID.value && data.groups.some((group) => group.id === selectedGroupID.value)
      ? selectedGroupID.value
      : data.groups[0]?.id ?? null
    selectedGroupID.value = preferred
  } catch (error: any) {
    appStore.showError(error?.message || '账号调度数据加载失败')
  } finally {
    loading.value = false
  }
}

function selectGroup(groupID: number) {
  selectedGroupID.value = groupID
}

async function saveSettings() {
  if (!selectedGroup.value) return
  saving.value = true
  try {
    await adminAPI.accountScheduler.updateSetting({
      group_id: selectedGroup.value.id,
      degradation_enabled: draft.enabled,
      interval_seconds: draft.intervalSeconds,
      test_model: draft.testModel,
      degradation_weight: draft.degradationWeight,
    })
    appStore.showSuccess('账号调度配置已保存')
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function runGroup() {
  if (!selectedGroup.value) return
  running.value = true
  try {
    const result = await adminAPI.accountScheduler.runNow({ group_id: selectedGroup.value.id })
    appStore.showSuccess(`测试完成：${result.run} 条，跳过 ${result.skipped} 条，失败 ${result.failed} 条`)
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || '立即测试失败')
  } finally {
    running.value = false
  }
}

async function runAccount(accountID: number) {
  if (!selectedGroup.value) return
  runningAccountID.value = accountID
  try {
    const result = await adminAPI.accountScheduler.runNow({
      group_id: selectedGroup.value.id,
      account_id: accountID,
    })
    if (result.run > 0) appStore.showSuccess('账号测试已记录')
    else appStore.showError('账号测试未完成')
    await load()
  } catch (error: any) {
    appStore.showError(error?.message || '账号测试失败')
  } finally {
    runningAccountID.value = null
  }
}

function platformLabel(platform: string) {
  const labels: Record<string, string> = {
    openai: 'OpenAI',
    deepseek: 'DeepSeek',
    zhipu: 'GLM',
    kimi: 'Kimi',
    minimax: 'MiniMax',
    composite: '聚合',
    grok: 'Grok',
  }
  return labels[platform] || platform
}

function probeStatusLabel(status: string) {
  return ({ pass: '通过', degraded: '降智', error: '请求失败' } as Record<string, string>)[status] || '未测试'
}

function formatRate(rate?: number) {
  return typeof rate === 'number' ? `${(rate * 100).toFixed(1)}%` : '—'
}

function rateClass(rate?: number) {
  if (typeof rate !== 'number') return ''
  if (rate >= 0.5) return 'danger'
  if (rate >= 0.2) return 'warning'
  return 'good'
}

function formatScore(score: number) {
  return Number.isFinite(score) ? score.toFixed(2) : '—'
}

function formatWeight(weight?: number) {
  return typeof weight === 'number' ? weight.toFixed(1) : '—'
}

function formatLatency(value: number) {
  return value > 0 ? `${Math.round(value)} ms` : '—'
}

function formatTime(value?: string) {
  if (!value) return '未测试'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

onMounted(load)
</script>

<style scoped>
.scheduler-page {
  min-height: 100%;
  padding: 24px;
  color: var(--text-primary);
  background: var(--bg-secondary);
}
.page-heading,
.status-strip,
.scheduler-workspace,
.inline-warning {
  width: min(1440px, 100%);
  margin-inline: auto;
}
.page-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 18px;
}
.page-heading h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 650;
  letter-spacing: 0;
}
.page-heading p,
.section-heading p {
  margin: 5px 0 0;
  color: var(--text-secondary);
  font-size: 13px;
}
.heading-actions {
  display: flex;
  gap: 8px;
}
.button-primary,
.button-secondary,
.row-action {
  border: 1px solid var(--border-color);
  border-radius: 7px;
  font-size: 13px;
  font-weight: 560;
}
.button-primary,
.button-secondary {
  min-height: 36px;
  padding: 0 14px;
}
.button-primary {
  color: #fff;
  border-color: #2563eb;
  background: #2563eb;
}
.button-secondary {
  color: var(--text-primary);
  background: var(--bg-primary);
}
.button-primary:disabled,
.button-secondary:disabled,
.row-action:disabled {
  opacity: .5;
  cursor: wait;
}
.status-strip {
  display: grid;
  grid-template-columns: 1.15fr repeat(4, 1fr);
  margin-bottom: 16px;
  border-block: 1px solid var(--border-color);
  background: var(--bg-primary);
}
.status-strip > div {
  min-width: 0;
  padding: 14px 18px;
  border-right: 1px solid var(--border-color);
}
.status-strip > div:last-child { border-right: 0; }
.status-strip span,
.status-strip strong {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.status-strip span {
  color: var(--text-tertiary);
  font-size: 11px;
  text-transform: uppercase;
}
.status-strip strong {
  margin-top: 4px;
  font-size: 16px;
  font-weight: 650;
}
.inline-warning {
  margin-bottom: 16px;
  padding: 10px 14px;
  border-left: 3px solid #d97706;
  color: #92400e;
  background: #fffbeb;
  font-size: 13px;
}
.scheduler-workspace {
  display: grid;
  grid-template-columns: 286px minmax(0, 1fr);
  min-height: 640px;
  overflow: hidden;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: var(--bg-primary);
}
.group-rail {
  min-width: 0;
  border-right: 1px solid var(--border-color);
  background: color-mix(in srgb, var(--bg-secondary) 65%, var(--bg-primary));
}
.rail-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 48px;
  padding: 0 15px;
  border-bottom: 1px solid var(--border-color);
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 650;
}
.rail-heading small {
  font-variant-numeric: tabular-nums;
}
.group-row {
  display: flex;
  width: 100%;
  min-height: 64px;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 14px;
  border: 0;
  border-bottom: 1px solid var(--border-color);
  color: var(--text-primary);
  background: transparent;
  text-align: left;
}
.group-row:hover { background: var(--bg-hover); }
.group-row.active {
  box-shadow: inset 3px 0 #2563eb;
  background: color-mix(in srgb, #2563eb 7%, var(--bg-primary));
}
.group-main {
  display: grid;
  min-width: 0;
  gap: 3px;
}
.group-main strong,
.group-main span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.group-main strong {
  font-size: 13px;
  font-weight: 650;
}
.group-main span {
  color: var(--text-tertiary);
  font-size: 11px;
}
.group-flags {
  display: flex;
  flex: none;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 4px;
}
.flag,
.rank-mark {
  padding: 2px 5px;
  border-radius: 4px;
  font-size: 9px;
  font-weight: 700;
}
.flag.oauth { color: #1d4ed8; background: #dbeafe; }
.flag.test { color: #047857; background: #d1fae5; }
.rank-mark { color: #92400e; background: #fef3c7; }
.main-panel {
  min-width: 0;
}
.group-toolbar {
  padding: 18px 20px;
  border-bottom: 1px solid var(--border-color);
}
.group-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.group-title h2,
.section-heading h3 {
  margin: 0;
  letter-spacing: 0;
}
.group-title h2 {
  font-size: 18px;
  font-weight: 650;
}
.group-title > div > span {
  color: var(--text-tertiary);
  font-size: 12px;
}
.mode-chip {
  padding: 4px 8px;
  border: 1px solid #bfdbfe;
  border-radius: 999px;
  color: #1d4ed8;
  background: #eff6ff;
  font-size: 11px;
  font-weight: 650;
}
.settings-grid {
  display: grid;
  grid-template-columns: 1fr .8fr minmax(220px, 1.5fr) .8fr auto;
  align-items: end;
  gap: 10px;
  margin-top: 16px;
}
.setting-field {
  display: grid;
  min-width: 0;
  gap: 6px;
}
.setting-field > span {
  color: var(--text-tertiary);
  font-size: 11px;
  font-weight: 620;
}
.setting-field input,
.setting-field select {
  width: 100%;
  height: 36px;
  padding: 0 10px;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  color: var(--text-primary);
  background: var(--bg-primary);
  font-size: 12px;
}
.toggle-field {
  grid-template-columns: 1fr auto;
  align-items: center;
  height: 36px;
  padding: 0 10px;
  border: 1px solid var(--border-color);
  border-radius: 6px;
}
.toggle-field > span { color: var(--text-primary); }
.switch {
  position: relative;
  width: 36px;
  height: 20px;
  padding: 0;
  border: 0;
  border-radius: 999px;
  background: #cbd5e1;
}
.switch span {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: #fff;
  transition: transform .16s ease;
}
.switch.on { background: #047857; }
.switch.on span { transform: translateX(16px); }
.run-button { height: 36px; white-space: nowrap; }
.accounts-section,
.probe-section {
  padding: 18px 20px;
  border-bottom: 1px solid var(--border-color);
}
.section-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}
.section-heading h3 {
  font-size: 14px;
  font-weight: 650;
}
.section-heading > span {
  color: var(--text-tertiary);
  font-size: 11px;
  white-space: nowrap;
}
.table-scroll {
  overflow-x: auto;
  border: 1px solid var(--border-color);
  border-radius: 7px;
}
table {
  width: 100%;
  min-width: 1050px;
  border-collapse: collapse;
  font-size: 12px;
}
th {
  height: 36px;
  padding: 0 10px;
  color: var(--text-tertiary);
  background: var(--bg-secondary);
  font-size: 10px;
  font-weight: 680;
  text-align: left;
  text-transform: uppercase;
  white-space: nowrap;
}
td {
  height: 56px;
  padding: 8px 10px;
  border-top: 1px solid var(--border-color);
  vertical-align: middle;
}
tbody tr:hover { background: var(--bg-hover); }
.lowest-row { background: color-mix(in srgb, #f59e0b 5%, transparent); }
.rank-number {
  display: grid;
  width: 24px;
  height: 24px;
  place-items: center;
  border-radius: 6px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  font-variant-numeric: tabular-nums;
  font-weight: 650;
}
.account-cell {
  min-width: 190px;
}
.account-cell > div {
  display: grid;
  gap: 2px;
}
.account-cell strong {
  max-width: 230px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 620;
}
.account-cell span,
.probe-cell small,
.factor-cell small {
  color: var(--text-tertiary);
  font-size: 10px;
}
.lowest-badge {
  display: inline-block;
  margin-top: 4px;
  padding: 2px 5px;
  border-radius: 4px;
  color: #92400e !important;
  background: #fef3c7;
  font-size: 9px !important;
  font-weight: 700;
}
.credential-chip {
  display: inline-flex;
  padding: 3px 6px;
  border-radius: 4px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  font-size: 10px;
  font-weight: 620;
  white-space: nowrap;
}
.credential-chip.oauth { color: #1d4ed8; background: #dbeafe; }
.availability {
  display: inline-flex;
  max-width: 150px;
  align-items: center;
  gap: 6px;
  color: #047857;
  white-space: nowrap;
}
.availability i {
  width: 6px;
  height: 6px;
  flex: none;
  border-radius: 50%;
  background: currentColor;
}
.availability.down { color: #b91c1c; }
.rate-cell,
.sample-cell,
.factor-cell,
.probe-cell {
  display: grid;
  gap: 2px;
}
.rate-cell strong {
  font-size: 14px;
  font-variant-numeric: tabular-nums;
}
.rate-cell strong.good { color: #047857; }
.rate-cell strong.warning { color: #b45309; }
.rate-cell strong.danger { color: #b91c1c; }
.rate-cell span,
.sample-cell span { color: var(--text-tertiary); font-size: 10px; }
.score-value {
  font-variant-numeric: tabular-nums;
  font-weight: 650;
}
.probe-status {
  display: inline-flex;
  width: fit-content;
  padding: 2px 6px;
  border-radius: 4px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  font-size: 10px;
  font-weight: 650;
  white-space: nowrap;
}
.probe-status.pass { color: #047857; background: #d1fae5; }
.probe-status.degraded { color: #b45309; background: #fef3c7; }
.probe-status.error { color: #b91c1c; background: #fee2e2; }
.row-action {
  min-width: 48px;
  height: 28px;
  color: var(--text-secondary);
  background: transparent;
}
.probe-section { border-bottom: 0; }
.probe-list {
  border-top: 1px solid var(--border-color);
}
.probe-row {
  display: grid;
  grid-template-columns: 66px 130px 140px 70px 90px minmax(180px, 1fr);
  align-items: center;
  gap: 10px;
  min-height: 42px;
  border-bottom: 1px solid var(--border-color);
  font-size: 11px;
}
.probe-row > strong,
.probe-row > span,
.probe-row > small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.probe-row > small,
.probe-message { color: var(--text-tertiary); }
.probe-empty,
.table-empty,
.empty-state {
  padding: 40px 20px;
  color: var(--text-tertiary);
  text-align: center;
  font-size: 13px;
}
.empty-state {
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: var(--bg-primary);
}
@media (max-width: 1100px) {
  .settings-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .run-button { width: fit-content; }
}
@media (max-width: 820px) {
  .scheduler-page { padding: 14px; }
  .page-heading { flex-direction: column; }
  .status-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .status-strip > div { border-bottom: 1px solid var(--border-color); }
  .scheduler-workspace { grid-template-columns: 1fr; }
  .group-rail {
    display: flex;
    overflow-x: auto;
    border-right: 0;
    border-bottom: 1px solid var(--border-color);
  }
  .rail-heading { display: none; }
  .group-row {
    width: 220px;
    flex: none;
    border-right: 1px solid var(--border-color);
  }
  .group-row.active { box-shadow: inset 0 -3px #2563eb; }
  .settings-grid { grid-template-columns: 1fr; }
  .section-heading { flex-direction: column; }
  .probe-row { grid-template-columns: 64px minmax(100px, 1fr) 80px; }
  .probe-row > span:nth-of-type(2),
  .probe-row > small { display: none; }
  .probe-message { grid-column: 1 / -1; padding-bottom: 8px; }
}
</style>
