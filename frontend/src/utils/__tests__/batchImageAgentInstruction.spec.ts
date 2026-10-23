import { describe, expect, it } from 'vitest'
import { buildBatchImageAgentInstruction } from '../batchImageAgentInstruction'
import { parseBatchImageConfigInput } from '../batchImageConfig'

const outputFormat = 'prompt1;prompt2;prompt3;prompt4,4K,16:9,gpt-image-2.5-sunburst'

describe('batch image PPT instruction template', () => {
  it('uses the requested PPT content and reference-image instructions', () => {
    const instruction = buildBatchImageAgentInstruction('zh', 'openai')
    expect(instruction).toContain('用户通常只提供主题，不会提供完整文案，因此你必须围绕主题主动补全有价值的页面内容、信息层级和视觉表达。')
    for (let page = 1; page <= 4; page++) {
      expect(instruction).toContain(`${page}. [第 ${page} 页主题：`)
    }
    expect(instruction).toContain('每条提示词必须以“根据参考图的风格以及背景”开头。')
    expect(instruction).toContain('每条提示词必须包含：页面主题、完整可见文案、信息结构、模块数量、版式、视觉元素、颜色层级、文字避让和画面完成度要求。')
    expect(instruction).toContain('不编造精确统计数据、调查结果、机构名称、引用来源或具体事实。')
    expect(instruction).toContain('只输出一行结构化提示词，不换行，不添加解释，不要使用 Markdown 或代码块')
    expect(instruction.trim().split('\n').at(-1)).toBe(outputFormat)
    expect(instruction).toContain('不要只写“AI”和“Agent”两个大词。')
  })

  it('provides a three-page example with ASCII delimiters accepted by the converter', () => {
    const instruction = buildBatchImageAgentInstruction('zh', 'openai')
    const example = instruction.split('\n').find(line => line.startsWith('根据参考图的风格以及背景，页面主题'))!
    expect(example.split(';')).toHaveLength(3)
    expect(example.split(',')).toHaveLength(4)
    expect(example).not.toContain('；')
    const result = parseBatchImageConfigInput(example)
    expect(result.ok).toBe(true)
    if (result.ok) expect(result.cards).toHaveLength(3)
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
