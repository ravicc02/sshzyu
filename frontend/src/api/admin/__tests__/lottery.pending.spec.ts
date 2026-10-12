import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getPendingDrawCount, approveDraw, rejectDraw, retryFulfillment, reverseGrant } from '../lottery'
const client = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('../../client', () => ({ apiClient: client }))
const listener = vi.fn()
beforeEach(() => {
  client.get.mockReset(); client.post.mockReset(); listener.mockReset()
  window.addEventListener('lottery-draws-updated', listener)
})
afterEach(() => window.removeEventListener('lottery-draws-updated', listener))
describe('lottery pending API', () => {
  it('queries global attention total with a single record and supports cancellation', async () => {
    client.get.mockResolvedValue({ data: { total: 91, items: [] } })
    const signal = new AbortController().signal
    expect(await getPendingDrawCount({ signal })).toBe(91)
    expect(client.get).toHaveBeenCalledWith('/admin/lottery/draws', {
      params: { page: 1, page_size: 1, needs_attention: true }, signal
    })
  })
  it.each([approveDraw, rejectDraw, retryFulfillment, reverseGrant])('notifies only after successful mutation %s', async action => {
    client.post.mockResolvedValueOnce({ data: { draw: { id: 1 } } }).mockRejectedValueOnce(new Error('failed'))
    await action(1)
    expect(listener).toHaveBeenCalledTimes(1)
    await expect(action(2)).rejects.toThrow('failed')
    expect(listener).toHaveBeenCalledTimes(1)
  })
})
