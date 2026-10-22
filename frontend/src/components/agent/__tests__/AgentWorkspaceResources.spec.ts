import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentWorkspaceResources from '../AgentWorkspaceResources.vue'
import type { AgentWorkPlan } from '@/api/agent'
const plan: AgentWorkPlan = { id: 'p1', title: '演示方案', brief: '', style: '', image_size: '1K', aspect_ratio: '16:9', items: [], shared_reference_asset_ids: [], conversation_id: 'c1', source_run_id: 'r1', revision: 3, status: 'draft', config_version: 1, image_api_key_id: 2, image_model: 'image', max_cost_usd: '0', item_max_cost_usd: [] }
describe('Agent workspace resources', () => {
  it('provides useful empty states without fabricating resources', () => {
    const w = mount(AgentWorkspaceResources, { props: { plans: [], images: [] } })
    expect(w.text()).toContain('还没有配置卡草稿'); expect(w.text()).toContain('生成的图片会保存在这里')
    expect(w.findAll('button')).toHaveLength(0); w.unmount()
  })
  it('opens the exact draft or generated image selected by the user', async () => {
    const w = mount(AgentWorkspaceResources, { props: { plans: [plan], images: [{ id: 'out', mime_type: 'image/png', url: 'blob:out', filename: '结果.png' }] } })
    expect(w.get('.resource-plan').text()).toContain('v3')
    await w.get('.resource-plan').trigger('click'); expect(w.emitted('plan')![0]).toEqual(['p1'])
    await w.get('.resource-image').trigger('click'); expect(w.emitted('image')![0]).toEqual(['out'])
    w.unmount()
  })
  it('retains expired result metadata and reports thumbnail failures', async () => {
    const w = mount(AgentWorkspaceResources, { props: { plans: [], images: [{ id: 'old', status: 'expired', content_available: false }, { id: 'bad', url: 'blob:bad' }] } })
    expect(w.get('.resource-image-state').text()).toBe('已过期')
    await w.get('.resource-thumbnail img').trigger('error'); expect(w.emitted('imageError')![0]).toEqual(['bad'])
    w.unmount()
  })
})
