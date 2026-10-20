import { beforeEach, describe, expect, it, vi } from 'vitest'
vi.mock('../client', () => ({ apiClient: { get: vi.fn(), post: vi.fn() }, buildApiUrl: (path: string) => '/api/v1' + path }))
import { apiClient } from '../client'
import { agentAPI, parseAgentEvent, terminal, watchAgentRun, type AgentRun } from '../agent'
describe('Agent persistent events', () => {
  beforeEach(() => vi.clearAllMocks())
  it('submits and retries without budget parameters while retaining idempotency', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({ data: {} })
    await agentAPI.submit('conversation', 'hello', [], 2, 'message-key')
    expect(apiClient.post).toHaveBeenLastCalledWith('/agent/conversations/conversation/messages', { content: 'hello', attachment_asset_ids: [], config_version: 2 }, { headers: { 'Idempotency-Key': 'message-key' } })
    await agentAPI.retry('run', 2, 'retry-key')
    expect(apiClient.post).toHaveBeenLastCalledWith('/agent/runs/run/retry', { config_version: 2 }, { headers: { 'Idempotency-Key': 'retry-key' } })
  })
  it('binds optional item selection to the quoted draft instead of adding it during submit', async () => {
    vi.mocked(apiClient.post).mockResolvedValue({ data: {} })
    const draft = { title: '方案', brief: '', style: '', image_size: '1K', aspect_ratio: '16:9', shared_reference_asset_ids: [], items: [] }
    await agentAPI.reviewWork('plan', 3, draft, ['p2'])
    expect(apiClient.post).toHaveBeenLastCalledWith('/agent/work-plans/plan/review', { revision: 3, draft, item_ids: ['p2'] })
    await agentAPI.submitWork('plan', 4)
    expect(apiClient.post).toHaveBeenLastCalledWith('/agent/work-plans/plan/submit', { revision: 4 })
  })
  it('ignores heartbeat and decodes event payload', () => {
    expect(parseAgentEvent(': heartbeat')).toBeNull()
    expect(parseAgentEvent('id: 2\r\nevent: asset.created\r\ndata: {"id":"asset1"}')).toEqual({ sequence: 2, type: 'asset.created', data: { id: 'asset1' } })
  })
  it('keeps reconciliation active and treats partial success as terminal', () => {
    expect(terminal({ status: 'reconciling' } as AgentRun)).toBe(false)
    expect(terminal({ status: 'partial_success' } as AgentRun)).toBe(true)
  })
  it('bounds read requests and forwards cancellation for conversation switching', async () => {
    const controller = new AbortController()
    vi.mocked(apiClient.get).mockResolvedValue({ data: {} })
    await agentAPI.detail('conversation/id', controller.signal)
    expect(apiClient.get).toHaveBeenCalledWith('/agent/conversations/conversation%2Fid', { signal: controller.signal, timeout: 15000, params: { _t: expect.any(Number) }, headers: { 'Cache-Control': 'no-cache' } })
    await agentAPI.run('run/id', controller.signal)
    expect(apiClient.get).toHaveBeenCalledWith('/agent/runs/run%2Fid', { signal: controller.signal, timeout: 15000, params: { _t: expect.any(Number) }, headers: { 'Cache-Control': 'no-cache' } })
  })
  it('handles split UTF-8, CRLF and replayed event IDs', async () => {
    const raw = new TextEncoder().encode('id: 1\nevent: run.started\ndata: {"text":"开始"}\n\nid: 1\nevent: run.started\ndata: {}\n\nid: 2\r\nevent: run.completed\r\ndata: {"text":"完成"}\r\n\r\n')
    const stream = new ReadableStream({ start(controller) { for (let i = 0; i < raw.length; i += 3) controller.enqueue(raw.slice(i, i + 3)); controller.close() } })
    const fetcher = vi.fn().mockResolvedValue({ ok: true, body: stream }); vi.stubGlobal('fetch', fetcher)
    localStorage.setItem('auth_token', 'test-jwt')
    const events: number[] = []
    await watchAgentRun('run1', 0, new AbortController().signal, event => events.push(event.sequence))
    expect(events).toEqual([1, 2]); expect(fetcher.mock.calls[0][1].headers.Authorization).toBe('Bearer test-jwt')
    vi.unstubAllGlobals(); localStorage.removeItem('auth_token')
  })
  it('loads per-model reasoning capabilities with cancellation and bounded timeout', async () => {
    const controller = new AbortController()
    const data = { models: ['gpt-6.1-sol'], model_capabilities: [{ id: 'gpt-6.1-sol', reasoning_levels: ['low', 'medium', 'high', 'xhigh', 'max'], default_reasoning_effort: 'low' }] }
    vi.mocked(apiClient.get).mockResolvedValue({ data })
    expect(await agentAPI.capabilities(7, 'text', controller.signal)).toEqual(data)
    expect(apiClient.get).toHaveBeenCalledWith('/agent/capabilities', { params: { api_key_id: 7, purpose: 'text' }, signal: controller.signal, timeout: 15000 })
  })

  it('loads only the authenticated user Agent usage with cancellation and no cached response', async () => {
    const controller = new AbortController(); vi.mocked(apiClient.get).mockResolvedValue({ data: { scope: 'all_agent_history' } })
    await agentAPI.usage(controller.signal)
    expect(apiClient.get).toHaveBeenCalledWith('/agent/usage', { signal: controller.signal, timeout: 15000, params: { _t: expect.any(Number) }, headers: { 'Cache-Control': 'no-cache' } })
  })

  it('forwards cancellation and file content when uploading right-pane reference material', async () => {
    const controller = new AbortController(), file = new File(['image'], 'reference.png', { type: 'image/png' })
    vi.mocked(apiClient.post).mockResolvedValue({ data: { id: 'material' } })
    await agentAPI.upload('conversation/id', file, controller.signal)
    expect(apiClient.post).toHaveBeenCalledWith('/agent/conversations/conversation%2Fid/assets', expect.any(FormData), { headers: { 'Content-Type': 'multipart/form-data' }, signal: controller.signal })
    expect((vi.mocked(apiClient.post).mock.calls[0]![1] as FormData).get('file')).toBe(file)
  })

})
