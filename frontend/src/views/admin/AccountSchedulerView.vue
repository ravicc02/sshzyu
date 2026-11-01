<template>
  <AppLayout>
    <main class="scheduler-page">
      <div class="scheduler-shell">
        <header class="monitor-toolbar">
          <p class="polling-caption">
            <span
              class="live-dot"
              :class="{ paused: overview?.advanced_enabled === false }"
            />
            {{
              overview?.advanced_enabled === false
                ? "评分调度未生效"
                : "评分调度运行中"
            }}
            <span v-if="overview"
              >· {{ formatTime(overview.generated_at) }} 更新</span
            >
          </p>
          <div class="actions">
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="loading"
              @click="load()"
            >
              <Icon name="refresh" size="sm" :class="{ spinning: loading }" />
              {{ loading ? "刷新中" : "刷新" }}
            </button>
            <button
              v-if="supportsDegradation"
              type="button"
              class="btn btn-primary"
              :disabled="!selectedGroup || !hasChanges || saving"
              @click="saveSettings"
            >
              <Icon name="check" size="sm" />
              {{ saving ? "保存中" : hasChanges ? "保存更改" : "已保存" }}
            </button>
          </div>
        </header>

        <p
          v-if="overview?.advanced_enabled === false"
          class="feedback feedback-warning"
          role="alert"
        >
          高级评分调度被系统设置显式关闭。当前页面仍可测试，但请求会走旧调度路径。
        </p>

        <section class="overview" aria-label="调度概览">
          <div>
            <span>可调度账号</span>
            <strong>
              {{ overview?.available_accounts ?? 0 }}
              <small>/ {{ overview?.total_accounts ?? 0 }}</small>
            </strong>
            <p>可用账号 / 账号总数</p>
          </div>
          <div>
            <span>自动探测</span>
            <strong>
              {{ overview?.enabled_groups ?? 0 }}
              <small>组</small>
            </strong>
            <p>{{ overview?.tested_groups ?? 0 }} 个分组已有探测记录</p>
          </div>
          <div>
            <span>当前分组账号</span>
            <strong>
              {{ selectedGroup?.accounts.length ?? 0 }}
              <small>个</small>
            </strong>
            <p class="overview-context" :title="selectedGroup?.name">
              {{ selectedGroup?.name || "暂无分组" }}
            </p>
          </div>
        </section>

        <section
          v-if="loading && !overview"
          class="loading-table"
          aria-label="加载中"
        >
          <div v-for="index in 7" :key="index" class="skeleton-row" />
        </section>
        <section
          v-else-if="!overview || overview.groups.length === 0"
          class="empty-state"
        >
          <Icon name="chartBar" size="lg" />
          <h2>暂无可调度分组</h2>
          <p>活跃分组及关联账号会出现在这里。</p>
        </section>

        <template v-else>
          <section
            class="scheduler-section"
            aria-labelledby="account-ranking-title"
          >
            <header class="section-heading">
              <div>
                <h2 id="account-ranking-title">
                  调度账号 <span>{{ filteredAccounts.length }}</span>
                </h2>
                <p>
                  {{
                    selectedGroup
                      ? `${platformLabel(selectedGroup.platform)} · ${selectedGroup.accounts.length} 个账号`
                      : "请选择分组"
                  }}
                  ·
                  {{
                    supportsDegradation
                      ? "单次未通过不等同账号降智"
                      : "仅按账号可用性排名"
                  }}
                </p>
              </div>
              <div class="actions">
                <span v-if="selectedGroup" class="mode-label">
                  {{
                    supportsDegradation
                      ? selectedGroup.oauth_first
                        ? "OAuth 优先"
                        : "统一评分池"
                      : "仅可用性"
                  }}
                </span>
                <label
                  v-if="supportsDegradation && selectedGroup"
                  class="test-model-select"
                >
                  <span>测试模型</span>
                  <select
                    v-if="selectedGroup.model_candidates.length"
                    v-model="draft.testModel"
                    :disabled="running"
                    aria-label="统一测试模型"
                  >
                    <option value="">自动模型</option>
                    <option
                      v-if="
                        draft.testModel &&
                        !selectedGroup.model_candidates.includes(draft.testModel)
                      "
                      :value="draft.testModel"
                    >
                      {{ draft.testModel }}
                    </option>
                    <option
                      v-for="model in selectedGroup.model_candidates"
                      :key="model"
                      :value="model"
                    >
                      {{ model }}
                    </option>
                  </select>
                  <input
                    v-else
                    v-model.trim="draft.testModel"
                    :disabled="running"
                    placeholder="填写测试模型"
                    aria-label="统一测试模型"
                  />
                </label>
                <button
                  v-if="supportsDegradation"
                  type="button"
                  class="btn btn-primary"
                  :disabled="!selectedGroup || running"
                  @click="runGroup"
                >
                  <Icon name="play" size="sm" />
                  {{ running ? "测试中" : "统一测试" }}
                </button>
              </div>
            </header>

            <div class="filters">
              <label class="search-control">
                <Icon name="search" size="sm" />
                <input
                  v-model.trim="accountQuery"
                  type="search"
                  placeholder="搜索账号名称、ID 或平台"
                  aria-label="搜索账号"
                />
              </label>
              <select
                v-model.number="selectedGroupID"
                aria-label="选择调度分组"
              >
                <option
                  v-for="group in overview.groups"
                  :key="group.id"
                  :value="group.id"
                >
                  {{ group.name }}
                </option>
              </select>
              <select v-model="availabilityFilter" aria-label="筛选账号可用性">
                <option value="">全部可用性</option>
                <option value="available">仅可用账号</option>
                <option value="unavailable">仅不可用账号</option>
                <option v-if="supportsDegradation" value="degraded">存在未通过记录</option>
                <option value="oauth">仅 OAuth 账号</option>
              </select>
            </div>

            <div v-if="supportsDegradation" class="config-band">
              <div class="config-intro">
                <span>自动探测配置</span>
                <strong :class="{ enabled: draft.enabled }">{{
                  draft.enabled ? "自动运行" : "仅手动运行"
                }}</strong>
                <small>固定使用同一提示词与判定标准，按分组独立配置。</small>
              </div>
              <div class="config-grid">
                <div class="config-toggle">
                  <span>自动探测</span>
                  <button
                    type="button"
                    class="switch"
                    :class="{ on: draft.enabled }"
                    :aria-pressed="draft.enabled"
                    @click="draft.enabled = !draft.enabled"
                  >
                    <span />
                  </button>
                </div>
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
                <div class="config-toggle">
                  <span>降智因子计入调度</span>
                  <button
                    type="button"
                    class="switch"
                    :class="{ on: draft.degradationFactorEnabled }"
                    :aria-pressed="draft.degradationFactorEnabled"
                    @click="
                      draft.degradationFactorEnabled =
                        !draft.degradationFactorEnabled
                    "
                  >
                    <span />
                  </button>
                </div>
              </div>
            </div>
            <div v-else class="availability-note">
              <Icon name="check" size="sm" />
              <div>
                <strong>按账号可用性排序</strong>
                <span>该类型分组不执行降智探测，只显示可用、冷却和停调状态。</span>
              </div>
            </div>

            <div class="table-scroll">
              <table
                class="ranking-table"
                :class="{ 'availability-only': !supportsDegradation }"
              >
                <thead>
                  <tr>
                    <th class="rank-column">排名</th>
                    <th>账号</th>
                    <th>可用性</th>
                    <th>调度次数</th>
                    <th v-if="supportsDegradation">通过率 / 降智率</th>
                    <th v-if="supportsDegradation">调度分</th>
                    <th v-if="supportsDegradation">最近探测</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="account in filteredAccounts"
                    :key="account.id"
                    :class="{ lowest: account.lowest_degradation }"
                  >
                    <td class="rank-column">
                      <span class="rank-number">{{ account.rank }}</span>
                    </td>
                    <td class="account-column">
                      <div class="account-line">
                        <strong :title="account.name">{{
                          account.name
                        }}</strong>
                        <span
                          v-if="supportsDegradation && account.lowest_degradation"
                          class="lowest-badge"
                          >最低降智</span
                        >
                      </div>
                      <small
                        >#{{ account.id }} ·
                        {{ platformLabel(account.platform) }}</small
                      >
                    </td>
                    <td class="state-cell">
                      <span
                        class="status-pill"
                        :class="account.available ? 'healthy' : 'danger'"
                      >
                        <i />
                        {{
                          account.available
                            ? "可用"
                            : account.availability_reason || "不可用"
                        }}
                      </span>
                      <small>{{ account.oauth ? "OAuth" : "API Key" }}</small>
                    </td>
                    <td class="dispatch-cell">
                      <div class="value-stack">
                        <strong>{{ formatCount(account.dispatch_count) }}</strong>
                        <small>近 24 小时</small>
                      </div>
                      <span
                        v-if="account.active_dispatches > 0"
                        class="active-dispatch"
                      >
                        <i />
                        正在调度
                      </span>
                    </td>
                    <td v-if="supportsDegradation">
                      <div class="value-stack">
                        <strong :class="passRateClass(account.pass_count, account.sample_count)">
                          {{ formatPassRate(account.pass_count, account.sample_count) }}
                        </strong>
                        <small>
                          降智率 {{ formatRate(account.degradation_rate) }} ·
                          {{ account.pass_count }}/{{ account.sample_count }} · 错误
                          {{ account.error_count }}
                        </small>
                      </div>
                    </td>
                    <td v-if="supportsDegradation">
                      <div class="value-stack">
                        <strong>
                          {{ formatScore(account.score) }}
                          <span class="score-max">
                            / {{ formatScore(selectedGroup?.score_max) }}
                          </span>
                        </strong>
                        <small v-if="selectedGroup?.degradation_factor_enabled">
                          降智因子
                          {{ account.degradation_factor.toFixed(2) }}
                        </small>
                        <small v-else>降智因子未计入</small>
                      </div>
                    </td>
                    <td v-if="supportsDegradation">
                      <div class="value-stack">
                        <span
                          class="probe-status"
                          :class="account.latest_status"
                        >
                          {{ probeStatusLabel(account.latest_status) }}
                        </span>
                        <small>{{
                          formatTime(account.latest_checked_at, true)
                        }}</small>
                      </div>
                    </td>
                  </tr>
                  <tr v-if="filteredAccounts.length === 0">
                    <td :colspan="supportsDegradation ? 7 : 4" class="table-empty">
                      没有符合筛选条件的账号。
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <footer class="table-caption">
              <span
                >显示 {{ filteredAccounts.length }} /
                {{ selectedGroup?.accounts.length ?? 0 }} 个账号</span
              >
              <span>{{
                supportsDegradation
                  ? "Priority 不参与排序 · OAuth 作为第一层路由"
                  : "Priority 不参与排序 · 仅按可用性排序"
              }}</span>
            </footer>
          </section>

          <section
            v-if="supportsDegradation"
            class="probe-section"
            aria-labelledby="probe-history-title"
          >
            <header class="section-heading">
              <div>
                <h2 id="probe-history-title">
                  探测记录 <span>{{ filteredProbes.length }}</span>
                </h2>
                <p>记录模型回复、请求错误与固定提示词判定结果。</p>
              </div>
              <div class="actions">
                <select v-model="probeStatusFilter" aria-label="筛选探测结果">
                  <option value="">全部结果</option>
                  <option value="pass">通过</option>
                  <option value="degraded">未通过</option>
                  <option value="error">请求失败</option>
                </select>
                <button
                  type="button"
                  class="icon-button"
                  title="刷新记录"
                  aria-label="刷新记录"
                  @click="load()"
                >
                  <Icon
                    name="refresh"
                    size="sm"
                    :class="{ spinning: loading }"
                  />
                </button>
              </div>
            </header>

            <p v-if="pagedProbes.length === 0" class="empty-log">
              当前分组暂无符合条件的探测记录。
            </p>
            <ol v-else class="probe-log">
              <li v-for="probe in pagedProbes" :key="probe.id">
                <span class="event-marker" :class="probe.status">
                  <Icon
                    :name="
                      probe.status === 'error'
                        ? 'x'
                        : probe.status === 'pass'
                          ? 'check'
                          : 'chart'
                    "
                    size="sm"
                  />
                </span>
                <div class="probe-content">
                  <div class="probe-heading">
                    <span class="probe-status" :class="probe.status">{{
                      probeStatusLabel(probe.status)
                    }}</span>
                    <strong>{{
                      probe.account_name || `账号 #${probe.account_id}`
                    }}</strong>
                    <span class="tag">{{ probe.model || "自动模型" }}</span>
                  </div>
                  <details>
                    <summary>
                      {{
                        probe.status === "error"
                          ? "查看错误信息"
                          : "查看模型回复"
                      }}
                    </summary>
                    <pre>{{
                      probe.status === "error"
                        ? probe.error_message
                        : probe.response_text || "—"
                    }}</pre>
                  </details>
                </div>
                <div class="probe-meta">
                  <strong>{{ formatLatency(probe.latency_ms) }}</strong>
                  <span>{{ formatTime(probe.checked_at, true) }}</span>
                </div>
              </li>
            </ol>

            <footer v-if="filteredProbes.length" class="probe-footer">
              <span
                >显示 {{ pagedProbes.length }} /
                {{ filteredProbes.length }} 条</span
              >
              <div>
                <button
                  type="button"
                  class="icon-button"
                  aria-label="上一页探测记录"
                  :disabled="probePage <= 1"
                  @click="probePage--"
                >
                  <Icon name="chevronLeft" size="sm" />
                </button>
                <span>{{ probePage }} / {{ probePageCount }}</span>
                <button
                  type="button"
                  class="icon-button"
                  aria-label="下一页探测记录"
                  :disabled="probePage >= probePageCount"
                  @click="probePage++"
                >
                  <Icon name="chevronRight" size="sm" />
                </button>
              </div>
            </footer>
          </section>
        </template>
      </div>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import AppLayout from "@/components/layout/AppLayout.vue";
