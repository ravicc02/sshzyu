import type { GroupModelPriceGroup } from '@/api/admin/modelPrice'

/** 「模型价格」页的平台分节：一个平台 + 该平台下的全部分组。 */
export interface PlatformSection {
  platform: string
  groups: GroupModelPriceGroup[]
}

/**
 * 把「模型价格」页的全部分组按平台归类成分节（页面据此按类型展示）。
 *
 * - 节间顺序沿用 `platformOrder`（即平台目录声明顺序），未知平台排在最后并按名称兜底排序；
 * - 节内按分组名排序，保证同一平台下的浏览顺序稳定；
 * - `platform` 为空串的分组归入空平台节，页面渲染为「其他」。
 */
export function buildPlatformSections(
  groups: GroupModelPriceGroup[],
  platformOrder: readonly string[]
): PlatformSection[] {
  const buckets = new Map<string, GroupModelPriceGroup[]>()
  for (const group of groups) {
    const key = group.platform || ''
    const bucket = buckets.get(key)
    if (bucket) {
      bucket.push(group)
    } else {
      buckets.set(key, [group])
    }
  }

  const rank = (value: string) => {
    const index = platformOrder.indexOf(value)
    return index === -1 ? platformOrder.length : index
  }

  return [...buckets.entries()]
    .map(([platform, list]) => ({
      platform,
      groups: [...list].sort((a, b) => a.name.localeCompare(b.name))
    }))
    .sort((a, b) => rank(a.platform) - rank(b.platform) || a.platform.localeCompare(b.platform))
}

/**
 * 分组默认倍率的展示文本：去掉浮点长尾并统一带 x 后缀。
 * 非法值（NaN / Infinity / <= 0）回退为 `1x`，避免页面出现 "NaNx" 之类的脏文本。
 */
export function formatRate(value: number): string {
  const rate = Number.isFinite(value) && value > 0 ? value : 1
  return `${Number(rate.toFixed(3))}x`
}