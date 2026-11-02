export type TokenQuotaUnit = 'M' | 'B'

export const TOKEN_UNIT_FACTORS: Record<TokenQuotaUnit, number> = {
  M: 1_000_000,
  B: 1_000_000_000
}

export function tokenAmountToBase(value: number | string | null | undefined, unit: TokenQuotaUnit): number {
  const parsed = typeof value === 'number' ? value : Number.parseFloat(String(value ?? ''))
  if (!Number.isFinite(parsed) || parsed <= 0) return 0
  return Math.round(parsed * TOKEN_UNIT_FACTORS[unit])
}

export function tokenBaseToAmount(value: number | null | undefined, unit: TokenQuotaUnit): number | null {
  if (!value || value <= 0) return null
  const amount = value / TOKEN_UNIT_FACTORS[unit]
  return Number(amount.toFixed(amount >= 10 ? 1 : 2))
}

export function inferTokenQuotaUnit(value: number | null | undefined): TokenQuotaUnit {
  return value && value >= TOKEN_UNIT_FACTORS.B ? 'B' : 'M'
}

export function formatTokenQuota(value: number | null | undefined): string {
  if (!value || value <= 0) return '不限'
  if (value >= TOKEN_UNIT_FACTORS.B) {
    const amount = value / TOKEN_UNIT_FACTORS.B
    return `${Number(amount.toFixed(amount >= 10 ? 1 : 2))}B`
  }
  const amount = value / TOKEN_UNIT_FACTORS.M
  return `${Number(amount.toFixed(amount >= 10 ? 1 : 2))}M`
}
