import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { MonitorCoverage, MonitorMatrixRow } from '@/api/channelMonitorV2'
import type { UserMonitorView } from '@/api/channelMonitor'
import GroupStatusCards from '../GroupStatusCards.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string, values?: Record<string, string>) => values?.value ? `${key} ${values.value}` : key }) }
})

const coverage: MonitorCoverage = {
  requested_start: '2026-10-30T00:00:00Z', requested_end: '2026-10-30T01:30:00Z',
  coverage_start: '2026-10-30T00:00:00Z', data_through: '2026-10-30T01:30:00Z',
  computed_at: '2026-10-30T01:30:00Z', aggregation_lag_seconds: 0,
  coverage_complete: true, bucket_seconds: 60,
}

function row(platform: string, active = true): MonitorMatrixRow {
  const metrics = {
    // User responses redact absolute counts and may also hide RPM.
    request_count: 0, rpm: 0, has_requests: active, error_rate: active ? 0.12 : 0,
    cache_rate: active ? 0.36 : 0,
    ttft: { sample_count: 0, p50_ms: active ? 4500 : null, p95_ms: null, avg_ms: null },
  } as MonitorMatrixRow['metrics']
  const health = {
    overall: active ? 'healthy' : 'unknown', error_rate: 'healthy', ttft: 'healthy', minimum_sample: 20,
  } as MonitorMatrixRow['health']
  return {
    platform, group_id: platform === 'openai' ? 4 : 15, group_name: platform === 'openai' ? 'ChatGPT Plus' : 'Deepseek', rate_multiplier: platform === 'openai' ? 0.045 : 0.8,
    metrics, health,
    buckets: active ? [{ bucket_start: '2026-10-30T01:25:00Z', metrics, health: { ...health, overall: 'warning' } }] : [],
  }
}

function probe(overrides: Partial<UserMonitorView> = {}): UserMonitorView {
  return {
    id: 7,
    name: 'OpenAI primary',
    provider: 'openai',
    group_name: 'ChatGPT Plus',
    group_id: 4,
    primary_model: 'gpt-6.1-sol',
    interval_seconds: 60,
    primary_status: 'operational',
    primary_latency_ms: 1200,
    primary_ping_latency_ms: 40,
    availability_7d: 100,
    extra_models: [],
    timeline: [
      { status: 'operational', latency_ms: 1200, ping_latency_ms: 40, checked_at: '2026-10-30T01:29:00Z' },
      { status: 'failed', latency_ms: 800, ping_latency_ms: 40, checked_at: '2026-10-30T01:28:00Z' },
    ],
    intelligence_enabled: true,
    intelligence_timeline: [
      { status: 'operational', latency_ms: 2100, ping_latency_ms: null, checked_at: '2026-10-30T01:29:00Z' },
    ],
    ...overrides,
  }
}

