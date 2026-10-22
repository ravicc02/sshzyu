import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AgentArtifactPanel from '../AgentArtifactPanel.vue'
describe('Shared Agent artifact panel', () => {
  it('provides a nonmodal slot for plans, images and future artifacts', () => {
    const w = mount(AgentArtifactPanel, { props: { show: true, title: '配置卡', subtitle: '项目名称' }, slots: { default: '<div data-editor>自定义内容</div>' } })
    expect(w.get('[role=region]').attributes('aria-label')).toBe('配置卡')
    expect(w.find('dialog').exists()).toBe(false); expect(w.get('[data-editor]').text()).toBe('自定义内容'); w.unmount()
  })
  it('supports going back, closing and Escape without affecting the conversation', async () => {
    const w = mount(AgentArtifactPanel, { props: { show: true, title: '图片预览', canBack: true } })
    await w.get('[aria-label="返回配置卡"]').trigger('click'); expect(w.emitted('back')).toHaveLength(1)
    await w.get('[aria-label="关闭右侧面板"]').trigger('click'); await w.get('[role=region]').trigger('keydown', { key: 'Escape' })
    expect(w.emitted('close')).toHaveLength(2); w.unmount()
  })
  it('keeps pinned plans nonmodal at tablet sizes while generated previews may overlay', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    const w = mount(AgentArtifactPanel, { props: { show: true, title: '配置卡', persistent: true } })
    expect(w.get('.agent-artifact-panel').classes()).toContain('persistent')
    expect(w.emitted('overlay')!.at(-1)).toEqual([false])
    await w.setProps({ persistent: false })
    expect(w.emitted('overlay')!.at(-1)).toEqual([true])
    w.unmount(); vi.unstubAllGlobals()
  })
  it('keeps its mount point stable when hidden for teleported editors', async () => {
    const w = mount(AgentArtifactPanel, { props: { show: false, title: '配置卡' }, slots: { default: '<div id=editor-host></div>' } })
    expect(w.find('#editor-host').exists()).toBe(true); expect(w.get('[role=region]').attributes('style')).toContain('display: none')
    await w.setProps({ show: true }); expect(w.find('#editor-host').exists()).toBe(true); w.unmount()
  })
})
