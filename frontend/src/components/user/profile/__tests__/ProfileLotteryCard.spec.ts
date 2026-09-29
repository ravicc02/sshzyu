import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProfileLotteryCard from '@/components/user/profile/ProfileLotteryCard.vue'

const { getStatusMock } = vi.hoisted(() => ({
  getStatusMock: vi.fn()
}))

vi.mock('@/api/lottery', () => ({
  lotteryAPI: {
    getStatus: getStatusMock
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

const baseStatus = {
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
}

function mountCard() {
  return mount(ProfileLotteryCard, {
    global: {
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        Icon: true
      }
    }
  })
}

describe('ProfileLotteryCard', () => {
  it('shows next-draw-away hint when a threshold is pending', async () => {
    getStatusMock.mockResolvedValue({ ...baseStatus })
    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.get('[data-testid="profile-lottery-card"]').text()).toContain('lottery.nextDrawAway')
  })

  it('omits the hint when no threshold is pending', async () => {
    getStatusMock.mockResolvedValue({ ...baseStatus, next_threshold: 0 })
    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.text()).not.toContain('lottery.nextDrawAway')
  })

  it('renders idle text when status fails to load', async () => {
    getStatusMock.mockRejectedValue(new Error('unavailable'))
    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.get('[data-testid="profile-lottery-card"]').text()).toContain('lottery.cardIdle')
  })
})
