import { describe, expect, it } from 'vitest'
import { groupsForPlatform } from '../channelPlatformGroups'
import type { AdminGroup } from '@/types'

function grp(id: number, platform: string): AdminGroup {
  return { id, name: `group-${id}`, platform } as unknown as AdminGroup
}

describe('groupsForPlatform', () => {
  const all = [grp(1, 'openai'), grp(2, 'composite'), grp(3, 'anthropic'), grp(4, 'composite')]

  it('returns composite groups for the composite platform tab', () => {
    expect(groupsForPlatform(all, 'composite').map(g => g.id)).toEqual([2, 4])
  })

  it('does not mirror composite groups into concrete platform tabs', () => {
    expect(groupsForPlatform(all, 'openai').map(g => g.id)).toEqual([1])
    expect(groupsForPlatform(all, 'anthropic').map(g => g.id)).toEqual([3])
  })

  it('returns an empty list when a platform has no groups', () => {
    expect(groupsForPlatform(all, 'gemini')).toEqual([])
  })
})
