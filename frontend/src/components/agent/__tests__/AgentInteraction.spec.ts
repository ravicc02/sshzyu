import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AgentQuestion from '../AgentQuestion.vue'
import AgentHistory from '../AgentHistory.vue'
import AgentImagePreview from '../AgentImagePreview.vue'
import AgentTrace from '../AgentTrace.vue'
import { agentAPI, type AgentConversation } from '@/api/agent'
vi.mock('@/api/agent', () => ({ agentAPI: { manage: vi.fn() } }))
beforeEach(() => {
  vi.clearAllMocks()
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute('open', '') }
  HTMLDialogElement.prototype.close = function () { this.removeAttribute('open') }
})
const config = { model_api_key_id: 1, model: 'text', default_max_cost_usd: '1' }
const items: AgentConversation[] = [
  { id: 'one', title: '海边照片', config, config_version: 1, updated_at: '2026-10-19T00:00:00Z' },
  { id: 'two', title: '工作讨论', config, config_version: 1, updated_at: '2026-10-19T00:00:00Z' },
  { id: 'archive', title: '历史归档', config, config_version: 1, archived_at: '2026-10-19T00:00:00Z', updated_at: '2026-10-19T00:00:00Z' },
]
describe('Agent clarification and conversation interactions', () => {
  it('previews text as plain content and presents Office file metadata without image decoding', async () => {
    const w = mount(AgentImagePreview, { props: { url: 'blob:text', filename: '说明.md', mime: 'text/markdown', size: 1024, text: '<script>alert(1)</script>' } })
    await flushPromises(); expect(w.find('img').exists()).toBe(false)
    expect(w.get('pre').text()).toBe('<script>alert(1)</script>'); expect(w.find('script').exists()).toBe(false)
    expect(w.text()).toContain('1.0 KB'); expect(w.get('[aria-label=关闭文件预览]').exists()).toBe(true)
    await w.setProps({ filename: '分析.xlsx', mime: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', text: undefined })
    expect(w.text()).toContain('分析.xlsx'); expect(w.text()).toContain('XLSX'); expect(w.find('pre').exists()).toBe(false); w.unmount()
  })

  it('requires necessary details and lets users choose presets or enter their own answers', async () => {
    const w = mount(AgentQuestion, { props: { question: { id: 'q1', question: '你想生成什么照片？', fields: [{ id: 'subject', label: '照片内容', options: [], required: true }, { id: 'size', label: '分辨率', options: ['1K', '2K', '4K'], required: true }], deadline: '2026-10-19T05:00:00Z' } } })
    expect(w.get('button[type=submit]').attributes('disabled')).toBeDefined()
    expect(w.get('.question-submit').text()).toBe('确认并继续')
    await w.get('[id=q1-subject]').setValue('海边的猫')
    await w.findAll('.question-options button')[1]!.trigger('click')
    expect(w.find('[id=q1-size]').exists()).toBe(false)
    await w.get('form').trigger('submit')
    expect(w.emitted('answer')?.[0]).toEqual([{ subject: '海边的猫', size: '2K' }])
    await w.setProps({ sending: true }); expect(w.get('fieldset').attributes('disabled')).toBeDefined()
    expect(w.get('.question-submit').text()).toBe('正在提交…'); expect(w.get('form').attributes('aria-busy')).toBe('true')
    w.unmount()
  })
  it('only asks for text when the selected option is other', async () => {
    const w = mount(AgentQuestion, { props: { question: { id: 'q-other', question: '选择风格', fields: [{ id: 'style', label: '风格', options: ['写实摄影', '其他（请描述）'], required: true }], deadline: '2026-10-19T05:00:00Z' } } })
    const options = w.findAll('.question-options button')
    await options[0]!.trigger('click')
    expect(w.find('[id=q-other-style]').exists()).toBe(false)
    expect(w.get('button[type=submit]').attributes('disabled')).toBeUndefined()
    await options[1]!.trigger('click')
    expect(w.get('[id=q-other-style]').exists()).toBe(true)
    expect(w.get('button[type=submit]').attributes('disabled')).toBeDefined()
    await w.get('[id=q-other-style]').setValue('复古胶片风')
    await w.get('form').trigger('submit')
    expect(w.emitted('answer')?.[0]).toEqual([{ style: '复古胶片风' }])
    expect(w.get('.question-submit').text()).toContain('确认并继续')
    await w.get('[aria-label=退出本次询问]').trigger('click')
    expect(w.emitted('cancel')).toHaveLength(1)
    w.unmount()
  })
  it('keeps recent and archived sections together, searches and restores using the persisted API', async () => {
    vi.mocked(agentAPI.manage).mockResolvedValue(items)
    const w = mount(AgentHistory, { props: { items } })
    expect(w.findAll('.agent-conversation')).toHaveLength(2)
    await w.get('[aria-label=搜索对话]').setValue('海边')
    expect(w.findAll('.agent-conversation')).toHaveLength(1)
    await w.get('[aria-label=搜索对话]').setValue('')
    await w.get('[data-history-section=archived] .history-section-toggle').trigger('click')
    expect(w.findAll('.agent-conversation')).toHaveLength(3)
    expect(w.get('[data-history-section=archived] .agent-conversation').text()).toContain('历史归档')
    await w.get('[data-history-section=archived]').findAll('.menu-actions button')[1]!.trigger('click'); await flushPromises()
    expect(agentAPI.manage).toHaveBeenCalledWith(['archive'], 'restore', '')
    expect(w.emitted('changed')?.[0]).toEqual([items]); w.unmount()
  })
  it('provides new conversation, compact titles, search and load-more without long-term tasks', async () => {
    const many = Array.from({ length: 25 }, (_, n) => ({ ...items[0]!, id: `item-${n}`, title: `对话 ${n}` }))
    const w = mount(AgentHistory, { props: { items: many } })
    expect(w.text()).not.toContain('长期任务')
    expect(w.get('header h3').text()).toBe('对话')
    await w.get('.new-conversation').trigger('click'); expect(w.emitted('new')).toHaveLength(1)
    expect(w.findAll('.agent-conversation')).toHaveLength(20)
    await w.get('.load-more').trigger('click'); expect(w.findAll('.agent-conversation')).toHaveLength(25)
    await w.get('[aria-label=打开对话搜索]').trigger('click')
    await w.get('[aria-label=搜索对话]').setValue('对话 24'); expect(w.findAll('.agent-conversation')).toHaveLength(1)
    expect(w.get('.agent-conversation time').classes()).toContain('sr-only')
    w.unmount()
  })
  it('confirms batch deletion and keeps the selected rows when the server rejects it', async () => {
    vi.mocked(agentAPI.manage).mockRejectedValue(new Error('仍有任务执行中'))
    const w = mount(AgentHistory, { props: { items } })
    await w.get('[aria-label=多选对话]').trigger('click'); await w.get('.bulk-actions input').setValue(true)
    await w.findAll('.bulk-actions button')[1]!.trigger('click')
    expect(agentAPI.manage).not.toHaveBeenCalled()
    expect(w.get('[role=alertdialog]').attributes('open')).toBeDefined()
    await w.get('.delete-confirm').trigger('click'); await flushPromises()
    expect(agentAPI.manage).toHaveBeenCalledWith(['one', 'two'], 'delete', '')
    expect(w.get('[role=alertdialog]').text()).toContain('仍有任务执行中')
    expect(w.get('.bulk-actions span').text()).toBe('2 项'); w.unmount()
  })
  it('renames through a dialog and retains the proposed name on failure', async () => {
    vi.mocked(agentAPI.manage).mockRejectedValueOnce(new Error('网络中断')).mockResolvedValueOnce(items)
    const w = mount(AgentHistory, { props: { items } })
    await w.findAll('.menu-actions button')[0]!.trigger('click')
    await w.get('[aria-label=对话名称]').setValue('新的照片主题')
    await w.get('.history-dialog form').trigger('submit'); await flushPromises()
    expect(w.get('.history-dialog[open]').text()).toContain('网络中断')
    await w.get('.history-dialog form').trigger('submit'); await flushPromises()
    expect(agentAPI.manage).toHaveBeenLastCalledWith(['one'], 'rename', '新的照片主题')
    expect(w.find('.history-dialog[open]').exists()).toBe(false); w.unmount()
  })
  it('opens a native image preview with an original download and closes cleanly', async () => {
    const w = mount(AgentImagePreview, { props: { url: 'blob:local-image', filename: 'result.png' } })
    await flushPromises(); expect(w.get('dialog').attributes('open')).toBeDefined()
    expect(w.get('img').attributes('src')).toBe('blob:local-image')
    expect(w.get('a').attributes('download')).toBe('result.png')
    await w.get('[aria-label=关闭图片预览]').trigger('click'); expect(w.emitted('close')).toHaveLength(1)
    await w.setProps({ url: undefined }); await flushPromises(); expect(w.get('dialog').attributes('open')).toBeUndefined(); w.unmount()
  })
  it('folds the whole process by default and retains updates while folded', async () => {
    const step = { id: 'thought', kind: 'reasoning' as const, name: 'model', status: 'running', started_at: '2026-10-19T00:00:00Z', summary: '正在分析需求' }
    const w = mount(AgentTrace, { props: { steps: [step] } })
    expect(w.get('details').attributes('open')).toBeUndefined()
    await w.setProps({ steps: [{ ...step, summary: '已确认主题，需要询问分辨率。' }] })
    expect(w.get('details').attributes('open')).toBeUndefined()
    expect(w.get('.reasoning-summary').text()).toContain('需要询问分辨率')
    w.unmount()
  })
})
