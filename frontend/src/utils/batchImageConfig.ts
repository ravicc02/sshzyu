import type { BatchImageReferenceImage, BatchImageSubmitRequest } from '@/api/batchImage'
import { batchImageMimeTypes, geminiImageSizes, openAIImageSizes } from './batchImage'

export const BATCH_IMAGE_AUTO_MODEL = 'auto'
export type BatchImageConfigPlatform = 'openai' | 'gemini'

export type BatchImageConfigStatus =
  | 'empty'
  | 'converting'
  | 'converted'
  | 'stale'
  | 'invalid'
  | 'submitting'

export type BatchImageConfigCard = {
  localId: string
  custom_id: string
  prompt: string
  image_size: string
  aspect_ratio: string
  model: string
  output_count: number
  reference_images?: BatchImageReferenceImage[]
  reference_name?: string
  reference_names?: string[]
  reference_loading?: boolean
  reference_error?: string
}

export type BatchImageConfigParseResult =
  | { ok: true; cards: BatchImageConfigCard[] }
  | { ok: false; error: string }

export type BatchImageConfigModel = {
  value: string
  supported_image_sizes?: string[]
  supported_mime_types?: string[]
}

export type BatchImageConfigPreview = {
  groups: BatchImageSubmitRequest[]
  errors: Record<string, string[]>
}

const MAX_PROMPT_BYTES = 8000
const MAX_ITEMS_PER_GROUP = 200
const MAX_OUTPUT_IMAGES_PER_GROUP = 200
const MAX_REFERENCE_IMAGES_PER_GROUP = 1000
const MAX_REFERENCE_INLINE_BYTES_PER_GROUP = 128 * 1024 * 1024

function decodedBase64Bytes(value: string): number {
  const padding = value.endsWith('==') ? 2 : value.endsWith('=') ? 1 : 0
  return Math.max(0, Math.floor(value.length * 3 / 4) - padding)
}

export function batchImageConfigMatrix(platform: BatchImageConfigPlatform = 'openai') {
  return platform === 'gemini' ? geminiImageSizes : openAIImageSizes
}

export function batchImageConfigReferenceLimit(model: string, platform: BatchImageConfigPlatform = 'openai') {
  const normalized = model.trim().toLowerCase()
  if (platform === 'openai') return normalized.startsWith('gpt-image-') ? 16 : 0
  if (normalized.includes('pro-image')) return 14
  if (normalized.includes('flash-image')) return 3
  return 0
}

export function parseBatchImageConfigInput(input: string, platform: BatchImageConfigPlatform = 'openai'): BatchImageConfigParseResult {
  const matrix = batchImageConfigMatrix(platform)
  const imageSizeSet = new Set(Object.keys(matrix))
  const aspectRatioSet = new Set(Object.keys(matrix['1K'] || {}))
  const platformLabel = platform === 'gemini' ? 'Gemini' : 'OpenAI'
  if (!input.trim()) return { ok: false, error: '请输入需求和参数。' }

  const separator = input.indexOf(',')
  if (separator < 0) {
    return { ok: false, error: '格式应为：提示词1;提示词2,分辨率,画面比例,模型。请使用英文逗号 , 和分号 ;。' }
  }

  const slots = input.split(',')
  const position = (offset: number) => `第 ${Array.from(input.slice(0, offset)).length + 1} 个字符`
  if (slots.length !== 4) {
    const offset = slots.length > 4 ? slots.slice(0, 4).join(',').length : input.length
    return { ok: false, error: `${position(offset)}：须使用三个英文逗号分隔 image_size、aspect_ratio、model 槽位，空槽沿用默认值。提示词中的英文逗号请改用中文逗号，或转换后在卡片中编辑。` }
  }

  const imageSize = slots[1].trim() || '1K'
  const aspectRatio = slots[2].trim() || '1:1'
  const modelValue = slots[3].trim()
  if (!imageSizeSet.has(imageSize)) {
    return { ok: false, error: `${position(separator + 1)}：image_size=${imageSize} 无效，允许值为 1K、2K、4K。` }
  }
  if (!aspectRatioSet.has(aspectRatio) || !matrix[imageSize]?.[aspectRatio]) {
    return { ok: false, error: `${position(separator + slots[1].length + 2)}：aspect_ratio=${aspectRatio} 不在 ${platformLabel} 参数矩阵中，请选择 ${[...aspectRatioSet].join('、')}。` }
  }

  const promptSlots = slots[0].split(';')
  const prompts = promptSlots.map(prompt => prompt.trim())
  let promptOffset = 0
  for (const [index, prompt] of prompts.entries()) {
    const location = `第 ${index + 1} 条 prompt（${position(promptOffset)}）`
    if (!prompt) {
      return { ok: false, error: `${location}：提示词不能为空，请填写描述或移除多余的英文分号 ;。` }
    }
    if (new TextEncoder().encode(prompt).length > MAX_PROMPT_BYTES) {
      return { ok: false, error: `${location}：prompt 超过 8000 字节，请缩短描述。` }
    }
    promptOffset += promptSlots[index].length + 1
  }

  const model = modelValue || BATCH_IMAGE_AUTO_MODEL
  return {
    ok: true,
    cards: prompts.map((prompt, index) => ({
      localId: `config-${index + 1}`,
      custom_id: `img_${String(index + 1).padStart(3, '0')}`,
      prompt,
      image_size: imageSize,
      aspect_ratio: aspectRatio,
      model,
      output_count: 1,
    })),
  }
}

export function resolveBatchImageConfigModel(card: BatchImageConfigCard, availableModels: BatchImageConfigModel[]) {
  return card.model === BATCH_IMAGE_AUTO_MODEL ? (availableModels[0]?.value || '') : card.model
}

