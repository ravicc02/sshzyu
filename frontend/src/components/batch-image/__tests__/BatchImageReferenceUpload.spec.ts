import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import BatchImageReferenceUpload from '../BatchImageReferenceUpload.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
afterEach(() => { vi.restoreAllMocks() })

function mountUpload(overrides: Record<string, unknown> = {}) {
  return mount(BatchImageReferenceUpload, {
    props: {
      images: [], names: [], label: 'Upload references', limit: 3, multiple: true,
      disabled: false, loading: false, testId: 'references', removeTestId: 'remove-reference', clearTestId: 'clear-references',
      ...overrides,
    },
    global: { stubs: { Icon: true } },
  })
}

describe('batch reference upload', () => {
  it('emits every selected file and exposes a labelled multi-file input', async () => {
    const wrapper = mountUpload()
    try {
      const input = wrapper.get('input')
      expect(input.attributes('multiple')).toBeDefined()
      expect(input.attributes('aria-label')).toBe('Upload references')
      const files = [new File(['one'], 'one.png'), new File(['two'], 'two.png')]
      Object.defineProperty(input.element, 'files', { configurable: true, value: files })
      await input.trigger('change')
      expect(wrapper.emitted('files')?.[0]).toEqual([files])
      Object.defineProperty(input.element, 'files', { configurable: true, value: [] })
      await input.trigger('change')
      expect(wrapper.emitted('files')).toHaveLength(1)
    } finally { wrapper.unmount() }
  })

  it('supports drop and individual removal without discarding the other images', async () => {
    const wrapper = mountUpload({
      images: [{ mime_type: 'image/png', data: 'b25l' }, { mime_type: 'image/jpeg', data: 'dHdv' }],
      names: ['one.png', 'two.jpg'],
    })
    try {
      const files = [new File(['third'], 'third.png')]
      await wrapper.get('label').trigger('drop', { dataTransfer: { files } })
      expect(wrapper.emitted('files')?.[0]).toEqual([files])
      expect(wrapper.findAll('img')).toHaveLength(2)
      await wrapper.findAll('[data-testid="remove-reference"]')[1].trigger('click')
      expect(wrapper.emitted('remove')?.[0]).toEqual([1])
      await wrapper.get('[data-testid="clear-references"]').trigger('click')
      expect(wrapper.emitted('clear')).toHaveLength(1)
    } finally { wrapper.unmount() }
  })

  it.each([
    { disabled: true },
    { loading: true },
    { limit: 0 },
    { images: Array.from({ length: 3 }, () => ({ mime_type: 'image/png', data: 'cmVm' })) },
  ])('blocks file selection and drop when unavailable: %j', async overrides => {
    const wrapper = mountUpload(overrides)
    try {
      expect(wrapper.get('input').attributes('disabled')).toBeDefined()
      const files = [new File(['image'], 'image.png')]
      Object.defineProperty(wrapper.get('input').element, 'files', { configurable: true, value: files })
      await wrapper.get('input').trigger('change')
      await wrapper.get('label').trigger('drop', { dataTransfer: { files } })
      expect(wrapper.emitted('files')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it('allows replacement in single-image mode and renders required, reading and error states', async () => {
    const wrapper = mountUpload({ multiple: false, required: true, compact: true })
    try {
      expect(wrapper.text()).toContain('batchImage.config.referenceRequired')
      await wrapper.setProps({ images: [{ mime_type: 'image/png', data: 'cmVm' }], names: ['reference.png'] })
      expect(wrapper.get('input').attributes('disabled')).toBeUndefined()
      expect(wrapper.get('input').attributes('multiple')).toBeUndefined()
      expect(wrapper.get('img').attributes('src')).toBe('data:image/png;base64,cmVm')
      expect(wrapper.text()).toContain('reference.png')
      await wrapper.setProps({ loading: true })
      expect(wrapper.get('[role="status"]').text()).toContain('referenceLoading')
      await wrapper.setProps({ loading: false, error: 'read failure' })
      expect(wrapper.get('[role="alert"]').text()).toBe('read failure')
    } finally { wrapper.unmount() }
  })

  it('allows re-upload of metadata-only recovery placeholders', async () => {
    const wrapper = mountUpload({ images: [{ mime_type: 'image/png' }], names: [], limit: 1 })
    try {
      expect(wrapper.get('input').attributes('disabled')).toBeUndefined()
      expect(wrapper.text()).toContain('referenceReupload')
      expect(wrapper.find('img').exists()).toBe(false)
      await wrapper.get('label').trigger('drop')
      expect(wrapper.emitted('files')).toBeUndefined()
    } finally { wrapper.unmount() }
  })
})
