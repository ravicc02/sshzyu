import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { ApiKey } from '@/types'
import type { APIKeyBatchSummary } from '@/api/batchKeys'
import BatchKeysView from '../BatchKeysView.vue'

const {
  listBatches,
  getBatch,
  createBatch,
  getAvailableGroups,
  bulkUpdate,
  deleteKey,
  copyToClipboard,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  listBatches: vi.fn(),
  getBatch: vi.fn(),
  createBatch: vi.fn(),
  getAvailableGroups: vi.fn(),
  bulkUpdate: vi.fn(),
  deleteKey: vi.fn(),
  copyToClipboard: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api', () => ({
  batchKeysAPI: { listBatches, createBatch, getBatch },
  keysAPI: { bulkUpdate, delete: deleteKey },
  userGroupsAPI: { getAvailable: getAvailableGroups },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, values?: Record<string, unknown>) =>
        values?.count != null ? `${key}:${values.count}` : key,
    }),
  }
})

const batch: APIKeyBatchSummary = {
  batch_id: 'batch-1',
  batch_name: 'October keys',
  group_id: 9,
  group_name: 'OpenAI group',
  group_platform: 'openai',
  key_count: 2,
  active_key_count: 2,
  quota_per_key: 5,
  quota_total: 10,
  quota_used_total: 1.5,
  created_at: '2026-10-30T00:00:00Z',
  last_used_at: null,
}

const key = (id: number): ApiKey => ({
  id,
  user_id: 1,
  key: `sk-batch-${id}`,
  name: `batch-${id}`,
  batch_id: 'batch-1',
  batch_name: 'October keys',
  group_id: 9,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 5,
  quota_used: id === 1 ? 1 : 0.5,
  expires_at: null,
  created_at: '2026-10-30T00:00:00Z',
  updated_at: '2026-10-30T00:00:00Z',
  current_concurrency: 0,
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
})

const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  BaseDialog: { template: '<div v-if="show"><slot /><slot name="footer" /></div>', props: ['show'] },
  ConfirmDialog: { template: '<div />' },
  Select: { template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', Number($event.target.value))"><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>', props: ['modelValue', 'options'], emits: ['update:modelValue'] },
  RouterLink: { template: '<a><slot /></a>' },
}

describe('BatchKeysView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listBatches.mockResolvedValue({ items: [batch] })
    getBatch.mockResolvedValue({ items: [key(1), key(2)] })
    getAvailableGroups.mockResolvedValue([])
    bulkUpdate.mockResolvedValue({ succeededIds: [], failures: [] })
    deleteKey.mockResolvedValue({ message: 'ok' })
    copyToClipboard.mockResolvedValue(true)
  })

  it('loads a batch and copies selected keys one per line', async () => {
    const wrapper = mount(BatchKeysView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('October keys')
    expect(wrapper.text()).toContain('batch-1')
    expect(wrapper.text()).toContain('batch-2')

    const rowCheckboxes = wrapper.findAll('tbody input[type="checkbox"]')
    await rowCheckboxes[0].trigger('change')
    await rowCheckboxes[1].trigger('change')
    const copyButton = wrapper.findAll('button').find(button => button.text().includes('batchKeys.copySelected'))
    expect(copyButton).toBeTruthy()
    await copyButton!.trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('sk-batch-1\nsk-batch-2', 'batchKeys.copied:2')
  })

  it('bulk updates all selected keys through the existing key endpoint', async () => {
    const wrapper = mount(BatchKeysView, { global: { stubs } })
    await flushPromises()
    const rowCheckboxes = wrapper.findAll('tbody input[type="checkbox"]')
    await rowCheckboxes[0].trigger('change')
    await rowCheckboxes[1].trigger('change')
    const disableButton = wrapper.findAll('button').find(button => button.text() === 'batchKeys.disableSelected')
    await disableButton!.trigger('click')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], { status: 'inactive' })
    expect(showSuccess).toHaveBeenCalled()
  })

  it.each([['M', 1500000], ['B', 1500000000]])('creates token-only batches with %s units', async (unit, expected) => {
    getAvailableGroups.mockResolvedValue([{ id: 9, name: 'OpenAI group', platform: 'openai' }])
    createBatch.mockResolvedValue({ batch_id: 'new-batch', keys: [] })
    const wrapper = mount(BatchKeysView, { global: { stubs } })
    await flushPromises()
    const create = wrapper.findAll('button').find(button => button.text().includes('batchKeys.createBatch'))
    expect(create).toBeTruthy()
    await create!.trigger('click')
    const numbers = wrapper.findAll('#batch-key-create-form input[type="number"]')
    await numbers[0].setValue('2')
    await numbers[1].setValue('0')
    await numbers[2].setValue('1.5')
    await wrapper.get('#batch-key-create-form select').setValue(unit)
    await wrapper.findAll('#batch-key-create-form select')[1].setValue('9')
    await wrapper.get('#batch-key-create-form').trigger('submit')
    await flushPromises()
    expect(createBatch).toHaveBeenCalledWith(expect.objectContaining({ count: 2, quota: 0, quota_tokens: expected, group_id: 9 }))
    wrapper.unmount()
  })

  it('exports the whole batch as one key per line', async () => {
    const blobs: Blob[] = []
    const originalCreateObjectURL = URL.createObjectURL
    const originalRevokeObjectURL = URL.revokeObjectURL
    const createObjectURL = vi.fn((value: Blob | MediaSource) => {
      blobs.push(value as Blob)
      return 'blob:batch-test'
    })
    const revokeObjectURL = vi.fn()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mount(BatchKeysView, { global: { stubs } })
    await flushPromises()

    const exportButton = wrapper.findAll('button').find(button => button.text().includes('batchKeys.exportAll'))
    await exportButton!.trigger('click')

    expect(createObjectURL).toHaveBeenCalledTimes(1)
    const exportedText = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result || ''))
      reader.onerror = () => reject(reader.error)
      reader.readAsText(blobs[0])
    })
    expect(exportedText).toBe('sk-batch-1\nsk-batch-2\n')
    expect(click).toHaveBeenCalledTimes(1)
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:batch-test')
    click.mockRestore()
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: originalCreateObjectURL })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: originalRevokeObjectURL })
  })
})
