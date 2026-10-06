import { describe, it, expect } from 'vitest'
import { buildPlatformSections, formatRate } from '../modelPriceView'
import type { GroupModelPriceGroup } from '@/api/admin/modelPrice'

function g(partial: Partial<GroupModelPriceGroup> & { id: number }): GroupModelPriceGroup {
  return {
    name: `group-${partial.id}`,
    platform: 'openai',
    rate_multiplier: 1,
    models: [],
    ...partial
  } as GroupModelPriceGroup
}

describe('buildPlatformSections', () => {
  it('按平台归类分组，节内按分组名排序', () => {
    const groups = [
      g({ id: 1, name: 'b', platform: 'openai' }),
      g({ id: 2, name: 'a', platform: 'openai' }),
      g({ id: 3, name: 'c', platform: 'anthropic' })
    ]

    const sections = buildPlatformSections(groups, ['anthropic', 'openai'])

    expect(sections.map((s) => s.platform)).toEqual(['anthropic', 'openai'])
    expect(sections[0].groups.map((x) => x.name)).toEqual(['c'])
    expect(sections[1].groups.map((x) => x.name)).toEqual(['a', 'b'])
  })

  it('节间顺序沿用平台目录顺序，未知平台排在最后', () => {
    const groups = [
      g({ id: 1, platform: 'zhipu' }),
      g({ id: 2, platform: 'mystery' }),
      g({ id: 3, platform: 'openai' })
    ]

    const sections = buildPlatformSections(groups, ['openai', 'anthropic', 'zhipu'])

    expect(sections.map((s) => s.platform)).toEqual(['openai', 'zhipu', 'mystery'])
  })

  it('平台为空的分组归入空平台节（页面渲染为「其他」）', () => {
    const sections = buildPlatformSections([g({ id: 1, platform: '' })], ['openai'])

    expect(sections).toHaveLength(1)
    expect(sections[0].platform).toBe('')
  })

  it('不修改入参数组的顺序', () => {
    const groups = [g({ id: 1, name: 'b' }), g({ id: 2, name: 'a' })]

    buildPlatformSections(groups, ['openai'])

    expect(groups.map((x) => x.name)).toEqual(['b', 'a'])
  })

  it('空输入返回空分节', () => {
    expect(buildPlatformSections([], ['openai'])).toEqual([])
  })
})

describe('formatRate', () => {
  it('去掉浮点长尾并带 x 后缀', () => {
    expect(formatRate(1)).toBe('1x')
    expect(formatRate(0.8)).toBe('0.8x')
    expect(formatRate(0.15)).toBe('0.15x')
    expect(formatRate(1.1000000001)).toBe('1.1x')
    expect(formatRate(1.23456)).toBe('1.235x')
  })

  it('非法值回退为 1x，不产生 NaNx', () => {
    expect(formatRate(0)).toBe('1x')
    expect(formatRate(-2)).toBe('1x')
    expect(formatRate(Number.NaN)).toBe('1x')
    expect(formatRate(Number.POSITIVE_INFINITY)).toBe('1x')
  })
})