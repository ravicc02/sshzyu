import type { Group, GroupPlatform } from '@/types'

export type KeyGroupProvider = 'anthropic' | 'openai' | 'domestic' | 'other'

export const KEY_GROUP_PROVIDERS = ['anthropic', 'openai', 'domestic', 'other'] as const

// Classify by the configured upstream platform, never by a group's display name.
const PROVIDER_BY_PLATFORM: Record<GroupPlatform, KeyGroupProvider> = {
  anthropic: 'anthropic',
  openai: 'openai',
  kimi: 'domestic',
  zhipu: 'domestic',
  deepseek: 'domestic',
  gemini: 'other',
  grok: 'other',
  antigravity: 'other',
  minimax: 'domestic',
  opencode_go: 'other',
  composite: 'other',
  typesafe: 'other',
  command_code: 'other',
  cline: 'other'
}

export function getKeyGroupProvider(platform: GroupPlatform): KeyGroupProvider {
  return PROVIDER_BY_PLATFORM[platform] ?? 'other'
}

export function getKeyGroupProviderForGroup(
  group: Pick<Group, 'platform' | 'provider_rates'> & {
    providerRates?: Array<{ platform: GroupPlatform; rate_multiplier: number }>
  }
): KeyGroupProvider {
  const providerRates = group.provider_rates ?? group.providerRates ?? []
  if (group.platform === 'composite' && providerRates.length) {
    const providers = providerRates.map((entry) => entry.platform)
    if (providers.every((platform) => PROVIDER_BY_PLATFORM[platform] === 'domestic')) {
      return 'domestic'
    }
  }
  return getKeyGroupProvider(group.platform)
}

// Collections use representative provider marks rather than an invented brand logo.
export const KEY_GROUP_PROVIDER_ICONS: Record<KeyGroupProvider, GroupPlatform[]> = {
  anthropic: ['anthropic'],
  openai: ['openai'],
  domestic: ['deepseek', 'kimi'],
  other: ['gemini', 'grok']
}
