import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('Composite channel platform options', () => {
  it('exposes composite as a first-class platform option', () => {
    const source = readFileSync(resolve('src/views/admin/ChannelsView.vue'), 'utf8')
    const declaration = source.match(/const platformOrder:[^=]+=[^\n]+/)?.[0]

    // composite 必须出现在平台选择列表里，聚合分组才能被选中创建渠道。
    expect(declaration).toContain("'composite'")
  })

  it('keeps the CN concrete providers available for pricing and model mapping', () => {
    const source = readFileSync(resolve('src/views/admin/ChannelsView.vue'), 'utf8')
    const declaration = source.match(/const platformOrder:[^=]+=[^\n]+/)?.[0]

    expect(declaration).toContain("'kimi'")
    expect(declaration).toContain("'zhipu'")
    expect(declaration).toContain("'deepseek'")
  })

  it('does not mirror composite groups into every concrete platform tab', () => {
    const source = readFileSync(resolve('src/views/admin/ChannelsView.vue'), 'utf8')
    // 旧的 compositePlatforms 镜像逻辑已移除，聚合分组只在 composite tab 出现。
    expect(source).not.toMatch(/const compositePlatforms:/)
  })
})