import Icon from "@/components/icons/Icon.vue";
import { adminAPI } from "@/api/admin";
import type {
  AccountSchedulerGroup,
  AccountSchedulerOverview,
} from "@/api/admin/accountScheduler";
import { useAppStore } from "@/stores";

const appStore = useAppStore();
const overview = ref<AccountSchedulerOverview | null>(null);
const selectedGroupID = ref<number | null>(null);
const accountQuery = ref("");
const availabilityFilter = ref("");
const probeStatusFilter = ref("");
const probePage = ref(1);
const probePageSize = 10;
const loading = ref(false);
const saving = ref(false);
const running = ref(false);
const draft = reactive({
  enabled: false,
  degradationFactorEnabled: true,
  intervalSeconds: 600,
  testModel: "",
});
let overviewTimer: number | undefined;
let overviewRequestInFlight = false;

const selectedGroup = computed<AccountSchedulerGroup | null>(() => {
  if (!overview.value || selectedGroupID.value === null) return null;
  return (
    overview.value.groups.find((group) => group.id === selectedGroupID.value) ??
    null
  );
});

const supportsDegradation = computed(
  () => selectedGroup.value?.degradation_supported === true,
);

const filteredAccounts = computed(() => {
  const accounts = selectedGroup.value?.accounts ?? [];
  const query = accountQuery.value.toLowerCase();
  return accounts.filter((account) => {
    const matchesQuery =
      !query ||
      account.name.toLowerCase().includes(query) ||
      String(account.id).includes(query) ||
      account.platform.toLowerCase().includes(query);
    const matchesAvailability =
      availabilityFilter.value === "available"
        ? account.available
        : availabilityFilter.value === "unavailable"
          ? !account.available
          : supportsDegradation.value &&
              availabilityFilter.value === "degraded"
            ? account.degraded_count > 0
            : availabilityFilter.value === "oauth"
              ? account.oauth
              : true;
    return matchesQuery && matchesAvailability;
  });
});

