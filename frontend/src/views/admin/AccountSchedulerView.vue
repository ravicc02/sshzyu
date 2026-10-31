<template>
  <AppLayout>
    <main class="scheduler-page">
      <header class="command-bar">
        <div class="runtime-summary">
          <span class="runtime-dot" :class="{ down: overview?.advanced_enabled === false }" />
          <div>
            <strong>{{ overview?.advanced_enabled === false ? '评分调度未生效' : '评分调度运行中' }}</strong>
            <p>OAuth 作为第一层路由，同层账号按实时健康度与降智率排序。</p>
          </div>
        </div>
        <div class="command-actions">
          <button type="button" class="button-secondary" :disabled="loading" @click="load">
            <Icon name="refresh" size="sm" :class="{ spinning: loading }" />
            {{ loading ? '刷新中' : '刷新' }}
          </button>
          <button
            type="button"
            :class="hasChanges ? 'button-primary' : 'button-secondary'"
            :disabled="!selectedGroup || !hasChanges || saving"
            @click="saveSettings"
          >
            <Icon name="check" size="sm" />
            {{ saving ? '保存中' : hasChanges ? '保存更改' : '已保存' }}
          </button>
        </div>
      </header>

      <section class="metric-strip" aria-label="调度概览">
        <div>
          <span>账号覆盖</span>
          <strong>{{ overview?.available_accounts ?? 0 }}<small>/{{ overview?.total_accounts ?? 0 }}</small></strong>
        </div>
        <div>
          <span>自动探测</span>
          <strong>{{ overview?.enabled_groups ?? 0 }}<small> 个分组</small></strong>
        </div>
        <div>
          <span>OAuth 优先</span>
          <strong>已开启</strong>
        </div>
        <div>
          <span>降智率权重</span>
          <strong>{{ formatWeight(overview?.degradation_weight) }}</strong>
        </div>
      </section>

      <div v-if="overview?.advanced_enabled === false" class="inline-warning">
        高级评分调度被系统设置显式关闭。当前页面仍可测试，但请求会走旧调度路径。
      </div>

      <section v-if="loading && !overview" class="loading-ledger" aria-label="加载中">
        <div v-for="index in 7" :key="index" class="skeleton-row" />
      </section>
      <section v-else-if="!overview || overview.groups.length === 0" class="empty-state">
        <Icon name="chartBar" size="lg" />
        <strong>暂无可调度分组</strong>
        <span>分组关联 OpenAI 兼容账号后会出现在这里。</span>
      </section>

      <div v-else class="scheduler-workspace">
        <aside class="group-panel">
          <div class="group-panel-heading">
            <div>
              <span>可调度分组</span>
              <small>{{ filteredGroups.length }} / {{ overview.groups.length }}</small>
            </div>
          </div>
          <label class="group-search">
            <Icon name="search" size="sm" />
            <input v-model.trim="groupQuery" type="search" placeholder="搜索分组" />
          </label>
          <div class="group-list">
            <button
              v-for="group in filteredGroups"
              :key="group.id"
              type="button"
              class="group-row"
              :class="{ active: selectedGroupID === group.id }"
              @click="selectGroup(group.id)"
            >
              <span class="group-copy">
                <strong>{{ group.name }}</strong>
                <small>{{ platformLabel(group.platform) }} · {{ group.accounts.length }} 个账号</small>
              </span>
              <span class="group-state">
                <span v-if="group.degradation_enabled" class="test-dot" title="自动探测已开启" />
                <span v-if="group.lowest_account_id" class="lowest-mark" title="存在最低降智账号">#1</span>
              </span>
            </button>
            <div v-if="filteredGroups.length === 0" class="group-empty">没有匹配的分组。</div>
          </div>
        </aside>

        <section v-if="selectedGroup" class="group-workspace">
          <header class="group-header">
            <div class="group-identity">
              <span class="section-label">当前分组</span>
              <div class="group-name-line">
                <h2>{{ selectedGroup.name }}</h2>
                <span class="mode-chip">{{ selectedGroup.oauth_first ? 'OAuth 优先' : '统一评分池' }}</span>
              </div>
              <div class="group-meta">
                <span>{{ platformLabel(selectedGroup.platform) }}</span>
                <span>{{ selectedGroup.accounts.length }} 个账号</span>
                <span>Priority 不参与排序</span>
              </div>
            </div>
            <button type="button" class="button-secondary" :disabled="running" @click="runGroup">
              <Icon name="play" size="sm" />
              {{ running ? '测试中' : '立即测试' }}
            </button>
          </header>

          <section class="detection-config">
            <div class="config-title">
              <div>
                <span class="section-label">降智探测</span>
                <strong>自动测试配置</strong>
              </div>
              <span class="config-state" :class="{ enabled: draft.enabled }">
                {{ draft.enabled ? '自动运行' : '仅手动运行' }}
              </span>
            </div>
            <div class="config-grid">
              <label class="config-toggle">
                <span>启用自动探测</span>
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

              <label class="config-field">
                <span>测试间隔</span>
                <select v-model.number="draft.intervalSeconds">
                  <option :value="300">5 分钟</option>
                  <option :value="600">10 分钟</option>
                  <option :value="1800">30 分钟</option>
                  <option :value="3600">1 小时</option>
                  <option :value="21600">6 小时</option>
                </select>
              </label>

              <label class="config-field model-config">
                <span>测试模型</span>
                <select v-if="selectedGroup.model_candidates.length" v-model="draft.testModel">
                  <option value="">自动选择</option>
                  <option v-for="model in selectedGroup.model_candidates" :key="model" :value="model">
                    {{ model }}
                  </option>
                </select>
                <input v-else v-model.trim="draft.testModel" placeholder="填写分组可用模型" />
              </label>

              <label class="config-field weight-config">
                <span>全局权重</span>
                <input v-model.number="draft.degradationWeight" type="number" min="0" max="100" step="0.1" />
              </label>
            </div>
          </section>

          <section class="ranking-section">
            <div class="ranking-heading">
              <div>
                <span class="section-label">实时排名</span>
                <strong>账号调度顺序</strong>
              </div>
              <p>单次未通过不等同账号降智；错误请求单独计数，不进入降智率。</p>
            </div>

            <div class="ranking-table-wrap">
              <table class="ranking-table">
                <thead>
                  <tr>
                    <th class="rank-column">#</th>
                    <th>账号</th>
                    <th>可用性</th>
                    <th>降智率</th>
                    <th>调度分</th>
                    <th>最近探测</th>
                    <th class="action-column" />
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="account in selectedGroup.accounts"
                    :key="account.id"
                    :class="{ lowest: account.lowest_degradation }"
                  >
                    <td class="rank-column">
                      <span class="rank-number">{{ account.rank }}</span>
                    </td>
                    <td class="account-column">
                      <div class="account-line">
                        <strong :title="account.name">{{ account.name }}</strong>
                        <span class="credential-chip" :class="{ oauth: account.oauth }">
                          {{ account.oauth ? 'OAuth' : 'API Key' }}
                        </span>
                        <span v-if="account.lowest_degradation" class="lowest-badge">最低降智</span>
                      </div>
                      <small>#{{ account.id }} · {{ platformLabel(account.platform) }}</small>
                    </td>
                    <td>
                      <span class="availability" :class="{ down: !account.available }">
                        <i />
                        {{ account.available ? '可用' : account.availability_reason || '不可用' }}
                      </span>
                    </td>
                    <td>
                      <div class="value-stack">
                        <strong :class="rateClass(account.degradation_rate)">
                          {{ formatRate(account.degradation_rate) }}
                        </strong>
                        <small>{{ account.degraded_count }} / {{ account.sample_count }} · 失败 {{ account.error_count }}</small>
                      </div>
                    </td>
                    <td>
                      <div class="value-stack">
                        <strong>{{ formatScore(account.score) }}</strong>
                        <small>降智因子 {{ account.degradation_factor.toFixed(2) }}</small>
                      </div>
                    </td>
                    <td>
                      <div class="latest-cell">
                        <span class="probe-status" :class="account.latest_status">
                          {{ probeStatusLabel(account.latest_status) }}
                        </span>
                        <small>{{ formatTime(account.latest_checked_at) }}</small>
                      </div>
                    </td>
                    <td class="action-column">
                      <button
                        type="button"
                        class="icon-action"
                        :disabled="runningAccountID === account.id"
                        title="测试此账号"
                        :aria-label="`测试账号 ${account.name}`"
                        @click="runAccount(account.id)"
                      >
                        <Icon name="play" size="sm" />
                      </button>
                    </td>
                  </tr>
                  <tr v-if="selectedGroup.accounts.length === 0">
                    <td colspan="7" class="table-empty">该分组没有关联账号。</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <details class="probe-history">
            <summary>
              <span class="history-title">
                <span class="section-label">审计明细</span>
                <strong>最近探测</strong>
                <small>{{ selectedProbes.length }} 条记录</small>
              </span>
              <span class="history-hint">展开查看模型回复与请求错误</span>
            </summary>
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
          </details>
        </section>
      </div>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type {
  AccountSchedulerGroup,
  AccountSchedulerOverview,
} from '@/api/admin/accountScheduler'
import { useAppStore } from '@/stores'

