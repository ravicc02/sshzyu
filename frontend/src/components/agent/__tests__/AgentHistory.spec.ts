import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentHistory from '../AgentHistory.vue'
import type { AgentConversation } from '@/api/agent'
vi.mock('@/api/agent', () => ({ agentAPI: { manage: vi.fn() } }))
const config = { model_api_key_id: 1, model: 'text', default_max_cost_usd: '1' }
const recent: AgentConversation = { id: 'recent', title: '当前对话', config, config_version: 1, updated_at: '2026-10-19T00:00:00Z' }
const archived: AgentConversation = { ...recent, id: 'archived', title: '历史对话', archived_at: recent.updated_at }
afterEach(() => { document.body.replaceChildren(); vi.restoreAllMocks() })
async function openMenu(menu: ReturnType<ReturnType<typeof mount>['get']>) {
  const details = menu.element as HTMLDetailsElement
  details.open = true
  await menu.trigger('toggle')
}
describe('Agent history sections and dismissal', () => {
  it('expands and collapses both sections independently on the same sidebar', async () => {
    const w = mount(AgentHistory, { props: { items: [recent, archived] } })
    const recentSection = w.get('[data-history-section=recent]'), archiveSection = w.get('[data-history-section=archived]')
    expect(recentSection.get('.history-section-toggle').attributes('aria-expanded')).toBe('true')
    expect(archiveSection.get('.history-section-toggle').attributes('aria-expanded')).toBe('false')
    expect(archiveSection.find('.agent-conversation').exists()).toBe(false)
    await archiveSection.get('.history-section-toggle').trigger('click')
    expect(w.findAll('.agent-conversation')).toHaveLength(2)
    await recentSection.get('.history-section-toggle').trigger('click')
    expect(recentSection.find('.agent-conversation').exists()).toBe(false)
    expect(archiveSection.get('.agent-conversation').text()).toContain('历史对话')
    await recentSection.get('.history-section-toggle').trigger('click')
    expect(w.findAll('.agent-conversation')).toHaveLength(2)
    w.unmount()
  })
  it('closes a menu on an outside click, Escape and opening another menu', async () => {
    const w = mount(AgentHistory, { attachTo: document.body, props: { items: [recent, { ...recent, id: 'second', title: '另一对话' }] } })
    const menus = w.findAll('.conversation-menu')
    await openMenu(menus[0]!)
    expect((menus[0]!.element as HTMLDetailsElement).open).toBe(true)
    const blank = document.createElement('button'); document.body.append(blank)
    blank.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    blank.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushPromises()
    expect((menus[0]!.element as HTMLDetailsElement).open).toBe(false)
    await openMenu(menus[0]!)
    await openMenu(menus[1]!)
    expect((menus[0]!.element as HTMLDetailsElement).open).toBe(false)
    expect((menus[1]!.element as HTMLDetailsElement).open).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect((menus[1]!.element as HTMLDetailsElement).open).toBe(false)
    expect(document.activeElement).toBe(menus[1]!.get('summary').element)
    w.unmount()
  })
  it('keeps the open menu during clicks inside and closes it on collapsing its section', async () => {
    const w = mount(AgentHistory, { attachTo: document.body, props: { items: [recent] } })
    const menu = w.get('.conversation-menu')
    await openMenu(menu)
    const inside = menu.get('.menu-actions').element
    inside.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    inside.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushPromises()
    expect((menu.element as HTMLDetailsElement).open).toBe(true)
    await w.get('.history-section-toggle').trigger('click')
    expect((menu.element as HTMLDetailsElement).open).toBe(false)
    expect(w.find('.conversation-menu').exists()).toBe(false)
    w.unmount()
  })
  it('keeps pagination and bulk selection scoped to the expanded section', async () => {
    const items = [
      ...Array.from({ length: 25 }, (_, i) => ({ ...recent, id: `r${i}` })),
      ...Array.from({ length: 25 }, (_, i) => ({ ...archived, id: `a${i}` })),
    ]
    const w = mount(AgentHistory, { props: { items } })
    const current = w.get('[data-history-section=recent]'), archive = w.get('[data-history-section=archived]')
    await current.get('.load-more').trigger('click')
    await archive.get('.history-section-toggle').trigger('click')
    expect(current.findAll('.agent-conversation')).toHaveLength(25)
    expect(archive.findAll('.agent-conversation')).toHaveLength(20)
    await archive.get('.bulk-mode').trigger('click')
    await archive.get('.bulk-actions input').setValue(true)
    expect(archive.get('.bulk-actions span').text()).toBe('20 项')
    expect(current.findAll('input[type=checkbox]')).toHaveLength(0)
    await archive.get('.history-section-toggle').trigger('click')
    await archive.get('.history-section-toggle').trigger('click')
    expect(archive.find('.bulk-actions').exists()).toBe(false)
    w.unmount()
  })
  it('opens the matching archived section during search without hiding the other heading', async () => {
    const w = mount(AgentHistory, { props: { items: [recent, archived] } })
    await w.get('[aria-label=搜索对话]').setValue('历史')
    expect(w.get('[data-history-section=archived] .history-section-toggle').attributes('aria-expanded')).toBe('true')
    expect(w.get('[data-history-section=archived] .agent-conversation').text()).toContain('历史对话')
    expect(w.findAll('.history-section-toggle')).toHaveLength(2)
    w.unmount()
  })
})
