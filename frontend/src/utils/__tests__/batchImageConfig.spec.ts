import { describe, expect, it } from 'vitest'
import { batchImageConfigPixels, batchImageConfigReferenceLimit, buildBatchImageConfigPreview, parseBatchImageConfigInput } from '../batchImageConfig'

describe('batch image config input validation', () => {
  it('preserves item references while a shared reference overrides every submitted item', () => {
    const result = parseBatchImageConfigInput('红杯;蓝杯,1K,1:1,')
    if (!result.ok) throw new Error(result.error)
    const own = { mime_type: 'image/png', data: 'b3du' }
    const shared = { mime_type: 'image/png', data: 'c2hhcmVk' }
    result.cards[0].reference_images = [own]
    const preview = () => buildBatchImageConfigPreview(result.cards, [{ value: 'gpt-image-2' }], '', 'image/png')
    expect(preview().groups[0].items[0].reference_images).toEqual([own])
    expect(preview().groups[0].items[1].reference_images).toBeUndefined()
    const unified = buildBatchImageConfigPreview(result.cards, [{ value: 'gpt-image-2' }], '', 'image/png', shared)
    expect(unified.groups[0].items.map(item => item.reference_images)).toEqual([[shared], [shared]])
    expect(preview().groups[0].items[0].reference_images).toEqual([own])
    result.cards[0].reference_images = [{ mime_type: 'image/png' }]
    expect(preview().errors['config-1']).toContain('参考图不可用，请重新上传 PNG、JPEG 或 WebP 图片')
  })

  it('blocks item reference reading and failures instead of silently generating without the image', () => {
    const result = parseBatchImageConfigInput('红杯,1K,1:1,')
    if (!result.ok) throw new Error(result.error)
    result.cards[0].reference_loading = true
    expect(buildBatchImageConfigPreview(result.cards, [{ value: 'gpt-image-2' }], '', 'image/png').errors['config-1']).toBeDefined()
    result.cards[0].reference_loading = false
    result.cards[0].reference_error = 'read failed'
    expect(buildBatchImageConfigPreview(result.cards, [{ value: 'gpt-image-2' }], '', 'image/png').errors['config-1']).toEqual(['read failed'])
  })

  it('defaults empty parameter slots without omitting the required separators', () => {
    const result = parseBatchImageConfigInput(' 红杯;蓝杯, , , ')
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.cards).toHaveLength(2)
    expect(result.cards[0]).toMatchObject({ prompt: '红杯', image_size: '1K', aspect_ratio: '1:1', model: 'auto' })
    const preview = buildBatchImageConfigPreview(result.cards, [{ value: 'gpt-image-2' }], '', 'image/png')
    expect(preview.errors).toEqual({})
    expect(preview.groups[0]).toMatchObject({ model: 'gpt-image-2', image_size: '1K', aspect_ratio: '1:1' })
  })

  it.each([
    ['  🌅海面,8K,1:1,', 'image_size=8K', '第 7 个字符'],
    ['灯塔,1K,5:4,', 'aspect_ratio=5:4', '第 7 个字符'],
    ['灯塔; ;港口,1K,1:1,', '第 2 条 prompt', '第 4 个字符'],
    ['灯塔;,1K,1:1,', '第 2 条 prompt', '第 4 个字符'],
    [',1K,1:1,', '第 1 条 prompt', '第 1 个字符'],
  ])('reports the invalid field and original character position for %s', (input, field, position) => {
    const result = parseBatchImageConfigInput(input)
    expect(result.ok).toBe(false)
    if (result.ok) return
    expect(result.error).toContain(field)
    expect(result.error).toContain(position)
  })

  it('rejects ambiguous commas and missing slots without guessing punctuation', () => {
    for (const input of ['red, blue,1K,1:1,', '灯塔,1K,1:1', '灯塔，1K，1:1，']) {
      expect(parseBatchImageConfigInput(input).ok).toBe(false)
    }
    const result = parseBatchImageConfigInput('雨夜，霓虹；路面反光。;清晨，公园。,1K,1:1,')
    expect(result.ok).toBe(true)
    if (result.ok) expect(result.cards.map(card => card.prompt)).toEqual(['雨夜，霓虹；路面反光。', '清晨，公园。'])
  })

  it('validates the UTF-8 prompt limit before returning any cards', () => {
    expect(parseBatchImageConfigInput(`${'图'.repeat(2666)}ab,1K,1:1,`).ok).toBe(true)
    const result = parseBatchImageConfigInput(`灯塔;${'图'.repeat(2667)},1K,1:1,`)
    expect(result).toMatchObject({ ok: false })
    if (!result.ok) {
      expect(result.error).toContain('第 2 条 prompt')
      expect(result.error).toContain('8000 字节')
    }
    expect(result).not.toHaveProperty('cards')
  })
})

