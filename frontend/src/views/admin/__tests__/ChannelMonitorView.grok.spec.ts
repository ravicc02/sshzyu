import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'
import ModelTagInput from '@/components/admin/channel/ModelTagInput.vue'
import {
  DEFAULT_GROK_ENDPOINT,
  DEFAULT_GROK_MODEL,
  PROVIDERS,
  PROVIDER_GROK,
} from '@/constants/channelMonitor'

const { listTemplates, accountsList, accountsGetById, groupCandidates, groupList, compositeRoutes } = vi.hoisted(() => ({
  listTemplates: vi.fn(),
  accountsList: vi.fn(),
  accountsGetById: vi.fn(),
  groupCandidates: vi.fn(),
  groupList: vi.fn(),
  compositeRoutes: vi.fn(),
}))


vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorV1Mode: () => true,
  isChannelMonitorV2Mode: () => false,
  getChannelMonitorMode: () => 'v1' as const,
}))

vi.mock('@/features/channel-monitor-v2/MonitorSettingsPanel.vue', () => ({
  default: { name: 'MonitorSettingsPanel', template: '<div data-testid="v2-settings" />' },
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      create: vi.fn(),
      update: vi.fn(),
    },
    channelMonitorTemplate: {
      list: listTemplates,
    },
    accounts: {
      list: (...args: unknown[]) => accountsList(...args),
      getById: (...args: unknown[]) => accountsGetById(...args),
    },
    groups: {
      getAllIncludingInactive: groupList,
      getAvailableModels: groupCandidates,
      listCompositeRoutes: compositeRoutes,
    },
  },
}))

vi.mock('@/api/keys', () => ({
  keysAPI: { list: vi.fn() },
}))

vi.mock('@/api/groups', () => ({
  userGroupsAPI: { getUserGroupRates: vi.fn() },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(MonitorFormDialog, {
    props: { show: true, monitor: null, ...props },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Toggle: true,
        Select: true,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
        MonitorAdvancedRequestConfig: true,
      },
    },
  })
}

