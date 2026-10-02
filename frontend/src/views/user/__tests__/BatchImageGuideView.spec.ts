import { webcrypto } from 'node:crypto'
import { defineComponent, nextTick } from 'vue'
import zhBatchImage from '@/i18n/locales/zh/batchImage'
import enBatchImage from '@/i18n/locales/en/batchImage'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BatchImageGuideView from '../BatchImageGuideView.vue'
import { geminiImageSizes, keyAllowsBatchImage, openAIImageSizes, supportsBatchImagePlatform } from '@/utils/batchImage'
import type { ApiKey } from '@/types'
import { batchImageConfigPixels, buildBatchImageConfigPreview, parseBatchImageConfigInput } from '@/utils/batchImageConfig'

const { listKeys, listModels, listJobs, listItems, submitJob, retryInput, downloadZip, mergeZips, testLocale, showError, showSuccess } = vi.hoisted(() => ({
  listKeys: vi.fn(), listModels: vi.fn(), listJobs: vi.fn(), listItems: vi.fn(), submitJob: vi.fn(), retryInput: vi.fn(), downloadZip: vi.fn(), mergeZips: vi.fn(), testLocale: { value: 'en' }, showError: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('@/api', () => ({ keysAPI: { list: listKeys } }))
vi.mock('@/api/batchImage', async importOriginal => ({
  ...await importOriginal<object>(), listBatchImageModels: listModels, listBatchImageJobs: listJobs, listBatchImageItems: listItems, submitBatchImageJob: submitJob, getBatchImageRetryInput: retryInput, downloadBatchImageZip: downloadZip
}))
vi.mock('@/utils/mergeBatchImageZips', async importOriginal => ({
  ...await importOriginal<object>(), mergeBatchImageZips: mergeZips
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({
  showError, showSuccess, fetchPublicSettings: vi.fn(), publicSettings: {}
}) }))
vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<object>(),
  useI18n: () => ({ t: (key: string, values: Record<string, unknown> = {}) => {
    if (!key.startsWith('batchImage.config.')) return key
    const messages = testLocale.value === 'en' ? enBatchImage.batchImage.config : zhBatchImage.batchImage.config
    const message = messages[key.slice('batchImage.config.'.length) as keyof typeof messages] || key
    return message.replace(/\{(\w+)\}/g, (_, name: string) => String(values[name] ?? `{${name}}`))
  }, locale: testLocale })
}))

