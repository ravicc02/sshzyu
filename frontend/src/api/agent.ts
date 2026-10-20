import { apiClient, buildApiUrl } from './client'
import type { AgentReasoningEffort } from '@/utils/agentReasoning'
export { agentReasoningEfforts, type AgentReasoningEffort } from '@/utils/agentReasoning'
export interface AgentModelCapability { id: string; reasoning_levels: AgentReasoningEffort[]; default_reasoning_effort: AgentReasoningEffort | '' }
export interface AgentCapabilities { models: string[]; model_capabilities?: AgentModelCapability[] }
export interface AgentWorkItem { id: string; title: string; content: string; prompt: string; reference_asset_ids: string[] }
export interface AgentWorkDraft { title: string; brief: string; style: string; image_size: string; aspect_ratio: string; shared_reference_asset_ids: string[]; items: AgentWorkItem[] }
export interface AgentWorkResult { item_id: string; status: string; asset_id?: string; error_code?: string }
export interface AgentWorkPlan extends AgentWorkDraft { target_item_ids?: string[]; inherited_results?: AgentWorkResult[]; id: string; conversation_id: string; source_run_id: string; revision: number; status: 'draft' | 'ready' | 'submitted' | 'superseded'; config_version: number; image_api_key_id: number; image_model: string; max_cost_usd: string; item_max_cost_usd: string[]; quoted_at?: string; run_id?: string; results?: AgentWorkResult[]; execution_status?: string }
export interface AgentConfig { model_api_key_id: number; model: string; image_api_key_id?: number; image_model?: string; reasoning_effort?: AgentReasoningEffort }
export interface AgentKey { id: number; name: string; group_name: string; purpose: string; platform?: string; disabled_reason?: string }
export interface AgentConversation { id: string; title: string; config: AgentConfig; config_version: number; updated_at: string; archived_at?: string }
export interface AgentTraceArguments { prompt: string; model: string; image_size: string; aspect_ratio: string; reference_count: number }
export interface AgentTraceStep { commentary?: string; id: string; kind: 'reasoning' | 'tool'; name: string; status: string; started_at: string; ended_at?: string; summary?: string; arguments?: AgentTraceArguments; asset_ids?: string[]; cost_usd?: string; error_code?: string }
export interface AgentReplyMetrics { input_tokens: number; output_tokens: number; cache_tokens: number; total_tokens: number; elapsed_ms?: number; generation_ms?: number }
export interface AgentMessage { run_status?: string; error_code?: string; can_recover_image?: boolean; pending?: boolean; metrics?: AgentReplyMetrics; created_at?: string; kind?: 'question_answer'; question_id?: string; answers?: { label: string; value: string }[]; id: string; role: string; text: string; asset_ids?: string[]; run_id?: string; trace?: AgentTraceStep[] }
export interface AgentQuestion { id: string; question: string; fields: { id: string; label: string; options: string[]; required: boolean }[]; deadline: string }
export interface AgentRun { recovery_only?: boolean; recovered_at?: string; work?: { plan: AgentWorkPlan; results: AgentWorkResult[]; next_item: number }; retry_of?: string; metrics?: AgentReplyMetrics; created_at?: string; question?: AgentQuestion; id: string; conversation_id: string; status: string; text: string; error_code?: string; trace?: AgentTraceStep[]; reconcile_deadline?: string; model_cost: string; image_cost: string; spent_usd: string; asset_ids: string[]; last_event_sequence: number; config: AgentConfig; cancel_requested: boolean }
export interface AgentAsset { status?: string; content_available?: boolean; expires_at?: string; filename?: string; id: string; mime_type: string; width: number; height: number; byte_size: number }
export interface AgentGrant { id: string; plan_id: string; image_api_key_id: number; image_model: string; purpose: 'sample' | 'output'; max_count: number; used_count: number; reserved_image_cost_usd: string; status: string; expires_at: string }
export interface AgentGrantRequest { request_id: string; revision: number; purpose: 'sample' | 'output'; max_count: number; expires_in_minutes: number }
export interface AgentDetail { execution_grants?: AgentGrant[]; work_plans?: AgentWorkPlan[]; conversation: AgentConversation; messages: AgentMessage[]; active_run?: AgentRun }
export interface AgentUsageGroup {
  group_id?: number; group_name?: string; requests: number; input_tokens: number; output_tokens: number; cache_read_tokens: number; cache_creation_tokens: number; total_tokens: number; image_count: number; cost_usd: string; pending_requests: number; missing_usage_records: number; missing_image_records: number
}
export interface AgentUsageSection { groups: AgentUsageGroup[]; totals: AgentUsageGroup }
export interface AgentUsageSummary { scope: 'all_agent_history'; text: AgentUsageSection; image: AgentUsageSection; total_cost_usd: string; updated_at: string }
export interface AgentEvent { sequence: number; type: string; data: unknown }
const base = '/agent'
export const agentAPI = {
  usage: async (signal?: AbortSignal) => (await apiClient.get<AgentUsageSummary>(`${base}/usage`, { signal, timeout: 15000, params: { _t: Date.now() }, headers: { 'Cache-Control': 'no-cache' } })).data,
  saveDraft: async (id: string, revision: number, draft: AgentWorkDraft) => (await apiClient.put<AgentWorkPlan>(`${base}/work-plans/${encodeURIComponent(id)}/draft`, { revision, draft })).data,
  createGrant: async (id: string, request: AgentGrantRequest) => (await apiClient.post<AgentGrant>(`${base}/work-plans/${encodeURIComponent(id)}/grants`, request)).data,
  revokeGrant: async (id: string) => { await apiClient.delete(`${base}/grants/${encodeURIComponent(id)}`) },
  reviewWork: async (id: string, revision: number, draft: AgentWorkDraft, itemIds?: string[]) => (await apiClient.post<AgentWorkPlan>(`${base}/work-plans/${encodeURIComponent(id)}/review`, { revision, draft, ...(itemIds?.length ? { item_ids: itemIds } : {}) })).data,
  submitWork: async (id: string, revision: number) => (await apiClient.post<AgentRun>(`${base}/work-plans/${encodeURIComponent(id)}/submit`, { revision })).data,
  info: async () => (await apiClient.get<{ enabled: boolean; history_available: boolean; conversation_memory?: boolean; asset_thumbnail_supported?: boolean; }>(`${base}/info`)).data,
  keys: async (purpose: 'text' | 'image') => (await apiClient.get<AgentKey[]>(`${base}/key-options`, { params: { purpose } })).data,
  capabilities: async (key: number, purpose: 'text' | 'image', signal?: AbortSignal) => (await apiClient.get<AgentCapabilities>(`${base}/capabilities`, { params: { api_key_id: key, purpose }, signal, timeout: 15000 })).data,
  models: async (key: number, purpose: 'text' | 'image', signal?: AbortSignal) => (await apiClient.get<{ models: string[] }>(`${base}/capabilities`, { params: { api_key_id: key, purpose }, signal, timeout: 15000 })).data.models,
  list: async () => (await apiClient.get<AgentConversation[]>(`${base}/conversations`)).data,
  manage: async (ids: string[], action: 'rename' | 'archive' | 'restore' | 'delete', title = '') => (await apiClient.post<AgentConversation[]>(`${base}/conversations/manage`, { ids, action, title })).data,
  answer: async (id: string, questionId: string, answers: Record<string, string>, assets: string[] = []) => (await apiClient.post<AgentRun>(`${base}/runs/${encodeURIComponent(id)}/answer`, { question_id: questionId, answers, ...(assets.length ? { attachment_asset_ids: assets } : {}) })).data,
  create: async (title: string, config: AgentConfig) => (await apiClient.post<AgentConversation>(`${base}/conversations`, { title, config })).data,
  detail: async (id: string, signal?: AbortSignal) => (await apiClient.get<AgentDetail>(`${base}/conversations/${encodeURIComponent(id)}`, { signal, timeout: 15000, params: { _t: Date.now() }, headers: { 'Cache-Control': 'no-cache' } })).data,
  configure: async (id: string, config: AgentConfig, version: number) => (await apiClient.patch<AgentDetail>(`${base}/conversations/${encodeURIComponent(id)}/config`, { config, config_version: version })).data,
  recoverImage: async (id: string) => (await apiClient.post<AgentRun>(`${base}/runs/${encodeURIComponent(id)}/recover-image`)).data,
  retry: async (id: string, version: number, key: string) => (await apiClient.post<AgentRun>(`${base}/runs/${encodeURIComponent(id)}/retry`, { config_version: version }, { headers: { 'Idempotency-Key': key } })).data,
  submit: async (id: string, content: string, assets: string[], version: number, idempotencyKey: string) => (await apiClient.post<AgentRun>(`${base}/conversations/${encodeURIComponent(id)}/messages`, { content, attachment_asset_ids: assets, config_version: version }, { headers: { 'Idempotency-Key': idempotencyKey } })).data,
  run: async (id: string, signal?: AbortSignal) => (await apiClient.get<AgentRun>(`${base}/runs/${encodeURIComponent(id)}`, { signal, timeout: 15000, params: { _t: Date.now() }, headers: { 'Cache-Control': 'no-cache' } })).data,
  cancel: async (id: string) => (await apiClient.post<AgentRun>(`${base}/runs/${encodeURIComponent(id)}/cancel`)).data,
  upload: async (conversation: string, file: File, signal?: AbortSignal) => { const form = new FormData(); form.append('file', file); return (await apiClient.post<AgentAsset>(`${base}/conversations/${encodeURIComponent(conversation)}/assets`, form, { headers: { 'Content-Type': 'multipart/form-data' }, ...(signal ? { signal } : {}) })).data },
  assetThumbnail: async (id: string) => (await apiClient.get<Blob>(`${base}/assets/${encodeURIComponent(id)}/thumbnail`, { responseType: 'blob' })).data,
  assetMeta: async (id: string) => (await apiClient.get<AgentAsset>(`${base}/assets/${encodeURIComponent(id)}/meta`)).data,
  asset: async (id: string, signal?: AbortSignal) => (await apiClient.get<Blob>(`${base}/assets/${encodeURIComponent(id)}`, { responseType: 'blob', ...(signal ? { signal } : {}) })).data,
}
export function terminal(run?: AgentRun | null): boolean { return !!run && ['completed', 'partial_success', 'failed', 'expired', 'cancelled'].includes(run.status) }
// Parse framing independently of fetch chunks. CRLF and UTF-8 boundaries are
// handled by TextDecoder streaming, not by assuming a network chunk is an event.
export function parseAgentEvent(block: string): AgentEvent | null {
  let id = 0; let type = ''; const data: string[] = []
  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith('id:')) id = Number(line.slice(3).trim())
    if (line.startsWith('event:')) type = line.slice(6).trim()
    if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
  }
  if (!data.length || !Number.isSafeInteger(id) || id <= 0 || !type) return null
  return { sequence: id, type, data: JSON.parse(data.join('\n')) }
}
export async function watchAgentRun(id: string, after: number, signal: AbortSignal, onEvent: (event: AgentEvent) => void): Promise<void> {
  const response = await fetch(buildApiUrl(`${base}/runs/${encodeURIComponent(id)}/events`), { headers: { Authorization: `Bearer ${localStorage.getItem('auth_token') || ''}`, 'Last-Event-ID': String(after) }, signal, cache: 'no-store' })
  if (!response.ok || !response.body) throw new Error(`事件连接失败（${response.status}）`)
  const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ''
  try {
    while (!signal.aborted) {
      const { done, value } = await reader.read()
      buffer += decoder.decode(value, { stream: !done })
      if (buffer.length > 2_000_000) throw new Error('事件过大')
      let match: RegExpExecArray | null
      while ((match = /\r?\n\r?\n/.exec(buffer))) {
        const block = buffer.slice(0, match.index); buffer = buffer.slice(match.index + match[0].length)
        const event = parseAgentEvent(block); if (event && event.sequence > after) { after = event.sequence; onEvent(event) }
      }
      if (done) break
    }
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
}
