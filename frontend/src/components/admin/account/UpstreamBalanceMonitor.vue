<template>
  <section class="upstream-theme upstream-monitor" aria-label="上游总余额监控">
    <header class="monitor-toolbar">
      <p class="polling-caption">
        <span class="live-dot" />每 5 分钟同步余额 · 仅监控已添加的上游
      </p>
      <div class="actions">
        <button class="btn btn-secondary" @click="openChannel">
          <Icon name="cog" size="sm" />飞书通知设置
        </button>
        <button class="btn btn-secondary" :disabled="loading" @click="load">
          <Icon name="refresh" size="sm" :class="{ spinning: loading }" />{{
            loading ? "刷新中…" : "刷新"
          }}
        </button>
        <button class="btn btn-primary" @click="openCandidates">
          <Icon name="plus" size="sm" />获取上游账号
        </button>
      </div>
    </header>
    <p v-if="error" class="feedback feedback-error" role="alert">
      {{ error
      }}<button aria-label="关闭提示" @click="error = ''">
        <Icon name="x" size="sm" />
      </button>
    </p>
    <p v-if="notice" class="feedback" role="status">
      {{ notice
      }}<button aria-label="关闭通知" @click="notice = ''">
        <Icon name="x" size="sm" />
      </button>
    </p>
    <div class="overview" aria-label="监控概览">
      <div>
        <span>已添加上游</span
        ><strong>{{ sites.length }}<small>个</small></strong>
        <p>{{ sites.filter((s) => s.enabled).length }} 个正在监控</p>
      </div>
      <div>
        <span>低余额预警</span
        ><strong :class="{ 'text-warning': lowCount }"
          >{{ lowCount }}<small>个</small></strong
        >
        <p>按各上游阈值判断</p>
      </div>
      <div>
        <span>需要处理</span
        ><strong :class="{ 'text-warning': exceptionCount }"
          >{{ exceptionCount }}<small>个</small></strong
        >
        <p>查询异常或凭证待配置</p>
      </div>
      <button class="channel-summary" @click="openChannel">
        <span>飞书通知</span
        ><strong class="channel-state"
          ><Icon :name="channel.has_secret ? 'check' : 'cog'" size="sm" />{{
            channel.has_secret ? "已配置" : "待配置"
          }}</strong
        >
        <p>查看通知设置 <Icon name="chevronRight" size="xs" /></p>
      </button>
    </div>
    <section aria-label="上游账户">
      <div class="section-heading">
        <div>
          <h2>
            监控上游 <span>{{ sites.length }}</span>
          </h2>
          <p>账户总余额，不包含 key 额度或订阅配额。</p>
        </div>
      </div>
      <div class="filters">
        <label class="search-control"
          ><Icon name="search" size="sm" /><input
            v-model="search"
            placeholder="搜索上游名称或 Base URL"
            aria-label="筛选上游"
        /></label>
        <select v-model="groupFilter" aria-label="筛选关联分组">
          <option value="">全部分组</option>
          <option v-for="g in groups" :key="g.id" :value="String(g.id)">
            {{ g.name }}
          </option>
        </select>
        <select v-model="statusFilter" aria-label="筛选监控状态">
          <option value="">全部状态</option>
          <option value="low">低余额</option>
          <option value="error">查询异常</option>
          <option value="unconfigured">待设阈值</option>
          <option value="paused">已暂停</option>
        </select>
      </div>
      <div
        v-if="selectedSites.length"
        class="selection-bar"
        role="region"
        aria-label="批量操作"
      >
        <span
          >已选择 <b>{{ selectedSites.length }}</b> 项</span
        >
        <div class="actions">
          <button
            :disabled="operationBusy"
            @click="runBatch('refresh', selectedSites)"
          >
            刷新余额</button
          ><button :disabled="operationBusy" @click="openBulkThreshold">
            设置阈值</button
          ><button
            :disabled="operationBusy"
            @click="runBatch('enable', selectedSites)"
          >
            启用</button
          ><button
            :disabled="operationBusy"
            @click="runBatch('pause', selectedSites)"
          >
            暂停</button
          ><button
            class="text-danger"
            :disabled="operationBusy"
            @click="removeTargets = [...selectedSites]"
          >
            移出</button
          ><button :disabled="operationBusy" @click="checkedSites = new Set()">
            取消选择
          </button>
        </div>
      </div>
      <div v-if="loading && !sites.length" class="empty-state" role="status">
        正在加载监控上游…
      </div>
      <div v-else-if="!sites.length" class="empty-state">
        <Icon name="chart" size="xl" />
        <h3>添加第一个监控上游</h3>
        <p>从账号池选择上游，确认类型后即可同步总余额。</p>
        <button class="btn btn-primary" @click="openCandidates">
          获取上游账号
        </button>
      </div>
      <div v-else class="table-scroll">
        <table>
          <thead>
            <tr>
              <th class="check-cell">
                <input
                  type="checkbox"
                  aria-label="全选当前结果"
                  :checked="allVisibleSelected"
                  :indeterminate="someVisibleSelected && !allVisibleSelected"
                  :disabled="operationBusy || !visibleSites.length"
                  @change="
                    selectVisible(($event.target as HTMLInputElement).checked)
                  "
                />
              </th>
              <th>上游账户</th>
              <th>关联分组</th>
              <th class="number">账户总余额</th>
              <th>监控状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="site in visibleSites"
              :key="site.id"
              :data-site-id="site.id"
              :class="{ 'row-selected': checkedSites.has(site.id) }"
            >
              <td class="check-cell">
                <input
                  type="checkbox"
                  :aria-label="'选择 ' + site.display_name"
                  :checked="checkedSites.has(site.id)"
                  :disabled="operationBusy"
                  @change="
                    selectSite(
                      site.id,
                      ($event.target as HTMLInputElement).checked,
                    )
                  "
                />
              </td>
              <td class="upstream-cell">
                <a
                  class="upstream-name"
                  :href="site.base_url"
                  :title="site.base_url"
                  target="_blank"
                  rel="noopener noreferrer"
                >{{ shortName(site) }}</a>
                <span class="type-label">{{
                  providerLabel(site.provider_type)
                }}</span>
              </td>
              <td class="groups-cell">
                <div class="group-tags">
                  <span
                    v-for="g in site.groups.slice(0, 2)"
                    :key="g.id"
                    class="tag"
                    :title="g.name"
                    >{{ g.name }}</span
                  ><span
                    v-if="site.groups.length > 2"
                    class="tag"
                    :title="
                      site.groups
                        .slice(2)
                        .map((g) => g.name)
                        .join('\n')
                    "
                    >+{{ site.groups.length - 2 }}</span
                  ><span v-if="!site.groups.length" class="muted"
                    >未关联分组</span
                  >
                </div>
              </td>
              <td class="number">
                <div class="balance-value">
                  <strong
                    class="balance"
                    :class="{ 'text-warning': site.alert_active }"
                    >{{ site.amount ?? "—" }}</strong
                  ><span>{{ site.currency }}</span>
                </div>
                <button
                  class="threshold-link"
                  :disabled="operationBusy"
                  @click="editSite(site)"
                >
                  {{
                    site.threshold !== null
                      ? "低于 " + site.threshold + " 预警"
                      : "设置预警阈值"
                  }}
                </button>
              </td>
              <td class="state-cell">
                <span class="status-pill" :class="statusTone(site)"
                  ><span />{{ stateLabel(site) }}</span
                ><small>{{ time(site.last_success_at) }}</small
                ><small
                  v-if="site.error_code"
                  class="text-danger"
                  :title="errorText(site.error_code)"
                  >{{ errorText(site.error_code) }}</small
                ><small v-else-if="isStale(site)" class="text-warning"
                  >余额待更新</small
                >
              </td>
              <td>
                <div class="row-actions">
                  <button
                    :disabled="operationBusy"
                    :aria-label="'刷新余额 ' + site.display_name"
                    title="刷新余额"
                    @click="runBatch('refresh', [site])"
                  >
                    <Icon name="refresh" size="sm" /></button
                  ><button
                    :disabled="operationBusy"
                    title="预警设置"
                    :aria-label="'预警设置 ' + site.display_name"
                    @click="editSite(site)"
                  >
                    <Icon name="cog" size="sm" /></button
                  ><button
                    title="查看日志"
                    :aria-label="'查看日志 ' + site.display_name"
                    @click="showSiteLogs(site)"
                  >
                    <Icon name="clock" size="sm" /></button
                  ><button
                    :disabled="operationBusy"
                    :title="site.enabled ? '暂停监控' : '启用监控'"
                    :aria-label="
                      (site.enabled ? '暂停监控 ' : '启用监控 ') +
                      site.display_name
                    "
                    @click="runBatch(site.enabled ? 'pause' : 'enable', [site])"
                  >
                    <Icon
                      :name="site.enabled ? 'pauseCircle' : 'play'"
                      size="sm"
                    /></button
                  ><button
                    :disabled="operationBusy"
                    title="移出监控"
                    :aria-label="'移出监控 ' + site.display_name"
                    @click="removeTargets = [site]"
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="!visibleSites.length">
              <td colspan="6" class="empty-state">没有符合筛选条件的上游</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="sites.length" class="table-caption">
        <span>显示 {{ visibleSites.length }} / {{ sites.length }} 个上游</span
        ><span>预警默认启用 · 默认阈值 5</span>
      </div>
    </section>
    <section ref="logsElement" class="logs" aria-label="监控日志">
      <header class="section-heading">
        <div>
          <h2>
            执行日志 <span>{{ events.total }}</span>
          </h2>
          <p>识别、查询、预警与通知投递记录。</p>
        </div>
        <div class="actions">
          <select
            v-model="logKind"
            aria-label="日志类型"
            @change="
              logPage = 1;
              loadEvents();
            "
          >
            <option value="">全部日志</option>
            <option value="identify">识别</option>
            <option value="query">查询</option>
            <option value="alert">预警</option>
            <option value="delivery">推送</option>
            <option value="settings">配置</option></select
          ><select
            v-model="logSite"
            aria-label="日志上游"
            @change="
              logPage = 1;
              loadEvents();
            "
          >
            <option value="">全部上游</option>
            <option v-for="site in sites" :key="site.id" :value="site.id">
              {{ shortName(site) }}
            </option></select
          ><button
            class="icon-button"
            aria-label="刷新日志"
            title="刷新日志"
            @click="loadEvents"
          >
            <Icon name="refresh" size="sm" />
          </button>
        </div>
      </header>
      <p v-if="!events.items.length" class="empty-log">暂无执行记录</p>
      <ol v-else>
        <li v-for="event in events.items" :key="event.id">
          <span
            class="event-marker"
            :class="{ warning: event.level === 'warning' }"
            ><Icon
              :name="
                event.kind === 'delivery'
                  ? 'link'
                  : event.kind === 'query'
                    ? 'refresh'
                    : event.kind === 'settings'
                      ? 'cog'
                      : 'clock'
              "
              size="sm"
          /></span>
          <div class="event-content">
            <div>
              <span class="tag">{{ kindLabel(event.kind) }}</span
              ><strong :class="{ 'text-warning': event.level === 'warning' }">{{
                event.message
              }}</strong
              ><span v-if="event.details.status" class="delivery-label">{{
                deliveryLabel(String(event.details.status))
              }}</span>
            </div>
            <small v-if="eventDetail(event)">{{ eventDetail(event) }}</small>
          </div>
          <time>{{ time(event.created_at) }}</time>
        </li>
      </ol>
      <footer>
        <span>共 {{ events.total }} 条</span
        ><button
          class="icon-button"
          aria-label="上一页日志"
          :disabled="logPage <= 1"
          @click="
            logPage--;
            loadEvents();
          "
        >
          <Icon name="chevronLeft" size="sm" /></button
        ><span>{{ logPage }}</span
        ><button
          class="icon-button"
          aria-label="下一页日志"
          :disabled="logPage * events.page_size >= events.total"
          @click="
            logPage++;
            loadEvents();
          "
        >
          <Icon name="chevronRight" size="sm" />
        </button>
      </footer>
    </section>
  </section>
  <BaseDialog
    :show="candidateOpen"
    title="获取上游账号"
    width="wide"
    @close="closeCandidates"
  >
    <div class="upstream-theme candidate-dialog">
      <nav class="import-steps" aria-label="添加上游步骤">
        <span :class="{ current: candidateStep === 1 }"><b>1</b>选择上游</span
        ><Icon name="chevronRight" size="sm" /><span
          :class="{ current: candidateStep === 2 }"
          ><b>2</b>配置类型</span
        >
      </nav>
      <p v-if="modalError" class="feedback feedback-error" role="alert">
        {{ modalError }}
      </p>
      <template v-if="candidateStep === 1">
        <div class="candidate-search">
          <label class="search-control"
            ><Icon name="search" size="sm" /><input
              v-model="candidateSearch"
              aria-label="搜索可添加上游"
              placeholder="搜索 Base URL 或分组" /></label
          ><button
            class="icon-button"
            :disabled="candidateLoading"
            aria-label="刷新上游账号"
            title="重新获取账号池"
            @click="fetchCandidates"
          >
            <Icon
              name="refresh"
              size="sm"
              :class="{ spinning: candidateLoading }"
            />
          </button>
        </div>
        <div class="candidate-list-heading">
          <label class="check-label"
            ><input
              type="checkbox"
              :checked="allCandidatesSelected"
              :indeterminate="someCandidatesSelected && !allCandidatesSelected"
              :disabled="!selectableCandidates.length || adding"
              @change="
                selectAllCandidates(($event.target as HTMLInputElement).checked)
              "
            />全选当前结果</label
          ><span>{{ visibleCandidates.length }} 个上游</span>
        </div>
        <p v-if="candidateLoading" class="empty-log" role="status">
          正在获取上游账号…
        </p>
        <div v-else class="candidate-list">
          <label
            v-for="candidate in visibleCandidates"
            :key="candidate.id"
            class="candidate-row"
            :class="{
              selected: selectedCandidates.has(candidate.id),
            }"
            ><input
              type="checkbox"
              :checked="selectedCandidates.has(candidate.id)"
              :disabled="adding"
              :aria-label="'选择上游 ' + candidate.base_url"
              @change="
                selectCandidate(
                  candidate.id,
                  ($event.target as HTMLInputElement).checked,
                )
              " /><span class="provider-mark"
              ><Icon name="link" size="sm" /></span
            ><span class="candidate-description"
              ><strong>{{ candidate.base_url }}</strong
              ><small>{{
                candidate.groups.map((g) => g.name).join(" · ") || "未关联分组"
              }}</small></span
            ><span class="tag">{{
              candidate.accounts.length + " 个可用账号"
            }}</span
            ><Icon
              v-if="selectedCandidates.has(candidate.id)"
              name="check"
              size="sm"
          /></label>
          <p v-if="!visibleCandidates.length" class="empty-log">
            {{ candidateSearch ? "没有符合条件的上游" : "暂无可添加的上游" }}
          </p>
        </div>
      </template>
      <template v-else>
        <div class="configuration-caption">
          <p>为已选上游分别选择类型和查询账号。</p>
          <button
            class="text-button"
            :disabled="adding"
            @click="discoverSelected"
          >
            {{ adding ? "识别中…" : "自动识别所选类型" }}
          </button>
        </div>
        <label class="common-threshold"
          >统一预警阈值<input
            v-model="importThreshold"
            aria-label="新增上游预警阈值"
            inputmode="decimal"
            placeholder="留空使用默认值 5"
          /><small>默认低于 5 预警，可按需修改；留空仍使用默认值。</small></label
        >
        <div class="candidate-configurations">
          <article
            v-for="candidate in chosenCandidates"
            :key="candidate.id"
            class="candidate-configuration"
          >
            <h3>{{ candidate.base_url }}</h3>
            <fieldset class="provider-options">
              <legend>上游类型</legend>
              <label
                :class="{ chosen: candidateTypes[candidate.id] === 'sub2api' }"
                ><input
                  v-model="candidateTypes[candidate.id]"
                  type="radio"
                  :name="'provider-' + candidate.id"
                  value="sub2api"
                /><span
                  ><strong>Sub2API</strong><small>读取账户钱包余额</small></span
                ></label
              ><label
                :class="{ chosen: candidateTypes[candidate.id] === 'newapi' }"
                ><input
                  v-model="candidateTypes[candidate.id]"
                  type="radio"
                  :name="'provider-' + candidate.id"
                  value="newapi"
                /><span
                  ><strong>New API</strong
                  ><small>读取账户总额度并换算</small></span
                ></label
              >
            </fieldset>
            <label
              >查询账号<select
                v-model="candidateAccounts[candidate.id]"
                :aria-label="'查询账号 ' + candidate.base_url"
              >
                <option
                  v-for="account in candidate.accounts"
                  :key="account.id"
                  :value="account.id"
                >
                  {{ account.name }} · {{ account.platform }} · #{{
                    account.id
                  }}
                </option>
              </select></label
            >
            <div
              v-if="candidateTypes[candidate.id] === 'newapi'"
              class="two-columns"
            >
              <label
                >上游用户 ID<input
                  v-model="candidateUsers[candidate.id]"
                  inputmode="numeric" /></label
              ><label
                >账户 Access Token<input
                  v-model="candidateTokens[candidate.id]"
                  type="password"
                  autocomplete="new-password"
                  placeholder="已开放 key 查询总余额时可留空"
              /></label>
            </div>
          </article>
        </div>
      </template>
    </div>
    <template #footer
      ><div class="upstream-theme dialog-footer">
        <span>已选择 {{ selectedCandidates.size }} 个上游</span>
        <div class="actions">
          <button
            class="btn btn-secondary"
            :disabled="adding"
            @click="
              candidateStep === 2 ? (candidateStep = 1) : closeCandidates()
            "
          >
            {{ candidateStep === 2 ? "上一步" : "取消" }}</button
          ><button
            v-if="candidateStep === 1"
            class="btn btn-primary"
            :disabled="!selectedCandidates.size || candidateLoading"
            @click="candidateStep = 2"
          >
            下一步：配置类型<Icon name="chevronRight" size="sm" /></button
          ><button
            v-else
            class="btn btn-primary"
            :disabled="!canImport || adding"
            @click="addSelected"
          >
            {{ adding ? "处理中…" : "加入监控" }}
          </button>
        </div>
      </div></template
    >
  </BaseDialog>
  <BaseDialog
    :show="!!editing"
    title="上游预警设置"
    width="wide"
    @close="closeRule"
  >
    <form
      v-if="editing"
      class="upstream-theme settings-form"
      @submit.prevent="saveRule"
    >
      <div class="form-context">
        <span class="provider-mark">{{
          editing.provider_type === "sub2api" ? "S" : "N"
        }}</span>
        <div>
          <strong>{{ shortName(editing) }}</strong
          ><small>{{ editing.base_url }}</small>
        </div>
        <span class="tag">{{ providerLabel(editing.provider_type) }}</span>
      </div>
      <p v-if="modalError" class="feedback feedback-error" role="alert">
        {{ modalError }}
      </p>
      <div class="two-columns">
        <label
          >显示名称<input
            v-model="rule.display_name"
            required
            maxlength="120" /></label
        ><label
          >查询账号<select v-model="rule.account_id">
            <option
              v-for="account in editingAccounts"
              :key="account.id"
              :value="account.id"
            >
              {{ account.name }} · #{{ account.id }}
            </option>
          </select></label
        >
      </div>
      <div class="threshold-section">
        <label
          >低于多少报警
          <div class="amount-input">
            <input
              v-model="thresholdText"
              inputmode="decimal"
              placeholder="例如 10，支持小数和 0"
            /><span>{{ editing.currency || "按上游币种" }}</span>
          </div></label
        >
        <p>默认低于 5 预警；等于阈值不推送，可为当前上游单独修改。</p>
      </div>
      <label
        >快速充值链接<input
          v-model="rule.recharge_url"
          type="url"
          required
          placeholder="上游站点或充值页面"
      /></label>
      <fieldset
        v-if="editing.provider_type === 'newapi'"
        class="credentials-section"
      >
        <legend>New API 账户凭证</legend>
        <p class="help">
          经典版需要账户 Access Token 与用户 ID；模型调用 key 不能替代账户令牌。
        </p>
        <label
          >上游用户 ID<input
            v-model="rule.upstream_user_id"
            inputmode="numeric"
            autocomplete="off" /></label
        ><label
          >账户 Access Token<input
            v-model="rule.access_token"
            type="password"
            autocomplete="new-password"
            :placeholder="
              editing.has_access_token
                ? '已保存；留空保留'
                : '上游已开放 key 查询总余额时可留空'
            " /></label
        ><label v-if="editing.has_access_token" class="check-label"
          ><input
            v-model="rule.clear_access_token"
            type="checkbox"
          />清除已保存的访问令牌</label
        >
      </fieldset>
      <div class="dialog-footer">
        <span class="help">持续低余额不会反复推送。</span>
        <div class="actions">
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="saving"
            @click="closeRule"
          >
            取消</button
          ><button type="submit" class="btn btn-primary" :disabled="saving">
            {{ saving ? "保存中…" : "保存设置" }}
          </button>
        </div>
      </div>
    </form>
  </BaseDialog>
  <BaseDialog
    :show="bulkThresholdOpen"
    title="批量设置预警阈值"
    @close="!operationBusy && (bulkThresholdOpen = false)"
    ><form
      class="upstream-theme settings-form"
      @submit.prevent="saveBulkThreshold"
    >
      <p class="help">
        应用到 {{ bulkTargets.length }} 个已选上游，金额按各上游余额币种计算。
      </p>
      <p v-if="modalError" class="feedback feedback-error" role="alert">
        {{ modalError }}
      </p>
      <label
        >低于多少报警<input
          v-model="bulkThreshold"
          required
          inputmode="decimal"
          placeholder="例如 10，支持小数和 0"
      /></label>
      <div class="dialog-footer">
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="operationBusy"
          @click="bulkThresholdOpen = false"
        >
          取消</button
        ><button
          type="submit"
          class="btn btn-primary"
          :disabled="operationBusy"
        >
          {{ operationBusy ? "保存中…" : "应用到所选上游" }}
        </button>
      </div>
    </form></BaseDialog
  >
  <BaseDialog
    :show="removeTargets.length > 0"
    title="移出监控"
    @close="!operationBusy && (removeTargets = [])"
    ><div class="upstream-theme settings-form">
      <p>将 {{ removeTargets.length }} 个上游移出监控？</p>
      <p class="help">停止后续余额查询与新预警，已有日志保留。</p>
      <div class="dialog-footer">
        <button
          class="btn btn-secondary"
          :disabled="operationBusy"
          @click="removeTargets = []"
        >
          取消</button
        ><button
          class="btn btn-primary"
          :disabled="operationBusy"
          @click="confirmRemove"
        >
          {{ operationBusy ? "处理中…" : "确认移出" }}
        </button>
      </div>
    </div></BaseDialog
  >
  <BaseDialog
    :show="channelOpen"
    title="飞书群通知"
    width="wide"
    @close="closeChannel"
    ><div class="upstream-theme channel-layout">
      <form class="settings-form" @submit.prevent="saveChannel">
        <div class="form-intro">
          <h3>连接飞书应用机器人</h3>
          <p>机器人需加入目标群，并开通应用消息发送权限。</p>
        </div>
        <p v-if="channelError" class="feedback feedback-error" role="alert">
          {{ channelError }}
        </p>
        <label
          >App ID<input
            v-model="channelForm.app_id"
            autocomplete="off"
            placeholder="cli_…" /></label
        ><label
          >App Secret<input
            v-model="channelForm.app_secret"
            type="password"
            autocomplete="new-password"
            :placeholder="
              channel.has_secret ? '已保存；留空保留' : '请输入应用密钥'
            "
          /><small>{{
            channel.has_secret
              ? "密钥已加密保存，不在页面回显。"
              : "凭证仅由服务端使用。"
          }}</small></label
        ><label
          >目标群 chat_id<input
            v-model="channelForm.chat_id"
            autocomplete="off"
            placeholder="oc_…" /></label
        ><button class="btn btn-primary" :disabled="saving || testing">
          {{ saving ? "保存中…" : "保存通知设置" }}
        </button>
      </form>
      <aside class="notification-preview">
        <span class="preview-label">群消息预览</span>
        <div class="message-preview">
          <span class="notification-icon"><Icon name="chart" size="sm" /></span>
          <div>
            <strong>预警--</strong>
            <p>上游：【示例上游】</p>
            <p>余额：8.50 USD</p>
            <span class="link-preview"
              >快速前往充值<Icon name="link" size="xs"
            /></span>
          </div>
        </div>
        <div class="test-notification">
          <button
            class="btn btn-secondary"
            :disabled="testing || saving || !channel.has_secret"
            @click="sendTest"
          >
            {{ testing ? "发送中…" : "向群里发送测试消息" }}
          </button>
          <p>发送一条标注为本地测试的消息。</p>
          <p v-if="channelNotice" class="success-text" role="status">
            {{ channelNotice }}
          </p>
        </div>
      </aside>
    </div></BaseDialog
  >
