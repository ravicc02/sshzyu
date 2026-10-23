import { describe, expect, it, vi } from 'vitest'
import { isSafeAgentDiagram, renderAgentDiagram } from '../agentDiagram'
const { render, initialize } = vi.hoisted(() => ({ render: vi.fn(), initialize: vi.fn() }))
vi.mock('mermaid', () => ({ default: { initialize, render } }))
describe('Agent Mermaid boundary', () => {
  it('accepts ordinary diagrams but rejects external resources, directives and clicks', () => {
    expect(isSafeAgentDiagram('flowchart LR\nA[开始] --> B[完成]')).toBe(true)
    for (const source of ['%%{init: {"securityLevel":"loose"}}%%\ngraph LR\nA-->B', 'flowchart LR\nA-->B; click A "javascript:alert(1)"', 'graph LR\nA@{img:"https://tracker.test/a"}', '---\nconfig:\n securityLevel: loose\n---\ngraph LR\nA-->B', 'graph LR\nstyle A fill:url(https://tracker.test)', 'x'.repeat(10001)]) expect(isSafeAgentDiagram(source)).toBe(false)
  })
  it('sanitizes generated SVG and does not bind interactive functions', async () => {
    const bindFunctions = vi.fn()
    render.mockResolvedValueOnce({ svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 300"><style>.x{fill:url("#local")}</style><rect class="x" width="10" height="10"/><script>alert(1)</script><a href="https://tracker.test"><text>bad</text></a><image href="https://tracker.test/img"/><foreignObject>bad</foreignObject></svg>', bindFunctions })
    const svg = await renderAgentDiagram('graph LR\nA-->B', false)
    expect(initialize).toHaveBeenLastCalledWith(expect.objectContaining({ htmlLabels: false, securityLevel: 'strict' })); expect(svg).toContain('<rect'); expect(svg).toContain('min-width: 720px'); expect(svg).toContain('width="800"'); expect(svg).not.toMatch(/<script|<image|<a\s|foreignObject|https:\/\/tracker/)
    expect(bindFunctions).not.toHaveBeenCalled()
  })
  it('bounds the rendered canvas before inserting oversized diagrams', async () => {
    render.mockResolvedValueOnce({ svg: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8000 16000"><text>large</text></svg>' })
    await expect(renderAgentDiagram('graph LR\nA-->B', false)).rejects.toThrow('Diagram layout too large')
  })
  it('rejects external CSS resources and recovers the queue after a rendering error', async () => {
    render.mockResolvedValueOnce({ svg: '<svg xmlns="http://www.w3.org/2000/svg"><style>.x{fill:url(https://tracker.test/x)}</style></svg>' })
    await expect(renderAgentDiagram('graph LR\nA-->B', false)).rejects.toThrow('External diagram resource')
    render.mockResolvedValueOnce({ svg: '<svg xmlns="http://www.w3.org/2000/svg"><text>safe</text></svg>' })
    expect(await renderAgentDiagram('graph LR\nA-->B', true)).toContain('safe')
  })
})
