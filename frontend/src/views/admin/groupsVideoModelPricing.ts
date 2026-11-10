export const grokVideoPriceResolutions = [
  { key: '480p', label: '480p' },
  { key: '720p', label: '720p' },
  { key: '1080p', label: '1080p' }
] as const

export const videoAPIResolutions = [
  { key: '480p', label: '480p' },
  { key: '720p', label: '720p' },
  { key: '768p', label: '768p' },
  { key: '1080p', label: '1080p' },
  { key: '2k', label: '2K' },
  { key: '4k', label: '4K' }
] as const

export const grokVideoPriceFamilies = [
  { key: 'grok-imagine-video', label: 'grok-imagine-video' },
  { key: 'grok-imagine-video-1.5', label: 'grok-imagine-video-1.5' }
] as const

export const videoAPIPriceFamilies = [
  { key: 'minimax_h3', label: 'minimax_h3' },
  { key: 'doubao-seedance-2-5', label: 'doubao-seedance-2-5' }
] as const

export type VideoModelPrices = Record<string, Record<string, number>>
export type VideoModelPricesForm = Record<string, Record<string, number | string | null>>

export function videoPriceResolutionsForPlatform(platform = 'grok') {
  return platform === 'video' ? videoAPIResolutions : grokVideoPriceResolutions
}

export function videoPriceFamiliesForPlatform(platform = 'grok') {
  return platform === 'video' ? videoAPIPriceFamilies : grokVideoPriceFamilies
}

function normalizeFamily(value: string): string {
  return value.trim().toLowerCase()
}

function normalizePrice(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const price = Number(value)
  return Number.isFinite(price) && price >= 0 ? price : null
}

function emptyTiers(platform = 'grok'): Record<string, number | string | null> {
  return Object.fromEntries(videoPriceResolutionsForPlatform(platform).map(({ key }) => [key, null]))
}

// Keep unknown families from an existing group so a future backend catalog is
// not silently discarded when an operator edits another group setting.
export function createVideoModelPricesForm(
  prices?: VideoModelPrices | null,
  platform = 'grok'
): VideoModelPricesForm {
  const form: VideoModelPricesForm = {}

  for (const [rawFamily, rawTiers] of Object.entries(prices ?? {})) {
    const family = normalizeFamily(rawFamily)
    if (!family || !rawTiers || typeof rawTiers !== 'object') continue
    form[family] = emptyTiers(platform)
    for (const [rawResolution, rawPrice] of Object.entries(rawTiers)) {
      const price = normalizePrice(rawPrice)
      if (price !== null) form[family][rawResolution.trim().toLowerCase()] = price
    }
  }

  for (const { key } of videoPriceFamiliesForPlatform(platform)) {
    form[key] ??= emptyTiers(platform)
  }
  return form
}

export function serializeVideoModelPrices(form: VideoModelPricesForm): VideoModelPrices {
  const result: VideoModelPrices = {}
  for (const [rawFamily, tiers] of Object.entries(form)) {
    const family = normalizeFamily(rawFamily)
    if (!family || !tiers || typeof tiers !== 'object') continue

    const normalizedTiers: Record<string, number> = {}
    for (const [rawResolution, rawPrice] of Object.entries(tiers)) {
      const resolution = rawResolution.trim().toLowerCase()
      const price = normalizePrice(rawPrice)
      if (resolution && price !== null) normalizedTiers[resolution] = price
    }
    if (Object.keys(normalizedTiers).length > 0) result[family] = normalizedTiers
  }
  return result
}

export function videoModelPriceFamilyRows(form: VideoModelPricesForm, platform = 'grok') {
  const knownFamilies = videoPriceFamiliesForPlatform(platform)
  const known = new Set<string>(knownFamilies.map(({ key }) => key))
  const extra = Object.keys(form)
    .map(normalizeFamily)
    .filter((family) => family && !known.has(family))
    .sort()
    .map((key) => ({ key, label: key }))
  return [...knownFamilies, ...extra]
}