describe('Gemini config capabilities', () => {
  const models = [
    { value: 'gemini-3-pro-image-preview', supported_image_sizes: ['1K', '2K', '4K'] },
    { value: 'gemini-2.5-flash-image', supported_image_sizes: ['1K'], supported_mime_types: ['image/png'] },
  ]
  const reference = { mime_type: 'image/png', data: 'cmVm' }

  it('uses the Gemini matrix without changing OpenAI pixels or batch fields', () => {
    const parsed = parseBatchImageConfigInput('灯塔;港口,4K,16:9,', 'gemini')
    if (!parsed.ok) throw new Error(parsed.error)
    expect(batchImageConfigPixels(parsed.cards[0], 'gemini')).toBe('5504x3072')
    expect(batchImageConfigPixels(parsed.cards[0])).toBe('3840x2160')
    parsed.cards[1].image_size = '2K'
    const preview = buildBatchImageConfigPreview(parsed.cards, models, 'Gemini', 'image/png', undefined, 'gemini')
    expect(preview.errors).toEqual({})
    expect(preview.groups).toHaveLength(2)
    expect(preview.groups[0]).toMatchObject({ model: models[0].value, image_size: '4K', aspect_ratio: '16:9', response_mime_type: 'image/png' })
    expect(preview.groups[0]).not.toHaveProperty('size')
    expect(preview.groups[0]).not.toHaveProperty('provider')
  })

  it.each([
    ['gemini-3-pro-image-preview', 14],
    ['gemini-2.5-flash-image', 3],
    ['gemini-3.1-flash-image-preview', 3],
    ['gemini-2.0-flash-exp-image-generation', 0],
    ['gpt-image-2', 0],
  ])('mirrors existing Gemini backend reference limits for %s', (model, limit) => {
    expect(batchImageConfigReferenceLimit(model, 'gemini')).toBe(limit)
  })

  it('blocks unsupported models, tiers, formats and reference counts', () => {
    const parsed = parseBatchImageConfigInput('灯塔,2K,16:9,gemini-2.5-flash-image', 'gemini')
    if (!parsed.ok) throw new Error(parsed.error)
    parsed.cards[0].reference_images = Array.from({ length: 4 }, () => ({ ...reference }))
    const errors = buildBatchImageConfigPreview(parsed.cards, models, '', 'image/webp', undefined, 'gemini').errors['config-1']
    expect(errors).toContain('当前模型不支持此 image_size')
    expect(errors).toContain('当前模型不支持此输出格式')
    expect(errors.some(error => error.includes('3 张'))).toBe(true)
    parsed.cards[0].model = 'gpt-image-2'
    expect(buildBatchImageConfigPreview(parsed.cards, [{ value: 'gpt-image-2' }], '', 'image/png', undefined, 'gemini').errors['config-1']).toBeDefined()
  })

  it('maps multiple shared references without overwriting item drafts and validates every model', () => {
    const parsed = parseBatchImageConfigInput('灯塔;港口,1K,1:1,', 'gemini')
    if (!parsed.ok) throw new Error(parsed.error)
    parsed.cards[0].reference_images = [{ mime_type: 'image/jpeg', data: 'b3du' }]
    const shared = Array.from({ length: 4 }, () => ({ ...reference }))
    const preview = buildBatchImageConfigPreview(parsed.cards, models, '', 'image/png', shared, 'gemini')
    expect(preview.errors).toEqual({})
    expect(preview.groups[0].items.every(item => item.reference_images?.length === 4)).toBe(true)
    expect(preview.groups[0].items[0].reference_images?.[0]).not.toBe(shared[0])
    parsed.cards[1].model = models[1].value
    expect(buildBatchImageConfigPreview(parsed.cards, models, '', 'image/png', shared, 'gemini').errors['config-2']).toBeDefined()
    expect(parsed.cards[0].reference_images[0].data).toBe('b3du')
    expect(buildBatchImageConfigPreview(parsed.cards, models, '', 'image/png', [], 'gemini').groups[0].items[0]).not.toHaveProperty('reference_images')
  })

  it('counts reference attachments after output expansion', () => {
    const parsed = parseBatchImageConfigInput('灯塔,1K,1:1,', 'gemini')
    if (!parsed.ok) throw new Error(parsed.error)
    const cards = Array.from({ length: 20 }, (_, index) => ({
      ...parsed.cards[0], localId: `card-${index}`, custom_id: `img-${index}`, output_count: 4,
      reference_images: Array.from({ length: 14 }, () => ({ ...reference })),
    }))
    const preview = buildBatchImageConfigPreview(cards, models, '', 'image/png', undefined, 'gemini')
    expect(preview.errors['card-0']).toContain('参考图按生成张数展开后超过 1000 张，请拆分任务')
  })
})