function key(id: number, platform: string, enabled = true, status = 'active') {
  return { id, name: `${platform}-${id}`, key: `test-key-${id}`, status,
    group: { platform, name: platform, allow_batch_image_generation: enabled, allow_image_generation: true } } as ApiKey
}
const openai = key(1, 'openai')
const gemini = key(2, 'gemini')
const keys = [openai, gemini, key(3, 'anthropic'), key(4, 'openai', false), key(5, 'gemini', true, 'inactive')]
const SlotStub = defineComponent({ template: '<div><slot /><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' })
const DialogStub = defineComponent({ props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' })

function mountGuide() {
  return mount(BatchImageGuideView, { global: { stubs: {
    AppLayout: SlotStub, TablePageLayout: SlotStub, DataTable: true,
    BaseDialog: DialogStub, Select: true, SearchInput: true, Icon: true, 'i18n-t': true
  } } })
}

async function openConfig(wrapper: ReturnType<typeof mountGuide>) {
  await flushPromises()
  await wrapper.findAll('button').find(button => button.text().includes('batchImage.actions.createJob'))!.trigger('click')
  await flushPromises()
}

function previewGroups(wrapper: ReturnType<typeof mountGuide>) {
  return JSON.parse(wrapper.get('[data-testid="config-payload-preview"] pre').text()) as Array<Record<string, any>>
}

beforeEach(() => {
  vi.clearAllMocks()
  testLocale.value = 'zh'
  localStorage.clear()
  localStorage.setItem('auth_user', JSON.stringify({ id: 42 }))
  vi.stubGlobal('crypto', webcrypto)
  listKeys.mockResolvedValue({ items: keys, pages: 1 })
  listModels.mockImplementation(async (token: string) => ({ data: [{ id: token === openai.key ? 'gpt-image-1' : 'gemini-3-pro-image-preview' }] }))
  listJobs.mockResolvedValue({ data: [], has_more: false })
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('batch image full results', () => {
  it('provides Chinese and English labels', () => {
    expect(zhBatchImage.batchImage.detail.viewFullResult).toBe('查看完整结果')
    expect(enBatchImage.batchImage.detail.viewFullResult).toBe('View full result')
    expect(zhBatchImage.batchImage.detail.fullResult).toContain('{id}')
    expect(enBatchImage.batchImage.detail.fullResult).toContain('{id}')
  })

  it('expands complete errors as safe, selectable text without changing jobs', async () => {
    const wrapper = mount(BatchImageGuideView, { attachTo: document.body, global: { stubs: {
      AppLayout: SlotStub, TablePageLayout: SlotStub, DataTable: true,
      BaseDialog: DialogStub, Select: true, SearchInput: true, Icon: true, 'i18n-t': true
    } } })
    try {
      await flushPromises()
      const message = '<img src=x onerror="alert(1)">\n' + 'unbroken-error'.repeat(500) + '\nEND'
      const state = wrapper.vm.$.setupState as unknown as {
        currentJob: Record<string, unknown>
        items: Record<string, unknown>[]
      }
      state.currentJob = { id: 'test-job', status: 'failed', success_count: 1, fail_count: 1, actual_cost: 0 }
      state.items = [
        { batch_id: 'test-job', custom_id: 'failed', status: 'failed', image_count: 0, error: { code: 'PROVIDER_ITEM_FAILED', message } },
        { batch_id: 'test-job', custom_id: 'success', status: 'succeeded', image_count: 0 },
        { batch_id: 'test-job', custom_id: 'waiting', status: 'pending', image_count: 0 },
        { batch_id: 'test-job', custom_id: 'code-only', status: 'failed', image_count: 0, error: { code: 'CUSTOM_ERROR', message: '' } },
      ]
      await nextTick()
      const disclosures = wrapper.findAll('[data-testid="item-full-result"]')
      expect(disclosures).toHaveLength(4)
      const disclosure = disclosures[0]
      const details = disclosure.element as HTMLDetailsElement
      expect(details.open).toBe(false)
      const summary = disclosure.get('summary')
      expect(summary.text()).toBe('batchImage.detail.viewFullResult')
      // Native summary supplies Enter/Space behavior in browsers; jsdom tests click activation.
      ;(summary.element as HTMLElement).focus()
      expect(document.activeElement).toBe(summary.element)
      await summary.trigger('click')
      expect(details.open).toBe(true)
      const content = disclosure.get('[role="region"]')
      expect(content.element.textContent).toBe('batchImage.itemResult.providerItemFailed\nPROVIDER_ITEM_FAILED\n' + message)
      expect(content.find('img').exists()).toBe(false)
      expect(content.attributes('tabindex')).toBe('0')
      expect(content.attributes('aria-label')).toBe('batchImage.detail.fullResult')
      expect(content.classes()).toEqual(expect.arrayContaining(['max-h-64', 'overflow-y-auto', 'whitespace-pre-wrap', 'select-text', '[overflow-wrap:anywhere]']))
      ;(content.element as HTMLElement).focus()
      expect(document.activeElement).toBe(content.element)
      await summary.trigger('click')
      expect(details.open).toBe(false)
      expect(disclosures[1].get('[role="region"]').text()).toBe('batchImage.itemResult.readyDownload')
      expect(disclosures[2].get('[role="region"]').text()).toBe('batchImage.itemResult.waiting')
      expect(disclosures[3].get('[role="region"]').text()).toBe('CUSTOM_ERROR')
      expect(submitJob).not.toHaveBeenCalled()
      expect(retryInput).not.toHaveBeenCalled()
      expect(listItems).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })
})

describe('OpenAI local config conversion', () => {
  it.each([
    { selectedKey: openai, language: 'zh', label: '一致性' },
    { selectedKey: gemini, language: 'zh', label: '一致性' },
    { selectedKey: openai, language: 'en', label: 'Consistency' },
    { selectedKey: gemini, language: 'en', label: 'Consistency' },
  ])('keeps the submission control outside scrolling content for $selectedKey.name in $language', async ({ selectedKey, language, label }) => {
    listKeys.mockResolvedValue({ items: [selectedKey], pages: 1 })
    testLocale.value = language
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      expect(wrapper.get('[data-testid="reference-settings"] p').text()).toBe(label)
      expect(wrapper.get('[data-testid="config-consistency"]').attributes('aria-label')).toBe(label)
      const footer = wrapper.get('[data-testid="config-footer"]')
      const submit = footer.get('[data-testid="submit-config"]')
      expect(wrapper.findAll('[data-testid="submit-config"]')).toHaveLength(1)
      expect(wrapper.get('[data-testid="config-workspace"]').find('[data-testid="submit-config"]').exists()).toBe(false)
      expect(submit.attributes('disabled')).toBeDefined()
      await wrapper.get('[data-testid="config-matrix"] summary').trigger('click')
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      expect(wrapper.get('[data-testid="config-footer"] [data-testid="submit-config"]').element).toBe(submit.element)
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      await wrapper.get('[data-testid="config-input"]').setValue('Lighthouse,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(wrapper.get('[data-testid="config-footer"] [data-testid="submit-config"]').element).toBe(submit.element)
      expect(submit.attributes('disabled')).toBeUndefined()
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('uploads item references, locks them under consistency and restores them when disabled', async () => {
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔;港口,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      const input = wrapper.get('[data-testid="config-item-reference"]')
      Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['own reference'], 'own.png', { type: 'image/png' })] })
      await input.trigger('change')
      await vi.waitFor(() => expect(previewGroups(wrapper)[0].items[0].reference_images).toBeDefined())
      expect(previewGroups(wrapper)[0].items[1].reference_images).toBeUndefined()
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      expect(wrapper.find('[data-testid="config-item-reference"]').exists()).toBe(false)
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('own.png')
      expect(previewGroups(wrapper)[0].items[0].reference_images).toBeDefined()
      await wrapper.get('[data-testid="clear-item-reference"]').trigger('click')
      expect(previewGroups(wrapper)[0].items[0].reference_images).toBeUndefined()
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('renders English creation controls and hides cards after the input is cleared', async () => {
    testLocale.value = 'en'
    const wrapper = mountGuide()
    try {
      await flushPromises()
      await wrapper.findAll('button').find(button => button.text() === 'batchImage.actions.createJob')!.trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain('Describe your images')
      expect(wrapper.get('[data-testid="config-matrix"]').attributes('open')).toBeUndefined()
      await wrapper.get('[data-testid="config-input"]').setValue('Lighthouse,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(wrapper.get('[data-testid="submit-config"]').text()).toBe('Submit and generate')
      await wrapper.get('[data-testid="config-input"]').setValue('')
      expect(wrapper.get('[data-testid="config-empty-state"]').text()).toContain('No configuration yet')
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('parses ASCII delimiters, defaults to auto and preserves protocol fields', () => {
    const result = parseBatchImageConfigInput('红色陶瓷杯;蓝色陶瓷杯,1K,1:1,')
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.cards).toHaveLength(2)
    expect(result.cards[0]).toMatchObject({ custom_id: 'img_001', prompt: '红色陶瓷杯', image_size: '1K', aspect_ratio: '1:1', model: 'auto' })
    expect(batchImageConfigPixels(result.cards[0])).toBe('1024x1024')
    expect(buildBatchImageConfigPreview(result.cards, [{ value: 'gpt-image-2' }], '杯子', 'image/png')).toMatchObject({
      errors: {}, groups: [{ model: 'gpt-image-2', task_name: '杯子', image_size: '1K', aspect_ratio: '1:1', response_mime_type: 'image/png', items: [
        { custom_id: 'img_001', prompt: '红色陶瓷杯', output_count: 1 },
        { custom_id: 'img_002', prompt: '蓝色陶瓷杯', output_count: 1 },
      ] }]
    })
    expect(submitJob).not.toHaveBeenCalled()
  })

  it('keeps Chinese punctuation inside descriptions and reserves ASCII separators', () => {
    const result = parseBatchImageConfigInput('雨夜，霓虹；路面反光。;清晨，公园。,4K,16:9,auto')
    expect(result.ok).toBe(true)
    if (!result.ok) return
    expect(result.cards.map(card => card.prompt)).toEqual(['雨夜，霓虹；路面反光。', '清晨，公园。'])
  })

  it('shows spacious input, Chinese field labels and removes verbose notices', async () => {
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      expect(wrapper.get('[data-testid="config-input"]').attributes('rows')).toBe('8')
      expect(wrapper.get('[data-testid="config-input"]').classes()).toContain('min-h-[190px]')
      expect(wrapper.get('[data-testid="config-workspace"]').classes()).toContain('md:grid-cols-2')
      expect(wrapper.text()).not.toContain('本页仅在浏览器内')
      expect(wrapper.text()).not.toContain('目前没有可用的 OpenAI 批量生图 API Key')
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      for (const text of ['画面描述', '分辨率', '画面比例', '生图模型', '生成张数']) {
        expect(wrapper.get('[data-testid="config-cards"]').text()).toContain(text)
      }
      expect(wrapper.get('[data-testid="submit-config"]').text()).toBe('提交并开始生成')
    } finally { wrapper.unmount() }
  })

  it('requires one shared reference when consistency is enabled and maps it to every item', async () => {
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔;港口,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      expect(wrapper.text()).not.toContain('gpt-image-2.5-sunburst')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      const input = wrapper.get('[data-testid="config-reference"]')
      Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['reference'], 'shared.png', { type: 'image/png' })] })
      await input.trigger('change')
      await vi.waitFor(() => expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined())
      const groups = previewGroups(wrapper)
      expect(groups[0].items).toHaveLength(2)
      for (const item of groups[0].items) {
        expect(item.reference_images).toEqual([{ mime_type: 'image/png', data: '[统一参考图 base64 已省略]' }])
      }
      expect(submitJob).not.toHaveBeenCalled()
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      expect(previewGroups(wrapper)[0].items.every((item: Record<string, unknown>) => !item.reference_images)).toBe(true)
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      await wrapper.findAll('button').find(button => button.text() === '移除')!.trigger('click')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
    } finally { wrapper.unmount() }
  })

  it('copies the reference into all parameter groups without changing the original protocol', () => {
    const result = parseBatchImageConfigInput('灯塔;港口,1K,1:1,auto')
    if (!result.ok) throw new Error(result.error)
    result.cards[1].image_size = '2K'
    const reference = { mime_type: 'image/png', data: 'cmVmZXJlbmNl' }
    const preview = buildBatchImageConfigPreview(result.cards, [{ value: 'gpt-image-2' }], '', 'image/png', reference)
    expect(preview.groups).toHaveLength(2)
    for (const group of preview.groups) expect(group.items[0].reference_images).toEqual([reference])
    expect(preview.groups[0].items[0].reference_images?.[0]).not.toBe(reference)
    expect(submitJob).not.toHaveBeenCalled()
  })

  it('rejects malformed delimiters and invalid size or ratio', () => {
    for (const input of ['风景，1K，1:1，', '风景,8K,1:1,', '风景,1K,5:4,', '风景;,1K,1:1,', '风景,1K,1:1']) {
      expect(parseBatchImageConfigInput(input)).toMatchObject({ ok: false })
    }
  })

  it('uses valid GPT Image 2 dimensions aligned with the image playground', () => {
    const ratios = ['1:1', '3:2', '2:3', '16:9', '9:16', '4:3', '3:4', '21:9']
    for (const [_tier, table] of Object.entries(openAIImageSizes)) {
      expect(Object.keys(table)).toEqual(ratios)
      for (const dimensions of Object.values(table)) {
        const [width, height] = dimensions.split('x').map(Number)
        expect(width % 16).toBe(0)
        expect(height % 16).toBe(0)
        expect(Math.max(width, height)).toBeLessThanOrEqual(3840)
        expect(width * height).toBeGreaterThanOrEqual(655360)
        expect(width * height).toBeLessThanOrEqual(8294400)
        expect(Math.max(width, height) / Math.min(width, height)).toBeLessThanOrEqual(3)
      }
    }
    // 与 image-playground 的 COMMON_SIZE_PRESETS（openai 表）逐值一致；
    // 1K 档部分比例的最长边超过 1024，计费按实际像素档（2K/4K）结算——与 image-playground 行为一致。
    expect(openAIImageSizes['1K']['16:9']).toBe('1280x720')
    expect(openAIImageSizes['1K']['3:2']).toBe('1536x1024')
    expect(openAIImageSizes['2K']['16:9']).toBe('2560x1440')
    expect(openAIImageSizes['4K']['1:1']).toBe('2880x2880')
  })

  it('uses valid Gemini dimensions aligned with the official Flash Image table', () => {
    const ratios = ['1:1', '3:2', '2:3', '16:9', '9:16', '4:3', '3:4', '21:9']
    for (const [_tier, table] of Object.entries(geminiImageSizes)) {
      expect(Object.keys(table)).toEqual(ratios)
    }
    // 与 image-playground 的 GEMINI_COMMON_SIZE_PRESETS 逐值一致；
    // 上游实测 2026-09-28：1K 16:9→1376x768、2K 16:9→2752x1536、4K 16:9→5504x3072。
    expect(geminiImageSizes['1K']['16:9']).toBe('1376x768')
    expect(geminiImageSizes['1K']['21:9']).toBe('1584x672')
    expect(geminiImageSizes['2K']['16:9']).toBe('2752x1536')
    expect(geminiImageSizes['4K']['16:9']).toBe('5504x3072')
  })

  it('converts on click and splits edited OpenAI cards into protocol groups without submitting', async () => {
    listModels.mockResolvedValue({ data: [
      { id: 'gpt-image-2', supported_image_sizes: ['1K', '2K'], supported_mime_types: ['image/png', 'image/jpeg'] },
      { id: 'gpt-image-2.5', supported_image_sizes: ['1K', '2K'], supported_mime_types: ['image/png', 'image/jpeg'] }
    ] })
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      expect(wrapper.text()).toContain('OpenAI 参数矩阵')
      expect(wrapper.text()).toContain('21:9')
      expect(wrapper.find('[data-testid="image-size"]').exists()).toBe(false)
      await wrapper.get('[data-testid="config-input"]').setValue('红色陶瓷杯;蓝色陶瓷杯,1K,1:1,')
      expect(wrapper.get('[data-testid="config-empty-state"]').exists()).toBe(true)
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(previewGroups(wrapper)[0].items).toHaveLength(2)
      const taskInput = wrapper.findAll('input').find(input => input.attributes('maxlength') === '255')!
      await taskInput.setValue('杯子任务')
      expect(previewGroups(wrapper)[0].task_name).toBe('杯子任务')
      await wrapper.get('[data-testid="config-mime"]').setValue('image/jpeg')
      await wrapper.get('[data-testid="config-cards"] [aria-label="prompt"]').setValue('红色玻璃杯')
      await wrapper.get('[data-testid="config-cards"] [aria-label="output_count"]').setValue('2')
      await wrapper.get('[data-testid="config-cards"] [aria-label="image_size"]').setValue('2K')
      await wrapper.get('[data-testid="config-cards"] [aria-label="aspect_ratio"]').setValue('16:9')
      await wrapper.get('[data-testid="config-cards"] [aria-label="model"]').setValue('gpt-image-2.5')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('2560x1440')
      expect(previewGroups(wrapper)).toEqual([
        { model: 'gpt-image-2.5', task_name: '杯子任务', image_size: '2K', aspect_ratio: '16:9', response_mime_type: 'image/jpeg', items: [{ custom_id: 'img_001', prompt: '红色玻璃杯', output_count: 2 }] },
        { model: 'gpt-image-2', task_name: '杯子任务', image_size: '1K', aspect_ratio: '1:1', response_mime_type: 'image/jpeg', items: [{ custom_id: 'img_002', prompt: '蓝色陶瓷杯', output_count: 1 }] }
      ])
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('blocks stale input, invalid edits and unsupported model capabilities until repaired', async () => {
    listModels.mockResolvedValue({ data: [{ id: 'gpt-image-2', supported_image_sizes: ['1K'], supported_mime_types: ['image/png'] }] })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('花朵,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined()
      await wrapper.get('[data-testid="config-input"]').setValue('星星,1K,1:1,')
      expect(wrapper.text()).toContain('输入已变化，请重新点击')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await wrapper.get('[data-testid="config-cards"] [aria-label="image_size"]').setValue('2K')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('当前模型不支持此 image_size')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      await wrapper.get('[data-testid="config-cards"] [aria-label="image_size"]').setValue('1K')
      await wrapper.get('[data-testid="config-cards"] [aria-label="prompt"]').setValue('  ')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('prompt 不能为空')
      await wrapper.get('[data-testid="config-cards"] [aria-label="prompt"]').setValue('星星')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined()
      await wrapper.get('[data-testid="config-mime"]').setValue('image/webp')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('当前模型不支持此输出格式')
      await wrapper.get('[data-testid="config-mime"]').setValue('image/png')
      await wrapper.get('[data-testid="config-input"]').setValue('星星,8K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(wrapper.get('[role="alert"]').text()).toContain('image_size=8K 无效')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('converts Gemini cards in the shared workspace and submits through the original protocol', async () => {
    submitJob.mockResolvedValue({ id: 'gemini-job', task_name: 'Gemini', status: 'queued', model: 'gemini-3-pro-image-preview', provider: 'gemini_api', item_count: 1, success_count: 0, fail_count: 0, estimated_cost: 1, hold_amount: 1, actual_cost: null, created_at: 1 })
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      const keySelect = wrapper.get('[data-testid="config-key"]')
      expect(keySelect.text()).toContain('[Gemini] gemini-2')
      await keySelect.setValue('2')
      await flushPromises()
      expect(wrapper.find('[data-testid="gemini-create-form"]').exists()).toBe(false)
      expect(wrapper.get('[data-testid="config-empty-state"]').exists()).toBe(true)
      expect(wrapper.get('[data-testid="config-matrix"]').text()).toContain('Gemini 参数矩阵')
      await wrapper.get('[data-testid="config-input"]').setValue('Gemini 配置 prompt,2K,21:9,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('3168x1344')
      await wrapper.get('[data-testid="submit-config"]').trigger('click')
      await flushPromises()
      await vi.waitFor(() => expect(submitJob).toHaveBeenCalledTimes(1))
      expect(submitJob).toHaveBeenCalledWith(gemini.key, expect.objectContaining({
        model: 'gemini-3-pro-image-preview', image_size: '2K', aspect_ratio: '21:9',
        items: [{ custom_id: 'img_001', prompt: 'Gemini 配置 prompt', output_count: 1 }],
        collection_id: expect.stringMatching(/^imgcol_/),
      }), expect.stringMatching(/^sub2api-ui-config-/))
    } finally { wrapper.unmount() }
  })

  it('keeps Gemini item references when shared mode is disabled and revalidates a lower-capability model', async () => {
    listKeys.mockResolvedValue({ items: [gemini], pages: 1 })
    listModels.mockResolvedValue({ data: [
      { id: 'gemini-3-pro-image-preview', supported_image_sizes: ['1K', '2K', '4K'] },
      { id: 'gemini-2.5-flash-image', supported_image_sizes: ['1K'], supported_mime_types: ['image/png'] },
    ] })
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔;港口,1K,16:9,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      const input = wrapper.get('[data-testid="config-item-reference"]')
      expect(input.attributes('multiple')).toBeDefined()
      Object.defineProperty(input.element, 'files', { configurable: true, value: Array.from({ length: 4 }, (_, index) => new File([`ref-${index}`], `ref-${index}.png`, { type: 'image/png' })) })
      await input.trigger('change')
      await vi.waitFor(() => expect(previewGroups(wrapper)[0].items[0].reference_images).toHaveLength(4))
      await wrapper.get('[aria-label="model"]').setValue('gemini-2.5-flash-image')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('3 张')
      await wrapper.get('[data-testid="remove-item-reference"]').trigger('click')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined()
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      const shared = wrapper.get('[data-testid="config-reference"]')
      Object.defineProperty(shared.element, 'files', { configurable: true, value: [new File(['shared'], 'shared.png', { type: 'image/png' })] })
      await shared.trigger('change')
      await vi.waitFor(() => expect(previewGroups(wrapper).every(group => group.items.every((item: any) => item.reference_images?.length === 1))).toBe(true))
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      expect(previewGroups(wrapper).flatMap(group => group.items).find(item => item.custom_id === 'img_001').reference_images).toHaveLength(3)
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('rechecks auto and explicit model against a changed OpenAI key', async () => {
    const secondOpenai = key(6, 'openai')
    listKeys.mockResolvedValue({ items: [openai, secondOpenai, gemini], pages: 1 })
    listModels.mockImplementation(async (token: string) => ({ data: [{ id: token === openai.key ? 'gpt-image-2' : 'gpt-image-2.5' }] }))
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      const keySelect = wrapper.get('[data-testid="config-key"]')
      expect(keySelect.text()).toContain('openai-1')
      expect(keySelect.text()).toContain('openai-6')
      expect(keySelect.text()).toContain('gemini-2')
      await wrapper.get('[data-testid="config-input"]').setValue('飞鸟,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(previewGroups(wrapper)[0].model).toBe('gpt-image-2')
      await keySelect.setValue('6')
      await flushPromises()
      expect(previewGroups(wrapper)[0].model).toBe('gpt-image-2.5')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined()
      await keySelect.setValue('1')
      await flushPromises()
      await wrapper.get('[data-testid="config-input"]').setValue('飞鸟,1K,1:1,gpt-image-2')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await keySelect.setValue('6')
      await flushPromises()
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('model 不属于当前 Key')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it.each([openai, gemini])('submits every valid protocol group with independent idempotency keys for $name', async selectedKey => {
    listKeys.mockResolvedValue({ items: [selectedKey], pages: 1 })
    const models = selectedKey === gemini ? ['gemini-3.1-flash-image-preview', 'gemini-3-pro-image-preview'] : ['gpt-image-2', 'gpt-image-2.5']
    const provider = selectedKey === gemini ? 'gemini_api' : 'openai'
    listModels.mockResolvedValue({ data: models.map(id => ({ id })) })
    const wrapper = mountGuide()
    const jobs = [
      { id: 'job-1', task_name: '杯子', status: 'queued', model: models[1], provider, item_count: 1, success_count: 0, fail_count: 0, estimated_cost: 1, hold_amount: 1, actual_cost: null, created_at: 1 },
      { id: 'job-2', task_name: '杯子', status: 'queued', model: models[0], provider, item_count: 1, success_count: 0, fail_count: 0, estimated_cost: 1, hold_amount: 1, actual_cost: null, created_at: 2 },
    ]
    submitJob.mockResolvedValueOnce(jobs[0]).mockResolvedValueOnce(jobs[1])
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('红杯;蓝杯,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await wrapper.get('[data-testid="config-cards"] [aria-label="model"]').setValue(models[1])
      await wrapper.get('[data-testid="config-cards"] [aria-label="image_size"]').setValue('2K')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined()
      expect(wrapper.get('[data-testid="config-status"]').text()).toContain('已编辑配置')
      await wrapper.get('[data-testid="submit-config"]').trigger('click')
      await flushPromises()
      await vi.waitFor(() => expect(submitJob).toHaveBeenCalledTimes(2))
      const firstKey = submitJob.mock.calls[0][2]
      const secondKey = submitJob.mock.calls[1][2]
      expect(firstKey).toMatch(/^sub2api-ui-config-/)
      expect(secondKey).toMatch(/^sub2api-ui-config-/)
      expect(secondKey).not.toBe(firstKey)
      expect(submitJob.mock.calls[0][1].items).toHaveLength(1)
      expect(submitJob.mock.calls[1][1].items).toHaveLength(1)
      const state = wrapper.vm.$.setupState as any
      state.batchJobs = jobs.map((job, index) => ({
        ...job,
        collection_id: submitJob.mock.calls[index][1].collection_id,
        api_key_id: selectedKey.id,
        api_key_name: selectedKey.name,
        child_count: 0,
      }))
      await nextTick()
      const tasks = state.visibleBatchJobs
      expect(tasks).toHaveLength(1)
      expect(tasks[0].technical_batch_count).toBe(2)
      expect(tasks[0].collection_jobs.map((job: any) => job.id)).toEqual(['job-2', 'job-1'])
      expect(wrapper.text()).not.toContain('模拟')
    } finally { wrapper.unmount() }
  })

  it.each([openai, gemini])('does not resubmit successful groups after a later group fails for $name', async selectedKey => {
    listKeys.mockResolvedValue({ items: [selectedKey], pages: 1 })
    const wrapper = mountGuide()
    const firstJob = { id: 'job-1', task_name: '杯子', status: 'queued', model: selectedKey === gemini ? 'gemini-3-pro-image-preview' : 'gpt-image-1', provider: selectedKey === gemini ? 'gemini_api' : 'openai', item_count: 1, success_count: 0, fail_count: 0, estimated_cost: 1, hold_amount: 1, actual_cost: null, created_at: 1 }
    const thirdJob = { ...firstJob, id: 'job-3', created_at: 3 }
    submitJob.mockResolvedValueOnce(firstJob).mockRejectedValueOnce(new Error('second group failed')).mockResolvedValueOnce(thirdJob)
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('红杯;绿杯;蓝杯,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await wrapper.get('[data-testid="config-cards"] [aria-label="image_size"]').setValue('2K')
      await wrapper.findAll('[data-testid="config-card-toggle"]')[2].trigger('click')
      await wrapper.get('[data-testid="config-cards"] [aria-label="image_size"]').setValue('4K')
      await wrapper.get('[data-testid="submit-config"]').trigger('click')
      await flushPromises()
      await vi.waitFor(() => expect(submitJob).toHaveBeenCalledTimes(3))
      expect(submitJob).toHaveBeenCalledTimes(3)
      expect(submitJob.mock.calls[0][1].items).toEqual([{ custom_id: 'img_001', prompt: '红杯', output_count: 1 }])
      expect(submitJob.mock.calls[1][1].items).toEqual([{ custom_id: 'img_002', prompt: '绿杯', output_count: 1 }])
      expect(submitJob.mock.calls[2][1].items).toEqual([{ custom_id: 'img_003', prompt: '蓝杯', output_count: 1 }])
      submitJob.mockResolvedValueOnce({ ...firstJob, id: 'job-2' })
      await wrapper.get('[data-testid="submit-config"]').trigger('click')
      await flushPromises()
      await vi.waitFor(() => expect(submitJob).toHaveBeenCalledTimes(4))
      expect(submitJob).toHaveBeenCalledTimes(4)
      expect(submitJob.mock.calls[3][1].items).toEqual([{ custom_id: 'img_002', prompt: '绿杯', output_count: 1 }])
      expect(submitJob.mock.calls[3][2]).toBe(submitJob.mock.calls[1][2])
      expect(submitJob.mock.calls[3][2]).not.toBe(submitJob.mock.calls[0][2])
      expect(submitJob.mock.calls[3][2]).not.toBe(submitJob.mock.calls[2][2])
    } finally { wrapper.unmount() }
  })

  it('submits from the form event using the same real path', async () => {
    const wrapper = mountGuide()
    const job = { id: 'job-form', task_name: '灯塔', status: 'queued', model: 'gpt-image-1', provider: 'openai', item_count: 1, success_count: 0, fail_count: 0, estimated_cost: 1, hold_amount: 1, actual_cost: null, created_at: 1 }
    submitJob.mockResolvedValue(job)
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await wrapper.get('form').trigger('submit')
      await vi.waitFor(() => expect(submitJob).toHaveBeenCalledTimes(1))
      await flushPromises()
      expect(submitJob.mock.calls[0][1]).toMatchObject({ model: 'gpt-image-1', items: [{ prompt: '灯塔' }] })
    } finally { wrapper.unmount() }
  })

  it.each([openai, gemini])('restores a lost response after reload with the original request and idempotency key for $name', async selectedKey => {
    listKeys.mockResolvedValue({ items: [selectedKey], pages: 1 })
    submitJob.mockRejectedValueOnce(new Error('response lost'))
    let wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      expect(submitJob).toHaveBeenCalledTimes(1)
      const originalPayload = submitJob.mock.calls[0][1]
      const originalKey = submitJob.mock.calls[0][2]
      expect(originalPayload.task_name).toMatch(/^批量生图-/)
      const stored = localStorage.getItem('sub2api-batch-image-config-attempts-v1:42')!
      expect(stored).not.toContain(selectedKey.key)
      expect(JSON.parse(stored)[0].status).toBe('pending')
      wrapper.unmount()
      wrapper = mountGuide()
      await openConfig(wrapper)
      expect(wrapper.get('[data-testid="config-recovery"]').text()).toContain('恢复配置')
      await wrapper.get('[data-testid="config-recovery"] button').trigger('click')
      await flushPromises()
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined()
      submitJob.mockResolvedValueOnce({ id: 'recovered', ...originalPayload, provider: selectedKey.group?.platform === 'gemini' ? 'gemini_api' : 'openai', status: 'queued', item_count: 1, success_count: 0, fail_count: 0 })
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      expect(submitJob.mock.calls[1][1]).toEqual(originalPayload)
      expect(submitJob.mock.calls[1][2]).toBe(originalKey)
      expect(JSON.parse(localStorage.getItem('sub2api-batch-image-config-attempts-v1:42')!)[0]).toMatchObject({ status: 'succeeded', job: { id: 'recovered' } })
      expect(wrapper.find('[data-testid="config-input"]').exists()).toBe(false)
      expect(showSuccess).toHaveBeenCalledWith('已真实提交 1 组批量任务。')
    } finally { wrapper.unmount() }
  })

  it.each([openai, gemini])('shares deterministic idempotency across two pages submitting concurrently for $name', async selectedKey => {
    listKeys.mockResolvedValue({ items: [selectedKey], pages: 1 })
    const first = mountGuide()
    const second = mountGuide()
    const releases: Array<(job: unknown) => void> = []
    submitJob.mockImplementation(() => new Promise(resolve => { releases.push(resolve) }))
    try {
      for (const wrapper of [first, second]) {
        await openConfig(wrapper)
        await wrapper.get('[data-testid="config-input"]').setValue('灯塔,1K,1:1,')
        await wrapper.get('[data-testid="convert-config"]').trigger('click')
      }
      const pending = [(first.vm.$.setupState as any).submitConfigJob(), (second.vm.$.setupState as any).submitConfigJob()]
      await vi.waitFor(() => expect(submitJob).toHaveBeenCalledTimes(2))
      expect(submitJob.mock.calls[0][2]).toBe(submitJob.mock.calls[1][2])
      expect(submitJob.mock.calls[0][1]).toEqual(submitJob.mock.calls[1][1])
      for (const release of releases) release({ id: 'same-server-job', status: 'queued', provider: 'openai', item_count: 1, success_count: 0, fail_count: 0 })
      await Promise.all(pending)
    } finally { first.unmount(); second.unmount() }
  })

  it.each(['shared', 'perItem'])('recovers Gemini multiple references in %s mode across platform changes without losing the original intent', async referenceMode => {
    listKeys.mockResolvedValue({ items: [gemini, openai], pages: 1 })
    submitJob.mockRejectedValueOnce(new Error('response lost'))
    let wrapper = mountGuide()
    const files = ['subject', 'background'].map(name => new File([name], `${name}.png`, { type: 'image/png' }))
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,2K,16:9,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      if (referenceMode === 'shared') await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      const referenceSelector = referenceMode === 'shared' ? '[data-testid="config-reference"]' : '[data-testid="config-item-reference"]'
      const input = wrapper.get(referenceSelector)
      Object.defineProperty(input.element, 'files', { configurable: true, value: files })
      await input.trigger('change')
      await vi.waitFor(() => expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined())
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      const originalPayload = submitJob.mock.calls[0][1]
      const originalIntent = submitJob.mock.calls[0][2]
      const stored = localStorage.getItem('sub2api-batch-image-config-attempts-v1:42')!
      expect(stored).not.toContain(originalPayload.items[0].reference_images[0].data)
      wrapper.unmount()
      listKeys.mockResolvedValue({ items: [openai, gemini], pages: 1 })
      wrapper = mountGuide()
      await openConfig(wrapper)
      const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
      await wrapper.get('[data-testid="config-recovery"] button').trigger('click')
      await flushPromises()
      expect(confirm).not.toHaveBeenCalled()
      expect(wrapper.get('[data-testid="config-key"]').element).toHaveProperty('value', '2')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('2752x1536')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      const restoredInput = wrapper.get(referenceSelector)
      Object.defineProperty(restoredInput.element, 'files', { configurable: true, value: files })
      await restoredInput.trigger('change')
      await vi.waitFor(() => expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined())
      submitJob.mockResolvedValueOnce({ id: 'recovered-gemini', ...originalPayload, provider: 'gemini_api', status: 'queued', item_count: 1, success_count: 0, fail_count: 0 })
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      expect(submitJob.mock.calls[1][1]).toEqual(originalPayload)
      expect(submitJob.mock.calls[1][2]).toBe(originalIntent)
    } finally { wrapper.unmount() }
  })

  it('rejects a whole over-limit reference selection and preserves existing attachments', async () => {
    listKeys.mockResolvedValue({ items: [gemini], pages: 1 })
    listModels.mockResolvedValue({ data: [{ id: 'gemini-2.5-flash-image' }] })
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      const input = wrapper.get('[data-testid="config-item-reference"]')
      const file = new File(['existing'], 'existing.png', { type: 'image/png' })
      Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
      await input.trigger('change')
      await vi.waitFor(() => expect(previewGroups(wrapper)[0].items[0].reference_images).toHaveLength(1))
      Object.defineProperty(input.element, 'files', { configurable: true, value: [file, file, file] })
      await input.trigger('change')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('3 张')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      expect((wrapper.vm.$.setupState as any).configCards[0].reference_images).toHaveLength(1)
      await wrapper.get('[data-testid="clear-item-reference"]').trigger('click')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it('confirms platform changes and clears only the provider-specific draft', async () => {
    const wrapper = mountGuide()
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,4K,16:9,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await wrapper.get('[data-testid="config-key"]').setValue('2')
      await flushPromises()
      expect(wrapper.get('[data-testid="config-key"]').element).toHaveProperty('value', '1')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('3840x2160')
      confirm.mockReturnValue(true)
      await wrapper.get('[data-testid="config-key"]').setValue('2')
      await flushPromises()
      expect(wrapper.get('[data-testid="config-input"]').element).toHaveProperty('value', '')
      expect(wrapper.get('[data-testid="config-empty-state"]').exists()).toBe(true)
      expect(wrapper.get('[data-testid="config-matrix"]').text()).toContain('Gemini 参数矩阵')
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it.each([openai, gemini])('does not submit when durable browser storage is unavailable for $name', async selectedKey => {
    listKeys.mockResolvedValue({ items: [selectedKey], pages: 1 })
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('quota exceeded') })
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      expect(submitJob).not.toHaveBeenCalled()
      expect(showError).toHaveBeenCalledWith(expect.stringContaining('浏览器无法保存提交恢复记录'))
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it.each([openai, gemini])('clears submitting state and reports unsupported crypto without sending a request for $name', async selectedKey => {
    listKeys.mockResolvedValue({ items: [selectedKey], pages: 1 })
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      vi.stubGlobal('crypto', {})
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      expect(submitJob).not.toHaveBeenCalled()
      expect(showError).toHaveBeenCalledWith(expect.stringContaining('不支持安全的提交恢复记录'))
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it('persists reference metadata only and requires re-upload when recovering', async () => {
    const wrapper = mountGuide()
    submitJob.mockRejectedValueOnce(new Error('response lost'))
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await wrapper.get('[data-testid="config-consistency"]').trigger('click')
      const input = wrapper.get('[data-testid="config-reference"]')
      Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['reference'], 'shared.png', { type: 'image/png' })] })
      await input.trigger('change')
      await vi.waitFor(() => expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeUndefined())
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      const references = submitJob.mock.calls[0][1].items[0].reference_images
      expect(references[0].data).toBeTruthy()
      const stored = JSON.parse(localStorage.getItem('sub2api-batch-image-config-attempts-v1:42')!)
      expect(stored[0].config.items[0].reference_images).toEqual([{ mime_type: 'image/png' }])
      vi.spyOn(window, 'confirm').mockReturnValue(true)
      await wrapper.get('[data-testid="config-recovery"] button').trigger('click')
      await flushPromises()
      expect(wrapper.get('[data-testid="config-consistency"]').attributes('aria-checked')).toBe('true')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      expect(wrapper.text()).toContain('请上传一张统一参考图')
    } finally { wrapper.unmount() }
  })

  it('uses a new intent only after confirmed insufficient balance and never reports it as submitted', async () => {
    const wrapper = mountGuide()
    submitJob.mockRejectedValueOnce(Object.assign(new Error('余额不足'), { code: 'INSUFFICIENT_BALANCE' }))
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('灯塔,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      expect(submitJob).toHaveBeenCalledTimes(1)
      expect(showSuccess).not.toHaveBeenCalled()
      const stored = JSON.parse(localStorage.getItem('sub2api-batch-image-config-attempts-v1:42')!)
      expect(stored[0].status).toBe('pending')
      expect(stored[0].idempotencyKey).toBe(`${submitJob.mock.calls[0][2]}-balance-1`)
      submitJob.mockResolvedValueOnce({ id: 'after-top-up', status: 'queued', provider: 'openai', item_count: 1, success_count: 0, fail_count: 0 })
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      expect(submitJob.mock.calls[1][2]).toBe(stored[0].idempotencyKey)
      expect(showSuccess).toHaveBeenCalledWith('已真实提交 1 组批量任务。')
    } finally { wrapper.unmount() }
  })

  it.each(['succeeded', 'pending'] as const)('handles a full recovery store containing %s entries without discarding unknown requests', async status => {
    const stored = Array.from({ length: 200 }, (_, index) => ({
      fingerprint: `old-${index}`, idempotencyKey: `old-${index}`, apiKeyId: 1,
      config: { model: 'gpt-image-1', items: [{ custom_id: `old-${index}`, prompt: '旧请求' }] },
      status, updatedAt: Date.now(), ...(status === 'succeeded' ? { job: { id: `old-${index}` } } : {}),
    }))
    localStorage.setItem('sub2api-batch-image-config-attempts-v1:42', JSON.stringify(stored))
    const wrapper = mountGuide()
    submitJob.mockResolvedValueOnce({ id: 'capacity-new', status: 'queued', provider: 'openai', item_count: 1, success_count: 0, fail_count: 0 })
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('新灯塔,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await (wrapper.vm.$.setupState as any).submitConfigJob()
      const saved = JSON.parse(localStorage.getItem('sub2api-batch-image-config-attempts-v1:42')!)
      expect(saved).toHaveLength(200)
      if (status === 'succeeded') {
        expect(submitJob).toHaveBeenCalledTimes(1)
        expect(saved.some((entry: any) => entry.job?.id === 'capacity-new')).toBe(true)
      } else {
        expect(submitJob).not.toHaveBeenCalled()
        expect(saved).toEqual(stored)
        expect(showError).toHaveBeenCalledWith(expect.stringContaining('浏览器无法保存提交恢复记录'))
      }
    } finally { wrapper.unmount() }
  })

  it('merges selected retry children instead of silently omitting their images', async () => {
    const wrapper = mountGuide()
    const rows = [
      { id: 'parent', api_key_id: 1, status: 'completed', success_count: 1, fail_count: 1 },
      { id: 'retry-child', parent_batch_id: 'parent', api_key_id: 1, status: 'completed', success_count: 1, fail_count: 0 },
    ]
    downloadZip.mockResolvedValue(new Blob(['zip']))
    mergeZips.mockResolvedValue(new Blob(['merged']))
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn(() => 'blob:test-merged')
      static revokeObjectURL = vi.fn()
    })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    try {
      await flushPromises()
      const state = wrapper.vm.$.setupState as any
      state.batchJobs = rows
      state.selectedJobIds = new Set(rows.map(row => row.id))
      await nextTick()
      expect(wrapper.find('[data-testid="merge-selected-zips"]').exists()).toBe(false)
      await wrapper.get('[data-testid="download-selected-jobs"]').trigger('click')
      await flushPromises()
      expect(downloadZip.mock.calls.map(call => call[1])).toEqual(['parent', 'retry-child'])
      expect(mergeZips.mock.calls[0][0].map((input: any) => input.batchId)).toEqual(['parent', 'retry-child'])
      expect(showSuccess).toHaveBeenCalledWith('已合并 2 个真实批量任务的 ZIP 结果。')
        await nextTick()
        const saveLink = wrapper.get('[data-testid="save-zip-link"]')
        expect(saveLink.attributes('href')).toBe('blob:test-merged')
        expect(saveLink.attributes('download')).toMatch(/^batch-image-merged-.*\.zip$/)
        expect(URL.revokeObjectURL).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('downloads one selected batch directly without merging or creating extra ZIPs', async () => {
    const wrapper = mountGuide()
    downloadZip.mockResolvedValue(new Blob(['single-zip']))
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn(() => 'blob:single-zip')
      static revokeObjectURL = vi.fn()
    })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    try {
      await flushPromises()
      const state = wrapper.vm.$.setupState as any
      state.batchJobs = [{ id: 'single', api_key_id: 1, model: 'gpt-image-1', status: 'completed', success_count: 1, fail_count: 0 }]
      state.selectedJobIds = new Set(['single'])
      await nextTick()
      expect(wrapper.findAll('[data-testid="download-selected-jobs"]')).toHaveLength(1)
      expect(wrapper.find('[data-testid="merge-selected-zips"]').exists()).toBe(false)
      await wrapper.get('[data-testid="download-selected-jobs"]').trigger('click')
      await flushPromises()
      expect(downloadZip).toHaveBeenCalledWith(openai.key, 'single')
      expect(mergeZips).not.toHaveBeenCalled()
      expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1)
      expect(URL.createObjectURL).toHaveBeenCalledTimes(2)
      expect(wrapper.get('[data-testid="save-zip-link"]').attributes('download')).toBe('single.zip')
    } finally { wrapper.unmount() }
  })

  it('includes failed and unfinished batches in the merged result instead of silently dropping them', async () => {
    const wrapper = mountGuide()
    downloadZip.mockResolvedValue(new Blob(['successful-zip']))
    mergeZips.mockResolvedValue(new Blob(['merged-with-errors']))
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn(() => 'blob:merged-with-errors')
      static revokeObjectURL = vi.fn()
    })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    try {
      await flushPromises()
      const state = wrapper.vm.$.setupState as any
      state.batchJobs = [
        { id: 'success', api_key_id: 1, model: 'gpt-image-1', status: 'completed', item_count: 1, success_count: 1, fail_count: 0 },
        { id: 'failed', api_key_id: 1, model: 'gpt-image-1', status: 'failed', item_count: 1, success_count: 0, fail_count: 1 },
        { id: 'running', api_key_id: 1, model: 'gpt-image-1', status: 'running', item_count: 1, success_count: 0, fail_count: 0 },
      ]
      state.selectedJobIds = new Set(['success', 'failed', 'running'])
      await nextTick()
      await wrapper.get('[data-testid="download-selected-jobs"]').trigger('click')
      await flushPromises()
      expect(downloadZip.mock.calls.map(call => call[1])).toEqual(['success'])
      const inputs = mergeZips.mock.calls[0][0]
      expect(inputs.map((input: any) => input.batchId)).toEqual(['success', 'failed', 'running'])
      expect(inputs[1].unavailable.code).toBe('FAILED')
      expect(inputs[2].unavailable.code).toBe('RUNNING')
      expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1)
      expect(URL.createObjectURL).toHaveBeenCalledTimes(2)
    } finally { wrapper.unmount() }
  })

  it('locks the unified download action against repeated clicks', async () => {
    const wrapper = mountGuide()
    let release: ((blob: Blob) => void) | undefined
    downloadZip.mockImplementationOnce(() => new Promise<Blob>(resolve => { release = resolve })).mockResolvedValue(new Blob(['zip']))
    mergeZips.mockResolvedValue(new Blob(['merged']))
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn(() => 'blob:locked-zip')
      static revokeObjectURL = vi.fn()
    })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    try {
      await flushPromises()
      const state = wrapper.vm.$.setupState as any
      state.batchJobs = ['one', 'two'].map(id => ({ id, api_key_id: 1, model: 'gpt-image-1', status: 'completed', success_count: 1, fail_count: 0 }))
      state.selectedJobIds = new Set(['one', 'two'])
      await nextTick()
      const button = wrapper.get('[data-testid="download-selected-jobs"]')
      await button.trigger('click')
      expect(button.attributes('disabled')).toBeDefined()
      await button.trigger('click')
      expect(downloadZip).toHaveBeenCalledTimes(1)
      release!(new Blob(['first-zip']))
      await flushPromises()
      expect(downloadZip).toHaveBeenCalledTimes(2)
      expect(mergeZips).toHaveBeenCalledTimes(1)
      expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1)
      expect(URL.createObjectURL).toHaveBeenCalledTimes(2)
      expect(button.attributes('disabled')).toBeUndefined()
    } finally { release?.(new Blob(['cleanup'])); wrapper.unmount() }
  })

  it('enforces merge limits before starting the unified download', async () => {
    const wrapper = mountGuide()
    try {
      await flushPromises()
      const state = wrapper.vm.$.setupState as any
      state.batchJobs = Array.from({ length: 9 }, (_, index) => ({ id: `limit-${index}`, api_key_id: 1, model: 'gpt-image-1', status: 'completed', success_count: 1, fail_count: 0 }))
      state.selectedJobIds = new Set(state.batchJobs.map((job: any) => job.id))
      await nextTick()
      await wrapper.get('[data-testid="download-selected-jobs"]').trigger('click')
      await flushPromises()
      expect(downloadZip).not.toHaveBeenCalled()
      expect(mergeZips).not.toHaveBeenCalled()
      expect(showError).toHaveBeenCalledWith(expect.stringContaining('一次最多合并 8'))
    } finally { wrapper.unmount() }
  })

  it('downloads every batch in a task without changing the current checkbox selection', async () => {
    const wrapper = mountGuide()
    downloadZip.mockResolvedValue(new Blob(['zip']))
    mergeZips.mockResolvedValue(new Blob(['merged']))
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = vi.fn(() => 'blob:task-zip')
      static revokeObjectURL = vi.fn()
    })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    try {
      await flushPromises()
      const state = wrapper.vm.$.setupState as any
      state.batchJobs = ['one', 'two'].map(id => ({ id, collection_id: 'collection', api_key_id: 1, model: 'gpt-image-1', status: 'completed', success_count: 1, fail_count: 0 }))
      state.selectedJobIds = new Set(['other-task'])
      await nextTick()
      await state.downloadTask(state.visibleBatchJobs[0])
      expect(downloadZip.mock.calls.map(call => call[1])).toEqual(['one', 'two'])
      expect(mergeZips).toHaveBeenCalledTimes(1)
      expect(state.selectedJobIds).toEqual(new Set(['other-task']))
      expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1)
      expect(URL.createObjectURL).toHaveBeenCalledTimes(2)
    } finally { wrapper.unmount() }
  })

  it('discards a late model response from a previously selected key', async () => {
    const secondOpenai = key(6, 'openai')
    let releaseFirst: ((value: unknown) => void) | undefined
    listKeys.mockResolvedValue({ items: [openai, secondOpenai], pages: 1 })
    listModels.mockImplementation((token: string) => token === openai.key
      ? new Promise(resolve => { releaseFirst = resolve })
      : Promise.resolve({ data: [{ id: 'gpt-image-2.5' }] }))
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('飞艇,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      await wrapper.get('[data-testid="config-key"]').setValue('6')
      await flushPromises()
      expect(previewGroups(wrapper)[0].model).toBe('gpt-image-2.5')
      releaseFirst?.({ data: [{ id: 'gpt-image-2' }] })
      await flushPromises()
      expect(previewGroups(wrapper)[0].model).toBe('gpt-image-2.5')
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('does not enable submission for an OpenAI key with no models', async () => {
    listModels.mockResolvedValue({ data: [] })
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('飞艇,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('当前 Key 没有可解析的 OpenAI 模型')
      expect(wrapper.get('[data-testid="config-status"]').text()).toBe('配置有误')
      expect(wrapper.get('[data-testid="submit-config"]').attributes('disabled')).toBeDefined()
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('keeps edited cards on cancelled reconversion and clears them after invalid input', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      await wrapper.get('[data-testid="config-input"]').setValue('海面,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      await wrapper.get('[data-testid="config-cards"] [aria-label="prompt"]').setValue('海浪')
      await wrapper.get('[data-testid="config-input"]').setValue('海鸥,1K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(confirm).toHaveBeenCalledTimes(1)
      expect(wrapper.get('[data-testid="config-cards"]').text()).toContain('海浪')
      expect(wrapper.get('[data-testid="config-status"]').text()).toContain('重新转换')
      confirm.mockReturnValue(true)
      await wrapper.get('[data-testid="config-input"]').setValue('海鸥,8K,1:1,')
      await wrapper.get('[data-testid="convert-config"]').trigger('click')
      expect(wrapper.get('[data-testid="config-empty-state"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('image_size=8K 无效')
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })

  it('retries with the original specification rather than the current create form', async () => {
    const wrapper = mount(BatchImageGuideView, { global: { stubs: {
      AppLayout: SlotStub, TablePageLayout: SlotStub, DataTable: true,
      BaseDialog: DialogStub, Select: true, SearchInput: true, Icon: true, 'i18n-t': true
    } } })
    try {
      await flushPromises()
      const references = [{ mime_type: 'image/png', data: 'b3JpZ2luYWw=' }]
      retryInput.mockResolvedValue({ model: 'gpt-image-2', provider: 'openai', image_size: '4K', aspect_ratio: '16:9', response_mime_type: 'image/webp', items: [{ custom_id: 'failed-1', prompt: 'Original prompt', reference_images: references }] })
      submitJob.mockRejectedValueOnce(new Error('test capture'))
      const state = wrapper.vm.$.setupState as unknown as {
        retryFailedJob: (job: Record<string, unknown>) => Promise<void>
      }
      await state.retryFailedJob({
        id: 'original', api_key_id: 1, model: 'gpt-image-2', provider: 'openai',
        status: 'failed', fail_count: 1, image_size: '4K', aspect_ratio: '16:9', response_mime_type: 'image/webp'
      })
      expect(submitJob).toHaveBeenCalledWith(openai.key, expect.objectContaining({
        model: 'gpt-image-2', provider: 'openai', parent_batch_id: 'original',
        image_size: '4K', aspect_ratio: '16:9', response_mime_type: 'image/webp',
        items: [expect.objectContaining({ prompt: 'Original prompt', reference_images: references })]
      }), expect.any(String))
    } finally { wrapper.unmount() }
  })

  it('replays failed retries with stable request fields and idempotency across pages', async () => {
    const first = mountGuide()
    const second = mountGuide()
    try {
      await flushPromises()
      retryInput.mockResolvedValue({ model: 'gpt-image-1', provider: 'openai', image_size: '1K', aspect_ratio: '1:1', response_mime_type: 'image/png', items: [{ custom_id: 'failed-1', prompt: '原始完整描述' }] })
      submitJob.mockRejectedValue(new Error('response lost'))
      const job = { id: 'original', api_key_id: 1, status: 'failed', fail_count: 1 }
      await (first.vm.$.setupState as any).retryFailedJob(job)
      testLocale.value = 'zh'
      await (second.vm.$.setupState as any).retryFailedJob(job)
      expect(submitJob).toHaveBeenCalledTimes(2)
      expect(submitJob.mock.calls[1][1]).toEqual(submitJob.mock.calls[0][1])
      expect(submitJob.mock.calls[1][2]).toBe(submitJob.mock.calls[0][2])
      expect(submitJob.mock.calls[0][1].items[0].custom_id).toMatch(/^failed-1_retry_[a-f0-9]{12}$/)
    } finally { first.unmount(); second.unmount() }
  })

  it('blocks unavailable original inputs and asks for re-upload without submitting previews', async () => {
    const wrapper = mount(BatchImageGuideView, { global: { stubs: {
      AppLayout: SlotStub, TablePageLayout: SlotStub, DataTable: true,
      BaseDialog: DialogStub, Select: true, SearchInput: true, Icon: true, 'i18n-t': true
    } } })
    try {
      await flushPromises()
      retryInput.mockRejectedValue(Object.assign(new Error('unavailable'), { code: 'BATCH_IMAGE_RETRY_INPUT_UNAVAILABLE' }))
      const state = wrapper.vm.$.setupState as unknown as { retryFailedJob: (job: Record<string, unknown>) => Promise<void> }
      await state.retryFailedJob({ id: 'expired', api_key_id: 1, status: 'failed', fail_count: 1 })
      expect(submitJob).not.toHaveBeenCalled()
      expect(listItems).not.toHaveBeenCalled()
      expect(showError).toHaveBeenCalledWith(expect.stringContaining('重新上传原参考图'))
    } finally { wrapper.unmount() }
  })

  it('fails closed when an older backend returns only item previews', async () => {
    const api = await vi.importActual<typeof import('@/api/batchImage')>('@/api/batchImage')
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true, json: async () => ({ data: [{ prompt_preview: 'not an original input' }] }) } as Response)
    await expect(api.getBatchImageRetryInput('test-token', 'task/id')).rejects.toMatchObject({ code: 'BATCH_IMAGE_RETRY_INPUT_UNAVAILABLE' })
    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('task%2Fid/items?retry_input=true'), expect.objectContaining({ cache: 'no-store' }))
  })

  it('keeps existing eligible OpenAI and Gemini keys for historical job access', () => {
    expect(keys.filter(keyAllowsBatchImage)).toEqual([openai, gemini])
    expect(supportsBatchImagePlatform(undefined)).toBe(false)
    expect(keyAllowsBatchImage({ ...openai, group: null } as ApiKey)).toBe(false)
    expect(keyAllowsBatchImage({ ...openai, group: { ...openai.group, allow_image_generation: false } } as ApiKey)).toBe(false)
    expect(keyAllowsBatchImage({ ...gemini, group: { ...gemini.group, allow_image_generation: false } } as ApiKey)).toBe(true)
  })

  it('only offers eligible OpenAI and Gemini keys for creation', async () => {
    const wrapper = mountGuide()
    try {
      await openConfig(wrapper)
      const keySelect = wrapper.get('[data-testid="config-key"]')
      expect(keySelect.text()).toContain('openai-1')
      expect(keySelect.text()).toContain('gemini-2')
      expect(keySelect.text()).not.toContain('anthropic-3')
      expect(keySelect.text()).not.toContain('openai-4')
      expect(keySelect.text()).not.toContain('gemini-5')
      expect(listModels).toHaveBeenCalledWith(openai.key)
      expect(submitJob).not.toHaveBeenCalled()
    } finally { wrapper.unmount() }
  })
})
