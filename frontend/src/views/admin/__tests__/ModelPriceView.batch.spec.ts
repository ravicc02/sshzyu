import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelPriceView from '../ModelPriceView.vue'
import messages from '@/i18n/locales/en/admin/modelPrice'

const api = vi.hoisted(() => ({ listAllGroupModelPrices: vi.fn(), setModelRateMultiplier: vi.fn(), clearModelRateMultiplier: vi.fn() }))
vi.mock('@/api/admin/modelPrice', () => ({ ...api, default: api }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({
  t: (key: string, values: Record<string, unknown> = {}) => {
    const message = messages.modelPrice[key.split('.').pop() as keyof typeof messages.modelPrice] ?? key
    return Object.entries(values).reduce((text, [name, value]) => text.replaceAll('{' + name + '}', String(value)), message)
  }
}) }))

async function setup() {
  const wrapper = mount(ModelPriceView, { global: {
    stubs: { AppLayout: { template: '<div><slot /></div>' }, PlatformIcon: true, Teleport: true }
  } })
  await flushPromises()
  await wrapper.find('button[aria-expanded]').trigger('click')
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  api.listAllGroupModelPrices.mockResolvedValue([
    { id: 1, name: 'DeepSeek', platform: 'deepseek', rate_multiplier: 0.3,
      models: ['model-a', 'model-b'].map(model => ({ model, custom_rate: false, rate_multiplier: null })) },
    { id: 2, name: 'Other', platform: 'deepseek', rate_multiplier: 1,
      models: [{ model: 'model-a', custom_rate: false, rate_multiplier: null }] }
  ])
  api.setModelRateMultiplier.mockResolvedValue(undefined)
})

describe('model pricing batch actions', () => {
  it('saves only selected models', async () => {
    const wrapper = await setup()
    await wrapper.get('[data-testid="select-all-1"]').setValue(true)
    await wrapper.get('[data-testid="batch-adjust-1"]').trigger('click')
    await wrapper.get('input[type="number"]').setValue('0.5')
    await wrapper.get('[data-testid="save-rate"]').trigger('click')
    await flushPromises()
    expect(api.setModelRateMultiplier.mock.calls).toEqual([[1, 'model-a', 0.5], [1, 'model-b', 0.5]])
    expect(wrapper.get('[data-testid="batch-adjust-1"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
  it('retains failed items and retries only those items', async () => {
    api.setModelRateMultiplier.mockRejectedValueOnce(new Error('failed'))
    const wrapper = await setup()
    await wrapper.get('[data-testid="select-all-1"]').setValue(true)
    await wrapper.get('[data-testid="batch-adjust-1"]').trigger('click')
    await wrapper.get('input[type="number"]').setValue('0.8')
    await wrapper.get('[data-testid="save-rate"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('1')
    expect(wrapper.get('[data-testid="select-model-1-model-a"]').element).toHaveProperty('checked', true)
    expect(wrapper.get('[data-testid="select-model-1-model-b"]').element).toHaveProperty('checked', false)
    await wrapper.get('[data-testid="save-rate"]').trigger('click')
    await flushPromises()
    expect(api.setModelRateMultiplier.mock.calls).toEqual([[1, 'model-a', 0.8], [1, 'model-b', 0.8], [1, 'model-a', 0.8]])
    wrapper.unmount()
  })
  it('rejects nonpositive multipliers and supports clearing selection', async () => {
    const wrapper = await setup()
    await wrapper.get('[data-testid="select-model-1-model-a"]').setValue(true)
    expect(wrapper.get('[data-testid="select-all-1"]').element).toHaveProperty('indeterminate', true)
    await wrapper.get('[data-testid="batch-adjust-1"]').trigger('click')
    await wrapper.get('input[type="number"]').setValue('0')
    await wrapper.get('[data-testid="save-rate"]').trigger('click')
    expect(api.setModelRateMultiplier).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="cancel-rate"]').trigger('click')
    await wrapper.get('[data-testid="clear-selection-1"]').trigger('click')
    expect(wrapper.get('[data-testid="batch-adjust-1"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
  it('keeps identically named models isolated by group', async () => {
    const wrapper = await setup()
    await wrapper.findAll('button[aria-expanded]')[1].trigger('click')
    await wrapper.get('[data-testid="select-model-1-model-a"]').setValue(true)
    expect(wrapper.get('[data-testid="select-model-2-model-a"]').element).toHaveProperty('checked', false)
    await wrapper.get('[data-testid="batch-adjust-1"]').trigger('click')
    await wrapper.get('input[type="number"]').setValue('1.2')
    await wrapper.get('[data-testid="save-rate"]').trigger('click')
    await flushPromises()
    expect(api.setModelRateMultiplier.mock.calls).toEqual([[1, 'model-a', 1.2]])
    expect(wrapper.text()).toContain('1x')
    wrapper.unmount()
  })
  it('blocks empty input, duplicate submission and cancellation during saving', async () => {
    let finish!: () => void
    api.setModelRateMultiplier.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    const wrapper = await setup()
    await wrapper.get('[data-testid="select-all-1"]').setValue(true)
    await wrapper.get('[data-testid="batch-adjust-1"]').trigger('click')
    await wrapper.get('[data-testid="save-rate"]').trigger('click')
    expect(api.setModelRateMultiplier).not.toHaveBeenCalled()
    await wrapper.get('input[type="number"]').setValue('0.7')
    await wrapper.get('[data-testid="save-rate"]').trigger('click')
    expect(wrapper.get('[data-testid="cancel-rate"]').attributes('disabled')).toBeDefined()
    await wrapper.get('input[type="number"]').trigger('keyup.enter')
    expect(api.setModelRateMultiplier).toHaveBeenCalledTimes(1)
    finish()
    await flushPromises()
    expect(api.setModelRateMultiplier).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
  it('toggles full selection off and leaves single-model adjustment available', async () => {
    const wrapper = await setup()
    await wrapper.get('[data-testid="select-all-1"]').setValue(true)
    await wrapper.get('[data-testid="select-all-1"]').setValue(false)
    expect(wrapper.get('[data-testid="batch-adjust-1"]').attributes('disabled')).toBeDefined()
    await wrapper.findAll('li button')[0].trigger('click')
    await wrapper.get('input[type="number"]').setValue('0.4')
    await wrapper.get('[data-testid="save-rate"]').trigger('click')
    await flushPromises()
    expect(api.setModelRateMultiplier.mock.calls).toEqual([[1, 'model-a', 0.4]])
    api.clearModelRateMultiplier.mockRejectedValueOnce(new Error('failed'))
    await wrapper.findAll('li button')[1].trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="status"]').text()).toContain('Could not reset')
    api.clearModelRateMultiplier.mockResolvedValueOnce(undefined)
    await wrapper.findAll('li button')[1].trigger('click')
    await flushPromises()
    expect(wrapper.findAll('li button')).toHaveLength(2)
    wrapper.unmount()
  })
})
