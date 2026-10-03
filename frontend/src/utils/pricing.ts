/**
 * formatScaled formats a per-token (or per-request) USD price scaled by `scale`.
 *
 *   formatScaled(0.000003, 1_000_000)    → "$3"      // per 1M tokens
 *   formatScaled(0.5,        1)          → "$0.5"    // per request
 *   formatScaled(null,       1_000_000)  → "-"
 *   formatScaled(0.000003, 1_000_000, 2) → "$3.00"   // pad to ≥2 decimals
 *   formatScaled(1.25e-8,  1_000_000, 2) → "$0.0125" // longer decimals kept as-is
 *
 * Uses toPrecision(10) then strips trailing zeros to avoid IEEE 754 display noise.
 * `minFractionDigits` pads the result back up to a minimum number of decimals.
 */
export function formatScaled(value: number | null, scale: number, minFractionDigits = 0): string {
  if (value == null) return '-'
  let s = (value * scale).toPrecision(10).replace(/\.?0+$/, '')
  if (minFractionDigits > 0 && !s.includes('e')) {
    const dot = s.indexOf('.')
    const digits = dot === -1 ? 0 : s.length - dot - 1
    if (digits < minFractionDigits) {
      s = (dot === -1 ? `${s}.` : s) + '0'.repeat(minFractionDigits - digits)
    }
  }
  return `$${s}`
}

type PriceValues = {
  input_price?: number | null
  output_price?: number | null
  cache_write_price?: number | null
  cache_write_1h_price?: number | null
  cache_read_price?: number | null
  per_request_price?: number | null
}

export function resolveIntervalPrices(
  interval: PriceValues & { input_multiplier?: number | null; output_multiplier?: number | null; cache_write_multiplier?: number | null; cache_read_multiplier?: number | null },
  base: PriceValues
): PriceValues {
  const inherited = (value: number | null | undefined, multiplier: number | null | undefined) =>
    value == null ? value : value * (multiplier ?? 1)
  return {
    input_price: interval.input_price ?? inherited(base.input_price, interval.input_multiplier),
    output_price: interval.output_price ?? inherited(base.output_price, interval.output_multiplier),
    cache_write_price: interval.cache_write_price ?? inherited(base.cache_write_price, interval.cache_write_multiplier),
    cache_write_1h_price: interval.cache_write_1h_price ?? interval.cache_write_price ?? inherited(base.cache_write_1h_price, interval.cache_write_multiplier),
    cache_read_price: interval.cache_read_price ?? inherited(base.cache_read_price, interval.cache_read_multiplier),
    per_request_price: interval.per_request_price ?? base.per_request_price
  }
}
