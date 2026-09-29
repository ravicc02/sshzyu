import type { ApiKey } from '@/types'

// Tier/aspect presets aligned 1:1 with the image playground
// (frontend/src/lib/size.ts COMMON_SIZE_PRESETS, openai variant): same 8 aspect
// ratios per tier and the same pixel values, so both UIs offer identical sizes.
// The backend (resolveOpenAIBatchImageSpec) mirrors this table and classifies
// billing by the resolved pixel dimensions' longest edge.
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

// Gemini tier/aspect presets aligned 1:1 with the image playground
// (frontend/src/lib/size.ts GEMINI_COMMON_SIZE_PRESETS, official Flash Image
// table). Upstream verified 2026-09-28 via generateContent imageConfig:
// 1K 16:9→1376x768, 9:16→768x1376, 21:9→1584x672, 2K 16:9→2752x1536,
// 4K 16:9→5504x3072 — the backend (resolveGeminiBatchImageSpec +
// geminiImageConfig) forwards these as aspectRatio/imageSize, and Gemini
// billing is per-image (group tier prices), so tier choice is cosmetic to cost.
export const geminiImageSizes: Record<string, Record<string, string>> = {
  '1K': {
    '1:1': '1024x1024', '3:2': '1264x848', '2:3': '848x1264',
    '16:9': '1376x768', '9:16': '768x1376', '4:3': '1200x896', '3:4': '896x1200', '21:9': '1584x672',
  },
  '2K': {
    '1:1': '2048x2048', '3:2': '2528x1696', '2:3': '1696x2528',
    '16:9': '2752x1536', '9:16': '1536x2752', '4:3': '2400x1792', '3:4': '1792x2400', '21:9': '3168x1344',
  },
  '4K': {
    '1:1': '4096x4096', '3:2': '5056x3392', '2:3': '3392x5056',
    '16:9': '5504x3072', '9:16': '3072x5504', '4:3': '4800x3584', '3:4': '3584x4800', '21:9': '6336x2688',
  },
}

export function supportsBatchImagePlatform(platform?: string): boolean {
  return platform === 'gemini' || platform === 'openai'
}

export function keyAllowsBatchImage(key: ApiKey): boolean {
  return key.status === 'active' &&
    supportsBatchImagePlatform(key.group?.platform) &&
    key.group?.allow_batch_image_generation === true &&
    (key.group.platform !== 'openai' || key.group.allow_image_generation === true)
}
