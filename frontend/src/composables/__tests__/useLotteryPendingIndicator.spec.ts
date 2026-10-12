import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { useLotteryPendingIndicator } from '../useLotteryPendingIndicator'

const api = vi.hoisted(() => ({ getPendingDrawCount: vi.fn() }))
vi.mock('@/api/admin/lottery', () => api)
let wrappers: VueWrapper[] = []
function setup(admin = true) {
  const enabled = ref(admin)
  const path = ref('/admin/dashboard')
  const wrapper = mount(defineComponent({
    setup() { return { ...useLotteryPendingIndicator(enabled, path) } },
    template: '<span>{{ hasPending }}</span>'
  }))
  wrappers.push(wrapper)
  return { wrapper, enabled, path }
}
beforeEach(() => {
  vi.useFakeTimers()
  api.getPendingDrawCount.mockReset().mockResolvedValue(0)
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
})
afterEach(() => { wrappers.forEach(w => w.unmount()); wrappers = []; vi.useRealTimers() })
describe('lottery pending indicator', () => {
  it('shows pending and clears after processing notification', async () => {
    api.getPendingDrawCount.mockResolvedValueOnce(4).mockResolvedValueOnce(0)
    const { wrapper } = setup()
    await flushPromises()
    expect(wrapper.text()).toBe('true')
    window.dispatchEvent(new Event('lottery-draws-updated'))
    await flushPromises()
    expect(wrapper.text()).toBe('false')
  })
  it('does not query for ordinary users and clears on permission loss', async () => {
    const { wrapper, enabled } = setup(false)
    await flushPromises()
    expect(api.getPendingDrawCount).not.toHaveBeenCalled()
    api.getPendingDrawCount.mockResolvedValue(1)
    enabled.value = true
    await flushPromises()
    expect(wrapper.text()).toBe('true')
    enabled.value = false
    await flushPromises()
    expect(wrapper.text()).toBe('false')
    await vi.advanceTimersByTimeAsync(60000)
    expect(api.getPendingDrawCount).toHaveBeenCalledTimes(1)
  })
  it('polls, refreshes on navigation/focus, and skips hidden tabs', async () => {
    const { path } = setup()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(60000)
    expect(api.getPendingDrawCount).toHaveBeenCalledTimes(2)
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    await vi.advanceTimersByTimeAsync(60000)
    expect(api.getPendingDrawCount).toHaveBeenCalledTimes(2)
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    path.value = '/admin/lottery'
    await flushPromises()
    window.dispatchEvent(new Event('focus'))
    await flushPromises()
    expect(api.getPendingDrawCount).toHaveBeenCalledTimes(5)
  })
  it('preserves last state after errors and stops on unmount', async () => {
    api.getPendingDrawCount.mockResolvedValueOnce(2).mockRejectedValueOnce(new Error('offline'))
    const { wrapper } = setup()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(60000)
    expect(wrapper.text()).toBe('true')
    wrapper.unmount()
    window.dispatchEvent(new Event('focus'))
    window.dispatchEvent(new Event('lottery-draws-updated'))
    await vi.advanceTimersByTimeAsync(60000)
    expect(api.getPendingDrawCount).toHaveBeenCalledTimes(2)
  })
  it('queues processing refresh behind an in-flight request', async () => {
    let resolve!: (n: number) => void
    api.getPendingDrawCount.mockImplementationOnce(() => new Promise<number>(r => { resolve = r })).mockResolvedValueOnce(0)
    const { wrapper } = setup()
    window.dispatchEvent(new Event('lottery-draws-updated'))
    resolve(1)
    await flushPromises()
    expect(api.getPendingDrawCount).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toBe('false')
  })
  it('aborts in-flight requests and ignores late results after logout', async () => {
    let resolve!: (n: number) => void
    api.getPendingDrawCount.mockImplementationOnce(() => new Promise<number>(r => { resolve = r }))
    const { wrapper, enabled } = setup()
    await flushPromises()
    const signal = api.getPendingDrawCount.mock.calls[0][0].signal as AbortSignal
    enabled.value = false
    await flushPromises()
    expect(signal.aborted).toBe(true)
    resolve(3)
    await flushPromises()
    expect(wrapper.text()).toBe('false')
  })
})
