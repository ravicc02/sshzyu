import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { post } }))

beforeEach(() => {
  vi.resetModules()
  post.mockReset()
  sessionStorage.clear()
  localStorage.setItem('auth_user', JSON.stringify({ id: 910 }))
})

describe('batch balance operation recovery', () => {
  it('recovers the same nonce after network failure and a module reload', async () => {
    const request = { user_ids: [901, 900, 901], amount: 5, operation: 'add' as const, notes: '  audit note  ' }
    post.mockRejectedValueOnce(new Error('network interruption'))
    const first = await import('../users')
    await expect(first.batchUpdateBalance(request)).rejects.toThrow('network interruption')
    const originalKey = post.mock.calls[0][2].headers['Idempotency-Key']
    expect(post.mock.calls[0][1]).toEqual({ ...request, user_ids: [900, 901], notes: 'audit note' })
    vi.resetModules()
    post.mockResolvedValueOnce({ data: { success: 2, failed: 0, items: [] } })
    const retry = await import('../users')
    await retry.batchUpdateBalance(request)
    expect(post.mock.calls[1][2].headers['Idempotency-Key']).toBe(originalKey)
    expect(sessionStorage.length).toBe(0)
  })

  it('uses a new key for an intentional new adjustment after a confirmed result', async () => {
    post.mockResolvedValue({ data: { success: 1, failed: 0, items: [] } })
    const api = await import('../users')
    const request = { user_ids: [902], amount: 8, operation: 'subtract' as const }
    await api.batchUpdateBalance(request)
    await api.batchUpdateBalance(request)
    expect(post.mock.calls[0][2].headers['Idempotency-Key']).not.toBe(post.mock.calls[1][2].headers['Idempotency-Key'])
  })

  it('blocks recovery after the safe retry window instead of issuing another adjustment', async () => {
    const request = { user_ids: [903], amount: 8, operation: 'add' as const }
    post.mockRejectedValueOnce(new Error('network interruption'))
    const api = await import('../users')
    await expect(api.batchUpdateBalance(request)).rejects.toThrow()
    const storageKey = sessionStorage.key(0)!
    const stored = JSON.parse(sessionStorage.getItem(storageKey)!)
    stored.createdAt = Date.now() - 25*60*60*1000
    sessionStorage.setItem(storageKey, JSON.stringify(stored))
    vi.resetModules()
    const retry = await import('../users')
    await expect(retry.batchUpdateBalance(request)).rejects.toThrow('safe retry window expired')
    expect(post).toHaveBeenCalledTimes(1)
  })
})
