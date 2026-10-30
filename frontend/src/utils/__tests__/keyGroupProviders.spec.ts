import { describe, expect, it } from 'vitest'
import { getKeyGroupProviderForGroup } from '../keyGroupProviders'
import type { Group } from '@/types'

function group(overrides: Partial<Group>): Group {
  return {
    id: 1,
    name: 'group',
    description: '',
    platform: 'composite',
    rate_multiplier: 1,
    ...overrides
  } as Group
}

describe('getKeyGroupProviderForGroup', () => {
  it('places a domestic-only composite group under Chinese AI providers', () => {
    expect(getKeyGroupProviderForGroup(group({
      provider_rates: [
        { platform: 'deepseek', rate_multiplier: 0.4 },
        { platform: 'zhipu', rate_multiplier: 0.4 },
        { platform: 'kimi', rate_multiplier: 0.4 }
      ]
    }))).toBe('domestic')
  })

  it('keeps mixed-provider composite groups under Other', () => {
    expect(getKeyGroupProviderForGroup(group({
      provider_rates: [
        { platform: 'deepseek', rate_multiplier: 0.4 },
        { platform: 'gemini', rate_multiplier: 1 }
      ]
    }))).toBe('other')
  })
})