describe('channel monitor Grok provider', () => {
  beforeEach(() => {
    listTemplates.mockReset().mockResolvedValue({ items: [] })
    accountsList.mockReset().mockResolvedValue({ items: [] })
    accountsGetById.mockReset()
    groupCandidates.mockReset().mockResolvedValue([])
    groupList.mockReset().mockResolvedValue([])
    compositeRoutes.mockReset().mockResolvedValue([])
  })

  it('offers Grok in the responsive provider grid and prefills its official defaults', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    expect(PROVIDERS).toContain(PROVIDER_GROK)
    const providerButtons = wrapper.findAll('[data-testid^="monitor-provider-"]')
    expect(providerButtons).toHaveLength(8)
    expect(providerButtons[0].element.parentElement?.className).toContain('grid-cols-2')
    expect(providerButtons[0].element.parentElement?.className).toContain('sm:grid-cols-4')

    const grokButton = wrapper.get('[data-testid="monitor-provider-grok"]')
    expect(grokButton.find('svg').exists()).toBe(true)
    expect(grokButton.text()).toContain('monitorCommon.providers.grok')
    await grokButton.trigger('click')
    expect(grokButton.classes().join(' ')).toContain('zinc')

    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')
    expect((endpoint.element as HTMLInputElement).value).toBe(DEFAULT_GROK_ENDPOINT)
    expect((model.element as HTMLInputElement).value).toBe(DEFAULT_GROK_MODEL)

    await wrapper.get('[data-testid="monitor-provider-anthropic"]').trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('')
    expect((model.element as HTMLInputElement).value).toBe('')

    await grokButton.trigger('click')
    await endpoint.setValue('https://gateway.example.com')
    await model.setValue('grok-custom')
    await wrapper.get('[data-testid="monitor-provider-openai"]').trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://gateway.example.com')
    expect((model.element as HTMLInputElement).value).toBe('grok-custom')
  })

  it('prefills only empty Grok fields and preserves existing provider values', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')
    const grokButton = wrapper.get('[data-testid="monitor-provider-grok"]')
    const anthropicButton = wrapper.get('[data-testid="monitor-provider-anthropic"]')

    await endpoint.setValue('https://gateway.example.com')
    await grokButton.trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://gateway.example.com')
    expect((model.element as HTMLInputElement).value).toBe(DEFAULT_GROK_MODEL)

    await anthropicButton.trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://gateway.example.com')
    expect((model.element as HTMLInputElement).value).toBe('')

    await endpoint.setValue('')
    await model.setValue('grok-custom')
    await grokButton.trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe(DEFAULT_GROK_ENDPOINT)
    expect((model.element as HTMLInputElement).value).toBe('grok-custom')
  })

  it('uses automatic routing for composite groups instead of a single provider', async () => {
    groupList.mockResolvedValue([
      { id: 65, name: '国模聚合分组', platform: 'composite', status: 'active' },
    ])
    groupCandidates.mockResolvedValue([
      'deepseek-v4.1-flash',
      'glm-5.3-flash',
      'kimi-k3.1-pro',
    ])
    compositeRoutes.mockResolvedValue([
      { id: 1, group_id: 65, public_model: 'deepseek-v4.1-flash', match_type: 'exact', target_platform: 'deepseek', upstream_model: 'deepseek-v4.1-flash', endpoint: 'chat_completions', priority: 1, enabled: true, notes: '' },
      { id: 2, group_id: 65, public_model: 'glm-5.3-flash', match_type: 'exact', target_platform: 'zhipu', upstream_model: 'glm-5.3-flash', endpoint: 'chat_completions', priority: 2, enabled: true, notes: '' },
      { id: 3, group_id: 65, public_model: 'kimi-k3.1-pro', match_type: 'exact', target_platform: 'kimi', upstream_model: 'kimi-k3.1-pro', endpoint: 'chat_completions', priority: 3, enabled: true, notes: '' },
    ])

    const wrapper = mountDialog({
      initialGroup: { id: 65, name: '国模聚合分组', platform: 'composite', status: 'active' },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="monitor-composite-routing"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid^="monitor-provider-"]')).toHaveLength(0)
    expect((wrapper.get('[data-testid="monitor-endpoint"]').element as HTMLInputElement).value).toBe(window.location.origin)
    expect((wrapper.get('[data-testid="monitor-primary-model"]').element as HTMLInputElement).value).toBe('deepseek-v4.1-flash')
    expect(wrapper.text()).toContain('admin.channelMonitor.form.compositeRoutingTitle')
    expect(wrapper.text()).toContain('monitorCommon.providers.deepseek')
    expect(wrapper.text()).toContain('monitorCommon.providers.zhipu')
    expect(wrapper.text()).toContain('monitorCommon.providers.kimi')
  })

  it('prefills a newly selected group and its /v1/models catalog', async () => {
    groupCandidates.mockResolvedValue(['gpt-5.6-sol', 'gpt-5.5'])
    const wrapper = mountDialog({
      initialGroup: {
        id: 18,
        name: '124K生图组',
        platform: 'openai',
        status: 'active',
        rate_multiplier: 1,
      },
      initialEnabled: true,
    })
    await flushPromises()

    expect(groupCandidates).toHaveBeenCalledWith(18)
    expect((wrapper.get('[data-testid="monitor-primary-model"]').element as HTMLInputElement).value).toBe('gpt-5.6-sol')
    expect(wrapper.get('[data-testid="monitor-provider-openai"]').classes().join(' ')).toContain('border-emerald-500')
    wrapper.unmount()
  })

  it('fetches group models into the multi-select picker', async () => {
    groupCandidates.mockResolvedValue(['gpt-5.6-sol', 'gpt-5.5', 'gpt-5.4'])
    const wrapper = mountDialog({
      initialGroup: {
        id: 25,
        name: 'OpenAI 高缓存',
        platform: 'openai',
        status: 'active',
        rate_multiplier: 0.2,
      },
      initialEnabled: true,
    })
    await flushPromises()

    const fetchButton = wrapper.findAll('button').find((item) => item.text().includes('admin.channelMonitor.form.fetchGroupModels'))!
    await fetchButton.trigger('click')
    await flushPromises()
    const selectedOption = wrapper.findAll('label').find((item) => item.text().includes('gpt-5.4'))!
    expect((selectedOption.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(true)
    await selectedOption.find('input[type="checkbox"]').setValue(false)
    const unselectedOption = wrapper.findAll('label').find((item) => item.text().includes('gpt-5.4'))!
    expect(unselectedOption.exists()).toBe(true)
    expect((unselectedOption.find('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false)

    await unselectedOption.find('input[type="checkbox"]').setValue(true)
    const applyButton = wrapper.findAll('button').find((item) => item.text().includes('admin.channelMonitor.form.groupModelsApply'))!
    await applyButton.trigger('click')

    expect(wrapper.findComponent(ModelTagInput).props('models')).toContain('gpt-5.4')
    wrapper.unmount()
  })
})