describe('GroupStatusCards', () => {
  it('shows real aggregate rates after redaction and reserves intelligence for OpenAI', () => {
    const wrapper = mount(GroupStatusCards, { props: { rows: [row('openai'), row('deepseek')], coverage } })
    expect(wrapper.text()).toContain('36.0%')
    expect(wrapper.text()).toContain('88.0%')
    expect(wrapper.text()).toContain('≈4.5s')
    expect(wrapper.text()).toContain('0.045×')
    expect(wrapper.findAll('section[aria-label="OpenAI"] article')).toHaveLength(1)
    expect(wrapper.findAll('section[aria-label="DeepSeek"] article')).toHaveLength(1)
    expect(wrapper.findAll('article')).toHaveLength(2)
    expect(wrapper.text().match(/channelStatus.intelligenceChecks/g)).toHaveLength(1)
    expect(wrapper.findAll('article')[0]?.findAll('[aria-label="channelMonitorV2.groupCards.realRequests"] span')).toHaveLength(20)
    expect(wrapper.findAll('article')[0]?.find('.bg-amber-400').exists()).toBe(true)
  })

  it('shows one composite total card with concrete provider details', () => {
    const total = { ...row('composite'), group_id: 65, group_name: '国模聚合分组', group_platform: 'composite' }
    const deepseek = { ...row('deepseek'), group_id: 65, group_name: '国模聚合分组', group_platform: 'composite' }
    const zhipu = { ...row('zhipu'), group_id: 65, group_name: '国模聚合分组', group_platform: 'composite' }
    const kimi = { ...row('kimi'), group_id: 65, group_name: '国模聚合分组', group_platform: 'composite' }
    const wrapper = mount(GroupStatusCards, { props: { rows: [total, deepseek, zhipu, kimi], coverage } })

    expect(wrapper.findAll('article')).toHaveLength(1)
    expect(wrapper.findAll('section[aria-label="channelMonitorV2.settings.compositePlatform"] article')).toHaveLength(1)
    expect(wrapper.text()).toContain('channelMonitorV2.groupCards.providerBreakdown')
    expect(wrapper.text()).toContain('DeepSeek')
    expect(wrapper.text()).toContain('GLM')
    expect(wrapper.text()).toContain('Kimi')
  })

  it('derives composite provider details from active probe models without traffic rows', () => {
    const total = { ...row('composite', false), group_id: 65, group_name: '国模聚合分组', group_platform: 'composite' }
    const compositeProbe = probe({
      group_id: 65,
      group_name: '国模聚合分组',
      provider: 'openai',
      primary_model: 'deepseek-v4.1-flash',
      primary_status: 'operational',
      extra_models: [
        { model: 'glm-5.3-flash', status: 'degraded', latency_ms: 2100 },
        { model: 'kimi-k3.1-pro', status: 'failed', latency_ms: null },
      ],
      probe_fallback_enabled: true,
    })
    const wrapper = mount(GroupStatusCards, {
      props: { rows: [total], coverage, probes: [compositeProbe], hybrid: true },
    })

    expect(wrapper.findAll('article')).toHaveLength(1)
    expect(wrapper.text()).toContain('DeepSeek')
    expect(wrapper.text()).toContain('GLM')
    expect(wrapper.text()).toContain('Kimi')
    expect(wrapper.text()).toContain('channelMonitorV2.groupCards.states.activeFailed')
    expect(wrapper.text()).toContain('channelMonitorV2.groupCards.states.activeDegraded')
  })

  it('shows unobserved groups as empty without inventing healthy readings', () => {
    const wrapper = mount(GroupStatusCards, { props: { rows: [row('openai', false)], coverage } })
    expect(wrapper.findAll('article')).toHaveLength(1)
    expect(wrapper.text()).toContain('channelMonitorV2.groupCards.noData')
    expect(wrapper.text()).not.toContain('100.0%')
    expect(wrapper.findAll('article .bg-emerald-500')).toHaveLength(0)
  })

  it('uses the configured error-rate thresholds for request history colors', () => {
    const openai = row('openai')
    openai.buckets[0].health.thresholds = { warning_error_rate: 0.15, critical_error_rate: 0.25 } as MonitorMatrixRow['health']['thresholds']
    const wrapper = mount(GroupStatusCards, { props: { rows: [openai], coverage } })
    expect(wrapper.get('[aria-label="channelMonitorV2.groupCards.realRequests"] .bg-emerald-500').exists()).toBe(true)
    expect(wrapper.find('[aria-label="channelMonitorV2.groupCards.realRequests"] .bg-amber-400').exists()).toBe(false)
  })

  it('falls back to active probes only when the group has no real traffic', () => {
    const wrapper = mount(GroupStatusCards, {
      props: { rows: [row('openai', false)], coverage, probes: [probe()], hybrid: true },
    })
    expect(wrapper.text()).toContain('channelMonitorV2.groupCards.activeProbe')
    expect(wrapper.text()).toContain('50.0%')
    expect(wrapper.text()).toContain('1.2s')
    expect(wrapper.text()).toContain('channelMonitorV2.groupCards.intelligenceNotDegraded')
    expect(wrapper.get('[aria-label="channelMonitorV2.groupCards.activeProbe"]').findAll('span')).toHaveLength(20)
  })

  it('keeps real traffic as the primary source when a probe is also linked', () => {
    const wrapper = mount(GroupStatusCards, {
      props: { rows: [row('openai')], coverage, probes: [probe()], hybrid: true },
    })
    expect(wrapper.text()).toContain('channelMonitorV2.groupCards.realRequests')
    expect(wrapper.text()).not.toContain('channelMonitorV2.groupCards.activeProbe')
    expect(wrapper.get('[aria-label="channelMonitorV2.groupCards.realRequests"]').findAll('span')).toHaveLength(20)
  })

  it('does not attribute one probe to unrelated groups', () => {
    const unrelated = { ...row('openai', false), group_id: 70, group_name: 'Unrelated group' }
    const wrapper = mount(GroupStatusCards, {
      props: { rows: [row('openai', false), unrelated], coverage, probes: [probe()], hybrid: true },
    })
    const cards = wrapper.findAll('article')
    expect(cards).toHaveLength(2)
    expect(cards[1]?.text()).toContain('channelMonitorV2.groupCards.noData')
    expect(cards[1]?.text()).not.toContain('channelMonitorV2.groupCards.activeProbe')
    expect(cards[1]?.find('[aria-label="channelMonitorV2.groupCards.intelligenceActive"]').exists()).toBe(false)
  })

  it('honors per-monitor fallback and reasoning switches independently', () => {
    const fallbackOff = probe({ probe_fallback_enabled: false, intelligence_enabled: true })
    const wrapper = mount(GroupStatusCards, {
      props: { rows: [row('openai', false)], coverage, probes: [fallbackOff], hybrid: true },
    })
    expect(wrapper.text()).toContain('channelMonitorV2.groupCards.noData')
    expect(wrapper.text()).not.toContain('channelMonitorV2.groupCards.activeProbe')
    expect(wrapper.text()).toContain('channelMonitorV2.groupCards.intelligenceNotDegraded')
  })

  it('does not show reasoning results when the monitor reasoning switch is off', () => {
    const reasoningOff = probe({ intelligence_enabled: false })
    const wrapper = mount(GroupStatusCards, {
      props: { rows: [row('openai', false)], coverage, probes: [reasoningOff], hybrid: true },
    })
    expect(wrapper.find('[aria-label="channelMonitorV2.groupCards.intelligenceActive"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('channelStatus.intelligenceChecks')
  })

  it('collapses unobserved rows only when the list is large', async () => {
    const rows = Array.from({ length: 13 }, (_, index) => ({ ...row('openai', false), group_id: index + 1 }))
    const wrapper = mount(GroupStatusCards, { props: { rows, coverage } })
    expect(wrapper.findAll('article')).toHaveLength(0)
    await wrapper.get('button[aria-expanded="false"]').trigger('click')
    expect(wrapper.findAll('article')).toHaveLength(13)
  })
})
