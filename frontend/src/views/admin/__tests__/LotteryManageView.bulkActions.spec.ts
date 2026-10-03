import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LotteryManageView from '../LotteryManageView.vue'

const mocks = vi.hoisted(() => ({
  listDraws: vi.fn(),
  getActivities: vi.fn(),
  getPrizes: vi.fn(),
  approveDraw: vi.fn(),
  rejectDraw: vi.fn(),
  retryFulfillment: vi.fn(),
  reverseGrant: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin/lottery', () => ({
  listDraws: mocks.listDraws,
  getActivities: mocks.getActivities,
  getPrizes: mocks.getPrizes,
  updateActivity: vi.fn(),
  updatePrize: vi.fn(),
  updatePrizeWeights: vi.fn(),
  retryFulfillment: mocks.retryFulfillment,
  adjustDraws: vi.fn(),
  approveDraw: mocks.approveDraw,
  rejectDraw: mocks.rejectDraw,
  reverseGrant: mocks.reverseGrant
}))

vi.mock('@/api/admin/usage', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/admin/usage')>()
  return { ...actual, searchUsers: vi.fn().mockResolvedValue([]) }
})
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: mocks.showSuccess, showError: mocks.showError })
}))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

const draws = [
  {
    id: 1, user_id: 10, activity_id: 1, prize_id: 2, prize_name: 'Balance', prize_type: 'balance_bonus',
    prize_value: 1, rules_version: 1, source: 'first', balance_spent_at_draw: 0,
    fulfillment_status: 'pending_review', fulfilled_at: null, created_at: '2026-10-03T00:00:00Z'
  },
  {
    id: 2, user_id: 11, activity_id: 1, prize_id: 3, prize_name: 'Granted', prize_type: 'balance_bonus',
    prize_value: 1, rules_version: 1, source: 'threshold', balance_spent_at_draw: 1,
    fulfillment_status: 'granted', fulfilled_at: '2026-10-03T00:00:00Z', created_at: '2026-10-03T00:00:00Z'
  }
]

function mountView() {
  return mount(LotteryManageView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        BaseDialog: { template: '<div v-if="show"><slot /><slot name="footer" /></div>', props: ['show'] },
        EmptyState: { template: '<div />' },
        Icon: true,
        Pagination: true,
        Select: true
      }
    }
  })
}

describe('LotteryManageView bulk actions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.listDraws.mockResolvedValue({ items: draws, total: draws.length, page: 1, page_size: 20, pages: 1 })
    mocks.getActivities.mockResolvedValue({ activities: [] })
    mocks.getPrizes.mockResolvedValue({ prizes: [] })
    mocks.approveDraw.mockImplementation(async (id: number) => ({ draw: { ...draws[0], id, fulfillment_status: 'granted' } }))
  })

  it('runs a bulk approve only for selected rows eligible for approval', async () => {
    const wrapper = mountView()
    await flushPromises()

    const rowCheckboxes = wrapper.findAll<HTMLInputElement>('[data-test="select-row"]')
    expect(rowCheckboxes).toHaveLength(2)
    await rowCheckboxes[0].setValue(true)
    await rowCheckboxes[1].setValue(true)

    const approve = wrapper.get('[data-test="bulk-approve"]')
    expect(approve.text()).toContain('1')
    await approve.trigger('click')
    await flushPromises()

    expect(mocks.approveDraw).toHaveBeenCalledTimes(1)
    expect(mocks.approveDraw).toHaveBeenCalledWith(1)
    expect(mocks.showSuccess).toHaveBeenCalled()
  })
})
