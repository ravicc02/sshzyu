import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentSettings from '../AgentSettings.vue'
import { agentAPI } from '@/api/agent'
vi.mock('@/api/agent', () => ({ agentAPI: { models: vi.fn(), usage: vi.fn() } }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 1 } }) }))
const config = { model_api_key_id: 1, model: 'text-model', image_api_key_id: 0, image_model: '' }
const keys = [{ id: 1, name: 'text', group_name: 'text-group', purpose: 'text' }, { id: 2, name: 'image', group_name: 'image-group', purpose: 'image' }, { id: 3, name: 'DS', group_name: 'DeepSeek', purpose: 'text' }]
function make() { return mount(AgentSettings, { props: { show: false, config: { ...config }, textKeys: keys, imageKeys: keys, busy: false, saving: false }, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } }) }
beforeEach(() => { vi.clearAllMocks(); vi.mocked(agentAPI.models).mockImplementation(async (key, purpose) => purpose === 'image' ? ['image-model'] : key === 3 ? ['new-model'] : ['text-model', 'other-text']); HTMLDialogElement.prototype.showModal = function () { this.open = true }; HTMLDialogElement.prototype.close = function () { this.open = false } })
describe('Agent connections and model import', () => {
  it('fetches into a checkbox dialog with existing models checked and no manual ID input', async () => {
    const w = make(); await w.setProps({ show: true }); await flushPromises()
    expect(agentAPI.models).not.toHaveBeenCalled()
    expect(w.find('[placeholder="输入模型 ID"]').exists()).toBe(false)
    expect(w.find('.model-discovery').exists()).toBe(false)
    await w.get('.fetch-models').trigger('click'); await flushPromises()
    expect(agentAPI.models).toHaveBeenCalledWith(1, 'text', expect.any(AbortSignal))
    expect((w.get('.agent-model-import').element as HTMLDialogElement).open).toBe(true)
    expect((w.get('[aria-label="选择 text-model"]').element as HTMLInputElement).checked).toBe(true)
    expect((w.get('[aria-label="选择 other-text"]').element as HTMLInputElement).checked).toBe(false)
    expect(w.findAll('.added-models li')).toHaveLength(1); w.unmount()
  })
  it('replaces the list with checked models and preserves the current model when still selected', async () => {
    const w = make(); await w.setProps({ show: true }); await w.get('.fetch-models').trigger('click'); await flushPromises()
    await w.get('[aria-label="选择 other-text"]').setValue(true)
    await w.get('.apply-import').trigger('click')
    expect(w.findAll('.added-models li')).toHaveLength(2)
    await w.get('form').trigger('submit'); await flushPromises()
    expect(w.emitted('save')?.[0]?.[0]).toMatchObject({ model: 'text-model' })
    expect(w.emitted('save')?.[0]?.[1]).toMatchObject({ text: [{ key_id: 1, models: ['text-model', 'other-text'] }] }); w.unmount()
  })
  it('removes unchecked models and chooses a remaining model', async () => {
    const w = make(); await w.setProps({ show: true }); await w.get('.fetch-models').trigger('click'); await flushPromises()
    await w.get('[aria-label="选择 text-model"]').setValue(false); await w.get('[aria-label="选择 other-text"]').setValue(true)
    await w.get('.apply-import').trigger('click'); await w.get('form').trigger('submit'); await flushPromises()
    expect(w.emitted('save')?.[0]?.[0]).toMatchObject({ model: 'other-text' }); w.unmount()
  })
  it('supports an independent image connection and DS text key', async () => {
    const w = make(); await w.setProps({ show: true }); await w.get('[aria-label="文本 API key"]').setValue('3')
    await w.get('[data-purpose="text"]').trigger('click'); await flushPromises()
    await w.get('[aria-label="选择 new-model"]').setValue(true); await w.get('.apply-import').trigger('click')
    await w.get('[aria-label="生图 API key"]').setValue('2'); await w.get('[data-purpose="image"]').trigger('click'); await flushPromises()
    await w.get('[aria-label="选择 image-model"]').setValue(true); await w.get('.apply-import').trigger('click')
    await w.get('form').trigger('submit'); await flushPromises()
    expect(w.emitted('save')?.[0]?.[0]).toMatchObject({ model_api_key_id: 3, model: 'new-model', image_api_key_id: 2, image_model: 'image-model' }); w.unmount()
  })
  it('keeps per-key lists when switching connections', async () => {
    const w = make(); await w.setProps({ show: true }); await w.get('[aria-label="文本 API key"]').setValue('3')
    await w.get('.fetch-models').trigger('click'); await flushPromises(); await w.get('[aria-label="选择 new-model"]').setValue(true); await w.get('.apply-import').trigger('click')
    await w.get('[aria-label="文本 API key"]').setValue('1'); expect(w.get('.added-models').text()).toContain('text-model')
    await w.get('[aria-label="文本 API key"]').setValue('3'); expect(w.get('.added-models').text()).toContain('new-model'); w.unmount()
  })
  it('cancels import without changing the added list', async () => {
    const w = make(); await w.setProps({ show: true }); await w.get('.fetch-models').trigger('click'); await flushPromises()
    await w.get('[aria-label="选择 text-model"]').setValue(false); await w.get('[aria-label="关闭导入模型"]').trigger('click')
    expect(w.get('.added-models').text()).toContain('text-model'); expect(w.emitted('close')).toBeUndefined(); w.unmount()
  })
  it('ignores late model fetches for an earlier key', async () => {
    let finish!: (value: string[]) => void
    vi.mocked(agentAPI.models).mockImplementation(key => key === 1 ? new Promise(resolve => { finish = resolve }) : Promise.resolve(['new-model']))
    const w = make(); await w.setProps({ show: true }); await w.get('.fetch-models').trigger('click')
    await w.get('[aria-label="文本 API key"]').setValue('3'); await w.get('.fetch-models').trigger('click'); await flushPromises()
    finish(['stale-model']); await flushPromises()
    expect(w.get('.available-models').text()).toContain('new-model'); expect(w.get('.available-models').text()).not.toContain('stale-model'); w.unmount()
  })
  it('never saves after the settings close during validation', async () => {
    let finish!: (value: string[]) => void
    vi.mocked(agentAPI.models).mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const w = make(); await w.setProps({ show: true }); await w.get('form').trigger('submit')
    await w.get('.agent-settings-dialog').trigger('cancel'); await w.setProps({ show: false }); finish(['text-model']); await flushPromises()
    expect(w.emitted('save')).toBeUndefined(); w.unmount()
  })
  it('shows fetch errors and allows cancel', async () => {
    vi.mocked(agentAPI.models).mockRejectedValue(new Error('offline'))
    const w = make(); await w.setProps({ show: true }); await w.get('.fetch-models').trigger('click'); await flushPromises()
    expect(w.get('.agent-model-import [role=alert]').text()).toContain('读取失败')
    expect((w.get('.apply-import').element as HTMLButtonElement).disabled).toBe(true)
    await w.get('[aria-label="关闭导入模型"]').trigger('click'); w.unmount()
  })
  it('limits a batch selection to 20 and allows searching', async () => {
    vi.mocked(agentAPI.models).mockResolvedValue(Array.from({ length: 25 }, (_, n) => `model-${n}`))
    const w = make(); await w.setProps({ show: true }); await w.get('.fetch-models').trigger('click'); await flushPromises()
    await w.get('.selection-summary button:last-child').trigger('click')
    expect(w.findAll('.available-models input:checked')).toHaveLength(20)
    await w.get('[aria-label="选择 model-24"]').setValue(true)
    expect(w.get('.agent-model-import [role=alert]').text()).toContain('20')
    expect(w.findAll('.available-models input:checked')).toHaveLength(20)
    await w.get('[placeholder="搜索模型 ID"]').setValue('model-24'); expect(w.findAll('.available-models li')).toHaveLength(1); w.unmount()
  })
  it('does not change active tasks or close while saving', async () => {
    const w = make(); await w.setProps({ show: true, busy: true }); await w.get('form').trigger('submit'); await flushPromises()
    expect(w.emitted('save')).toBeUndefined(); await w.setProps({ saving: true }); await w.get('.agent-settings-dialog').trigger('cancel'); expect(w.emitted('close')).toBeUndefined(); w.unmount()
  })
  it('keeps image library when image connection is disabled', async () => {
    const w = make(); await w.setProps({ config: { ...config, image_api_key_id: 2, image_model: 'image-model' }, show: true }); await flushPromises()
    await w.get('[aria-label="生图 API key"]').setValue('0'); await w.get('form').trigger('submit'); await flushPromises()
    expect(w.emitted('save')?.[0]?.[0]).toMatchObject({ image_api_key_id: 0, image_model: '' })
    expect(w.emitted('save')?.[0]?.[1]).toMatchObject({ image: [{ key_id: 2, models: ['image-model'] }] }); w.unmount()
  })
  it('shows keys for every provider and explains native protocols without chat support', async () => {
    const all = ['openai', 'anthropic', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'commandcode', 'cline', 'composite', 'typesafe'].map((platform, n) => ({ id: n + 1, name: platform, group_name: platform, purpose: 'text', platform, disabled_reason: platform === 'typesafe' ? '仅支持评分/分类' : '' }))
    const w = make(); await w.setProps({ textKeys: all, show: true }); await flushPromises()
    const options = w.get('[aria-label="文本 API key"]').findAll('option')
    expect(options).toHaveLength(all.length + 1)
    expect((options[options.length - 1]!.element as HTMLOptionElement).disabled).toBe(true)
    expect(options[options.length - 1]!.text()).toContain('仅支持评分/分类')
    for (const option of options.slice(1, -1)) expect((option.element as HTMLOptionElement).disabled).toBe(false)
    w.unmount()
  })

  it('uses three navigation tabs and preserves connection edits while switching panels', async () => {
    const w = make(); await w.setProps({ show: true }); await flushPromises()
    expect(w.findAll('[role=tab]').map(tab => tab.text())).toEqual(['对话配置', '生图配置', '用量统计'])
    await w.get('[aria-label="文本 API key"]').setValue('3')
    await w.get('#agent-settings-tab-image').trigger('click')
    expect(w.get('#agent-settings-content').attributes('aria-labelledby')).toBe('agent-settings-tab-image')
    await w.get('[aria-label="生图 API key"]').setValue('2')
    await w.get('#agent-settings-tab-text').trigger('click')
    expect((w.get('[aria-label="文本 API key"]').element as HTMLSelectElement).value).toBe('3')
    expect((w.get('[aria-label="生图 API key"]').element as HTMLSelectElement).value).toBe('2')
    await w.get('#agent-settings-tab-text').trigger('keydown', { key: 'ArrowRight' }); await flushPromises()
    expect(w.get('#agent-settings-tab-image').attributes('aria-selected')).toBe('true'); w.unmount()
  })
  it('keeps read-only usage available during active tasks and hides save on that panel', async () => {
    const w = make(); await w.setProps({ show: true, busy: true })
    await w.get('#agent-settings-tab-usage').trigger('click'); await flushPromises()
    expect(agentAPI.usage).toHaveBeenCalledWith(expect.any(AbortSignal))
    expect(w.find('.agent-usage').exists()).toBe(true)
    expect(w.find('.agent-settings-dialog > form > footer .primary').exists()).toBe(false)
    expect(w.get('fieldset.connections').attributes('disabled')).toBeDefined(); w.unmount()
  })

})
