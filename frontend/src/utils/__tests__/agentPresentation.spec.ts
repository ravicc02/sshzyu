import { describe, expect, it } from 'vitest'
import { agentState, agentStates, agentError, renderAgentMarkdown } from '../agentPresentation'
describe('Agent presentation', () => {
  it('gives every backend run state an explicit label', () => { expect(Object.keys(agentStates)).toHaveLength(12); for (const status of ['queued','running','waiting_user','waiting_tool','executing_tool','resuming_model','reconciling','completed','partial_success','failed','expired','cancelled']) expect(agentState(status).label).not.toBe('正在处理') })
  it('does not suggest retrying an uncertain paid request', () => { expect(agentState('reconciling').detail).toContain('不会自动重发'); expect(agentState('reconciling').tone).toBe('warning') })
  it('distinguishes a terminal reconciliation timeout from an active unknown result', () => {
    expect(agentState('failed', 'AGENT_RECONCILIATION_TIMEOUT').label).toBe('结果核对超时')
    expect(agentState('partial_success', 'AGENT_RECONCILIATION_TIMEOUT').label).toContain('部分完成')
    expect(agentState('failed', 'AGENT_RECONCILIATION_TIMEOUT').detail).toContain('执行名额已释放')
    expect(agentError('AGENT_RECONCILIATION_TIMEOUT')).toContain('费用以最终账单为准')
  })
  it('explains unsupported image models and oversized image inputs', () => { expect(agentError('AGENT_MODEL_VISION_UNSUPPORTED')).toContain('切换到支持识图'); expect(agentError('AGENT_VISION_IMAGE_TOO_LARGE')).toContain('缩小或裁剪') })
  it('renders rich text but strips model-controlled media, scripts and event handlers', () => { const html = renderAgentMarkdown('**你好**\n\n<script>alert(1)</script><img src="https://tracker.test/a" onerror="alert(1)"><a href="javascript:alert(1)" onclick="alert(1)">x</a><iframe src="https://tracker.test"></iframe>'); expect(html).toContain('<strong>你好</strong>'); expect(html).not.toMatch(/<script|<img|<iframe|javascript:|onclick|onerror/) })
  it('retains code, tables and safe external links', () => { const html = renderAgentMarkdown('```js\nconst x = 1\n```\n\n[文档](https://example.com)\n\n| a | b |\n|---|---|\n| 1 | 2 |'); expect(html).toContain('<pre>'); expect(html).toContain('<table>'); expect(html).toContain('href="https://example.com"') })
})
