import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentExecution from '../AgentExecution.vue'
import AgentChat from '../AgentChat.vue'
import type { AgentMessage, AgentTraceStep } from '@/api/agent'
const model: AgentTraceStep = { id: 'm1', kind: 'reasoning', name: 'gpt-6.1-sol', status: 'completed', started_at: '2026-10-20T00:00:00Z', ended_at: '2026-10-20T00:00:04Z', summary: '已理解用户的主题', commentary: '先确认主题，再开始设计。' }
const ask: AgentTraceStep = { id: 'q1', kind: 'tool', name: 'ask_user', status: 'completed', started_at: '2026-10-20T00:00:04Z', ended_at: '2026-10-20T00:00:14Z', summary: '主题是什么？' }
const resume: AgentTraceStep = { ...model, id: 'm2', started_at: '2026-10-20T00:00:14Z', ended_at: '2026-10-20T00:00:17Z', commentary: '需求已确认，正在调用生图工具。', summary: '使用用户确认的内容。' }
const tool: AgentTraceStep = { id: 'image', kind: 'tool', name: 'generate_image', status: 'running', started_at: '2026-10-20T00:00:17Z', arguments: { prompt: '蓝色纸船', model: 'image-model', image_size: '1K', aspect_ratio: '16:9', reference_count: 1 } }
const answer: AgentMessage = { id: 'answer', kind: 'question_answer', question_id: 'q1', role: 'user', text: '主题：纸船', answers: [{ label: '主题', value: '蓝色纸船' }], run_id: 'r1' }
async function openProcess(w: ReturnType<typeof mount>) { const process = w.get('details.agent-execution'); (process.element as HTMLDetailsElement).open = true; await process.trigger('toggle'); await flushPromises() }
afterEach(() => vi.useRealTimers())
describe('Agent staged execution states', () => {
  it('folds ended phases while keeping current tools, replies and user input in order', async () => {
    const w = mount(AgentExecution, { props: { steps: [model, ask, resume, tool], status: 'executing_tool', answers: [answer] } })
    const phases = w.findAll('.execution-phase')
    expect(phases).toHaveLength(2)
    expect(phases[0]!.get('.tool-stage').attributes('open')).toBeUndefined()
    expect(phases[1]!.get('.tool-stage').attributes('open')).toBeDefined()
    expect(phases[0]!.get('.stage-commentary').text()).toContain('先确认主题')
    expect(phases[0]!.get('.stage-answer dd').text()).toBe('蓝色纸船')
    expect(phases[1]!.get('.stage-commentary').text()).toContain('需求已确认')
    expect(w.get('.current-tool').text()).toContain('正在调用 · 生成图片')
    expect(w.find('.tool-prompt').exists()).toBe(false)
    await phases[1]!.get('.step-toggle').trigger('click')
    expect(w.get('.tool-prompt').text()).toBe('蓝色纸船'); w.unmount()
  })
  it('shows completed and active tools together with actual progress', () => {
    const w = mount(AgentExecution, { props: { steps: [{ ...tool, id: 'done', status: 'completed', ended_at: '2026-10-20T00:00:19Z' }, tool], status: 'executing_tool' } })
    expect(w.get('.stage-heading').text()).toContain('生成图片 × 2')
    expect(w.get('.stage-counts').text()).toContain('已完成 1/2')
    expect(w.find('.tool-check').exists()).toBe(true); expect(w.find('.tool-spinner').exists()).toBe(true); w.unmount()
  })
  it('folds all process details after completion and leaves the final reply outside', async () => {
    const message: AgentMessage = { id: 'final', role: 'assistant', text: '最终回复，只显示一次。', run_id: 'r1', run_status: 'completed', trace: [model, ask, resume, { ...tool, status: 'completed', ended_at: '2026-10-20T00:00:19Z' }] }
    const w = mount(AgentChat, { props: { messages: [answer, message] } })
    expect(w.get('details.agent-execution').attributes('open')).toBeUndefined()
    expect(w.find('.execution-timeline').exists()).toBe(false)
    expect(w.find('.answer-confirmation').exists()).toBe(false)
    expect(w.get('.message-markdown').text()).toContain('最终回复')
    await openProcess(w)
    expect(w.findAll('.stage-answer')).toHaveLength(1)
    expect(w.findAll('.reasoning-summary')).toHaveLength(2)
    expect(w.get('.agent-execution').text()).not.toContain('最终回复')
    expect(w.get('.message-markdown').text()).toBe('最终回复，只显示一次。'); w.unmount()
  })
  it('does not create a thinking section or placeholder when the model provides none', async () => {
    const w = mount(AgentExecution, { props: { steps: [{ ...model, summary: '', commentary: '' }, { ...tool, status: 'completed', ended_at: '2026-10-20T00:00:19Z' }], status: 'completed' } })
    await openProcess(w)
    expect(w.find('.thought-disclosure').exists()).toBe(false); expect(w.find('.reasoning-summary').exists()).toBe(false)
    expect(w.text()).not.toContain('未返回'); expect(w.find('.current-tool').exists()).toBe(false); w.unmount()
  })
  it('streams public stage commentary once and folds automatically when the run ends', async () => {
    const active = { ...model, status: 'running', ended_at: undefined, summary: '公开摘要', commentary: '正在输出阶段文字' }
    const w = mount(AgentChat, { props: { messages: [{ id: 'live', role: 'assistant', text: active.commentary, run_id: 'r1', run_status: 'running', trace: [active] }] } })
    expect(w.findAll('.stage-commentary')).toHaveLength(1); expect(w.find('.message-markdown').exists()).toBe(false)
    await w.setProps({ messages: [{ id: 'live', role: 'assistant', text: '最终回答', run_id: 'r1', run_status: 'completed', trace: [{ ...active, status: 'completed', ended_at: '2026-10-20T00:00:19Z', commentary: '' }] }] })
    expect(w.get('details.agent-execution').attributes('open')).toBeUndefined(); expect(w.get('.message-markdown').text()).toBe('最终回答'); w.unmount()
  })
  it('keeps user manual folding during stream updates', async () => {
    const w = mount(AgentExecution, { props: { steps: [model, tool], status: 'executing_tool' } })
    const group = w.get('.tool-stage'); (group.element as HTMLDetailsElement).open = false; await group.trigger('toggle')
    await w.setProps({ steps: [{ ...model, commentary: '更新阶段说明' }, tool] })
    expect((group.element as HTMLDetailsElement).open).toBe(false); w.unmount()
  })
  it('sanitizes summaries and stage commentary and renders user values as text', () => {
    const w = mount(AgentExecution, { props: { steps: [{ ...model, summary: '<script>bad()</script><strong>摘要</strong>', commentary: '<img src="https://tracker.test/a"><strong>阶段</strong>' }, ask, resume, tool], status: 'executing_tool', answers: [{ ...answer, answers: [{ label: '内容', value: '<img src=x onerror=bad()>' }] }] } })
    expect(w.find('script').exists()).toBe(false); expect(w.find('img').exists()).toBe(false)
    expect(w.get('.stage-answer dd').text()).toContain('<img'); w.unmount()
  })
  it('stops live timers at completion and excludes waiting for user input', async () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-20T00:00:20Z'))
    const w = mount(AgentExecution, { props: { steps: [model, ask, resume, tool], status: 'executing_tool', answers: [answer] } })
    expect(w.get('.current-tool').text()).toContain('3 秒')
    await vi.advanceTimersByTimeAsync(2000); expect(w.get('.current-tool').text()).toContain('5 秒')
    await w.setProps({ status: 'completed', steps: [model, ask, resume, { ...tool, status: 'completed', ended_at: '2026-10-20T00:00:22Z' }] })
    expect(w.get('.execution-heading').text()).toContain('12 秒')
    await vi.advanceTimersByTimeAsync(60000); expect(w.get('.execution-heading').text()).toContain('12 秒'); w.unmount()
  })
})
