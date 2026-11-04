import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AccountSchedulerView from '../AccountSchedulerView.vue'

const { getOverview } = vi.hoisted(() => ({ getOverview: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accountScheduler: { getOverview } } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
const emptyOverview = { groups: [], recent_probes: [], total_accounts: 0, available_accounts: 0, enabled_groups: 0, tested_groups: 0 }
const mountView = () => shallowMount(AccountSchedulerView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
let wrapper: ReturnType<typeof mountView> | undefined
let hidden: boolean
beforeEach(() => {
 vi.useFakeTimers(); hidden = false
 vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden)
 getOverview.mockReset().mockResolvedValue(emptyOverview)
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers(); vi.restoreAllMocks() })

describe('account scheduler background refresh', () => {
 it('pauses requests in a hidden tab and refreshes immediately when visible', async () => {
  wrapper = mountView(); await flushPromises(); expect(getOverview).toHaveBeenCalledTimes(1)
  hidden = true; document.dispatchEvent(new Event('visibilitychange'))
  await vi.advanceTimersByTimeAsync(60000); expect(getOverview).toHaveBeenCalledTimes(1)
  hidden = false; document.dispatchEvent(new Event('visibilitychange')); await flushPromises()
  expect(getOverview).toHaveBeenCalledTimes(2)
 })
 it('does not overlap focus, visibility and timer requests', async () => {
  let finish!: (value: typeof emptyOverview) => void
  getOverview.mockReturnValue(new Promise(resolve => { finish = resolve }))
  wrapper = mountView()
  window.dispatchEvent(new Event('focus')); document.dispatchEvent(new Event('visibilitychange'))
  await vi.advanceTimersByTimeAsync(30000); expect(getOverview).toHaveBeenCalledTimes(1)
  finish(emptyOverview); await flushPromises()
  await vi.advanceTimersByTimeAsync(15000); expect(getOverview).toHaveBeenCalledTimes(2)
 })
 it('removes timers and listeners when the page is closed', async () => {
  wrapper = mountView(); await flushPromises(); wrapper.unmount(); wrapper = undefined
  window.dispatchEvent(new Event('focus')); document.dispatchEvent(new Event('visibilitychange'))
  await vi.advanceTimersByTimeAsync(60000); expect(getOverview).toHaveBeenCalledTimes(1)
 })
})
