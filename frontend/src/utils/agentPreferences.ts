import type { AgentConfig } from '@/api/agent'
import { agentReasoningEfforts } from './agentReasoning'

export type AgentModelPurpose = 'text' | 'image'
export interface AgentModelBinding { key_id: number; models: string[] }
export interface AgentModelLibrary { text: AgentModelBinding[]; image: AgentModelBinding[] }
export function emptyAgentModelLibrary(): AgentModelLibrary { return { text: [], image: [] } }
export function normalizeAgentModelLibrary(value: unknown): AgentModelLibrary {
  const result = emptyAgentModelLibrary()
  if (!value || typeof value !== 'object') return result
  for (const purpose of ['text', 'image'] as const) {
    const bindings = (value as Partial<AgentModelLibrary>)[purpose]
    if (!Array.isArray(bindings)) continue
    const seen = new Set<number>()
    for (const entry of bindings.slice(0, 50)) {
      if (!entry || !Number.isSafeInteger(entry.key_id) || entry.key_id <= 0 || seen.has(entry.key_id) || !Array.isArray(entry.models)) continue
      const models = [...new Set(entry.models.filter((model): model is string => typeof model === 'string' && !!model.trim() && model.length <= 128).map(model => model.trim()))].slice(0, 200)
      result[purpose].push({ key_id: entry.key_id, models }); seen.add(entry.key_id)
    }
  }
  return result
}
export function agentModelsForKey(library: AgentModelLibrary, purpose: AgentModelPurpose, keyId?: number): string[] {
  return library[purpose].find(binding => binding.key_id === keyId)?.models || []
}
export function withAgentModels(library: AgentModelLibrary, purpose: AgentModelPurpose, keyId: number, models: string[]): AgentModelLibrary {
  const result = normalizeAgentModelLibrary(library)
  if (keyId <= 0) return result
  result[purpose] = result[purpose].filter(binding => binding.key_id !== keyId)
  result[purpose].push({ key_id: keyId, models: [...new Set(models.map(model => model.trim()).filter(Boolean))] })
  return normalizeAgentModelLibrary(result)
}
export function withCurrentAgentModels(library: AgentModelLibrary, config: AgentConfig): AgentModelLibrary {
  let result = normalizeAgentModelLibrary(library)
  for (const purpose of ['text', 'image'] as const) {
    const keyId = purpose === 'text' ? config.model_api_key_id : config.image_api_key_id
    const model = purpose === 'text' ? config.model : config.image_model
    if (keyId && model) result = withAgentModels(result, purpose, keyId, [...agentModelsForKey(result, purpose, keyId), model])
  }
  return result
}

const prefix = 'sshzy:agent:preferences:v1:'
function storageKey(userId?: number): string | undefined {
  return Number.isSafeInteger(userId) && Number(userId) > 0 ? prefix + userId : undefined
}
export function emptyAgentConfig(): AgentConfig {
  return { model_api_key_id: 0, model: '', image_api_key_id: 0, image_model: '', reasoning_effort: 'medium' }
}
// Pick explicit metadata fields only. Never persist raw keys, tokens, messages,
// uploads or arbitrary properties from an API response or a tampered cache.
export function storedAgentConfig(value: unknown): AgentConfig | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const c = value as Partial<AgentConfig>
  if (!Number.isSafeInteger(c.model_api_key_id) || Number(c.model_api_key_id) <= 0 || typeof c.model !== 'string' || !c.model.trim() || c.model.length > 128) return undefined
  const imageKey = c.image_api_key_id ?? 0, imageModel = c.image_model ?? ''
  if (!Number.isSafeInteger(imageKey) || imageKey < 0 || imageKey === c.model_api_key_id || typeof imageModel !== 'string' || imageModel.length > 128 || (!!imageKey !== !!imageModel.trim())) return undefined
  const effort = c.reasoning_effort ?? 'medium'
  if (!agentReasoningEfforts.some(value => value === effort)) return undefined
  return { model_api_key_id: c.model_api_key_id!, model: c.model, image_api_key_id: imageKey, image_model: imageModel, reasoning_effort: effort as AgentConfig['reasoning_effort'] }
}
function readSaved(userId?: number): { version?: number; config?: unknown; model_library?: unknown } | undefined {
  const key = storageKey(userId)
  if (!key) return undefined
  try {
    const raw = localStorage.getItem(key)
    if (!raw || raw.length > 65536) return undefined
    const saved = JSON.parse(raw)
    return saved?.version === 1 ? saved : undefined
  } catch { return undefined }
}
export function readAgentPreferences(userId?: number): AgentConfig | undefined {
  return storedAgentConfig(readSaved(userId)?.config)
}
export function readAgentModelLibrary(userId?: number): AgentModelLibrary {
  const saved = readSaved(userId)
  const library = normalizeAgentModelLibrary(saved?.model_library)
  const config = storedAgentConfig(saved?.config)
  return config ? withCurrentAgentModels(library, config) : library
}
export function saveAgentPreferences(userId: number | undefined, value: AgentConfig, library?: AgentModelLibrary): boolean {
  const key = storageKey(userId), config = storedAgentConfig(value)
  if (!key || !config) return false
  const existing = readSaved(userId)?.model_library
  const models = library || (existing ? normalizeAgentModelLibrary(existing) : undefined)
  const saved = { version: 1, config, ...(models ? { model_library: withCurrentAgentModels(models, config) } : {}) }
  try {
    const raw = JSON.stringify(saved)
    if (raw.length > 65536) return false
    localStorage.setItem(key, raw)
    return true
  } catch { return false }
}
