import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgentMarkdown from '../AgentMarkdown.vue'
import AgentBlockCopy from '../AgentBlockCopy.vue'
import AgentCodeBlock from '../AgentCodeBlock.vue'
import AgentMath from '../AgentMath.vue'
import { parseAgentMarkdown } from '@/utils/agentMarkdown'
import { highlightAgentCode } from '@/utils/agentCodeHighlight'

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })
function clipboard() { const writeText = vi.fn().mockResolvedValue(undefined); Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } }); return writeText }

describe('Agent rich Markdown blocks', () => {
  it('copies only the selected code, without the fence, label or surrounding reply', async () => {
    const writeText = clipboard()
    const source = '说明文字\n\n```typescript\nconst count: number = 2\nconsole.log(count)\n```\n\n结尾文字'
    const w = mount(AgentMarkdown, { props: { text: source } })
    expect(w.get('.content-block-label').text()).toBe('TYPESCRIPT')
    await w.get('[aria-label="复制代码块"]').trigger('click'); await flushPromises()
    expect(writeText).toHaveBeenCalledWith('const count: number = 2\nconsole.log(count)')
    expect(writeText).not.toHaveBeenCalledWith(source)
    expect(w.get('.agent-code-block pre').attributes('tabindex')).toBe('0')
    w.unmount()
  })
  it('copies each prose, prompt, table and list as an independent block', async () => {
    const writeText = clipboard()
    const w = mount(AgentMarkdown, { props: { text: '第一段 **内容**。\n\n第二段。\n\n```prompt\n电影感，暖色，保留主体\n```\n\n- 需求一\n- 需求二\n\n| 参数 | 值 |\n|---|---|\n| 比例 | 16:9 |' } })
    await w.findAll('[aria-label="复制此段内容"]')[1]!.trigger('click'); await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('第二段。')
    await w.get('[aria-label="复制文本块"]').trigger('click'); await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('电影感，暖色，保留主体')
    await w.get('[aria-label="复制列表内容"]').trigger('click'); await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('- 需求一\n- 需求二')
    await w.get('[aria-label="复制表格内容"]').trigger('click'); await flushPromises()
    expect(writeText.mock.lastCall?.[0]).toContain('| 参数 | 值 |')
    w.unmount()
  })
  it('retains copyable code nested inside a list or quote', async () => {
    const writeText = clipboard()
    const w = mount(AgentMarkdown, { props: { text: '> 使用以下命令：\n>\n> ```bash\n> echo "hello"\n> ```\n\n- 步骤\n\n  ```js\n  const x = 1\n  ```' } })
    expect(w.findAll('.agent-code-block')).toHaveLength(2)
    await w.findAll('[aria-label="复制代码块"]')[1]!.trigger('click'); await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('const x = 1'); w.unmount()
  })
  it('renders real highlighting but keeps unknown languages readable and escaped', () => {
    expect(highlightAgentCode('const x = "hello"', 'javascript')).toContain('hljs-keyword')
    expect(highlightAgentCode('<script>alert(1)</script>', 'xml')).not.toContain('<script>')
    expect(highlightAgentCode('anything', 'unknown-language')).toBe('')
    const w = mount(AgentCodeBlock, { props: { source: '<img src=x onerror=alert(1)>', language: 'unknown-language' } })
    expect(w.get('code').text()).toBe('<img src=x onerror=alert(1)>'); expect(w.find('img').exists()).toBe(false); w.unmount()
  })
  it('updates streamed code and still copies the latest exact source', async () => {
    const writeText = clipboard(); vi.useFakeTimers()
    const w = mount(AgentCodeBlock, { props: { source: 'const x =', language: 'js' } })
    await w.setProps({ source: 'const x = 42' })
    await vi.advanceTimersByTimeAsync(100); await flushPromises()
    await w.get('[aria-label="复制代码块"]').trigger('click'); await flushPromises()
    expect(writeText).toHaveBeenLastCalledWith('const x = 42'); w.unmount()
  })
  it('supports inline/block math, fenced LaTeX and escaped math delimiters', async () => {
    const w = mount(AgentMarkdown, { props: { text: '公式 $x^2 + y^2$表示平方和，与 \\(a+b\\)。\n\n$$\nE=mc^2\n$$\n\n\\[x+y\\]\n\n```latex\n\\frac{1}{2}\n```' } })
    await flushPromises()
    expect(w.findAll('.agent-math-inline')).toHaveLength(2)
    expect(w.findAll('.agent-math-block')).toHaveLength(3)
    await vi.waitFor(() => expect(w.find('.katex').exists()).toBe(true), { timeout: 3000 })
    expect(w.find('math').exists()).toBe(true); w.unmount()
  })
  it('does not reinterpret money, escaped dollars or code as mathematical formulas', () => {
    const w = mount(AgentMarkdown, { props: { text: '价格 $20 and $30，折合 50 元。\\$100\n\n`$name$`\n\n```js\nconst price = "$name$"\n```' } })
    expect(w.findAll('.agent-math-inline')).toHaveLength(0)
    expect(w.findAll('.agent-math-block')).toHaveLength(0)
    expect(w.text()).toContain('$20 and $30'); w.unmount()
  })
  it('keeps malformed or unsafe formulas as readable source, with no active link', async () => {
    const w = mount(AgentMath, { props: { source: '\\href{javascript:alert(1)}{click}', display: true } })
    await flushPromises(); expect(w.find('a').exists()).toBe(false)
    expect(w.get('[aria-label="复制公式源码"]').exists()).toBe(true)
    await w.setProps({ source: '\\frac{' }); await flushPromises()
    expect(w.text()).toContain('\\frac{'); w.unmount()
  })
  it('sanitizes raw HTML, dangerous links and forged copy controls without fetching media', () => {
    const w = mount(AgentMarkdown, { props: { text: '<script>alert(1)</script>\n<img src="https://tracker.test/x" onerror="alert(1)">\n<button class="block-copy" onclick="alert(1)" data-copy="secret">forged</button>\n\n[x](javascript:alert(1))\n\n![track](https://tracker.test/image)' } })
    expect(w.find('script, img, iframe, [onclick], [onerror], [data-copy]').exists()).toBe(false)
    expect(w.find('a[href^="javascript:"]').exists()).toBe(false)
    expect(w.html()).not.toContain('https://tracker.test/image'); w.unmount()
  })
  it('handles partial streaming delimiters and keeps rich text semantics', () => {
    expect(() => parseAgentMarkdown('正在输出 $x +\n\n```mermaid\nflowchart LR\nA -->')).not.toThrow()
    const w = mount(AgentMarkdown, { props: { text: '# 标题\n\n**粗体**、*斜体*、~~删除~~、[链接](https://example.com)\n\n- [x] 已完成\n- [ ] 待完成\n\n---' } })
    expect(w.find('h1').text()).toBe('标题'); expect(w.find('strong').text()).toBe('粗体')
    expect(w.find('em').exists()).toBe(true); expect(w.find('del').exists()).toBe(true)
    expect(w.get('a').attributes('href')).toBe('https://example.com'); expect(w.find('hr').exists()).toBe(true)
    expect(w.findAll('.task-mark')).toHaveLength(2); w.unmount()
  })
})

describe('Block copy feedback', () => {
  it('provides pending, success, reset, failure/retry and disabled states', async () => {
    vi.useFakeTimers()
    let finish: (() => void) | undefined
    const writeText = vi.fn().mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve })).mockRejectedValueOnce(new Error('denied')).mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const w = mount(AgentBlockCopy, { props: { text: 'only this block' } })
    await w.get('button').trigger('click'); expect(w.get('button').attributes('disabled')).toBeDefined()
    finish?.(); await flushPromises(); expect(w.get('button').attributes('data-state')).toBe('success')
    await vi.advanceTimersByTimeAsync(2100); expect(w.get('button').attributes('data-state')).toBe('default')
    await w.get('button').trigger('click'); await flushPromises(); expect(w.get('button').attributes('data-state')).toBe('error')
    await w.get('button').trigger('click'); await flushPromises(); expect(w.get('button').attributes('data-state')).toBe('success')
    await w.setProps({ disabled: true }); expect(w.get('button').attributes('disabled')).toBeDefined(); w.unmount()
  })
})