const appStore = useAppStore()
const overview = ref<AccountSchedulerOverview | null>(null)
const selectedGroupID = ref<number | null>(null)
const groupQuery = ref('')
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

const filteredGroups = computed(() => {
  const query = groupQuery.value.toLowerCase()
  if (!overview.value) return []
  if (!query) return overview.value.groups
  return overview.value.groups.filter((group) =>
    group.name.toLowerCase().includes(query) ||
    group.platform.toLowerCase().includes(query),
  )
})

const selectedProbes = computed(() => {
  if (!overview.value || !selectedGroup.value) return []
  return overview.value.recent_probes
    .filter((probe) => probe.group_id === selectedGroup.value?.id)
    .slice(0, 20)
})

const hasChanges = computed(() => {
  const group = selectedGroup.value
  if (!group || !overview.value) return false
  return draft.enabled !== group.degradation_enabled ||
    draft.intervalSeconds !== group.interval_seconds ||
    draft.testModel !== group.test_model ||
    draft.degradationWeight !== overview.value.degradation_weight
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
  return ({ pass: '通过', degraded: '未通过', error: '请求失败' } as Record<string, string>)[status] || '未测试'
}

function formatRate(rate?: number) {
  return typeof rate === 'number' ? `${(rate * 100).toFixed(1)}%` : '暂无'
}

function rateClass(rate?: number) {
  if (typeof rate !== 'number') return ''
  if (rate >= 0.5) return 'danger'
  if (rate >= 0.2) return 'warning'
  return 'good'
}

function formatScore(score: number) {
  return Number.isFinite(score) ? score.toFixed(2) : '-'
}

function formatWeight(weight?: number) {
  return typeof weight === 'number' ? weight.toFixed(1) : '-'
}

function formatLatency(value: number) {
  return value > 0 ? `${Math.round(value)} ms` : '-'
}

function formatTime(value?: string) {
  if (!value) return '未测试'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
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
  padding: 20px 24px 32px;
  color: var(--text-primary);
  background:
    linear-gradient(to bottom, color-mix(in srgb, var(--bg-primary) 45%, transparent), transparent 180px),
    var(--bg-secondary);
}
.command-bar,
.metric-strip,
.scheduler-workspace,
.inline-warning,
.loading-ledger,
.empty-state {
  width: min(1440px, 100%);
  margin-inline: auto;
}
.command-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  min-height: 52px;
  padding-bottom: 14px;
}
.runtime-summary {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 11px;
}
.runtime-dot {
  width: 8px;
  height: 8px;
  flex: none;
  border-radius: 50%;
  background: #22c55e;
  box-shadow: 0 0 0 4px color-mix(in srgb, #22c55e 14%, transparent);
}
.runtime-dot.down {
  background: #f59e0b;
  box-shadow: 0 0 0 4px color-mix(in srgb, #f59e0b 14%, transparent);
}
.runtime-summary strong,
.runtime-summary p {
  display: block;
  min-width: 0;
}
.runtime-summary strong {
  font-size: 13px;
  font-weight: 650;
}
.runtime-summary p {
  margin: 2px 0 0;
  overflow: hidden;
  color: var(--text-secondary);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.command-actions {
  display: flex;
  flex: none;
  gap: 8px;
}
.button-primary,
.button-secondary {
  display: inline-flex;
  min-height: 34px;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 0 12px;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  font-size: 12px;
  font-weight: 620;
  white-space: nowrap;
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
.icon-action:disabled {
  opacity: .45;
  cursor: not-allowed;
}
.button-primary:not(:disabled):active,
.button-secondary:not(:disabled):active,
.icon-action:not(:disabled):active {
  transform: translateY(1px);
}
.spinning {
  animation: spin .9s linear infinite;
}
.metric-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin-bottom: 14px;
  overflow: hidden;
  border-block: 1px solid var(--border-color);
  background: color-mix(in srgb, var(--bg-primary) 75%, transparent);
}
.metric-strip > div {
  min-width: 0;
  padding: 12px 18px;
  border-right: 1px solid var(--border-color);
}
.metric-strip > div:last-child {
  border-right: 0;
}
.metric-strip span,
.metric-strip strong {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-strip span {
  color: var(--text-tertiary);
  font-size: 11px;
  font-weight: 650;
}
.metric-strip strong {
  margin-top: 3px;
  font-size: 16px;
  font-variant-numeric: tabular-nums;
  font-weight: 650;
}
.metric-strip small {
  color: var(--text-tertiary);
  font-size: 11px;
  font-weight: 500;
}
.inline-warning {
  margin-bottom: 14px;
  padding: 9px 12px;
  border: 1px solid color-mix(in srgb, #d97706 45%, var(--border-color));
  border-radius: 6px;
  color: #92400e;
  background: #fffbeb;
  font-size: 12px;
}
.scheduler-workspace {
  display: grid;
  grid-template-columns: 238px minmax(0, 1fr);
  min-height: 650px;
  overflow: hidden;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: var(--bg-primary);
  box-shadow: 0 2px 8px color-mix(in srgb, var(--text-primary) 5%, transparent);
}
.group-panel {
  display: flex;
  min-width: 0;
  flex-direction: column;
  border-right: 1px solid var(--border-color);
  background: color-mix(in srgb, var(--bg-secondary) 72%, var(--bg-primary));
}
.group-panel-heading {
  display: flex;
  min-height: 45px;
  align-items: center;
  padding: 0 14px;
  border-bottom: 1px solid var(--border-color);
}
.group-panel-heading > div {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.group-panel-heading span {
  font-size: 12px;
  font-weight: 650;
}
.group-panel-heading small {
  color: var(--text-tertiary);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}
.group-search {
  position: relative;
  display: flex;
  align-items: center;
  margin: 10px 10px 6px;
}
.group-search :deep(svg) {
  position: absolute;
  left: 9px;
  color: var(--text-tertiary);
  pointer-events: none;
}
.group-search input {
  width: 100%;
  height: 32px;
  padding: 0 10px 0 31px;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  color: var(--text-primary);
  background: var(--bg-primary);
  font-size: 11px;
  outline: none;
}
.group-search input:focus {
  border-color: #2563eb;
  box-shadow: 0 0 0 2px color-mix(in srgb, #2563eb 14%, transparent);
}
.group-list {
  min-height: 0;
  overflow-y: auto;
  padding: 4px 6px 10px;
}
.group-row {
  position: relative;
  display: flex;
  width: 100%;
  min-height: 55px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 8px 8px 10px;
  border: 1px solid transparent;
  border-bottom-color: color-mix(in srgb, var(--border-color) 72%, transparent);
  border-radius: 6px;
  color: var(--text-primary);
  background: transparent;
  text-align: left;
}
.group-row:hover {
  background: var(--bg-hover);
}
.group-row.active {
  border-color: color-mix(in srgb, #2563eb 28%, var(--border-color));
  background: color-mix(in srgb, #2563eb 9%, var(--bg-primary));
}
.group-copy {
  display: grid;
  min-width: 0;
  gap: 2px;
}
.group-copy strong,
.group-copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.group-copy strong {
  font-size: 12px;
  font-weight: 620;
}
.group-copy small {
  color: var(--text-tertiary);
  font-size: 10px;
}
.group-state {
  display: flex;
  flex: none;
  align-items: center;
  gap: 6px;
}
.test-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #22c55e;
}
.lowest-mark {
  padding: 1px 5px;
  border-radius: 999px;
  color: #92400e;
  background: #fef3c7;
  font-size: 9px;
  font-weight: 700;
}
.group-empty {
  padding: 24px 10px;
  color: var(--text-tertiary);
  font-size: 11px;
  text-align: center;
}
.group-workspace {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 8px;
  background: color-mix(in srgb, var(--bg-secondary) 88%, var(--bg-primary));
}
.group-header {
  display: flex;
  min-height: 88px;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 16px 20px;
  border-block: 1px solid var(--border-color);
  background:
    linear-gradient(90deg, color-mix(in srgb, #2563eb 5%, transparent), transparent 55%),
    var(--bg-primary);
}
.group-identity {
  min-width: 0;
}
.section-label {
  display: block;
  color: var(--text-tertiary);
  font-size: 10px;
  font-weight: 700;
}
.group-name-line {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 9px;
  margin-top: 4px;
}
.group-name-line h2 {
  max-width: min(540px, 56vw);
  margin: 0;
  overflow: hidden;
  font-size: 19px;
  font-weight: 650;
  letter-spacing: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mode-chip {
  flex: none;
  padding: 2px 7px;
  border: 1px solid #bfdbfe;
  border-radius: 999px;
  color: #1d4ed8;
  background: #eff6ff;
  font-size: 9px;
  font-weight: 700;
}
.group-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 5px;
  color: var(--text-tertiary);
  font-size: 10px;
}
.group-meta span:not(:last-child)::after {
  content: "";
  display: inline-block;
  width: 2px;
  height: 2px;
  margin-left: 6px;
  border-radius: 50%;
  vertical-align: middle;
  background: currentColor;
  opacity: .7;
}
.detection-config {
  padding: 0;
  border-block: 1px solid var(--border-color);
  background: color-mix(in srgb, var(--bg-secondary) 62%, var(--bg-primary));
}
.config-title {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin: 0;
  padding: 11px 20px;
  border-bottom: 1px solid var(--border-color);
  background: color-mix(in srgb, var(--bg-secondary) 78%, var(--bg-primary));
}
.config-title > div {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.config-title strong {
  font-size: 12px;
  font-weight: 650;
}
.config-state {
  color: var(--text-tertiary);
  font-size: 10px;
}
.config-state.enabled {
  color: #047857;
}
.config-grid {
  display: grid;
  grid-template-columns: minmax(150px, .9fr) minmax(118px, .65fr) minmax(210px, 1.45fr) minmax(105px, .55fr);
  align-items: end;
  gap: 10px;
  padding: 13px 20px 15px;
}
.config-field,
.config-toggle {
  display: grid;
  min-width: 0;
  gap: 5px;
}
.config-field > span {
  color: var(--text-tertiary);
  font-size: 9px;
  font-weight: 650;
}
.config-field input,
.config-field select,
.config-toggle {
  height: 34px;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  color: var(--text-primary);
  background: var(--bg-primary);
  font-size: 11px;
}
.config-field input,
.config-field select {
  width: 100%;
  padding: 0 9px;
  outline: none;
}
.config-field select {
  color-scheme: inherit;
}
:global(.dark .config-field option) {
  color: #e5e7eb;
  background: #111827;
}
.config-field input:focus,
.config-field select:focus {
  border-color: #2563eb;
  box-shadow: 0 0 0 2px color-mix(in srgb, #2563eb 13%, transparent);
}
.config-field select:disabled {
  opacity: .55;
  cursor: not-allowed;
}
.config-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 9px;
}
.config-toggle > span {
  color: var(--text-secondary);
  font-size: 10px;
  font-weight: 620;
}
.switch {
  position: relative;
  width: 32px;
  height: 18px;
  flex: none;
  padding: 0;
  border: 0;
  border-radius: 999px;
  background: #64748b;
  opacity: .65;
}
.switch span {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #fff;
  transition: transform .16s ease;
}
.switch.on {
  background: #2563eb;
  opacity: 1;
}
.switch.on span {
  transform: translateX(14px);
}
.ranking-section {
  padding: 0 0 12px;
  border-block: 1px solid var(--border-color);
  background: var(--bg-primary);
}
.ranking-heading {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 18px;
  margin: 0;
  padding: 11px 20px;
  border-bottom: 1px solid var(--border-color);
  background: color-mix(in srgb, var(--bg-secondary) 52%, var(--bg-primary));
}
.ranking-heading > div {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.ranking-heading strong {
  font-size: 12px;
  font-weight: 650;
}
.ranking-heading p {
  margin: 0;
  color: var(--text-tertiary);
  font-size: 10px;
}
.ranking-table-wrap {
  margin: 0 20px;
  overflow-x: auto;
  border-top: 1px solid var(--border-color);
}
.ranking-table {
  width: 100%;
  min-width: 600px;
  border-collapse: collapse;
  table-layout: fixed;
}
.ranking-table th {
  height: 32px;
  padding: 0 8px;
  color: var(--text-tertiary);
  font-size: 10px;
  font-weight: 700;
  text-align: left;
  white-space: nowrap;
  background: color-mix(in srgb, var(--bg-secondary) 82%, var(--bg-primary));
  border-bottom: 1px solid var(--border-color);
}
.ranking-table th + th,
.ranking-table td + td {
  border-left: 1px solid var(--border-color);
}
.ranking-table td {
  height: 54px;
  padding: 7px 8px;
  border-top: 1px solid var(--border-color);
  vertical-align: middle;
}
.ranking-table tbody tr:hover {
  background: var(--bg-hover);
}
.ranking-table tbody tr:nth-child(even) {
  background: color-mix(in srgb, var(--bg-secondary) 24%, transparent);
}
.ranking-table tbody tr.lowest {
  background: color-mix(in srgb, #f59e0b 5%, transparent);
}
.ranking-table tbody tr:last-child td {
  border-bottom: 1px solid var(--border-color);
}
.rank-column {
  width: 34px;
}
.account-column {
  width: 29%;
}
.action-column {
  width: 36px;
  text-align: right;
}
.rank-number {
  display: inline-grid;
  width: 23px;
  height: 23px;
  place-items: center;
  border-radius: 6px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
  font-weight: 650;
}
.account-line {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 5px;
}
.account-line > strong {
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  font-weight: 620;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.account-column > small {
  display: block;
  margin-top: 2px;
  color: var(--text-tertiary);
  font-size: 10px;
}
.credential-chip,
.lowest-badge,
.probe-status {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  font-size: 9px;
  font-weight: 700;
  white-space: nowrap;
}
.credential-chip {
  padding: 1px 5px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
}
.credential-chip.oauth {
  color: #1d4ed8;
  background: #dbeafe;
}
.lowest-badge {
  padding: 1px 5px;
  color: #92400e;
  background: #fef3c7;
}
.availability {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 5px;
  color: #047857;
  font-size: 10px;
  white-space: nowrap;
}
.availability i {
  width: 5px;
  height: 5px;
  flex: none;
  border-radius: 50%;
  background: currentColor;
}
.availability.down {
  color: #b91c1c;
}
.value-stack,
.latest-cell {
  display: grid;
  min-width: 0;
  gap: 2px;
}
.value-stack strong {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  font-weight: 650;
}
.value-stack strong.good {
  color: #047857;
}
.value-stack strong.warning {
  color: #b45309;
}
.value-stack strong.danger {
  color: #b91c1c;
}
.value-stack small,
.latest-cell small {
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.probe-status {
  width: fit-content;
  padding: 1px 5px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
}
.probe-status.pass {
  color: #047857;
  background: #d1fae5;
}
.probe-status.degraded {
  color: #b45309;
  background: #fef3c7;
}
.probe-status.error {
  color: #b91c1c;
  background: #fee2e2;
}
.icon-action {
  display: inline-grid;
  width: 28px;
  height: 28px;
  place-items: center;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 6px;
  color: var(--text-secondary);
  background: transparent;
}
.icon-action:hover {
  border-color: var(--border-color);
  color: #2563eb;
  background: var(--bg-primary);
}
.probe-history {
  margin: 0;
  border-block: 1px solid var(--border-color);
  background: color-mix(in srgb, var(--bg-secondary) 48%, var(--bg-primary));
}
.probe-history summary {
  display: flex;
  min-height: 46px;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 0 20px;
  color: var(--text-secondary);
  cursor: pointer;
  list-style: none;
}
.probe-history summary::-webkit-details-marker {
  display: none;
}
.probe-history summary > span {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.probe-history summary strong {
  font-size: 11px;
  font-weight: 650;
}
.probe-history summary small,
.history-hint {
  color: var(--text-tertiary);
  font-size: 9px;
}
.probe-history summary::after {
  content: "+";
  color: var(--text-tertiary);
  font-size: 16px;
  line-height: 1;
}
.probe-history[open] summary::after {
  content: "-";
}
.probe-list {
  border-top: 1px solid var(--border-color);
  background: var(--bg-primary);
}
.probe-row {
  display: grid;
  grid-template-columns: 58px minmax(110px, .8fr) minmax(130px, 1fr) 64px 86px minmax(160px, 1.5fr);
  min-height: 39px;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid var(--border-color);
  font-size: 10px;
}
.probe-row > strong,
.probe-row > span,
.probe-row > small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.probe-row > small,
.probe-message {
  color: var(--text-tertiary);
}
.probe-empty,
.table-empty {
  padding: 28px 12px;
  color: var(--text-tertiary);
  font-size: 11px;
  text-align: center;
}
.loading-ledger,
.empty-state {
  overflow: hidden;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: var(--bg-primary);
}
.loading-ledger {
  padding: 4px 18px;
}
.skeleton-row {
  height: 58px;
  border-bottom: 1px solid var(--border-color);
  background: linear-gradient(90deg, transparent, color-mix(in srgb, var(--bg-secondary) 70%, transparent), transparent) 0 0 / 220% 100%;
  animation: shimmer 1.4s ease-in-out infinite;
}
.skeleton-row:last-child {
  border-bottom: 0;
}
.empty-state {
  display: flex;
  min-height: 240px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--text-tertiary);
}
.empty-state strong {
  color: var(--text-primary);
  font-size: 13px;
}
.empty-state span {
  font-size: 11px;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
@keyframes shimmer {
  to { background-position: -120% 0; }
}
@media (prefers-reduced-motion: reduce) {
  .spinning,
  .skeleton-row,
  .switch span {
    animation: none;
    transition: none;
  }
}
@media (max-width: 1100px) {
  .scheduler-workspace {
    grid-template-columns: 210px minmax(0, 1fr);
  }
  .config-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .ranking-heading {
    align-items: flex-start;
    flex-direction: column;
    gap: 3px;
  }
}
@media (max-width: 820px) {
  .scheduler-page {
    padding: 14px;
  }
  .command-bar {
    align-items: flex-start;
    flex-direction: column;
    gap: 12px;
  }
  .command-actions {
    width: 100%;
  }
  .command-actions button {
    flex: 1;
  }
  .metric-strip {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .metric-strip > div:nth-child(2) {
    border-right: 0;
  }
  .metric-strip > div:nth-child(-n+2) {
    border-bottom: 1px solid var(--border-color);
  }
  .scheduler-workspace {
    grid-template-columns: 1fr;
  }
  .group-panel {
    border-right: 0;
    border-bottom: 1px solid var(--border-color);
  }
  .group-panel-heading {
    min-height: 38px;
  }
  .group-list {
    display: flex;
    overflow-x: auto;
    padding: 4px 8px 8px;
  }
  .group-row {
    width: 205px;
    flex: none;
  }
  .group-row.active {
    border-color: color-mix(in srgb, #2563eb 28%, var(--border-color));
  }
  .group-header {
    align-items: flex-start;
    flex-direction: column;
    gap: 12px;
  }
  .group-name-line h2 {
    max-width: 75vw;
  }
  .config-grid {
    grid-template-columns: 1fr;
  }
  .ranking-section {
    padding-inline: 0;
  }
  .probe-history {
    margin-inline: 0;
  }
  .config-title,
  .config-grid,
  .ranking-heading,
  .probe-history summary {
    padding-inline: 14px;
  }
  .ranking-table-wrap {
    margin-inline: 14px;
  }
  .history-hint {
    display: none;
  }
  .probe-row {
    grid-template-columns: 58px minmax(100px, 1fr) 70px;
    padding-block: 7px;
  }
  .probe-row > span:nth-of-type(2),
  .probe-row > small {
    display: none;
  }
  .probe-message {
    grid-column: 1 / -1;
  }
}
</style>
