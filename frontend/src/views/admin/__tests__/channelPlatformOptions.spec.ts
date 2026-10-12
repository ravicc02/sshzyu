import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { listPlatformIds } from '@/constants/platformCatalog'

describe('channel platform options', () => {
  it('derives concrete providers from the catalog and preserves the composite tab', () => {
    const source = readFileSync(resolve('src/views/admin/ChannelsView.vue'), 'utf8')
    expect(source).toContain("const platformOrder = computed<GroupPlatform[]>(() => [...listPlatformIds(), 'composite'])")
    expect(listPlatformIds()).toEqual(expect.arrayContaining([
      'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'command_code', 'cline'
    ]))
  })

  it('does not mirror composite groups into concrete provider tabs', () => {
    const source = readFileSync(resolve('src/views/admin/ChannelsView.vue'), 'utf8')
    expect(source).toContain('return groupsForPlatform(allGroups.value, platform)')
    expect(source).not.toContain('const compositePlatforms')
  })
})
