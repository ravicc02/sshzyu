import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LotteryView from '@/views/user/LotteryView.vue'

const {
  drawMock,
  getActivityMock,
  getRecordsMock,
  getStatusMock
} = vi.hoisted(() => ({
  drawMock: vi.fn(),
  getActivityMock: vi.fn(),
  getRecordsMock: vi.fn(),
  getStatusMock: vi.fn()
}))

vi.mock('@/api/lottery', () => ({
  lotteryAPI: {
    getActivity: getActivityMock,
    getStatus: getStatusMock,
    draw: drawMock,
    getRecords: getRecordsMock,
    getRewards: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
  }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh' }
    })
  }
})

const activityPayload = {
  id: 1,
  name: '新人幸运抽奖',
  status: 'active',
  rules_version: 1,
  starts_at: null,
  ends_at: null,
  is_open: true,
  server_time: '2026-09-24T00:00:00Z',
  prizes: [
    { id: 1, name: '谢谢参与', prize_type: 'none', value: 0, weight: 70, min_tier: 0, stock: -1, stock_issued: 0, probability: 70 },
    { id: 2, name: '$0.1 余额', prize_type: 'balance_bonus', value: 0.1, weight: 20, min_tier: 0, stock: 100, stock_issued: 3, probability: 20 }
  ],
  tiers: [
    { tier: 0, name: '青铜', threshold: 0, prizes: [
      { prize_id: 1, name: '谢谢参与', value: 0, prize_type: 'none', probability: 70 },
      { prize_id: 2, name: '$0.1 余额', value: 0.1, prize_type: 'balance_bonus', probability: 30 }
    ] },
    { tier: 1, name: '白银', threshold: 5, prizes: [
      { prize_id: 1, name: '谢谢参与', value: 0, prize_type: 'none', probability: 50 },
      { prize_id: 2, name: '$0.1 余额', value: 0.1, prize_type: 'balance_bonus', probability: 30 }
    ] },
    { tier: 2, name: '黄金', threshold: 15, prizes: [] },
    { tier: 3, name: '钻石', threshold: 25, prizes: [] },
    { tier: 4, name: '王者', threshold: 35, prizes: [] }
  ]
}

function mountView() {
  return mount(LotteryView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { template: '<div><slot /></div>', props: ['modelValue', 'title'] },
        EmptyState: { template: '<div />', props: ['description'] },
        Icon: true,
        RouterLink: { template: '<a><slot /></a>', props: ['to'] }
      }
    }
  })
}

describe('LotteryView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getActivityMock.mockResolvedValue(activityPayload)
    getStatusMock.mockResolvedValue({
      activity_open: true,
      available_draws: 1,
      used_draws: 0,
      first_draw_granted: true,
      threshold_entitlement: 0,
      manual_adjustment: 0,
      balance_spent: 2.5,
      next_threshold: 5,
      current_tier: 0,
      tier_name: '青铜',
      rules_version: 1
    })
    getRecordsMock.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
  })

  it('renders activity, status and prizes', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="available-draws"]').text()).toBe('1')
    expect(wrapper.findAll('[data-testid="prize-item"]')).toHaveLength(2)
    expect(wrapper.get('[data-testid="draw-button"]').attributes('disabled')).toBeUndefined()
    // 距下次抽奖进度条: spent 2.5 / threshold 5 -> 进度 50%,还差金额提示渲染
    expect(wrapper.get('[data-testid="next-draw-away"]').text()).toBe('lottery.nextDrawAway')
    expect(wrapper.get('[data-testid="tier-panel"] .bg-primary-500').attributes('style')).toContain('width: 50%')
  })

  it('disables draw button when no draws left', async () => {
    getStatusMock.mockResolvedValue({
      activity_open: true,
      available_draws: 0,
      used_draws: 1,
      first_draw_granted: true,
      threshold_entitlement: 0,
      manual_adjustment: 0,
      balance_spent: 4,
      next_threshold: 5,
      current_tier: 0,
      tier_name: '青铜',
      rules_version: 1
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="draw-button"]').attributes('disabled')).toBeDefined()
    expect(drawMock).not.toHaveBeenCalled()
  })

  it('submits a draw with idempotency key and refreshes state', async () => {
    drawMock.mockResolvedValue({
      draw: {
        id: 10,
        user_id: 1,
        activity_id: 1,
        prize_id: 2,
        prize_name: '$0.1 余额',
        prize_type: 'balance_bonus',
        prize_value: 0.1,
        rules_version: 1,
        source: 'first',
        balance_spent_at_draw: 2.5,
        fulfillment_status: 'granted',
        fulfilled_at: '2026-09-24T00:00:01Z',
        created_at: '2026-09-24T00:00:01Z'
      },
      remaining_draws: 0
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="draw-button"]').trigger('click')
    await flushPromises()

    expect(drawMock).toHaveBeenCalledTimes(1)
    const [idempotencyKey] = drawMock.mock.calls[0]
    expect(typeof idempotencyKey).toBe('string')
    expect(idempotencyKey.length).toBeGreaterThan(0)
    // 抽奖后刷新状态与记录
    expect(getStatusMock.mock.calls.length).toBeGreaterThanOrEqual(2)
    expect(getRecordsMock.mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it('shows closed state when activity is not open', async () => {
    getActivityMock.mockResolvedValue({ ...activityPayload, is_open: false })
    getStatusMock.mockResolvedValue({
      activity_open: false,
      available_draws: 1,
      used_draws: 0,
      first_draw_granted: true,
      threshold_entitlement: 0,
      manual_adjustment: 0,
      balance_spent: 0,
      next_threshold: 5,
      current_tier: 0,
      tier_name: '青铜',
      rules_version: 1
    })
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.get('[data-testid="draw-button"]')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.text()).not.toContain('lottery.drawNow')
  })

  it('renders tier panel with current tier highlighted and tier prize pool', async () => {
    // 白银阶梯: balance_spent 6 -> tier 1
    getStatusMock.mockResolvedValue({
      activity_open: true,
      available_draws: 2,
      used_draws: 0,
      first_draw_granted: true,
      threshold_entitlement: 1,
      manual_adjustment: 0,
      balance_spent: 6,
      next_threshold: 15,
      current_tier: 1,
      tier_name: '白银',
      rules_version: 1
    })
    const wrapper = mountView()
    await flushPromises()

    // 侧边阶梯面板渲染 5 个阶梯,当前为白银(tier 1)
    const tierItems = wrapper.findAll('[data-testid="tier-list"] li')
    expect(tierItems).toHaveLength(5)
    expect(wrapper.get('[data-testid="current-tier"]').text()).toBe('白银')
    // 概率不外放: 不渲染概率卡片,页面文本不含百分比
    expect(wrapper.find('[data-testid="tier-prizes"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('%')
    // 进度条改用服务端 next_threshold 计算,阶梯提升后仍渲染
    expect(wrapper.get('[data-testid="next-draw-away"]').text()).toBe('lottery.nextDrawAway')
  })
})
