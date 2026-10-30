import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MonitorSettingsPanel from '../MonitorSettingsPanel.vue'
import type { MonitorConfig } from '@/api/channelMonitorV2'
import type { AdminGroup } from '@/types'

const mocks = vi.hoisted(() => ({
  getConfig: vi.fn(),
  updateConfig: vi.fn(),
  groups: vi.fn(),
  availableModels: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/channelMonitorV2', () => ({
  getConfig: mocks.getConfig,
  updateConfig: mocks.updateConfig,
  MONITOR_ERROR_CATEGORIES: ['authentication', 'network'],
}))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      getAllIncludingInactive: mocks.groups,
      getAvailableModels: mocks.availableModels,
    },
  },
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { channel_monitor_enabled: true },
    showError: mocks.showError,
    showSuccess: mocks.showSuccess,
  }),
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key, te: () => false }),
}))

function makeGroup(id: number, name: string, platform: string): AdminGroup {
  return {
    id,
    name,
    platform,
    status: 'active',
    rate_multiplier: 1,
  } as AdminGroup
}

function config(platforms: MonitorConfig['platforms'] = []): MonitorConfig {
  return {
    version: 2,
    enabled: true,
    refresh_interval_seconds: 60,
    platforms,
    group_ids: [],
    ignored_error_categories: [],
    health_thresholds: {
      minimum_sample: 50,
      warning_error_rate: 0.05,
      critical_error_rate: 0.2,
      target_ttft_ms: 3000,
      warning_ttft_ms: 3000,
      critical_ttft_ms: 10000,
      warning_cache_rate: 0.85,
      critical_cache_rate: 0.6,
      error_weight: 0.6,
      ttft_weight: 0.2,
      cache_weight: 0.2,
    },
  }
}

async function panel(value = config()) {
  mocks.getConfig.mockResolvedValue(value)
  const wrapper = mount(MonitorSettingsPanel, {
    global: { stubs: { Teleport: true } },
  })
  await flushPromises()
  return wrapper
}

function button(wrapper: Awaited<ReturnType<typeof panel>>, key: string) {
  return wrapper.findAll('button').find((item) => item.text().includes(`channelMonitorV2.settings.${key}`))
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.groups.mockResolvedValue([
    makeGroup(1, 'OpenAI 高缓存', 'openai'),
    makeGroup(2, 'OpenAI 特惠', 'openai'),
    makeGroup(3, 'Claude 企业组', 'anthropic'),
    makeGroup(4, '国模聚合', 'composite'),
  ])
  mocks.availableModels.mockImplementation(async (id: number) => {
    if (id === 1) return ['gpt-b', 'gpt-a']
    if (id === 2) return ['gpt-c', 'gpt-a']
    return ['claude-a']
  })
  mocks.updateConfig.mockImplementation(async (value) => JSON.parse(JSON.stringify(value)))
})

describe('MonitorSettingsPanel platform and group workflow', () => {
  it('bulk-adds platforms and fills deduplicated models from group /v1/models catalogs', async () => {
    const wrapper = await panel()
    await button(wrapper, 'addPlatform')!.trigger('click')
    await flushPromises()

    const openAIOption = wrapper.findAll('label').find((item) => item.text().includes('OpenAI'))!
    await openAIOption.find('input[type="checkbox"]').setValue(true)
    await button(wrapper, 'platformAddSelected')!.trigger('click')
    await flushPromises()

    await button(wrapper, 'save')!.trigger('click')
    await flushPromises()
    expect(mocks.updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      platforms: [{
        platform: 'openai',
        enabled: true,
        models: ['gpt-a', 'gpt-b', 'gpt-c'],
      }],
    }))
    wrapper.unmount()
  })

  it('offers the composite platform in the batch picker', async () => {
    const wrapper = await panel()
    await button(wrapper, 'addPlatform')!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('channelMonitorV2.settings.compositePlatform')
    wrapper.unmount()
  })

  it('refreshes composite models from the exact group catalog instead of stale Claude defaults', async () => {
    mocks.availableModels.mockResolvedValue(['deepseek-v4.1-flash', 'glm-5.3-flash', 'kimi-k3.1-pro'])
    const wrapper = await panel(config([{ platform: 'composite', enabled: true, models: ['claude-fable-5'] }]))
    await button(wrapper, 'save')!.trigger('click')
    await flushPromises()
    expect(mocks.updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      platforms: [{
        platform: 'composite',
        enabled: true,
        models: ['deepseek-v4.1-flash', 'glm-5.3-flash', 'kimi-k3.1-pro'],
      }],
    }))
    wrapper.unmount()
  })

  it('only shows groups belonging to selected platforms and selects them together', async () => {
    const wrapper = await panel({
      ...config([{ platform: 'openai', enabled: true, models: [] }]),
      group_ids: [1, 4],
    })
    expect(wrapper.text()).toContain('OpenAI 高缓存')
    expect(wrapper.text()).not.toContain('Claude 企业组')
    expect(wrapper.text()).not.toContain('国模聚合')

    await button(wrapper, 'groupsSelectAll')!.trigger('click')
    await button(wrapper, 'save')!.trigger('click')
    await flushPromises()
    expect(mocks.updateConfig).toHaveBeenCalledWith(expect.objectContaining({
      group_ids: [1, 2],
    }))
    wrapper.unmount()
  })
})
