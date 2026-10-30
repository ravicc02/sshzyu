import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupOptionItem from '../GroupOptionItem.vue'
import GroupProviderRatesTooltip from '../GroupProviderRatesTooltip.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

describe('GroupOptionItem description layout', () => {
  it('applies multiline and overflow-safe text styles', () => {
    const description = 'First section\nvery-long-unbroken-description-value-that-must-not-overflow'
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'Example group',
        platform: 'openai',
        description,
      },
      global: {
        stubs: {
          GroupBadge: true,
        },
      },
    })

    const descriptionElement = wrapper
      .findAll('span')
      .find((element) => element.text() === description)

    expect(descriptionElement).toBeDefined()
    expect(descriptionElement?.classes()).toContain('whitespace-pre-line')
    expect(descriptionElement?.classes()).toContain('[overflow-wrap:anywhere]')
    expect(descriptionElement?.classes()).toContain('line-clamp-3')
    expect(wrapper.find('[title]').exists()).toBe(false)
  })

  it('shows provider rates beside the group name for composite groups', () => {
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: '国模聚合分组',
        platform: 'composite',
        providerRates: [
          { platform: 'deepseek', rate_multiplier: 0.4 },
          { platform: 'zhipu', rate_multiplier: 0.35 },
          { platform: 'kimi', rate_multiplier: 0.5 }
        ]
      },
      global: {
        stubs: {
          GroupBadge: true,
          GroupProviderRatesTooltip: {
            props: ['rates'],
            template: '<div data-testid="provider-rates" />'
          }
        }
      }
    })

    const tooltip = wrapper.findComponent(GroupProviderRatesTooltip)
    expect(tooltip.exists()).toBe(true)
    expect(tooltip.props('rates')).toEqual([
      { platform: 'deepseek', rate_multiplier: 0.4 },
      { platform: 'zhipu', rate_multiplier: 0.35 },
      { platform: 'kimi', rate_multiplier: 0.5 }
    ])
  })
})
