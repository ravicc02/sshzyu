import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { ChannelMonitor } from '@/api/admin/channelMonitor'
import type { AdminGroup } from '@/types'
import ChannelMonitorView from '@/views/admin/ChannelMonitorView.vue'

const { listMonitors, updateMonitor } = vi.hoisted(() => ({
  listMonitors: vi.fn(),
  updateMonitor: vi.fn(),
}))

const groups = [
  { id: 101, name: 'OpenAI 高缓存', platform: 'openai', status: 'active', rate_multiplier: 0.25 },
  { id: 102, name: 'OpenAI 特惠', platform: 'openai', status: 'active', rate_multiplier: 0.2 },
] as AdminGroup[]

vi.mock('@/features/channel-monitor-v2/MonitorSettingsPanel.vue', () => ({
  default: {
    name: 'MonitorSettingsPanel',
    template: '<div data-testid="settings"><slot name="probe-monitor" :groups="groups" /></div>',
    setup() {
      return { groups }
    },
  },
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      list: listMonitors,
      update: updateMonitor,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const AppLayoutStub = defineComponent({ template: '<main><slot /></main>' })
const ToggleStub = defineComponent({
  props: { modelValue: { type: Boolean, default: false }, disabled: Boolean },
  emits: ['update:modelValue'],
  template: '<button class="probe-toggle" @click="$emit(\'update:modelValue\', !modelValue)">toggle</button>',
})

function makeMonitor(overrides: Partial<ChannelMonitor> = {}): ChannelMonitor {
  return {
    id: 42,
    name: 'OpenAI 高缓存 主动探测',
    provider: 'openai',
    api_mode: 'chat_completions',
    endpoint: 'https://api.example.com',
    api_key_masked: 'sk-t***',
    primary_model: 'gpt-5.6-sol',
    extra_models: ['gpt-5.5'],
    group_id: 101,
    group_name: 'OpenAI 高缓存',
    enabled: false,
    interval_seconds: 60,
    jitter_seconds: 0,
    last_checked_at: null,
    created_by: 1,
    created_at: '2026-07-16T00:00:00Z',
    updated_at: '2026-07-16T00:00:00Z',
    primary_status: '',
    primary_latency_ms: null,
    availability_7d: 0,
    extra_models_status: [],
    template_id: null,
    extra_headers: {},
    body_override_mode: 'off',
    body_override: null,
    check_mode: 'probe',
    account_id: null,
    probe_fallback_enabled: true,
    intelligence_enabled: true,
    ...overrides,
  }
}

function mountView() {
  return mount(ChannelMonitorView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        Toggle: ToggleStub,
        MonitorFormDialog: true,
      },
    },
  })
}

describe('ChannelMonitorView active probe groups', () => {
  beforeEach(() => {
    listMonitors.mockReset()
    updateMonitor.mockReset()
    listMonitors.mockResolvedValue({
      items: [makeMonitor()],
      total: 1,
      page: 1,
      page_size: 200,
      pages: 1,
    })
    updateMonitor.mockResolvedValue(makeMonitor({ enabled: true }))
  })

  it('renders every selected group and joins its existing monitor', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('OpenAI 高缓存')
    expect(wrapper.text()).toContain('OpenAI 特惠')
    expect(wrapper.text()).toContain('gpt-5.6-sol')
    expect(wrapper.text()).toContain('channelMonitorV2.settings.probeNotConfigured')
    wrapper.unmount()
  })

  it('toggles an existing group monitor without opening configuration', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-probe-toggle-group-id="101"]').trigger('click')
    await flushPromises()
    expect(updateMonitor).toHaveBeenCalledWith(42, { enabled: true })
    expect(wrapper.findComponent({ name: 'MonitorFormDialog' }).props('show')).toBe(false)
    wrapper.unmount()
  })

  it('opens details with the selected group prefilled for an unconfigured probe', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-probe-toggle-group-id="102"]').trigger('click')
    await flushPromises()
    const dialog = wrapper.findComponent({ name: 'MonitorFormDialog' })
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('initialGroup')).toMatchObject({ id: 102, name: 'OpenAI 特惠' })
    expect(dialog.props('initialEnabled')).toBe(true)
    wrapper.unmount()
  })
})
