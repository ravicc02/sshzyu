import type { ApiKey } from '@/types'

// Tier/aspect presets aligned 1:1 with the image playground's openai size table
// (8 ratios per tier, identical pixel values). Billing classifies by the resolved
// pixel dimensions' longest edge, so a wide "1K" request settles at its actual
// pixel tier. Backend mirror: resolveOpenAIBatchImageSpec.
export const openAIImageSizes: Record<string, Record<string, string>> = {
  '1K': {
    '1:1': '1024x1024', '3:2': '1536x1024', '2:3': '1024x1536',
    '16:9': '1280x720', '9:16': '720x1280', '4:3': '1024x768', '3:4': '768x1024', '21:9': '1280x544',
  },
  '2K': {
    '1:1': '2048x2048', '3:2': '2160x1440', '2:3': '1440x2160',
    '16:9': '2560x1440', '9:16': '1440x2560', '4:3': '2048x1536', '3:4': '1536x2048', '21:9': '2560x1088',
  },
  '4K': {
    '1:1': '2880x2880', '3:2': '3456x2304', '2:3': '2304x3456',
    '16:9': '3840x2160', '9:16': '2160x3840', '4:3': '3200x2400', '3:4': '2400x3200', '21:9': '3840x1600',
  },
}

export const batchImageMimeTypes = ['image/png', 'image/jpeg', 'image/webp']

export function supportsBatchImagePlatform(platform?: string): boolean {
  return platform === 'gemini' || platform === 'openai'
}

export function keyAllowsBatchImage(key: ApiKey): boolean {
  return key.status === 'active' &&
    supportsBatchImagePlatform(key.group?.platform) &&
    key.group?.allow_batch_image_generation === true &&
    (key.group.platform !== 'openai' || key.group.allow_image_generation === true)
}
