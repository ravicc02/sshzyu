import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentChat from '../AgentChat.vue'
afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })
describe('Agent message presentation', () => {
  it('presents a confirmed answer as a folded record instead of another user request', () => {
    const w = mount(AgentChat, { props: { messages: [
      { id: 'user', role: 'user', text: '帮我分析这个想法' },
      { id: 'answer', kind: 'question_answer', role: 'user', text: '主题：学习', answers: [{ label: '主题', value: '学习' }] },
      { id: 'reply', role: 'assistant', text: '继续分析。' },
    ] } })
    expect(w.findAll('.from-user')).toHaveLength(1)
    expect(w.get('.answer-confirmation summary').text()).toBe('已确认 1 项')
    expect(w.get('.answer-confirmation').attributes('open')).toBeUndefined()
    expect(w.get('.answer-confirmation dd').text()).toBe('学习')
    expect(w.findAll('.user-content')).toHaveLength(1); w.unmount()
  })
  it('keeps a legacy clarification record folded without deleting its content', () => {
    const w = mount(AgentChat, { props: { messages: [{ id: 'old', kind: 'question_answer', role: 'user', text: '之前的问题\n主题：学习' }] } })
    expect(w.find('.user-content').exists()).toBe(false)
    expect(w.get('.answer-confirmation summary').text()).toBe('已确认回答')
    expect(w.get('.legacy-answer').text()).toContain('主题：学习'); w.unmount()
  })
  it('copies through an icon button with accessible success and failure feedback', async () => {
    const writeText = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('denied'))
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    vi.useFakeTimers()
    const w = mount(AgentChat, { props: { messages: [{ id: 'reply', role: 'assistant', text: '答案' }] } })
    expect(w.get('.copy-message').text()).toBe(''); expect(w.get('.copy-message svg').exists()).toBe(true)
    await w.get('[aria-label=复制回复]').trigger('click'); await flushPromises()
    expect(writeText).toHaveBeenCalledWith('答案'); expect(w.get('[aria-label=已复制]').text()).toBe('')
    await vi.advanceTimersByTimeAsync(2100)
    await w.get('[aria-label=复制回复]').trigger('click'); await flushPromises()
    expect(w.get('.copy-message').attributes('aria-label')).toContain('复制失败'); expect(w.get('.copy-message').text()).toBe(''); w.unmount()
  })
  it('shows real reply metrics and the retry icon beside copy', async () => {
    const message = { id: 'a', run_id: 'run-a', role: 'assistant', text: '答案', created_at: '2026-10-19T08:13:00Z', metrics: { elapsed_ms: 19000, total_tokens: 88000, input_tokens: 84593, output_tokens: 3407, cache_tokens: 0, generation_ms: 10000 } }
    const w = mount(AgentChat, { props: { messages: [message], retryRunId: 'run-a' } })
    expect(w.get('.reply-metrics').text()).toContain('19s')
    expect(w.get('.reply-metrics').text()).toContain('88.0k tokens')
    expect(w.get('.reply-metrics').text()).toContain('340.7 tok/s')
    expect(w.get('.reply-time').text()).toMatch(/\d{2}:\d{2}/)
    expect(w.get('.reply-actions').findAll('button').map(b => b.attributes('aria-label'))).toEqual(['复制回复', '重试回复'])
    await w.get('[aria-label=重试回复]').trigger('click')
    expect(w.emitted('retry')?.[0]).toEqual([message])
    await w.setProps({ retryDisabled: true }); await w.get('[aria-label=重试回复]').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
    w.unmount()
  })

  it('separates the assistant label from the model name with a visible delimiter', () => {
    const w=mount(AgentChat,{props:{messages:[{id:'reply',role:'assistant',text:'你好',trace:[{id:'step',kind:'reasoning',name:'deepseek-v4.1-flash',status:'completed',started_at:'2026-10-19T00:00:00Z'}]}]}})
    expect(w.get('.message-author').text()).toBe('Agent')
    expect(w.get('.message-role-separator').text()).toBe('·')
    expect(w.get('.message-model').text()).toBe('deepseek-v4.1-flash')
    expect(w.get('.message-model').attributes('title')).toBe('deepseek-v4.1-flash')
    w.unmount()
  })
  it('keeps the reply element when a streaming message becomes a saved message', async () => {
    const w=mount(AgentChat,{props:{messages:[{id:'run-a',run_id:'run-a',role:'assistant',text:'流式内容'}]}})
    const article=w.get('.from-assistant').element
    await w.setProps({messages:[{id:'msg-run-a',run_id:'run-a',role:'assistant',text:'完整内容'}]})
    expect(w.get('.from-assistant').element).toBe(article)
    expect(w.get('.message-markdown').text()).toBe('完整内容')
    w.unmount()
  })
  it('does not invent token counts, speed or timestamps for old records', () => {
    const w = mount(AgentChat, { props: { messages: [{ id: 'a', role: 'assistant', text: '旧回复' }] } })
    expect(w.find('.reply-metrics').exists()).toBe(false)
    expect(w.find('.reply-time').exists()).toBe(false)
    w.unmount()
  })

  it('suppresses stale image promises on a failed tool and offers saved-result recovery', async () => {
    const message = { id: 'failed', role: 'assistant', text: '现在开始生成。', run_id: 'r-failed', run_status: 'failed', error_code: 'AGENT_RECONCILIATION_TIMEOUT', can_recover_image: true, trace: [{ id: 'tool', kind: 'tool' as const, name: 'generate_image', status: 'failed', started_at: '2026-10-20T01:00:00Z' }] }
    const w = mount(AgentChat, { props: { messages: [message] } })
    expect(w.find('.message-markdown').exists()).toBe(false)
    expect(w.get('.copy-message').attributes('disabled')).toBeDefined()
    await w.get('.recover-message').trigger('click'); expect(w.emitted('recover')?.[0]).toEqual([message])
    await w.setProps({ recoverDisabled: true }); expect(w.get('.recover-message').attributes('disabled')).toBeDefined(); w.unmount()
  })

})
