import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentTokenUsage from '../AgentTokenUsage.vue'
import type { AgentUsageGroup } from '@/api/agent'
const usage: AgentUsageGroup = { requests: 2, input_tokens: 100, output_tokens: 200, cache_read_tokens: 50, cache_creation_tokens: 10, total_tokens: 360, image_count: 0, cost_usd: '0', pending_requests: 0, missing_usage_records: 0, missing_image_records: 0 }
describe('Agent token hover details', () => {
  it('exposes only the total at rest, opens on focus and dismisses on Escape', async () => {
    const w = mount(AgentTokenUsage, { props: { usage } })
    expect(w.text()).toBe('360'); expect(document.querySelector('[role=tooltip]')).toBeNull()
    await w.get('.token-usage').trigger('focus'); await flushPromises()
    expect(document.querySelector('[role=tooltip]')?.textContent).toContain('缓存写入')
    expect(w.get('.token-usage').attributes('aria-describedby')).toBeTruthy()
    const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    document.dispatchEvent(event); await flushPromises()
    expect(event.defaultPrevented).toBe(true)
    expect(document.querySelector('[role=tooltip]')).toBeNull(); w.unmount()
  })
  it('places hover content inside the native settings dialog top layer', async () => {
    const dialog = document.createElement('dialog'); document.body.append(dialog)
    const w = mount(AgentTokenUsage, { props: { usage }, attachTo: dialog })
    await w.get('.token-usage').trigger('mouseenter'); await flushPromises()
    expect(dialog.querySelector('[role=tooltip]')).not.toBeNull()
    await w.get('.token-usage').trigger('mouseleave'); await flushPromises()
    expect(dialog.querySelector('[role=tooltip]')).toBeNull(); w.unmount(); dialog.remove()
  })
  it('distinguishes completely missing telemetry from a recorded zero', async () => {
    const w = mount(AgentTokenUsage, { props: { usage: { ...usage, missing_usage_records: 2 } } })
    expect(w.text()).toBe('未记录')
    await w.get('.token-usage').trigger('mouseenter'); await flushPromises()
    expect(document.querySelector('[role=tooltip]')?.textContent).toContain('2 次调用缺少 Token 记录')
    expect(document.querySelector('[role=tooltip] dl')).toBeNull(); w.unmount()
  })
  it('keeps partial totals explicit and removes the popup on scroll or unmount', async () => {
    const w = mount(AgentTokenUsage, { props: { usage: { ...usage, missing_usage_records: 1 } } })
    await w.get('.token-usage').trigger('mouseenter'); await flushPromises()
    expect(document.querySelector('[role=tooltip]')?.textContent).toContain('仅统计已记录用量')
    window.dispatchEvent(new Event('scroll')); await flushPromises()
    expect(document.querySelector('[role=tooltip]')).toBeNull()
    await w.get('.token-usage').trigger('click'); await flushPromises()
    expect(document.querySelector('[role=tooltip]')).not.toBeNull()
    w.unmount(); expect(document.querySelector('[role=tooltip]')).toBeNull()
  })
})
