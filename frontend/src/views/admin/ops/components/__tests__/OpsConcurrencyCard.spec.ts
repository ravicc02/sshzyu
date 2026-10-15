import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpsConcurrencyCard from '../OpsConcurrencyCard.vue'

const { getConcurrencyStats, getAccountAvailabilityStats, getUserConcurrencyStats } = vi.hoisted(() => ({
  getConcurrencyStats: vi.fn(), getAccountAvailabilityStats: vi.fn(), getUserConcurrencyStats: vi.fn(),
}))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { getConcurrencyStats, getAccountAvailabilityStats, getUserConcurrencyStats } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const user = { user_id: 1, user_email: 'user@example.test', username: 'Test user', current_in_use: 3, max_capacity: 5, waiting_in_queue: 0, load_percentage: 60 }
async function openUsers() {
  const wrapper = mount(OpsConcurrencyCard, { props: { refreshToken: 0 } })
  await flushPromises()
  await wrapper.get('button[title="admin.ops.concurrency.switchToUser"]').trigger('click')
  await flushPromises()
  return wrapper
}
beforeEach(() => {
  vi.clearAllMocks()
  getConcurrencyStats.mockResolvedValue({ enabled: true, platform: {}, group: {}, account: {} })
  getAccountAvailabilityStats.mockResolvedValue({ enabled: true, platform: {}, group: {}, account: {} })
})
describe('user concurrency active groups', () => {
  it('shows the groups of active requests on the user row', async () => {
    getUserConcurrencyStats.mockResolvedValue({ enabled: true, user: { 1: { ...user, active_groups: [
      { group_id: 2, group_name: 'GPT mixed', current_in_use: 2 },
      { group_id: 78, group_name: 'DS v4', current_in_use: 1 },
    ] } } })
    const wrapper = await openUsers()
    expect(wrapper.get('[data-testid="user-active-groups-1"]').text()).toContain('GPT mixed')
    expect(wrapper.get('[data-testid="user-active-groups-1"]').text()).toContain('DS v4')
    expect(wrapper.text()).toContain('Test user')
    await wrapper.setProps({ refreshToken: 1 })
    getUserConcurrencyStats.mockResolvedValue({ enabled: true, user: { 1: { ...user, active_groups: [] } } })
    await wrapper.setProps({ refreshToken: 2 })
    await flushPromises()
    expect(wrapper.text()).not.toContain('GPT mixed')
    wrapper.unmount()
  })
  it('supports older backends without active group data', async () => {
    getUserConcurrencyStats.mockResolvedValue({ enabled: true, user: { 1: user } })
    const wrapper = await openUsers()
    expect(wrapper.text()).toContain('admin.ops.concurrency.noActiveGroups')
    expect(wrapper.text()).toContain('Test user')
    wrapper.unmount()
  })
})
