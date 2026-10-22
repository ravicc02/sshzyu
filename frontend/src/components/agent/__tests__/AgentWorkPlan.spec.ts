import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AgentWorkPlan from '../AgentWorkPlan.vue'
import type { AgentWorkPlan as WorkPlan, AgentWorkDraft } from '@/api/agent'
import type { AgentReferenceUploader, AgentReferenceUploadResult } from '@/utils/agentAttachments'
const plan: WorkPlan = { id: 'p1', title: '学习方法', brief: '面向新生的分享', style: '白底蓝色', image_size: '1K', aspect_ratio: '16:9', shared_reference_asset_ids: ['template'], items: [{ id: 'page1', title: '行动计划', content: '先确定目标', prompt: '三列布局', reference_asset_ids: ['award'] }], conversation_id: 'c1', source_run_id: 'r1', revision: 1, status: 'draft', config_version: 1, image_api_key_id: 2, image_model: 'image-model', max_cost_usd: '0', item_max_cost_usd: [] }
const assets = [{ id: 'template', mime_type: 'image/png', filename: '模板.png', url: 'blob:template' }, { id: 'award', mime_type: 'image/png', filename: '奖状.png', url: 'blob:award' }]
function make(value = plan, expanded = true, uploadAssets?: AgentReferenceUploader) { return mount(AgentWorkPlan, { props: { plan: structuredClone(value), assets, expanded, panelTarget: document.createElement('div'), uploadAssets }, global: { stubs: { Teleport: true } } }) }
async function item(w: ReturnType<typeof make>) { await w.get('.plan-page').trigger('click'); await flushPromises() }
async function upload(w: ReturnType<typeof make>, files: File[], title = '统一模板') { const input = w.get<HTMLInputElement>(`[aria-label="上传${title}"]`); Object.defineProperty(input.element, 'files', { configurable: true, value: files }); await input.trigger('change'); await flushPromises() }
const picture = (name = 'new.png') => new File(['image'], name, { type: 'image/png' })
describe('Compact visual work card and right-side editor', () => {
  it('generates only the selected complete item even if another page is incomplete', async () => {
    const w = make({ ...plan, items: [plan.items[0]!, { ...plan.items[0]!, id: 'later', prompt: '' }] })
    await item(w)
    expect(w.get('.primary').attributes('disabled')).toBeDefined()
    expect(w.get('.generate-item').attributes('disabled')).toBeUndefined()
    await w.get('.generate-item').trigger('click')
    expect(w.emitted('approveItem')![0]![1]).toBe('page1')
    expect((w.emitted('approveItem')![0]![0] as AgentWorkDraft).items).toHaveLength(2)
    expect(w.emitted('approve')).toBeUndefined(); w.unmount()
  })
  it('offers regeneration and preview below a completed item while preserving its previous image', async () => {
    const w = make({ ...plan, status: 'submitted' }); await w.setProps({ results: [{ item_id: 'page1', status: 'succeeded', asset_id: 'out' }], assets: [...assets, { id: 'out', mime_type: 'image/png', filename: 'out.png', url: 'blob:out' }] })
    await item(w)
    expect(w.get('.generate-item').text()).toContain('重新生成该项')
    await w.get('[aria-label="预览本项图片"]').trigger('click'); expect(w.emitted('preview')![0]).toEqual(['out'])
    await w.get('.generate-item').trigger('click'); expect(w.emitted('approveItem')![0]![1]).toBe('page1')
    await w.get('.item-download').trigger('click'); expect(w.emitted('download')!.at(-1)).toEqual(['out']); w.unmount()
  })
  it.each(['running', 'reconciling'])('shows per-item %s progress and prevents duplicate generation', async status => {
    const w = make(); await w.setProps({ results: [{ item_id: 'page1', status }] }); await item(w)
    expect(w.get('.item-output').attributes('data-status')).toBe(status)
    expect(w.get('.generate-item').attributes('disabled')).toBeDefined()
    await w.get('.generate-item').trigger('click'); expect(w.emitted('approveItem')).toBeUndefined(); w.unmount()
  })
  it('offers an isolated retry for a failed page', async () => {
    const w = make({ ...plan, status: 'submitted' }); await w.setProps({ results: [{ item_id: 'page1', status: 'failed', error_code: 'AGENT_GATEWAY_REJECTED' }] }); await item(w)
    expect(w.get('.generate-item').text()).toContain('重试该项')
    await w.get('.generate-item').trigger('click'); expect(w.emitted('approveItem')![0]![1]).toBe('page1'); w.unmount()
  })
  it('keeps ten pages compact until the user opens details', async () => {
    const value = { ...plan, items: Array.from({ length: 10 }, (_, i) => ({ ...plan.items[0]!, id: `page${i}`, title: `第${i + 1}页` })) }
    const w = make(value, false)
    expect(w.findAll('.plan-page')).toHaveLength(3); expect(w.text()).toContain('查看全部 10 项')
    expect(w.find('textarea').exists()).toBe(false); expect(w.find('.work-plan-editor').exists()).toBe(false)
    await w.get('.plan-open').trigger('click'); expect(w.emitted('open')).toHaveLength(1); w.unmount()
  })
  it('edits one page at a time and quotes the full changed draft', async () => {
    const w = make(); await item(w)
    await w.get('.plan-item textarea').setValue('用户最终确认的文案')
    await w.findAll('.editor-tabs button').find(button => button.text() === '概览与模板')!.trigger('click')
    await w.get('.plan-fields select').setValue('2K'); await w.get('.primary').trigger('click')
    const draft = w.emitted('approve')![0]![0] as AgentWorkDraft
    expect(draft.items[0]!.content).toBe('用户最终确认的文案'); expect(draft.image_size).toBe('2K')
    expect(w.emitted('submit')).toBeUndefined(); expect(draft).not.toHaveProperty('status'); w.unmount()
  })
  it('keeps template images with the visual description and removes the separate material page', async () => {
    const w = make({ ...plan, style: '' }, true, vi.fn())
    expect(w.findAll('.editor-tabs button').map(button => button.text())).toEqual(['概览与模板', '逐项内容'])
    expect(w.get('.visual-template [aria-label="上传统一模板"]').exists()).toBe(true)
    expect(w.find('.card-materials').exists()).toBe(false)
    expect(w.get('.primary').attributes('disabled')).toBeUndefined()
    await item(w)
    expect(w.get('[aria-label="上传本项素材"]').exists()).toBe(true)
    expect(w.find('[aria-label="上传统一模板"]').exists()).toBe(false)
    expect(w.get('.item-pagination').text()).toContain('1 / 1')
    w.unmount()
  })
  it('routes uploaded template and page previews separately from generated results', async () => {
    const w = make()
    await w.get('.card-template-summary button').trigger('click')
    expect(w.emitted('referencePreview')![0]).toEqual(['template'])
    expect(w.emitted('preview')).toBeUndefined()
    await item(w); await w.get('.reference-materials [aria-label="预览图片"]').trigger('click')
    expect(w.emitted('referencePreview')![1]).toEqual(['award'])
    w.unmount()
  })
  it('preserves edits when another page is selected', async () => {
    const w = make({ ...plan, items: [plan.items[0]!, { ...plan.items[0]!, id: 'p2', title: '第二页', content: '原第二页' }] }); await item(w)
    await w.get('.plan-item textarea').setValue('修改第一页'); await w.get('[aria-label="下一项"]').trigger('click')
    expect((w.get('.plan-item textarea').element as HTMLTextAreaElement).value).toBe('原第二页')
    await w.get('[aria-label="上一项"]').trigger('click')
    expect((w.get('.plan-item textarea').element as HTMLTextAreaElement).value).toBe('修改第一页'); expect(w.findAll('.plan-item')).toHaveLength(1); w.unmount()
  })
  it('retains unsaved edits when the right pane closes and reopens', async () => {
    const w = make(); await item(w); await w.get('.plan-item textarea').setValue('未保存修改')
    await w.setProps({ expanded: false }); expect(w.find('.work-plan-editor').exists()).toBe(false)
    await w.setProps({ expanded: true }); expect((w.get('.plan-item textarea').element as HTMLTextAreaElement).value).toBe('未保存修改'); w.unmount()
  })

  it('preserves local edits across refreshes of the same server revision', async () => {
    const w = make(); await item(w); await w.get('.plan-item textarea').setValue('尚未保存的内容')
    await w.setProps({ plan: structuredClone(plan) })
    expect((w.get('.plan-item textarea').element as HTMLTextAreaElement).value).toBe('尚未保存的内容'); w.unmount()
  })
  it('shows actual prompt and de-duplicates shared and page references in transmission order', async () => {
    const w = make({ ...plan, items: [{ ...plan.items[0]!, reference_asset_ids: ['template', 'award'] }] }); await item(w)
    expect(w.findAll('.wire-preview li').map(node => node.text())).toEqual(['模板.png', '奖状.png'])
    expect(w.get('.wire-preview pre').text()).toContain('先确定目标'); w.unmount()
  })
  it('keeps exactly two editor actions and approves the current draft without a left footer', async () => {
    const w = make({ ...plan, status: 'ready', revision: 3 })
    expect(w.find('.plan-footer').exists()).toBe(false)
    expect(w.findAll('.editor-approval-actions button').map(button => button.text())).toEqual(['保存草稿', '批准配置'])
    await w.get('.primary').trigger('click')
    expect(w.emitted('approve')![0]![0]).toEqual(expect.objectContaining({ title: plan.title }))
    await item(w); await w.get('.plan-item textarea').setValue('新内容')
    await w.get('.primary').trigger('click')
    expect((w.emitted('approve')![1]![0] as AgentWorkDraft).items[0]!.content).toBe('新内容')
    expect(w.emitted('submit')).toBeUndefined(); w.unmount()
  })
  it('keeps historical and pending fields read-only while allowing page navigation', async () => {
    const w = make({ ...plan, status: 'submitted', items: [plan.items[0]!, { ...plan.items[0]!, id: 'p2' }] }); await item(w)
    expect(w.get('.plan-item textarea').attributes('disabled')).toBeDefined()
    expect(w.get('[aria-label="选择输出项"]').attributes('disabled')).toBeUndefined()
    await w.get('[aria-label="下一项"]').trigger('click'); expect(w.get('.plan-item header').text()).toContain('2 / 2')
    expect(w.find('.primary').exists()).toBe(false); w.unmount()
  })
  it('previews generated results and permits selective follow-up', async () => {
    const w = make({ ...plan, status: 'submitted' }); await w.setProps({ results: [{ item_id: 'page1', status: 'succeeded', asset_id: 'out1' }] })
    await w.get('.plan-results button').trigger('click'); expect(w.emitted('preview')![0]).toEqual(['out1'])
    expect(w.find('.plan-footer').exists()).toBe(false); expect(w.find('.plan-result-summary').text()).toContain('已完成'); w.unmount()
  })
  it('requires new pricing after a model binding changes', async () => {
    const w = make({ ...plan, status: 'ready' }); await w.setProps({ configVersion: 2 })
    expect(w.get('.primary').text()).toBe('批准配置'); expect(w.text()).toContain('重新核价'); w.unmount()
  })
  it('retains incomplete ten-page content without enabling execution', async () => {
    const w = make({ ...plan, image_size: '', items: Array.from({ length: 10 }, (_, n) => ({ ...plan.items[0]!, id: `p${n}`, prompt: '' })) })
    expect(w.get('.primary').attributes('disabled')).toBeDefined(); expect(w.text()).toContain('待补充')
    await w.get('.plan-fields input').setValue('新的草稿名称')
    await w.findAll('.editor-approval-actions button').find(button => button.text() === '保存草稿')!.trigger('click')
    const draft = w.emitted('save')![0]![0] as AgentWorkDraft
    expect(draft.title).toBe('新的草稿名称'); expect(draft.items).toHaveLength(10); expect(w.emitted('submit')).toBeUndefined(); w.unmount()
  })
  it('does not add a separate authorization wizard and permits revoking existing scopes', async () => {
    const w = make(); await w.setProps({ imageModel: 'image-model', grants: [{ id: 'g1', plan_id: plan.id, purpose: 'sample', max_count: 1, used_count: 0, reserved_image_cost_usd: '0', status: 'open', expires_at: '2026-10-21T00:00:00Z', image_api_key_id: 2, image_model: 'image-model' }] })
    expect(w.find('.grant-panel').exists()).toBe(false)
    expect(w.get('.overview-actions').text()).not.toContain('执行授权')
    await w.get('.existing-grants button').trigger('click')
    expect(w.emitted('revoke')![0]).toEqual(['g1']); w.unmount()
  })
  it('uploads several shared images and binds them without changing page-specific images', async () => {
    const uploader = vi.fn<AgentReferenceUploader>().mockResolvedValue({ assets: [{ id: 'new1' }, { id: 'new2' }], errors: [] })
    const w = make(plan, true, uploader); const files = [picture('1.png'), picture('2.png')]
    expect(w.get<HTMLInputElement>('[aria-label="上传统一模板"]').element.multiple).toBe(true)
    await upload(w, files); expect(uploader).toHaveBeenCalledWith(files)
    await w.get('.primary').trigger('click'); const draft = w.emitted('approve')![0]![0] as AgentWorkDraft
    expect(draft.shared_reference_asset_ids).toEqual(['template', 'new1', 'new2']); expect(draft.items[0]!.reference_asset_ids).toEqual(['award']); w.unmount()
  })
  it('binds newly uploaded material to the selected page only', async () => {
    const uploader = vi.fn<AgentReferenceUploader>().mockResolvedValue({ assets: [{ id: 'page-photo' }], errors: [] })
    const w = make({ ...plan, items: [plan.items[0]!, { ...plan.items[0]!, id: 'p2', reference_asset_ids: [] }] }, true, uploader)
    await item(w); await w.get('[aria-label="下一项"]').trigger('click'); await upload(w, [picture()], '本项素材')
    await w.get('.primary').trigger('click'); const draft = w.emitted('approve')![0]![0] as AgentWorkDraft
    expect(draft.shared_reference_asset_ids).toEqual(['template']); expect(draft.items[0]!.reference_asset_ids).toEqual(['award']); expect(draft.items[1]!.reference_asset_ids).toEqual(['page-photo']); w.unmount()
  })
  it('keeps successful files and describes individual upload failures', async () => {
    const w = make(plan, true, async () => ({ assets: [{ id: 'ok' }], errors: ['failed.png：上传失败'] }))
    await upload(w, [picture(), picture('failed.png')]); expect(w.text()).toContain('failed.png')
    await w.get('.primary').trigger('click'); expect((w.emitted('approve')![0]![0] as AgentWorkDraft).shared_reference_asset_ids).toEqual(['template', 'ok']); w.unmount()
  })
  it('prevents executing or editing while reference upload is unfinished', async () => {
    let finish!: (value: AgentReferenceUploadResult) => void
    const w = make(plan, true, () => new Promise(resolve => { finish = resolve }))
    const input = w.get<HTMLInputElement>('[aria-label="上传统一模板"]'); Object.defineProperty(input.element, 'files', { configurable: true, value: [picture()] })
    await input.trigger('change'); expect(w.get('.primary').attributes('disabled')).toBeDefined(); expect(w.get('.plan-fields input').attributes('disabled')).toBeDefined()
    finish({ assets: [{ id: 'ok' }], errors: [] }); await flushPromises(); expect(w.get('.primary').attributes('disabled')).toBeUndefined(); w.unmount()
  })
  it('checks the combined eight-reference limit before uploading', async () => {
    const uploader = vi.fn<AgentReferenceUploader>()
    const w = make({ ...plan, shared_reference_asset_ids: Array.from({ length: 7 }, (_, n) => `a${n}`) }, true, uploader)
    await upload(w, [picture()]); expect(uploader).not.toHaveBeenCalled(); expect(w.text()).toContain('合计最多 8 张'); w.unmount()
  })
  it('rejects unsupported and empty reference files before dispatch', async () => {
    const uploader = vi.fn<AgentReferenceUploader>(), w = make(plan, true, uploader)
    await upload(w, [new File(['pdf'], 'source.pdf')]); expect(uploader).not.toHaveBeenCalled(); expect(w.text()).toContain('PNG')
    await upload(w, [new File([], 'empty.png')]); expect(uploader).not.toHaveBeenCalled(); w.unmount()
  })
  it('discards a stale upload binding after a server revision replaces the draft', async () => {
    let finish!: (value: AgentReferenceUploadResult) => void
    const w = make(plan, true, () => new Promise(resolve => { finish = resolve }))
    const input = w.get<HTMLInputElement>('[aria-label="上传统一模板"]'); Object.defineProperty(input.element, 'files', { configurable: true, value: [picture()] }); await input.trigger('change')
    await w.setProps({ plan: { ...plan, revision: 2, shared_reference_asset_ids: [] } })
    finish({ assets: [{ id: 'stale' }], errors: [] }); await flushPromises(); await w.get('.primary').trigger('click')
    expect((w.emitted('approve')![0]![0] as AgentWorkDraft).shared_reference_asset_ids).toEqual([]); expect(w.get('.primary').attributes('disabled')).toBeUndefined(); w.unmount()
  })
  it('lets the user unlink expired materials and reupload replacements', async () => {
    const w = make(); await w.setProps({ assets: [{ ...assets[0]!, url: undefined, status: 'expired', content_available: false }] })
    expect(w.findAll('.reference-options img')).toHaveLength(0)
    await w.get('[aria-label="移除素材 模板.png"]').trigger('click'); await w.get('.primary').trigger('click')
    expect((w.emitted('approve')![0]![0] as AgentWorkDraft).shared_reference_asset_ids).toEqual([]); w.unmount()
  })
})
