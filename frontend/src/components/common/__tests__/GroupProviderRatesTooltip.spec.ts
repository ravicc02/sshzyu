import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import GroupProviderRatesTooltip from '../GroupProviderRatesTooltip.vue'

describe('GroupProviderRatesTooltip', () => {
  it('renders provider icons and colored surfaces for every rate', () => {
    const wrapper = mount(GroupProviderRatesTooltip, {
      props: {
        rates: [
          { platform: 'deepseek', rate_multiplier: 0.4 },
          { platform: 'zhipu', rate_multiplier: 0.35 },
          { platform: 'kimi', rate_multiplier: 0.5 }
        ]
      },
      global: {
        stubs: {
          HelpTooltip: { template: '<div data-testid="tooltip"><slot /></div>' },
          PlatformIcon: true
        }
      }
    })

    const cards = wrapper.findAll('[data-testid="tooltip"] > div > div')
    expect(cards).toHaveLength(3)
    expect(cards[0].text()).toContain('DeepSeek')
    expect(cards[0].text()).toContain('0.4x')
    expect(cards[0].classes()).toContain('bg-teal-500/10')
    expect(cards[1].text()).toContain('Zhipu GLM')
    expect(cards[1].classes()).toContain('bg-indigo-500/10')
    expect(cards[2].text()).toContain('Kimi')
    expect(cards[2].classes()).toContain('bg-pink-500/10')
  })
})