</template>
<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, reactive, ref } from "vue";
import BaseDialog from "@/components/common/BaseDialog.vue";
import Icon from "@/components/icons/Icon.vue";
import {
  upstreamMonitorAPI as api,
  monitorError,
  type MonitorCandidate,
  type MonitorSite,
  type MonitorSiteInput,
  type MonitorEvents,
  type MonitorChannel,
  type MonitorEvent,
} from "@/api/admin/upstreamMonitor";
const DEFAULT_THRESHOLD = "5";

type Provider = "sub2api" | "newapi";
type Operation = "refresh" | "enable" | "pause" | "remove" | "threshold";
const sites = ref<MonitorSite[]>([]),
  candidates = ref<MonitorCandidate[]>([]);
const events = ref<MonitorEvents>({
  items: [],
  total: 0,
  page: 1,
  page_size: 30,
});
const channel = ref<MonitorChannel>({
  app_id: "",
  chat_id: "",
  has_secret: false,
  revision: 0,
});
const error = ref(""),
  notice = ref(""),
  loading = ref(false),
  saving = ref(false),
  testing = ref(false),
  operationBusy = ref(false);
const search = ref(""),
  groupFilter = ref(""),
  statusFilter = ref(""),
  logSite = ref(""),
  logKind = ref(""),
  logPage = ref(1),
  logsElement = ref<HTMLElement | null>(null);