const selectedProbes = computed(() => {
  if (!overview.value || !selectedGroup.value) return [];
  return overview.value.recent_probes
    .filter((probe) => probe.group_id === selectedGroup.value?.id)
    .slice(0, 50);
});

const filteredProbes = computed(() => {
  if (!probeStatusFilter.value) return selectedProbes.value;
  return selectedProbes.value.filter(
    (probe) => probe.status === probeStatusFilter.value,
  );
});

const probePageCount = computed(() =>
  Math.max(1, Math.ceil(filteredProbes.value.length / probePageSize)),
);

const pagedProbes = computed(() => {
  const start = (probePage.value - 1) * probePageSize;
  return filteredProbes.value.slice(start, start + probePageSize);
});

const hasChanges = computed(() => {
  const group = selectedGroup.value;
  if (!group || !overview.value || !supportsDegradation.value) return false;
  return (
    draft.enabled !== group.degradation_enabled ||
    draft.degradationFactorEnabled !== group.degradation_factor_enabled ||
    draft.intervalSeconds !== group.interval_seconds ||
    draft.testModel !== group.test_model
  );
});

watch(
  selectedGroup,
  (group, previousGroup) => {
    if (!group || previousGroup?.id === group.id) return;
    draft.enabled = group.degradation_supported && group.degradation_enabled;
    draft.degradationFactorEnabled = group.degradation_factor_enabled;
    draft.intervalSeconds = group.interval_seconds || 600;
    draft.testModel = group.degradation_supported ? group.test_model || "" : "";
  },
  { immediate: true },
);

