import { describe, expect, it } from 'vitest'
import { buildBatchImageAgentInstruction } from '../batchImageAgentInstruction'
import { parseBatchImageConfigInput } from '../batchImageConfig'

const outputFormat = 'prompt1;prompt2;prompt3;prompt4,4K,16:9,gpt-image-2.5-sunburst'

describe('batch image PPT instruction template', () => {
  it('uses the requested PPT content and reference-image instructions', () => {
    const instruction = buildBatchImageAgentInstruction('zh', 'openai')
    expect(instruction).toContain('请根据以下 PPT 内容，为每个主题编写一条生图提示词，用于生成对应的整页幻灯片图片。')
    for (let page = 1; page <= 4; page++) {
      expect(instruction).toContain(`${page}. [第 ${page} 页：主题、标题和核心要点]`)
    }
    expect(instruction).toContain('每条提示词必须以“根据参考图的风格以及背景”开头。')
    expect(instruction).toContain('但不要另外描述视觉风格')
    expect(instruction).toContain('不编造数据、产品能力或人物经历')
    expect(instruction).toContain('只输出一行结构化提示词，不换行，不添加说明，也不要用代码块包裹结果')
    expect(instruction.trim().split('\n').at(-1)).toBe(outputFormat)
    expect(instruction).not.toContain('先沟通')
  })

  it('preserves the fixed parameters regardless of the selected platform or locale', () => {
    for (const locale of ['zh', 'en']) {
      for (const platform of ['openai', 'gemini'] as const) {
        expect(buildBatchImageAgentInstruction(locale, platform)).toContain(outputFormat)
      }
    }
  })

  it('produces four pages with parameters accepted by the actual converter', () => {
    const result = parseBatchImageConfigInput(outputFormat)
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.cards).toHaveLength(4)
    for (const card of result.cards) {
      expect(card.image_size).toBe('4K')
      expect(card.aspect_ratio).toBe('16:9')
      expect(card.model).toBe('gpt-image-2.5-sunburst')
    }
  })
})
