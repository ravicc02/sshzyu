import { mount, flushPromises } from '@vue/test-utils'
import { reactive } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AgentUsage from '../AgentUsage.vue'
import { agentAPI, type AgentUsageGroup, type AgentUsageSummary } from '@/api/agent'
const auth = reactive<{ user: { id: number } | null }>({ user: { id: 1 } })
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('@/api/agent', () => ({ agentAPI: { usage: vi.fn() } }))
const empty = (): AgentUsageGroup => ({ requests: 0, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_creation_tokens: 0, total_tokens: 0, image_count: 0, cost_usd: '0.0000000000', pending_requests: 0, missing_usage_records: 0, missing_image_records: 0 })
function data(): AgentUsageSummary {
  const text = { ...empty(), group_id: 11, group_name: 'DeepSeek 历史分组', requests: 2, input_tokens: 100, output_tokens: 200, cache_read_tokens: 50, cache_creation_tokens: 10, total_tokens: 360, cost_usd: '0.0123456789' }
  const image = { ...empty(), group_id: 12, group_name: '124K 生图组', requests: 2, image_count: 3, cost_usd: '0.1700000000' }
  return { scope: 'all_agent_history', text: { groups: [text], totals: { ...text } }, image: { groups: [image], totals: { ...image } }, total_cost_usd: '0.1823456789', updated_at: '2026-10-20T06:00:00Z' }
}
beforeEach(() => { vi.clearAllMocks(); auth.user = { id: 1 }; vi.mocked(agentAPI.usage).mockResolvedValue(data()) })
afterEach(() => { vi.unstubAllGlobals() })
describe('Agent usage receipts', () => {
  it('shows actual grouped conversation tokens, image counts and precise costs', async () => {
    const w = mount(AgentUsage); await flushPromises()
    expect(w.text()).toContain('DeepSeek 历史分组'); expect(w.text()).toContain('124K 生图组')
    expect(w.text()).toContain('360'); expect(w.text()).toContain('3 张')
    expect(w.text()).toContain('$0.0123456789'); expect(w.text()).toContain('$0.1823456789')
    expect(w.find('details').exists()).toBe(false); expect(w.text()).not.toContain('明细'); expect(w.text()).not.toContain('美元'); expect(w.text()).toContain('仅统计 Agent')
    await w.get('tbody .token-usage').trigger('mouseenter'); await flushPromises()
    expect(document.querySelector('[role=tooltip]')?.textContent).toContain('缓存读取')
    expect(agentAPI.usage).toHaveBeenCalledWith(expect.any(AbortSignal)); w.unmount()
  })
  it('shows aggregate token usage and reveals totals only when the number is hovered', async () => {
    const value = data(); value.image.totals.input_tokens = 20; value.image.totals.output_tokens = 5; value.image.totals.cache_read_tokens = 3; value.image.totals.cache_creation_tokens = 2; value.image.totals.total_tokens = 30
    vi.mocked(agentAPI.usage).mockResolvedValue(value)
    const w = mount(AgentUsage); await flushPromises()
    expect(w.get('.usage-token-total strong').text()).toBe('390')
    expect(document.querySelector('[role=tooltip]')).toBeNull()
    await w.get('.usage-token-total .token-usage').trigger('mouseenter'); await flushPromises()
    const detail = document.querySelector('[role=tooltip]')!
    expect([...detail.querySelectorAll('dd')].map(node => node.textContent)).toEqual(['120', '205', '53', '12', '390'])
    await w.get('.usage-token-total .token-usage').trigger('mouseleave'); await flushPromises()
    expect(document.querySelector('[role=tooltip]')).toBeNull(); w.unmount()
  })
  it('disables refresh and exposes a stable loading state until the receipt arrives', async () => {
    let finish!: (value: AgentUsageSummary) => void
    vi.mocked(agentAPI.usage).mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const w = mount(AgentUsage)
    expect(w.get('[role=status]').text()).toContain('正在读取'); expect(w.get('.usage-refresh').attributes('disabled')).toBeDefined()
    finish(data()); await flushPromises(); expect(w.find('.usage-loading').exists()).toBe(false); w.unmount()
  })
  it('can retry a failed request and never labels failure as zero usage', async () => {
    vi.mocked(agentAPI.usage).mockRejectedValueOnce(new Error('offline'))
    const w = mount(AgentUsage); await flushPromises()
    expect(w.get('[role=alert]').text()).toContain('重新加载'); expect(w.find('.usage-totals').exists()).toBe(false)
    await w.get('.usage-error button').trigger('click'); await flushPromises()
    expect(agentAPI.usage).toHaveBeenCalledTimes(2); expect(w.find('[role=alert]').exists()).toBe(false); expect(w.text()).toContain('DeepSeek'); w.unmount()
  })
  it('shows a clear empty history without inventing calls', async () => {
    vi.mocked(agentAPI.usage).mockResolvedValue({ ...data(), text: { groups: [], totals: empty() }, image: { groups: [], totals: empty() }, total_cost_usd: '0.0000000000' })
    const w = mount(AgentUsage); await flushPromises()
    expect(w.text()).toContain('还没有 Agent 用量'); expect(w.find('table').exists()).toBe(false); expect(w.text()).toContain('0 次调用'); w.unmount()
  })
  it('distinguishes missing telemetry from zero tokens and marks unresolved requests', async () => {
    const value = data(); value.text.groups[0]!.missing_usage_records = 2; value.text.totals.missing_usage_records = 2
    value.image.totals.pending_requests = 1
    vi.mocked(agentAPI.usage).mockResolvedValue(value)
    const w = mount(AgentUsage); await flushPromises()
    expect(w.findAll('table')[0]!.text()).toContain('未记录'); expect(w.text()).toContain('1 个请求仍在核对'); w.unmount()
  })
  it('rejects a late response after changing users and clears the old receipt', async () => {
    let finish!: (value: AgentUsageSummary) => void
    vi.mocked(agentAPI.usage).mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const w = mount(AgentUsage)
    const other = data(); other.text.groups[0]!.group_name = '当前用户自己的分组'
    vi.mocked(agentAPI.usage).mockResolvedValue(other); auth.user = { id: 2 }; await flushPromises()
    expect(w.text()).toContain('当前用户自己的分组')
    finish(data()); await flushPromises()
    expect(w.text()).not.toContain('DeepSeek 历史分组'); expect(w.text()).toContain('当前用户自己的分组'); w.unmount()
  })
  it('aborts the request when the usage tab closes', () => {
    vi.mocked(agentAPI.usage).mockImplementation(() => new Promise(() => undefined))
    const w = mount(AgentUsage), signal = vi.mocked(agentAPI.usage).mock.calls[0]![0]!
    w.unmount(); expect(signal.aborted).toBe(true)
  })
})