const checkedSites = ref(new Set<string>());
const candidateOpen = ref(false),
  candidateLoading = ref(false),
  candidateStep = ref(1),
  candidateSearch = ref(""),
  adding = ref(false),
  selectedCandidates = ref(new Set<string>()),
  importThreshold = ref(DEFAULT_THRESHOLD);
const candidateAccounts = reactive<Record<string, number>>({}),
  candidateTypes = reactive<Record<string, Provider | "">>({}),
  candidateUsers = reactive<Record<string, string>>({}),
  candidateTokens = reactive<Record<string, string>>({});
const modalError = ref(""),
  channelError = ref(""),
  channelNotice = ref(""),
  channelOpen = ref(false),
  editing = ref<MonitorSite | null>(null),
  thresholdText = ref(""),
  removeTargets = ref<MonitorSite[]>([]);
const bulkThresholdOpen = ref(false),
  bulkThreshold = ref(""),
  bulkTargets = ref<MonitorSite[]>([]);
const rule = reactive<MonitorSiteInput>({
  provider_type: "sub2api",
  account_id: 0,
  display_name: "",
  enabled: true,
  warning_enabled: true,
  threshold: null,
  recharge_url: "",
  access_token: "",
  clear_access_token: false,
  upstream_user_id: "",
  revision: 0,
});
const channelForm = reactive({
  app_id: "",
  app_secret: "",
  chat_id: "",
  revision: 0,
});
const controller = new AbortController();
let alive = true;
let timer: ReturnType<typeof setInterval> | undefined;
let testRequest = "";
let eventRequest = 0;
const groups = computed(() => [
  ...new Map(
    sites.value.flatMap((s) => s.groups).map((g) => [g.id, g]),
  ).values(),
]);
const lowCount = computed(
  () => sites.value.filter((s) => s.enabled && s.alert_active).length,
);
const exceptionCount = computed(
  () =>
    sites.value.filter(
      (s) => s.enabled && !["ok", "pending"].includes(s.status),
    ).length,
);
const visibleSites = computed(() =>
  sites.value.filter(
    (s) =>
      (!search.value ||
        (s.base_url + " " + s.display_name)
          .toLowerCase()
          .includes(search.value.toLowerCase())) &&
      (!groupFilter.value ||
        s.groups.some((g) => String(g.id) === groupFilter.value)) &&
      (!statusFilter.value ||
        (statusFilter.value === "low"
          ? s.alert_active
          : statusFilter.value === "paused"
            ? !s.enabled
            : statusFilter.value === "unconfigured"
              ? s.threshold === null
              : !["ok", "pending"].includes(s.status))),
  ),
);
const selectedSites = computed(() =>
  sites.value.filter((s) => checkedSites.value.has(s.id)),
);
const allVisibleSelected = computed(
  () =>
    visibleSites.value.length > 0 &&
    visibleSites.value.every((s) => checkedSites.value.has(s.id)),
);
const someVisibleSelected = computed(() =>
  visibleSites.value.some((s) => checkedSites.value.has(s.id)),
);
const visibleCandidates = computed(() =>
  candidates.value.filter(
    (c) =>
      !c.monitored &&
      (!candidateSearch.value ||
        (c.base_url + " " + c.groups.map((g) => g.name).join(" "))
          .toLowerCase()
          .includes(candidateSearch.value.toLowerCase())),
  ),
);
const selectableCandidates = computed(() =>
  visibleCandidates.value.filter((c) => !c.monitored),
);
const chosenCandidates = computed(() =>
  candidates.value.filter(
    (c) => selectedCandidates.value.has(c.id) && !c.monitored,
  ),
);
const allCandidatesSelected = computed(
  () =>
    selectableCandidates.value.length > 0 &&
    selectableCandidates.value.every((c) => selectedCandidates.value.has(c.id)),
);
const someCandidatesSelected = computed(() =>
  selectableCandidates.value.some((c) => selectedCandidates.value.has(c.id)),
);
const canImport = computed(
  () =>
    chosenCandidates.value.length > 0 &&
    chosenCandidates.value.every(
      (c) => candidateTypes[c.id] && candidateAccounts[c.id],
    ),
);
const editingAccounts = computed(
  () =>
    candidates.value.find((c) => c.base_url === editing.value?.base_url)
      ?.accounts || [],
);
function providerLabel(value: string) {
  return value === "sub2api" ? "Sub2API" : "New API";
}
function shortName(site: MonitorSite) {
  if (site.display_name !== site.base_url) return site.display_name;
  try {
    return new URL(site.base_url).host;
  } catch {
    return site.display_name;
  }
}
function errorText(code: string) {
  return monitorError({ response: { data: { message: code } } });
}
function time(value: string | null) {
  return value
    ? new Date(value).toLocaleString("zh-CN", {
        timeZone: "Asia/Shanghai",
        hour12: false,
      })
    : "尚未查询";
}
function isStale(site: MonitorSite) {
  return (
    !!site.last_success_at &&
    Date.now() - Date.parse(site.last_success_at) > 600000
  );
}
function stateLabel(site: MonitorSite) {
  if (!site.enabled) return "已暂停";
  if (site.status === "needs_configuration") return "待配置凭证";
  if (site.status === "unsupported") return "总余额未开放";
  if (site.status === "error") return "查询异常";
  if (site.alert_active) return "低余额预警";
  return site.status === "ok" ? "正常" : "待查询";
}
function statusTone(site: MonitorSite) {
  return !site.enabled
    ? "neutral"
    : site.alert_active || !["ok", "pending"].includes(site.status)
      ? "warning"
      : site.status === "ok"
        ? "healthy"
        : "neutral";
}
function kindLabel(kind: string) {
  return (
    (
      {
        identify: "识别",
        query: "查询",
        alert: "预警",
        delivery: "推送",
        settings: "配置",
      } as Record<string, string>
    )[kind] || kind
  );
}
function deliveryLabel(status: string) {
  return (
    (
      {
        sent: "飞书已接收",
        pending: "待发送",
        sending: "发送中",
        retry: "重试中",
        failed: "失败",
        needs_config: "待配置",
        cancelled: "已取消",
        awaiting_balance: "等待新余额",
      } as Record<string, string>
    )[status] || status
  );
}
function eventDetail(event: MonitorEvent) {
  const d = event.details;
  return [
    d.base_url,
    d.amount != null ? String(d.amount) + " " + String(d.currency || "") : "",
    d.error_code ? errorText(String(d.error_code)) : "",
    d.message_id ? "消息 ID：" + String(d.message_id) : "",
    d.is_test ? "本地测试" : "",
  ]
    .filter(Boolean)
    .join(" · ");
}
function selectSite(id: string, checked: boolean) {
  const next = new Set(checkedSites.value);
  checked ? next.add(id) : next.delete(id);
  checkedSites.value = next;
}
function selectVisible(checked: boolean) {
  for (const site of visibleSites.value) selectSite(site.id, checked);
}
function selectCandidate(id: string, checked: boolean) {
  const next = new Set(selectedCandidates.value);
  checked ? next.add(id) : next.delete(id);
  selectedCandidates.value = next;
}
function selectAllCandidates(checked: boolean) {
  for (const candidate of selectableCandidates.value)
    selectCandidate(candidate.id, checked);
}
function validAmount(value: string) {
  return /^-?\d+(\.\d{1,12})?$/.test(value);
}
async function loadEvents() {
  const request = ++eventRequest;
  try {
    const data = await api.events(
      { site_id: logSite.value, kind: logKind.value, page: logPage.value },
      controller.signal,
    );
    if (alive && request === eventRequest) events.value = data;
  } catch (e) {
    if (alive && request === eventRequest) error.value = monitorError(e);
  }
}
async function load() {
  if (loading.value) return;
  loading.value = true;
  try {
    const results = await Promise.allSettled([
      api.sites(controller.signal),
      api.channel(controller.signal),
    ]);
    if (!alive) return;
    const [a, b] = results;
    if (a.status === "fulfilled") {
      sites.value = a.value;
      checkedSites.value = new Set(
        [...checkedSites.value].filter((id) =>
          a.value.some((s) => s.id === id),
        ),
      );
    } else error.value = monitorError(a.reason);
    if (b.status === "fulfilled") channel.value = b.value;
    else error.value = monitorError(b.reason);
    await loadEvents();
  } finally {
    if (alive) loading.value = false;
  }
}
async function fetchCandidates() {
  candidateLoading.value = true;
  try {
    const result = await api.candidates(controller.signal);
    if (!alive) return;
    candidates.value = result;
    selectedCandidates.value = new Set(
      [...selectedCandidates.value].filter((id) =>
        result.some((c) => c.id === id && !c.monitored),
      ),
    );
    for (const c of result) {
      if (!c.accounts.some((a) => a.id === candidateAccounts[c.id]))
        candidateAccounts[c.id] = c.accounts[0]?.id || 0;
      if (candidateTypes[c.id] === undefined)
        candidateTypes[c.id] =
          c.provider_type === "sub2api" || c.provider_type === "newapi"
            ? c.provider_type
            : "";
    }
  } catch (e) {
    if (alive) modalError.value = monitorError(e);
  } finally {
    candidateLoading.value = false;
  }
}
async function openCandidates() {
  modalError.value = "";
  candidateSearch.value = "";
  selectedCandidates.value = new Set();
  candidateStep.value = 1;
  importThreshold.value = DEFAULT_THRESHOLD;
  candidateOpen.value = true;
  await fetchCandidates();
}
function clearCandidateSecrets() {
  for (const key of Object.keys(candidateTokens)) delete candidateTokens[key];
}
function closeCandidates() {
  if (adding.value) return;
  candidateOpen.value = false;
  clearCandidateSecrets();
}
async function discoverSelected() {
  adding.value = true;
  modalError.value = "";
  try {
    const result = await api.discover([...selectedCandidates.value]);
    if (!alive) return;
    candidates.value = result;
    for (const c of chosenCandidates.value)
      if (c.provider_type === "sub2api" || c.provider_type === "newapi")
        candidateTypes[c.id] = c.provider_type;
  } catch (e) {
    modalError.value = monitorError(e);
  } finally {
    adding.value = false;
  }
}
async function addSelected() {
  if (!canImport.value || adding.value) return;
  if (importThreshold.value && !validAmount(importThreshold.value)) {
    modalError.value = "请输入有效金额，最多保留 12 位小数。";
    return;
  }
  adding.value = true;
  modalError.value = "";
  let added = 0;
  try {
    for (const c of [...chosenCandidates.value]) {
      await api.add({
        candidate_id: c.id,
        provider_type: candidateTypes[c.id] as Provider,
        account_id: candidateAccounts[c.id]!,
        display_name: c.base_url,
        enabled: true,
        warning_enabled: true,
        threshold: importThreshold.value || DEFAULT_THRESHOLD,
        recharge_url: c.base_url,
        upstream_user_id: candidateUsers[c.id] || "",
        access_token: candidateTokens[c.id] || "",
      });
      added++;
      selectCandidate(c.id, false);
      c.monitored = true;
      delete candidateTokens[c.id];
    }
    notice.value = `已添加 ${added} 个上游，预警默认启用。`;
    candidateOpen.value = false;
    clearCandidateSecrets();
  } catch (e) {
    modalError.value = `已添加 ${added} 项，剩余项目未完成：${monitorError(e)}`;
  } finally {
    adding.value = false;
    await load();
  }
}
function inputFor(site: MonitorSite): MonitorSiteInput {
  return {
    provider_type: site.provider_type,
    account_id: site.account_id,
    display_name: site.display_name,
    enabled: site.enabled,
    warning_enabled: true,
    threshold: site.threshold,
    recharge_url: site.recharge_url,
    upstream_user_id: site.upstream_user_id,
    revision: site.revision,
  };
}
async function editSite(site: MonitorSite) {
  modalError.value = "";
  editing.value = site;
  Object.assign(rule, inputFor(site), {
    access_token: "",
    clear_access_token: false,
  });
  thresholdText.value = site.threshold ?? DEFAULT_THRESHOLD;
  await fetchCandidates();
}
function closeRule() {
  if (saving.value) return;
  rule.access_token = "";
  editing.value = null;
}
async function saveRule() {
  if (!editing.value) return;
  if (thresholdText.value && !validAmount(thresholdText.value)) {
    modalError.value = "请输入有效金额，最多保留 12 位小数。";
    return;
  }
  saving.value = true;
  modalError.value = "";
  try {
    await api.update(editing.value.id, {
      ...rule,
      warning_enabled: true,
      threshold: thresholdText.value || DEFAULT_THRESHOLD,
    });
    rule.access_token = "";
    editing.value = null;
    notice.value = "预警设置已保存。";
    await load();
  } catch (e) {
    modalError.value = monitorError(e);
  } finally {
    saving.value = false;
  }
}
async function runBatch(
  operation: Operation,
  targets: MonitorSite[],
  amount?: string,
) {
  if (operationBusy.value || !targets.length)
    return { succeeded: 0, failed: 0 };
  operationBusy.value = true;
  error.value = "";
  const queue = [...targets];
  const failures: { site: MonitorSite; message: string }[] = [];
  let succeeded = 0;
  const worker = async () => {
    while (queue.length && alive) {
      const site = queue.shift()!;
      try {
        if (operation === "refresh") await api.refresh(site.id);
        else if (operation === "remove") await api.remove(site);
        else
          await api.update(site.id, {
            ...inputFor(site),
            ...(operation === "threshold"
              ? { threshold: amount! }
              : { enabled: operation === "enable" }),
          });
        succeeded++;
      } catch (e) {
        failures.push({ site, message: monitorError(e) });
      }
    }
  };
  try {
    await Promise.all([worker(), worker()]);
    checkedSites.value = new Set(failures.map((f) => f.site.id));
    notice.value = `操作完成：${succeeded} 项成功${failures.length ? "，" + failures.length + " 项未完成" : ""}。`;
    if (failures.length)
      error.value = failures
        .map((f) => shortName(f.site) + "：" + f.message)
        .join("；");
    await load();
    return { succeeded, failed: failures.length };
  } finally {
    operationBusy.value = false;
  }
}
function openBulkThreshold() {
  modalError.value = "";
  bulkThreshold.value = DEFAULT_THRESHOLD;
  bulkTargets.value = [...selectedSites.value];
  bulkThresholdOpen.value = true;
}
async function saveBulkThreshold() {
  if (!validAmount(bulkThreshold.value)) {
    modalError.value = "请输入有效金额。";
    return;
  }
  const result = await runBatch(
    "threshold",
    bulkTargets.value,
    bulkThreshold.value,
  );
  if (result.failed) {
    bulkTargets.value = [...selectedSites.value];
    modalError.value = "部分上游未完成，已保留失败项。请刷新后重试。";
  } else bulkThresholdOpen.value = false;
}
async function confirmRemove() {
  const targets = [...removeTargets.value];
  await runBatch("remove", targets);
  removeTargets.value = [];
}
function showSiteLogs(site: MonitorSite) {
  logSite.value = site.id;
  logPage.value = 1;
  void loadEvents();
  logsElement.value?.scrollIntoView({ behavior: "auto", block: "start" });
}
function openChannel() {
  channelError.value = "";
  channelNotice.value = "";
  Object.assign(channelForm, { ...channel.value, app_secret: "" });
  channelOpen.value = true;
}
function closeChannel() {
  if (saving.value || testing.value) return;
  channelForm.app_secret = "";
  channelOpen.value = false;
}
async function saveChannel() {
  saving.value = true;
  channelError.value = "";
  try {
    channel.value = await api.saveChannel({ ...channelForm });
    channelForm.app_secret = "";
    channelForm.revision = channel.value.revision;
    testRequest = "";
    channelNotice.value = "通知设置已保存。";
    await loadEvents();
  } catch (e) {
    channelError.value = monitorError(e);
  } finally {
    saving.value = false;
  }
}
async function sendTest() {
  if (testing.value) return;
  testing.value = true;
  channelError.value = "";
  try {
    testRequest ||= crypto.randomUUID();
    const d = await api.test(testRequest);
    if (d.status === "sent") {
      channelNotice.value = "测试消息已被飞书接收。";
      testRequest = "";
    } else channelNotice.value = "测试投递状态：" + deliveryLabel(d.status);
    await loadEvents();
  } catch (e) {
    channelError.value = monitorError(e);
  } finally {
    testing.value = false;
  }
}
onMounted(() => {
  void load();
  timer = setInterval(() => {
    if (document.visibilityState === "visible" && !operationBusy.value)
      void load();
  }, 30000);
});
onBeforeUnmount(() => {
  alive = false;
  controller.abort();
  if (timer) clearInterval(timer);
  rule.access_token = "";
  channelForm.app_secret = "";
  clearCandidateSecrets();
});
</script>
<style scoped>
.upstream-theme {
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
  color: var(--monitor-ink);
  font-size: 13px;
  color-scheme: light;
}
:global(.dark .upstream-theme) {
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
.upstream-monitor > section { min-width: 0; }
.upstream-monitor {
  display: grid;
  gap: 28px;
  min-width: 0;
  padding: 6px 0 0;
}
.actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.monitor-toolbar,
.section-heading,
.filters,
.selection-bar,
.dialog-footer,
.candidate-search,
.candidate-list-heading {
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
  font-size: 12px;
  color: var(--monitor-muted);
}
.live-dot {
  width: 6px;
  height: 6px;
  background: var(--monitor-positive);
  border-radius: 50%;
}
.btn {
  display: inline-flex;
  justify-content: center;
  align-items: center;
  gap: 7px;
  white-space: nowrap;
  font-size: 12px;
  min-height: 38px;
}
.feedback {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  background: var(--monitor-selected);
  border-radius: 10px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.feedback-error {
  background: color-mix(in srgb, var(--monitor-danger) 9%, var(--monitor-bg));
  color: var(--monitor-danger);
}
.overview {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  border-bottom: 1px solid var(--monitor-line);
  padding-bottom: 26px;
}
.overview > * {
  padding: 0 25px;
  min-width: 0;
  text-align: left;
}
.overview > :first-child {
  padding-left: 0;
}
.overview > * + * {
  border-left: 1px solid var(--monitor-line);
}
.overview > * > span {
  font-size: 12px;
  color: var(--monitor-muted);
}
.overview strong {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 12px 0 8px;
  font-size: 32px;
  line-height: 1.1;
  font-weight: 600;
  letter-spacing: -0.035em;
  font-variant-numeric: tabular-nums;
}
.overview strong small {
  font-size: 12px;
  font-weight: 400;
  color: var(--monitor-muted);
  letter-spacing: 0;
}
.overview p {
  color: var(--monitor-muted);
  font-size: 11px;
  line-height: 1.6;
}
.overview .channel-state {
  font-size: 23px;
  letter-spacing: -0.02em;
  align-items: center;
  min-height: 35px;
}
.channel-summary p {
  display: flex;
  align-items: center;
  gap: 5px;
}
.channel-summary:hover .channel-state {
  color: var(--monitor-accent);
}
.section-heading h2 {
  font-size: 15px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 8px;
}
.section-heading h2 span {
  font-size: 11px;
  border-radius: 5px;
  padding: 2px 6px;
  background: var(--monitor-soft);
  color: var(--monitor-muted);
  font-weight: 500;
}
.section-heading p,
.help,
.form-intro p,
.common-threshold > small {
  color: var(--monitor-muted);
  font-size: 12px;
  line-height: 1.7;
}
.section-heading p {
  margin-top: 5px;
}
.filters {
  margin: 18px 0 14px;
}
input:not([type="checkbox"]):not([type="radio"]),
select {
  width: 100%;
  min-width: 0;
  border: 1px solid var(--monitor-line);
  border-radius: 9px;
  padding: 9px 11px;
  min-height: 39px;
  background: var(--monitor-input);
  color: var(--monitor-ink);
  font-size: 12px;
  line-height: 1.5;
  transition:
    border-color 0.15s,
    box-shadow 0.15s;
}
input::placeholder {
  color: var(--monitor-muted);
  opacity: 0.85;
}
select {
  padding-right: 28px;
}
select option {
  background: var(--monitor-input);
  color: var(--monitor-ink);
}
input[type="checkbox"],
input[type="radio"] {
  accent-color: #0071e3;
  width: 15px;
  height: 15px;
  flex-shrink: 0;
  cursor: pointer;
}
input:focus,
select:focus {
  outline: 2px solid color-mix(in srgb, var(--monitor-accent) 30%, transparent);
  outline-offset: 1px;
  border-color: var(--monitor-accent);
}
button:focus-visible,
a:focus-visible {
  outline: 2px solid var(--monitor-accent);
  outline-offset: 3px;
}
button:disabled,
input:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.search-control {
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 160px;
  position: relative;
}
.search-control > svg {
  position: absolute;
  left: 12px;
  color: var(--monitor-muted);
  pointer-events: none;
}
.search-control > input {
  padding-left: 36px !important;
}
.filters > select {
  max-width: 220px;
  width: 24%;
}
.selection-bar {
  background: var(--monitor-selected);
  border-radius: 9px;
  padding: 10px 13px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.selection-bar > span {
  font-size: 12px;
  white-space: nowrap;
}
.selection-bar .actions {
  gap: 5px;
}
.selection-bar button {
  padding: 5px 8px;
  border-radius: 6px;
  font-size: 12px;
}
.selection-bar button:hover {
  background: var(--monitor-bg);
}
.table-scroll {
  overflow-x: auto;
  border: 1px solid var(--monitor-line);
  border-radius: 12px;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  text-align: left;
}
th {
  background: var(--monitor-soft);
  color: var(--monitor-muted);
  padding: 12px 13px;
  font-weight: 500;
  white-space: nowrap;
}
td {
  padding: 17px 13px;
  border-top: 1px solid var(--monitor-line);
  vertical-align: middle;
}
.check-cell {
  width: 35px;
  padding-right: 0;
}
tbody tr {
  transition: background-color 0.15s;
}
tbody tr:hover {
  background: var(--monitor-soft);
}
tr.row-selected {
  background: var(--monitor-selected);
}
.upstream-cell {
  min-width: 155px;
  max-width: 240px;
}
.upstream-name {
  display: block;
  width: fit-content;
  max-width: 210px;
  color: var(--monitor-ink);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.6;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.upstream-name:hover {
  color: var(--monitor-accent);
  text-decoration: underline;
  text-underline-offset: 3px;
}
.provider-mark {
  width: 30px;
  height: 30px;
  border: 1px solid var(--monitor-line);
  border-radius: 9px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: var(--monitor-soft);
  color: var(--monitor-muted);
  font-size: 12px;
  font-weight: 600;
}
.type-label {
  display: block;
  color: var(--monitor-muted);
  font-size: 10px;
  margin: 5px 0 0;
}
small {
  display: block;
  font-size: 11px;
  line-height: 1.6;
  color: var(--monitor-muted);
  margin-top: 3px;
}
.groups-cell {
  width: 23%;
  max-width: 250px;
}
.group-tags {
  display: flex;
  gap: 5px;
  flex-wrap: wrap;
  align-items: center;
}
.tag {
  display: inline-flex;
  padding: 3px 6px;
  background: var(--monitor-soft);
  color: var(--monitor-muted);
  border-radius: 5px;
  font-size: 10px;
  line-height: 1.4;
  max-width: 170px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.number {
  text-align: right;
  white-space: nowrap;
}
.balance-value {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: flex-end;
  column-gap: 5px;
}
.balance {
  font-size: 16px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.025em;
}
.balance-value > span {
  font-size: 10px;
  color: var(--monitor-muted);
}
.threshold-link {
  margin-top: 7px;
  color: var(--monitor-muted);
  font-size: 10px;
}
.threshold-link:hover {
  color: var(--monitor-accent);
}
.state-cell {
  max-width: 185px;
  min-width: 135px;
}
.state-cell > small {
  font-size: 10px;
  overflow-wrap: anywhere;
}
.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  line-height: 1.6;
  white-space: nowrap;
}
.status-pill > span {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
}
.healthy {
  color: var(--monitor-positive);
}
.warning {
  color: var(--monitor-warning);
}
.neutral {
  color: var(--monitor-muted);
}
.row-actions {
  display: flex;
  align-items: center;
  gap: 1px;
}
.row-actions > button,
.icon-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 30px;
  height: 32px;
  border-radius: 7px;
  color: var(--monitor-muted);
  flex-shrink: 0;
}
.row-actions > button:hover,
.icon-button:hover {
  background: var(--monitor-soft);
  color: var(--monitor-ink);
}
.row-actions > button:last-child:hover {
  color: var(--monitor-danger);
}
.table-caption {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  margin-top: 10px;
  font-size: 11px;
  color: var(--monitor-muted);
}
.empty-state {
  display: grid;
  justify-items: center;
  gap: 12px;
  padding: 45px 20px;
  text-align: center;
  color: var(--monitor-muted);
}
.empty-state h3 {
  font-size: 15px;
  color: var(--monitor-ink);
  font-weight: 600;
}
.logs {
  border-top: 1px solid var(--monitor-line);
  padding-top: 24px;
  scroll-margin-top: 20px;
}
.logs .section-heading {
  align-items: flex-start;
}
.logs select {
  max-width: 180px;
}
.logs ol {
  margin-top: 15px;
}
.logs li {
  display: flex;
  gap: 12px;
  padding: 14px 0;
  border-bottom: 1px solid var(--monitor-line);
}
.event-marker {
  width: 29px;
  height: 29px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--monitor-soft);
  color: var(--monitor-muted);
  border-radius: 8px;
  flex-shrink: 0;
}
.event-marker.warning {
  color: var(--monitor-warning);
}
.event-content {
  flex: 1;
  min-width: 0;
}
.event-content > div {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.event-content strong {
  font-size: 12px;
  font-weight: 500;
}
.event-content small {
  overflow-wrap: anywhere;
}
.delivery-label {
  color: var(--monitor-muted);
  font-size: 10px;
}
.logs time {
  color: var(--monitor-muted);
  font-size: 10px;
  white-space: nowrap;
  padding-top: 4px;
}
.logs footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  color: var(--monitor-muted);
  font-size: 11px;
  margin-top: 12px;
}
.empty-log {
  padding: 28px 12px;
  text-align: center;
  color: var(--monitor-muted);
  font-size: 12px;
}
.candidate-dialog {
  display: grid;
  gap: 18px;
}
.import-steps {
  display: flex;
  gap: 16px;
  align-items: center;
  color: var(--monitor-muted);
  margin-bottom: 4px;
}
.import-steps > span {
  display: flex;
  align-items: center;
  gap: 7px;
  font-size: 12px;
}
.import-steps b {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 23px;
  height: 23px;
  border-radius: 50%;
  background: var(--monitor-soft);
  font-size: 11px;
  font-weight: 500;
}
.import-steps .current {
  color: var(--monitor-accent);
  font-weight: 600;
}
.import-steps .current b {
  background: var(--monitor-selected);
}
.candidate-search .search-control {
  min-width: 0;
}
.candidate-list-heading {
  color: var(--monitor-muted);
  font-size: 11px;
}
.check-label {
  display: flex !important;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.candidate-list,
.candidate-configurations {
  max-height: 430px;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.candidate-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 15px 12px;
  border-radius: 9px;
  border: 1px solid transparent;
  cursor: pointer;
  margin-bottom: 5px;
}
.candidate-row:hover {
  background: var(--monitor-soft);
}
.candidate-row.selected {
  background: var(--monitor-selected);
  border-color: color-mix(
    in srgb,
    var(--monitor-accent) 28%,
    var(--monitor-line)
  );
}
.candidate-row.monitored {
  opacity: 0.6;
  cursor: default;
}
.candidate-description {
  flex: 1;
  min-width: 0;
}
.candidate-description strong {
  font-size: 12px;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.candidate-description small {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 600px;
}
.configuration-caption {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
  font-size: 12px;
  color: var(--monitor-muted);
}
.text-button {
  color: var(--monitor-accent);
  white-space: nowrap;
  font-size: 12px;
}
.common-threshold {
  display: grid;
  gap: 7px;
  font-size: 12px;
  padding: 14px;
  background: var(--monitor-soft);
  border-radius: 10px;
}
.common-threshold input {
  max-width: 300px;
}
.candidate-configuration {
  display: grid;
  gap: 13px;
  padding: 18px 0;
  border-top: 1px solid var(--monitor-line);
}
.candidate-configuration h3 {
  font-size: 13px;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.candidate-configuration label {
  display: grid;
  gap: 7px;
  font-size: 12px;
}
.provider-options {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}
.provider-options legend {
  margin-bottom: 7px;
  color: var(--monitor-muted);
  font-size: 11px;
}
.provider-options label {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 13px;
  border: 1px solid var(--monitor-line);
  border-radius: 10px;
  cursor: pointer;
}
.provider-options label.chosen {
  border-color: var(--monitor-accent);
  background: var(--monitor-selected);
}
.provider-options input {
  margin-top: 3px;
}
.provider-options strong {
  font-size: 12px;
  font-weight: 600;
}
.provider-options small {
  font-size: 11px;
}
.dialog-footer {
  width: 100%;
  font-size: 12px;
  color: var(--monitor-muted);
  flex-wrap: wrap;
}
.settings-form {
  display: grid;
  gap: 18px;
  min-width: 0;
}
.settings-form label {
  display: grid;
  gap: 7px;
  font-size: 12px;
}
.two-columns {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}
.form-context {
  display: flex;
  align-items: center;
  gap: 12px;
  background: var(--monitor-soft);
  padding: 14px;
  border-radius: 10px;
}
.form-context > div {
  min-width: 0;
  flex: 1;
}
.form-context strong {
  font-size: 13px;
  font-weight: 600;
}
.form-context small {
  overflow-wrap: anywhere;
}
.threshold-section {
  border: 1px solid var(--monitor-line);
  border-radius: 12px;
  padding: 18px;
}
.threshold-section p {
  color: var(--monitor-muted);
  font-size: 11px;
  line-height: 1.7;
  margin-top: 10px;
}
.amount-input {
  display: flex;
  position: relative;
}
.amount-input input {
  padding-right: 110px !important;
}
.amount-input > span {
  position: absolute;
  right: 12px;
  top: 11px;
  font-size: 11px;
  color: var(--monitor-muted);
  pointer-events: none;
}
.credentials-section {
  display: grid;
  gap: 12px;
  border-top: 1px solid var(--monitor-line);
  padding-top: 15px;
}
.credentials-section legend {
  font-weight: 600;
  font-size: 13px;
  padding: 0 4px 8px 0;
}
.channel-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.05fr) minmax(0, 1fr);
  gap: 25px;
}
.form-intro h3 {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 5px;
}
.notification-preview {
  background: var(--monitor-soft);
  border: 1px solid var(--monitor-line);
  border-radius: 13px;
  padding: 18px;
  min-width: 0;
}
.preview-label {
  font-size: 11px;
  color: var(--monitor-muted);
}
.message-preview {
  display: flex;
  gap: 9px;
  margin-top: 18px;
}
.notification-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  background: var(--monitor-bg);
  border: 1px solid var(--monitor-line);
  border-radius: 8px;
  color: var(--monitor-accent);
  flex-shrink: 0;
}
.message-preview > div {
  padding: 12px;
  background: var(--monitor-bg);
  border: 1px solid var(--monitor-line);
  border-radius: 0 10px 10px 10px;
  flex: 1;
  min-width: 0;
}
.message-preview strong {
  font-size: 12px;
  font-weight: 600;
}
.message-preview p {
  font-size: 11px;
  margin-top: 8px;
}
.link-preview {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--monitor-accent);
  margin-top: 10px;
}
.test-notification {
  border-top: 1px solid var(--monitor-line);
  margin-top: 24px;
  padding-top: 18px;
}
.test-notification button {
  width: 100%;
}
.test-notification p {
  font-size: 11px;
  color: var(--monitor-muted);
  line-height: 1.7;
  margin-top: 9px;
}
.test-notification .success-text {
  color: var(--monitor-positive);
}
.text-warning {
  color: var(--monitor-warning) !important;
}
.text-danger {
  color: var(--monitor-danger) !important;
}
.muted {
  color: var(--monitor-muted);
}
.spinning {
  animation: monitor-spin 1s linear infinite;
}
@keyframes monitor-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (max-width: 1100px) {
  .overview > * {
    padding: 0 16px;
  }
  .groups-cell {
    max-width: 170px;
  }
  .group-tags .tag {
    max-width: 145px;
  }
  .section-heading {
    flex-wrap: wrap;
  }
}
@media (max-width: 700px) {
  .upstream-monitor {
    gap: 22px;
  }
  .monitor-toolbar > .actions {
    width: 100%;
  }
  .overview {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    row-gap: 24px;
  }
  .overview > :nth-child(3) {
    border-left: 0;
    padding-left: 0;
  }
  .overview strong {
    font-size: 28px;
  }
  .filters {
    flex-wrap: wrap;
  }
  .filters .search-control {
    flex-basis: 100%;
  }
  .filters > select {
    width: calc(50% - 6px);
    max-width: none;
  }
  .table-caption {
    flex-direction: column;
  }
  .logs time {
    white-space: normal;
    max-width: 100px;
  }
  .logs .actions {
    width: 100%;
  }
  .logs select {
    flex: 1;
    min-width: 90px;
  }
  .candidate-row {
    gap: 8px;
    padding: 12px 6px;
  }
  .candidate-row > .provider-mark {
    display: none;
  }
  .candidate-row .tag {
    max-width: 90px;
  }
  .two-columns,
  .channel-layout {
    grid-template-columns: minmax(0, 1fr);
  }
  .configuration-caption {
    align-items: flex-start;
    flex-direction: column;
  }
  .provider-options label {
    padding: 10px;
  }
  .provider-options small {
    display: none;
  }
  .dialog-footer > .actions {
    margin-left: auto;
  }
  .logs li {
    gap: 8px;
  }
  .event-marker {
    width: 24px;
    height: 24px;
  }
}
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation: none !important;
    transition: none !important;
    scroll-behavior: auto !important;
  }
}
</style>
