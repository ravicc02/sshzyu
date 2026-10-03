import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GrokMediaEligibilityMode, GrokMediaEligibilityState } from '@/types'

const { get, put } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, put } }))

import accountsAPI, {
  getGrokMediaEligibility,
  updateGrokMediaEligibility
} from '@/api/admin/accounts'

describe('admin Grok media eligibility API', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
  })

  it('reads the dedicated account endpoint and returns the decision', async () => {
    const state: GrokMediaEligibilityState = {
      account_id: 12, mode: 'auto', eligible: false, reason: 'billing_inconclusive'
    }
    get.mockResolvedValueOnce({ data: state })
    await expect(getGrokMediaEligibility(12)).resolves.toEqual(state)
    expect(get).toHaveBeenCalledWith('/admin/accounts/12/grok-media-eligibility')
    expect(accountsAPI.getGrokMediaEligibility).toBe(getGrokMediaEligibility)
  })

  it.each<GrokMediaEligibilityMode>(['auto', 'enabled', 'disabled'])(
    'updates only the mode through PUT for %s',
    async (mode) => {
      const state: GrokMediaEligibilityState = {
        account_id: 12, mode, eligible: mode === 'enabled', reason: 'override'
      }
      put.mockResolvedValueOnce({ data: state })
      await expect(updateGrokMediaEligibility(12, mode)).resolves.toEqual(state)
      expect(put).toHaveBeenCalledWith('/admin/accounts/12/grok-media-eligibility', { mode })
      expect(accountsAPI.updateGrokMediaEligibility).toBe(updateGrokMediaEligibility)
    }
  )

  it('propagates read and write errors instead of inventing eligibility', async () => {
    const failure = new Error('unsupported account')
    get.mockRejectedValueOnce(failure)
    put.mockRejectedValueOnce(failure)
    await expect(getGrokMediaEligibility(12)).rejects.toBe(failure)
    await expect(updateGrokMediaEligibility(12, 'enabled')).rejects.toBe(failure)
  })
})
