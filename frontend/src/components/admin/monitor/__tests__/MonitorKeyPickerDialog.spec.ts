import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MonitorKeyPickerDialog from '../MonitorKeyPickerDialog.vue'
import type { ApiKey } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = {
  props: { show: Boolean },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
}

function key(id: number, groupID: number, groupName: string, platform: string): ApiKey {
  return {
    id,
    user_id: 1,
    key: `sk-${id}`,
    name: `key-${id}`,
    group_id: groupID,
    status: 'active',
    ip_whitelist: [],
    ip_blacklist: [],
    last_used_at: null,
    last_used_ip: null,
    quota: 0,
    quota_used: 0,
    expires_at: null,
    created_at: '2026-10-31T00:00:00Z',
    updated_at: '2026-10-31T00:00:00Z',
    current_concurrency: 0,
    group: { id: groupID, name: groupName, platform, rate_multiplier: 1, subscription_type: null },
    rate_limit_5h: 0,
    rate_limit_1d: 0,
    rate_limit_7d: 0,
    usage_5h: 0,
    usage_1d: 0,
    usage_7d: 0,
    window_5h_start: null,
    window_1d_start: null,
    window_7d_start: null,
    reset_5h_at: null,
    reset_1d_at: null,
    reset_7d_at: null,
  } as ApiKey
}

describe('MonitorKeyPickerDialog', () => {
  it('filters by the linked group instead of the internal OpenAI transport provider', () => {
    const wrapper = mount(MonitorKeyPickerDialog, {
      props: {
        show: true,
        loading: false,
        provider: 'openai',
        groupId: 65,
        keys: [
          key(1, 4, 'ChatGPT PRO', 'openai'),
          key(2, 65, '国模聚合分组', 'composite'),
        ],
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          GroupBadge: true,
        },
      },
    })

    expect(wrapper.text()).toContain('key-2')
    expect(wrapper.text()).not.toContain('key-1')
    expect(wrapper.text()).toContain('admin.channelMonitor.form.selectGroupKeyHint')
  })
})