export function batchImageConfigPixels(card: Pick<BatchImageConfigCard, 'image_size' | 'aspect_ratio'>, platform: BatchImageConfigPlatform = 'openai') {
  return batchImageConfigMatrix(platform)[card.image_size]?.[card.aspect_ratio] || ''
}

/** 纯本地映射：保留现有批量 API 字段，不调用网络，也不把 auto 发给后端。 */
export function buildBatchImageConfigPreview(
  cards: BatchImageConfigCard[],
  models: BatchImageConfigModel[],
  taskName: string,
  responseMimeType: string,
  referenceImage?: BatchImageReferenceImage | BatchImageReferenceImage[],
  platform: BatchImageConfigPlatform = 'openai',
): BatchImageConfigPreview {
  const matrix = batchImageConfigMatrix(platform)
  const platformLabel = platform === 'gemini' ? 'Gemini' : 'OpenAI'
  const errors: Record<string, string[]> = {}
  const groups = new Map<string, BatchImageSubmitRequest>()
  const seenIds = new Set<string>()
  for (const card of cards) {
    const issues: string[] = []
    const references = referenceImage ? (Array.isArray(referenceImage) ? referenceImage : [referenceImage]) : card.reference_images || []
    if (!referenceImage && card.reference_loading) issues.push('参考图读取中，请稍候')
    if (!referenceImage && card.reference_error) issues.push(card.reference_error)
    if (references.some(image => !image.data || image.file_uri || !batchImageMimeTypes.includes(image.mime_type))) {
      issues.push('参考图不可用，请重新上传 PNG、JPEG 或 WebP 图片')
    }
    if (!card.custom_id.trim() || seenIds.has(card.custom_id)) issues.push('custom_id 不能为空或重复')
    seenIds.add(card.custom_id)
    const model = resolveBatchImageConfigModel(card, models)
    const modelSpecs = models.find(option => option.value === model)
    if (!card.prompt.trim()) issues.push('prompt 不能为空')
    if (new TextEncoder().encode(card.prompt.trim()).length > MAX_PROMPT_BYTES) issues.push('prompt 超过 8000 字节')
    if (!matrix[card.image_size]) issues.push(`image_size 不在 ${platformLabel} 参数矩阵中`)
    if (!batchImageConfigPixels(card, platform)) issues.push(`aspect_ratio 不在 ${platformLabel} 参数矩阵中`)
    if (!modelSpecs) {
      issues.push(card.model === BATCH_IMAGE_AUTO_MODEL ? `当前 Key 没有可解析的 ${platformLabel} 模型` : 'model 不属于当前 Key 的可用模型')
    }
    const referenceLimit = batchImageConfigReferenceLimit(model, platform)
    if (references.length && references.length > referenceLimit) {
      issues.push(referenceLimit ? `当前模型每项最多 ${referenceLimit} 张参考图` : '当前模型不支持参考图')
    }
    if (modelSpecs?.supported_image_sizes && !modelSpecs.supported_image_sizes.includes(card.image_size)) {
      issues.push('当前模型不支持此 image_size')
    }
    if (!batchImageMimeTypes.includes(responseMimeType) || (modelSpecs?.supported_mime_types && !modelSpecs.supported_mime_types.includes(responseMimeType))) {
      issues.push('当前模型不支持此输出格式')
    }
    if (!Number.isInteger(card.output_count) || card.output_count < 1 || card.output_count > 4) {
      issues.push('output_count 应为 1–4')
    }
    if (issues.length) {
      errors[card.localId] = issues
      continue
    }
    const key = JSON.stringify([model, card.image_size, card.aspect_ratio, responseMimeType])
    let payload = groups.get(key)
    if (!payload) {
      payload = { model, task_name: taskName.trim(), image_size: card.image_size, aspect_ratio: card.aspect_ratio, response_mime_type: responseMimeType, items: [] }
      groups.set(key, payload)
    }
    payload.items.push({ custom_id: card.custom_id, prompt: card.prompt.trim(), output_count: card.output_count,
      ...(references.length ? { reference_images: references.map(image => ({ ...image })) } : {}),
    })
  }
  for (const group of groups.values()) {
    const count = group.items.reduce((sum, item) => sum + (item.output_count || 1), 0)
    const referenceCount = group.items.reduce((sum, item) => sum + (item.reference_images?.length || 0) * (item.output_count || 1), 0)
    const referenceBytes = group.items.reduce((sum, item) => sum + (item.reference_images || []).reduce((inner, image) => inner + (image.data ? decodedBase64Bytes(image.data) : 0), 0) * (item.output_count || 1), 0)
    let groupError = ''
    if (count > MAX_OUTPUT_IMAGES_PER_GROUP || group.items.length > MAX_ITEMS_PER_GROUP) {
      groupError = '单个参数组超过 200 条或 200 张，本阶段不自动拆批'
    } else if (referenceCount > MAX_REFERENCE_IMAGES_PER_GROUP) {
      groupError = '参考图按生成张数展开后超过 1000 张，请拆分任务'
    } else if (referenceBytes > MAX_REFERENCE_INLINE_BYTES_PER_GROUP) {
      groupError = '参考图按生成张数展开后超过 128 MB，请压缩图片或拆分任务'
    }
    if (groupError) {
      for (const item of group.items) {
        const card = cards.find(entry => entry.custom_id === item.custom_id)
        if (card) (errors[card.localId] ||= []).push(groupError)
      }
    }
  }
  return { groups: [...groups.values()], errors }
}
