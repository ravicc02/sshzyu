import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { agentModelsForKey, emptyAgentConfig, emptyAgentModelLibrary, normalizeAgentModelLibrary, readAgentModelLibrary, readAgentPreferences, saveAgentPreferences, storedAgentConfig, withAgentModels } from '../agentPreferences'
const config = { model_api_key_id: 1, model: 'gpt-text', image_api_key_id: 2, image_model: 'gpt-image', reasoning_effort: 'medium' }
beforeEach(() => localStorage.clear())
afterEach(() => vi.unstubAllGlobals())
describe('Agent preferences', () => {
  it('restores both model selections without a budget', () => {
    expect(saveAgentPreferences(1, config)).toBe(true)
    expect(readAgentPreferences(1)).toEqual(config)
    expect(emptyAgentConfig().model_api_key_id).toBe(0)
  })
  it('isolates preferences by user and never stores an anonymous default', () => {
    saveAgentPreferences(1, config)
    expect(readAgentPreferences(2)).toBeUndefined()
    expect(saveAgentPreferences(undefined, config)).toBe(false)
    expect(readAgentPreferences()).toBeUndefined()
    saveAgentPreferences(2, { ...config, model: 'other-model' })
    expect(readAgentPreferences(1)?.model).toBe('gpt-text')
    expect(readAgentPreferences(2)?.model).toBe('other-model')
  })
  it('stores only metadata fields and drops secret or upload properties', () => {
    saveAgentPreferences(1, { ...config, api_key: 'test-value-not-a-real-key', reference_images: ['private-input'], token: 'test-value' } as typeof config)
    const raw = localStorage.getItem('sshzy:agent:preferences:v1:1')!
    expect(JSON.parse(raw)).toEqual({ version: 1, config })
    expect(raw).not.toMatch(/api_key"|private-input|test-value/)
  })
  it.each(['{broken', 'null', '{"version":2}', '{"version":1,"config":{}}', 'x'.repeat(4097)])('ignores invalid or unsupported cached data', raw => {
    localStorage.setItem('sshzy:agent:preferences:v1:1', raw)
    expect(readAgentPreferences(1)).toBeUndefined()
  })
  it('supports a text-only configuration', () => {
    const text = { ...config, image_api_key_id: 0, image_model: '' }
    expect(storedAgentConfig(text)).toEqual(text)
    expect(saveAgentPreferences(1, text)).toBe(true)
    expect(readAgentPreferences(1)).toEqual(text)
  })
  it.each([
    { ...config, model_api_key_id: -1 }, { ...config, model: '' },
    { ...config, image_api_key_id: 1 }, { ...config, image_model: '' },
  ])('rejects invalid cached configuration', value => { expect(storedAgentConfig(value)).toBeUndefined() })
  it.each(['0', '-1', 'Infinity', '0.01'])('drops historical budget %s while preserving model settings', legacy => {
    expect(storedAgentConfig({ ...config, default_max_cost_usd: legacy })).toEqual(config)
    localStorage.setItem('sshzy:agent:preferences:v1:1', JSON.stringify({ version: 1, config: { ...config, default_max_cost_usd: legacy } }))
    expect(readAgentPreferences(1)).toEqual(config)
  })
  it('handles blocked browser storage without throwing', () => {
    vi.stubGlobal('localStorage', { getItem: () => { throw new Error('disabled') }, setItem: () => { throw new Error('quota') } })
    expect(readAgentPreferences(1)).toBeUndefined()
    expect(saveAgentPreferences(1, config)).toBe(false)
  })
  it('migrates the previous single model choices into the model library', () => {
    saveAgentPreferences(1, config)
    expect(readAgentModelLibrary(1)).toEqual({ text: [{ key_id: 1, models: ['gpt-text'] }], image: [{ key_id: 2, models: ['gpt-image'] }] })
  })
  it('keeps models isolated by user, key and purpose and preserves them on model preference saves', () => {
    let library = withAgentModels(emptyAgentModelLibrary(), 'text', 1, ['gpt-text', 'other-text'])
    library = withAgentModels(library, 'text', 3, ['third-key'])
    library = withAgentModels(library, 'image', 2, ['gpt-image', 'other-image'])
    saveAgentPreferences(1, config, library)
    saveAgentPreferences(1, { ...config, model: 'other-text' })
    expect(agentModelsForKey(readAgentModelLibrary(1), 'text', 1)).toEqual(['gpt-text', 'other-text'])
    expect(agentModelsForKey(readAgentModelLibrary(1), 'text', 3)).toEqual(['third-key'])
    expect(agentModelsForKey(readAgentModelLibrary(1), 'image', 2)).toEqual(['gpt-image', 'other-image'])
    expect(readAgentModelLibrary(2)).toEqual(emptyAgentModelLibrary())
  })
  it('sanitizes model library metadata, duplicates and unknown secret fields', () => {
    const normalized = normalizeAgentModelLibrary({ text: [{ key_id: 1, models: ['a', 'a', '', ' b '], api_key: 'not-a-real-secret' }, { key_id: -1, models: ['bad'] }], image: [] })
    expect(normalized).toEqual({ text: [{ key_id: 1, models: ['a', 'b'] }], image: [] })
    const original = emptyAgentModelLibrary()
    withAgentModels(original, 'text', 1, ['a'])
    expect(original.text).toEqual([])
  })

  it.each(['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const)('preserves the declared %s reasoning effort across refresh', effort => {
    saveAgentPreferences(1, { ...config, reasoning_effort: effort })
    expect(readAgentPreferences(1)?.reasoning_effort).toBe(effort)
  })

})
