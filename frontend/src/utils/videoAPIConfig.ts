export type VideoAPIConfigValue = Record<string, unknown>

export function formatVideoAPIConfig(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') {
    const text = value.trim()
    if (!text) return ''
    try {
      return JSON.stringify(JSON.parse(text), null, 2)
    } catch {
      return text
    }
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return ''
  }
}

export function parseVideoAPIConfig(text: string): VideoAPIConfigValue | null {
  const value = text.trim()
  if (!value) return null
  const parsed: unknown = JSON.parse(value)
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('video API config must be a JSON object')
  }
  return parsed as VideoAPIConfigValue
}
