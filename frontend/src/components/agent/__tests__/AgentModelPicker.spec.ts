import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentModelPicker from '../AgentModelPicker.vue'
const config = { model_api_key_id: 1, model: 'text-a', image_api_key_id: 2, image_model: 'image-a', default_max_cost_usd: '1.00' }
const popup = () => document.querySelector<HTMLElement>('.agent-model-popover')!
function make(extra = {}) { return mount(AgentModelPicker, { attachTo: document.body, props: { config, capabilities: [{ id: 'text-a', reasoning_levels: ['low', 'medium', 'high', 'xhigh', 'max'], default_reasoning_effort: 'medium' }], textModels: ['text-a', 'text-b'], imageModels: ['image-a', 'image-b'], ...extra } }) }
afterEach(() => { vi.restoreAllMocks(); document.body.innerHTML = '' })
describe('Agent model picker', () => {
  it('opens a lightweight menu and selects a text model without entering settings', async () => {
    const w = make(); await w.get('.model-summary').trigger('click'); await flushPromises()
    expect(popup().getAttribute('role')).toBe('dialog')
    expect(popup().textContent).toContain('切换模型')
    const choice = Array.from(popup().querySelectorAll<HTMLButtonElement>('[data-model-purpose=text]')).find(node => node.textContent?.includes('text-b'))!
    choice.click(); await flushPromises()
    expect(w.emitted('select')).toEqual([['text', 'text-b']])
    expect(w.emitted('manage')).toBeUndefined()
    expect(document.querySelector('.agent-model-popover')).not.toBeNull(); w.unmount()
  })
  it('switches the independent image model and does not emit for the already selected model', async () => {
    const w = make(); await w.get('.model-summary').trigger('click'); await flushPromises()
    const choices = Array.from(popup().querySelectorAll<HTMLButtonElement>('[data-model-purpose=image]'))
    choices.find(node => node.textContent?.includes('image-b'))!.click(); await flushPromises()
    expect(w.emitted('select')).toEqual([['image', 'image-b']])
    popup().querySelector<HTMLButtonElement>('[data-model-purpose=text][aria-checked=true]')!.click(); await flushPromises()
    expect(w.emitted('select')).toHaveLength(1); w.unmount()
  })
  it('has no search and keeps keyboard selection and explicit Escape dismissal', async () => {
    const w = make(); await w.get('.model-summary').trigger('click'); await flushPromises()
    expect(popup().querySelector('input[type=search], .picker-search')).toBeNull()
    expect(popup().querySelectorAll('.model-option')).toHaveLength(4)
    expect(document.activeElement?.textContent).toContain('text-a')
    document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true })); await flushPromises()
    expect(document.activeElement?.textContent).toContain('text-b')
    popup().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); await flushPromises()
    expect(document.querySelector('.agent-model-popover')).toBeNull()
    expect(document.activeElement).toBe(w.get('.model-summary').element); w.unmount()
  })
  it('closes on an outside click and only enters settings through an explicit action', async () => {
    const w = make(); await w.get('.model-summary').trigger('click'); await flushPromises()
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true })); await flushPromises()
    expect(document.querySelector('.agent-model-popover')).toBeNull()
    await w.get('.model-summary').trigger('click'); await flushPromises()
    popup().querySelector<HTMLButtonElement>('.manage-models')!.click(); await flushPromises()
    expect(w.emitted('manage')).toHaveLength(1); w.unmount()
  })
  it('remains viewable while busy but cannot change an active run model', async () => {
    const w = make({ busy: true }); await w.get('.model-summary').trigger('click'); await flushPromises()
    expect(popup().textContent).toContain('任务结束后')
    for (const button of popup().querySelectorAll<HTMLButtonElement>('.model-option')) expect(button.disabled).toBe(true)
    expect(w.emitted('select')).toBeUndefined(); w.unmount()
  })
  it('shows an empty list without automatically navigating to settings', async () => {
    const w = make({ config: { ...config, model: '', image_api_key_id: 0, image_model: '' }, textModels: [], imageModels: [] })
    await w.get('.model-summary').trigger('click'); await flushPromises()
    expect(popup().textContent).toContain('在设置中添加常用模型')
    expect(w.emitted('manage')).toBeUndefined(); w.unmount()
  })
  it('positions the floating panel with CSS length units and streams effort changes separately', async () => {
    const w = make(); vi.spyOn(w.get('.model-summary').element, 'getBoundingClientRect').mockReturnValue({ top: 700, bottom: 732, left: 400, right: 560, width: 160, height: 32, x: 400, y: 700, toJSON() {} })
    await w.get('.model-summary').trigger('click'); await flushPromises()
    expect(popup().style.left).toBe('400px'); expect(popup().style.width).toBe('350px'); expect(popup().style.bottom).toMatch(/px$/)
    const slider = popup().querySelector<HTMLInputElement>('[type=range]')!
    slider.value = '2'; slider.dispatchEvent(new Event('input', { bubbles: true })); slider.dispatchEvent(new Event('change', { bubbles: true })); await flushPromises()
    expect(w.emitted('effort')).toEqual([['high']]); expect(w.emitted('select')).toBeUndefined()
    const arrow = new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true, cancelable: true }); slider.dispatchEvent(arrow); expect(arrow.defaultPrevented).toBe(false)
    w.unmount()
  })

  it('keeps the menu open through selection, saving, focus and scroll, until an outside click', async () => {
    const w = make(); await w.get('.model-summary').trigger('click'); await flushPromises()
    popup().querySelectorAll<HTMLButtonElement>('[data-model-purpose=text]')[1]!.click(); await flushPromises()
    await w.setProps({ loading: true }); await w.setProps({ loading: false }); await flushPromises()
    document.body.dispatchEvent(new Event('focusin', { bubbles: true })); document.dispatchEvent(new Event('scroll')); await flushPromises()
    expect(popup()).not.toBeNull()
    await w.get('.model-summary').trigger('click'); expect(popup()).not.toBeNull()
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true })); await flushPromises()
    expect(document.querySelector('.agent-model-popover')).toBeNull(); w.unmount()
  })
  it('uses the supported five stops and emits xhigh/max without dismissing the menu', async () => {
    const w = make(); await w.get('.model-summary').trigger('click'); await flushPromises()
    const slider = popup().querySelector<HTMLInputElement>('[type=range]')!
    expect(slider.max).toBe('4'); expect(popup().textContent).toContain('超高'); expect(popup().textContent).toContain('最高')
    for (const index of ['3', '4']) { slider.value = index; slider.dispatchEvent(new Event('input', { bubbles: true })); slider.dispatchEvent(new Event('change', { bubbles: true })); await flushPromises(); expect(popup()).not.toBeNull() }
    expect(w.emitted('effort')).toEqual([['xhigh'], ['max']]); w.unmount()
  })
  it('supports declared none/minimal and rolls an unsuccessful save back to the confirmed value', async () => {
    const w = make({ capabilities: [{ id: 'text-a', reasoning_levels: ['none', 'minimal', 'low', 'medium', 'high'], default_reasoning_effort: 'medium' }] })
    await w.get('.model-summary').trigger('click'); await flushPromises()
    const slider = popup().querySelector<HTMLInputElement>('[type=range]')!
    slider.value = '1'; slider.dispatchEvent(new Event('input', { bubbles: true })); slider.dispatchEvent(new Event('change', { bubbles: true })); await flushPromises()
    expect(w.emitted('effort')).toEqual([['minimal']])
    await w.setProps({ loading: true }); await w.setProps({ loading: false }); expect(slider.value).toBe('3')
    expect(popup().textContent).not.toContain('最高'); w.unmount()
  })

})