watch(selectedGroupID, () => {
  accountQuery.value = "";
  availabilityFilter.value = "";
  probeStatusFilter.value = "";
  probePage.value = 1;
});

watch(probeStatusFilter, () => {
  probePage.value = 1;
});

watch(probePageCount, (count) => {
  if (probePage.value > count) probePage.value = count;
});

async function load(silent = false) {
  if (overviewRequestInFlight) return;
  overviewRequestInFlight = true;
  if (!silent) loading.value = true;
  try {
    const data = await adminAPI.accountScheduler.getOverview();
    overview.value = data;
    const preferred =
      selectedGroupID.value &&
      data.groups.some((group) => group.id === selectedGroupID.value)
        ? selectedGroupID.value
        : (data.groups[0]?.id ?? null);
    selectedGroupID.value = preferred;
  } catch (error: any) {
    if (!silent) {
      appStore.showError(error?.message || "账号调度数据加载失败");
    }
  } finally {
    overviewRequestInFlight = false;
    if (!silent) loading.value = false;
  }
}

async function saveSettings() {
  if (!selectedGroup.value || !supportsDegradation.value) return;
  saving.value = true;
  try {
    await adminAPI.accountScheduler.updateSetting({
      group_id: selectedGroup.value.id,
      degradation_enabled: draft.enabled,
      degradation_factor_enabled: draft.degradationFactorEnabled,
      interval_seconds: draft.intervalSeconds,
      test_model: draft.testModel,
    });
    appStore.showSuccess("账号调度配置已保存");
    await load(true);
  } catch (error: any) {
    appStore.showError(error?.message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function runGroup() {
  if (!selectedGroup.value || !supportsDegradation.value) return;
  running.value = true;
  try {
    const result = await adminAPI.accountScheduler.runNow({
      group_id: selectedGroup.value.id,
      model: draft.testModel,
    });
    const summary = `测试完成：${result.run} 条，跳过 ${result.skipped} 条，失败 ${result.failed} 条`;
    const errorMessage = firstRunError(result);
    if (errorMessage) {
      appStore.showError(`${summary}；错误：${errorMessage}`);
    } else if (result.run === 0) {
      appStore.showError(`没有执行测试，跳过 ${result.skipped} 条`);
    } else {
      appStore.showSuccess(summary);
    }
    await load(true);
  } catch (error: any) {
    appStore.showError(error?.message || "立即测试失败");
  } finally {
    running.value = false;
  }
}

function platformLabel(platform: string) {
  const labels: Record<string, string> = {
    openai: "OpenAI",
    deepseek: "DeepSeek",
    zhipu: "GLM",
    kimi: "Kimi",
    minimax: "MiniMax",
    composite: "聚合",
    grok: "Grok",
  };
  return labels[platform] || platform;
}

function probeStatusLabel(status: string) {
  return (
    (
      { pass: "通过", degraded: "未通过", error: "请求失败" } as Record<
        string,
        string
      >
    )[status] || "未测试"
  );
}

function firstRunError(result: {
  errors?: string[];
  results: Array<{ status: string; error_message: string }>;
}) {
  const probeError = result.results.find(
    (probe) => probe.status === "error" && probe.error_message,
  )?.error_message;
  return result.errors?.[0] || probeError || "";
}

function formatRate(rate?: number) {
  return typeof rate === "number" ? `${(rate * 100).toFixed(1)}%` : "暂无";
}

function formatPassRate(passCount: number, sampleCount: number) {
  if (!sampleCount) return "暂无";
  return `${((passCount / sampleCount) * 100).toFixed(1)}%`;
}

function passRateClass(passCount: number, sampleCount: number) {
  if (!sampleCount) return "";
  const rate = passCount / sampleCount;
  if (rate < 0.5) return "danger";
  if (rate < 0.8) return "warning";
  return "good";
}

function formatScore(score?: number) {
  return typeof score === "number" && Number.isFinite(score)
    ? score.toFixed(2)
    : "-";
}

function formatCount(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value || 0);
}

function formatLatency(value: number) {
  return value > 0 ? `${Math.round(value)} ms` : "-";
}

function formatTime(value?: string, withDate = false) {
  if (!value) return "未测试";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(
    "zh-CN",
    withDate
      ? { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }
      : { hour: "2-digit", minute: "2-digit" },
  ).format(date);
}

onMounted(() => {
  void load();
  overviewTimer = window.setInterval(() => void load(true), 15000);
});

onBeforeUnmount(() => {
  if (overviewTimer !== undefined) {
    window.clearInterval(overviewTimer);
  }
});
</script>

<style scoped>
.scheduler-page {
  --monitor-bg: #fff;
  --monitor-soft: #f6f6f7;
  --monitor-input: #fff;
  --monitor-ink: #202124;
  --monitor-muted: #686a73;
  --monitor-line: #e8e8ec;
  --monitor-selected: #edf5ff;
  --monitor-accent: #0071e3;
  --monitor-warning: #986017;
  --monitor-danger: #c23842;
  --monitor-positive: #237653;
  min-height: 100%;
  padding: 20px 24px 40px;
  color: var(--monitor-ink);
  background: var(--bg-secondary);
  color-scheme: light;
}
.scheduler-shell {
  display: grid;
  width: min(1440px, 100%);
  margin-inline: auto;
  gap: 28px;
  font-size: 13px;
}
.monitor-toolbar,
.section-heading,
.filters,
.table-caption,
.probe-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.monitor-toolbar {
  flex-wrap: wrap;
}
.polling-caption {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--monitor-muted);
  font-size: 12px;
}
.live-dot {
  width: 6px;
  height: 6px;
  flex: none;
  border-radius: 50%;
  background: var(--monitor-positive);
}
.live-dot.paused {
  background: var(--monitor-warning);
}
.actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.btn {
  display: inline-flex;
  min-height: 38px;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 0 13px;
  border: 1px solid var(--monitor-line);
  border-radius: 8px;
  font-size: 12px;
  font-weight: 550;
  white-space: nowrap;
  transition:
    border-color 0.15s,
    background-color 0.15s,
    color 0.15s;
}
.btn-secondary {
  color: var(--monitor-ink);
  background: var(--monitor-bg);
}
.btn-secondary:hover {
  border-color: color-mix(
    in srgb,
    var(--monitor-accent) 35%,
    var(--monitor-line)
  );
  background: var(--monitor-soft);
}
.btn-primary {
  color: #fff;
  border-color: #0071e3;
  background: #0071e3;
}
.btn-primary:hover {
  background: #0064c8;
}
.btn:disabled,
.icon-button:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.spinning {
  animation: scheduler-spin 0.9s linear infinite;
}
.feedback {
  padding: 12px 14px;
  border-radius: 9px;
  line-height: 1.6;
}
.feedback-warning {
  color: var(--monitor-warning);
  background: color-mix(in srgb, var(--monitor-warning) 9%, var(--monitor-bg));
}
.overview {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  padding-bottom: 26px;
  border-bottom: 1px solid var(--monitor-line);
}
.overview > div {
  min-width: 0;
  padding: 0 25px;
}
.overview > div:first-child {
  padding-left: 0;
}
.overview > div + div {
  border-left: 1px solid var(--monitor-line);
}
.overview span {
  color: var(--monitor-muted);
  font-size: 12px;
}
.overview strong {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 12px 0 8px;
  overflow: hidden;
  font-size: 32px;
  font-weight: 600;
  letter-spacing: -0.035em;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.overview strong small {
  color: var(--monitor-muted);
  font-size: 12px;
  font-weight: 400;
  letter-spacing: 0;
}
.overview p {
  overflow: hidden;
  color: var(--monitor-muted);
  font-size: 11px;
  line-height: 1.6;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.section-heading {
  align-items: flex-start;
}
.section-heading h2 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
}
.section-heading h2 span {
  padding: 2px 6px;
  border-radius: 5px;
  color: var(--monitor-muted);
  background: var(--monitor-soft);
  font-size: 11px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}
.section-heading p {
  margin-top: 5px;
  color: var(--monitor-muted);
  font-size: 12px;
  line-height: 1.65;
}
.mode-label {
  display: inline-flex;
  min-height: 30px;
  align-items: center;
  padding: 0 9px;
  border-radius: 6px;
  color: var(--monitor-accent);
  background: var(--monitor-selected);
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}
.test-model-select {
  display: inline-flex;
  align-items: center;
  gap: 7px;
}
.test-model-select > span {
  color: var(--monitor-muted);
  font-size: 10px;
  white-space: nowrap;
}
.test-model-select select,
.test-model-select input {
  width: 160px;
  min-height: 34px;
  padding-block: 6px;
}
.filters {
  margin: 18px 0 14px;
}
.search-control {
  position: relative;
  display: flex;
  min-width: 180px;
  flex: 1;
  align-items: center;
}
.search-control :deep(svg) {
  position: absolute;
  left: 12px;
  color: var(--monitor-muted);
  pointer-events: none;
}
input:not([type="checkbox"]),
select {
  width: 100%;
  min-width: 0;
  min-height: 39px;
  padding: 8px 11px;
  border: 1px solid var(--monitor-line);
  border-radius: 8px;
  color: var(--monitor-ink);
  background: var(--monitor-input);
  font-size: 12px;
  line-height: 1.5;
  outline: none;
}
.search-control input {
  padding-left: 36px;
}
.filters > select {
  width: min(260px, 24%);
}
select option {
  color: var(--monitor-ink);
  background: var(--monitor-input);
}
input:focus,
select:focus {
  border-color: var(--monitor-accent);
  box-shadow: 0 0 0 2px
    color-mix(in srgb, var(--monitor-accent) 20%, transparent);
}
.config-band {
  display: grid;
  grid-template-columns: minmax(180px, 0.75fr) minmax(0, 2.25fr);
  gap: 22px;
  padding: 15px 16px;
  border-block: 1px solid var(--monitor-line);
  background: color-mix(in srgb, var(--monitor-soft) 72%, var(--monitor-bg));
}
.availability-note {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 13px 16px;
  border-block: 1px solid var(--monitor-line);
  color: var(--monitor-muted);
  background: color-mix(in srgb, var(--monitor-soft) 58%, var(--monitor-bg));
}
.availability-note > :deep(svg) {
  flex: none;
  color: var(--monitor-positive);
}
.availability-note > div {
  display: flex;
  align-items: baseline;
  gap: 9px;
  min-width: 0;
}
.availability-note strong {
  color: var(--monitor-ink);
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}
.availability-note span {
  font-size: 11px;
  line-height: 1.6;
}
.config-intro {
  display: grid;
  align-content: center;
  gap: 3px;
}
.config-intro > span {
  color: var(--monitor-muted);
  font-size: 10px;
  font-weight: 600;
}
.config-intro strong {
  font-size: 12px;
  font-weight: 600;
}
.config-intro strong.enabled {
  color: var(--monitor-positive);
}
.config-intro small {
  color: var(--monitor-muted);
  font-size: 10px;
  line-height: 1.55;
}
.config-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  align-items: end;
  gap: 10px;
}
.config-field,
.config-toggle {
  display: grid;
  min-width: 0;
  gap: 5px;
}
.config-field > span,
.config-toggle > span {
  color: var(--monitor-muted);
  font-size: 10px;
}
.config-field input,
.config-field select {
  min-height: 34px;
  padding-block: 6px;
}
.config-toggle {
  height: 34px;
  grid-template-columns: 1fr auto;
  align-items: center;
  padding: 0 9px;
  border: 1px solid var(--monitor-line);
  border-radius: 8px;
  background: var(--monitor-input);
}
.switch {
  position: relative;
  width: 32px;
  height: 18px;
  padding: 0;
  border: 0;
  border-radius: 999px;
  background: #64748b;
  opacity: 0.65;
}
.switch > span {
  position: absolute;
  top: 3px;
  left: 3px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #fff;
  transition: transform 0.16s ease;
}
.switch.on {
  background: var(--monitor-accent);
  opacity: 1;
}
.switch.on > span {
  transform: translateX(14px);
}
.table-scroll {
  overflow-x: auto;
  border: 1px solid var(--monitor-line);
  border-radius: 12px;
  background: var(--monitor-bg);
}
.ranking-table {
  width: 100%;
  min-width: 900px;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: 12px;
  text-align: left;
}
.ranking-table.availability-only {
  min-width: 560px;
}
.ranking-table th {
  padding: 12px 13px;
  color: var(--monitor-muted);
  background: var(--monitor-soft);
  font-size: 11px;
  font-weight: 500;
  white-space: nowrap;
}
.ranking-table td {
  padding: 14px 13px;
  border-top: 1px solid var(--monitor-line);
  vertical-align: middle;
}
.ranking-table tbody tr {
  transition: background-color 0.15s;
}
.ranking-table tbody tr:hover {
  background: var(--monitor-soft);
}
.ranking-table tbody tr.lowest {
  background: color-mix(in srgb, var(--monitor-warning) 5%, transparent);
}
.rank-column {
  width: 62px;
}
.account-column {
  width: 30%;
}
.rank-number {
  display: inline-grid;
  width: 25px;
  height: 25px;
  place-items: center;
  border-radius: 6px;
  color: var(--monitor-muted);
  background: var(--monitor-soft);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}
.account-line {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
}
.account-line strong {
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.account-column > small,
.value-stack small,
.state-cell small {
  display: block;
  margin-top: 3px;
  color: var(--monitor-muted);
  font-size: 10px;
  line-height: 1.55;
}
.lowest-badge,
.tag {
  display: inline-flex;
  flex: none;
  align-items: center;
  padding: 2px 6px;
  border-radius: 5px;
  font-size: 10px;
  line-height: 1.45;
}
.lowest-badge {
  color: var(--monitor-warning);
  background: color-mix(in srgb, var(--monitor-warning) 12%, transparent);
  font-weight: 600;
}
.tag {
  max-width: 190px;
  overflow: hidden;
  color: var(--monitor-muted);
  background: var(--monitor-soft);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.state-cell {
  min-width: 120px;
}
.dispatch-cell {
  min-width: 118px;
}
.active-dispatch {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-top: 6px;
  color: var(--monitor-positive);
  font-size: 10px;
  font-weight: 600;
}
.active-dispatch i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  box-shadow: 0 0 0 3px
    color-mix(in srgb, var(--monitor-positive) 13%, transparent);
}
.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  line-height: 1.6;
}
.status-pill i {
  width: 5px;
  height: 5px;
  flex: none;
  border-radius: 50%;
  background: currentColor;
}
.healthy,
.good,
.pass {
  color: var(--monitor-positive);
}
.warning,
.degraded {
  color: var(--monitor-warning);
}
.danger,
.error {
  color: var(--monitor-danger);
}
.value-stack {
  display: grid;
  min-width: 0;
  gap: 1px;
}
.value-stack > strong {
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}
.score-max {
  color: var(--monitor-muted);
  font-size: 10px;
  font-weight: 400;
}
.probe-status {
  font-size: 11px;
  font-weight: 600;
}
.icon-button {
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 7px;
  color: var(--monitor-muted);
}
.icon-button:not(:disabled):hover {
  color: var(--monitor-ink);
  background: var(--monitor-soft);
}
.table-caption {
  margin-top: 10px;
  color: var(--monitor-muted);
  font-size: 11px;
}
.table-empty,
.empty-log {
  padding: 30px 16px;
  color: var(--monitor-muted);
  font-size: 12px;
  text-align: center;
}
.probe-section {
  padding-top: 24px;
  border-top: 1px solid var(--monitor-line);
}
.probe-section .section-heading select {
  width: 150px;
  min-height: 34px;
}
.probe-log li {
  display: flex;
  gap: 12px;
  padding: 14px 0;
  border-bottom: 1px solid var(--monitor-line);
}
.event-marker {
  display: flex;
  width: 30px;
  height: 30px;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: var(--monitor-muted);
  background: var(--monitor-soft);
}
.probe-content {
  min-width: 0;
  flex: 1;
}
.probe-heading {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.probe-heading strong {
  font-size: 12px;
  font-weight: 600;
}
.probe-content details {
  margin-top: 6px;
}
.probe-content summary {
  width: fit-content;
  color: var(--monitor-muted);
  font-size: 10px;
  cursor: pointer;
}
.probe-content pre {
  max-height: 180px;
  margin-top: 7px;
  overflow: auto;
  padding: 10px 12px;
  border-radius: 7px;
  color: var(--monitor-ink);
  background: var(--monitor-soft);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 10px;
  line-height: 1.65;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.probe-meta {
  display: grid;
  flex: none;
  align-content: start;
  justify-items: end;
  gap: 4px;
  color: var(--monitor-muted);
  font-size: 10px;
  white-space: nowrap;
}
.probe-meta strong {
  color: var(--monitor-ink);
  font-size: 11px;
  font-weight: 500;
}
.probe-footer {
  justify-content: flex-end;
  margin-top: 12px;
  color: var(--monitor-muted);
  font-size: 11px;
}
.probe-footer > div {
  display: flex;
  align-items: center;
  gap: 5px;
}
.loading-table {
  overflow: hidden;
  border: 1px solid var(--monitor-line);
  border-radius: 12px;
}
.skeleton-row {
  height: 58px;
  border-top: 1px solid var(--monitor-line);
  background: linear-gradient(
    90deg,
    var(--monitor-soft),
    var(--monitor-bg),
    var(--monitor-soft)
  );
  background-size: 220% 100%;
  animation: skeleton-sweep 1.4s linear infinite;
}
.skeleton-row:first-child {
  border-top: 0;
}
.empty-state {
  display: grid;
  justify-items: center;
  gap: 9px;
  padding: 70px 20px;
  color: var(--monitor-muted);
  text-align: center;
}
.empty-state h2 {
  color: var(--monitor-ink);
  font-size: 15px;
  font-weight: 600;
}
.empty-state p {
  font-size: 12px;
}
@keyframes scheduler-spin {
  to {
    transform: rotate(360deg);
  }
}
@keyframes skeleton-sweep {
  to {
    background-position: -220% 0;
  }
}
@media (max-width: 980px) {
  .overview > div {
    padding-inline: 16px;
  }
  .config-band {
    grid-template-columns: 1fr;
    gap: 12px;
  }
}
@media (max-width: 700px) {
  .scheduler-page {
    padding: 16px 14px 32px;
  }
  .scheduler-shell {
    gap: 22px;
  }
  .monitor-toolbar > .actions,
  .section-heading > .actions {
    width: 100%;
  }
  .overview {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    row-gap: 24px;
  }
  .overview > div:nth-child(3) {
    padding-left: 0;
    border-left: 0;
  }
  .overview strong {
    font-size: 28px;
  }
  .section-heading {
    flex-direction: column;
  }
  .filters {
    align-items: stretch;
    flex-direction: column;
  }
  .filters .search-control {
    flex-basis: auto;
  }
  .filters > select {
    width: 100%;
  }
  .config-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .availability-note > div {
    align-items: flex-start;
    flex-direction: column;
    gap: 2px;
  }
  .availability-note strong {
    white-space: normal;
  }
  .model-field {
    grid-column: 1 / -1;
    grid-row: 2;
  }
  .table-caption,
  .probe-footer {
    align-items: flex-start;
    flex-direction: column;
  }
  .probe-log li {
    flex-wrap: wrap;
  }
  .probe-meta {
    width: 100%;
    grid-column: 2;
    justify-items: start;
    padding-left: 42px;
  }
}
@media (prefers-reduced-motion: reduce) {
  .spinning,
  .skeleton-row {
    animation: none;
  }
  .switch > span {
    transition: none;
  }
}
</style>

<style>
/* Kept unscoped because Vue's production scoped-CSS compiler drops the
   equivalent :global(.dark) rule and leaves the monitor tokens light. */
.dark .scheduler-page {
  --monitor-bg: #1e1e21;
  --monitor-soft: #28282c;
  --monitor-input: #252528;
  --monitor-ink: #f5f5f7;
  --monitor-muted: #aaaab3;
  --monitor-line: #35353b;
  --monitor-selected: #22334a;
  --monitor-accent: #8ac0ff;
  --monitor-warning: #e8b668;
  --monitor-danger: #ff9ba1;
  --monitor-positive: #7bd4aa;
  color-scheme: dark;
}
</style>
