import { describe, expect, it } from 'vitest'
import {
  GROK_CC_SWITCH_MODEL,
  OPENAI_CC_SWITCH_CODEX_MODEL,
  buildCcSwitchImportDeeplink
} from '@/utils/ccswitchImport'
import type { GroupPlatform } from '@/types'

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
}

describe('ccswitchImport utils', () => {
  it('defaults OpenAI CC Switch imports to the current Codex model', () => {
    expect(OPENAI_CC_SWITCH_CODEX_MODEL).toBe('gpt-6.1-sol')
  })

  it('defaults Grok Build imports to the current Grok model', () => {
    expect(GROK_CC_SWITCH_MODEL).toBe('grok-4.5')
  })

  const baseInput = {
    baseUrl: 'https://api.example.com',
    providerName: 'Sub2API',
    apiKey: 'sk-test',
    usageScript: 'return true'
  }

  it('adds the Codex model parameter for OpenAI imports', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'openai',
        clientType: 'claude'
      })
    )

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.get('model')).toBe(OPENAI_CC_SWITCH_CODEX_MODEL)
    expect(atob(params.get('usageScript') || '')).toBe(baseInput.usageScript)
    const payload = JSON.parse(atob(params.get('config') || '')) as { config: string }
    expect(params.get('configFormat')).toBe('json')
    expect(payload.config).toContain(`base_url = "${endpoint}"`)
    expect(payload.config).toContain(`model = "${OPENAI_CC_SWITCH_CODEX_MODEL}"`)
    expect(payload.config.split('[model_providers.')[0]).toContain('web_search = "live"')
    expect(payload.config).toContain('requires_openai_auth = false')
    expect(payload.config).toContain('experimental_bearer_token = "sk-test"')
    expect(payload.config).toContain('"x-openai-actor-authorization" = "local-image-extension"')
    // Global feature choices are preserved by CC Switch, not imported per provider.
    expect(payload.config).not.toContain('[features]')
  })

  it('encodes embedded Codex configuration as UTF-8 and escapes TOML strings', () => {
    const apiKey = 'sk-"test\\key'
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({
      ...baseInput,
      apiKey,
      baseUrl: 'https://example.com/网关',
      platform: 'openai',
      clientType: 'claude'
    }))
    const bytes = Uint8Array.from(atob(params.get('config') || ''), (char) => char.charCodeAt(0))
    const payload = JSON.parse(new TextDecoder().decode(bytes)) as { config: string }
    expect(payload.config).toContain('base_url = "https://example.com/网关"')
    expect(payload.config).toContain(`experimental_bearer_token = ${JSON.stringify(apiKey)}`)
    expect(params.get('apiKey')).toBe(apiKey)
  })

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('imports Grok Build with one /v1 suffix for base URL %s', (baseUrl) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        platform: 'grok',
        clientType: 'claude'
      })
    )

    expect(params.get('app')).toBe('grokbuild')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('model')).toBe(GROK_CC_SWITCH_MODEL)
    expect(params.has('config')).toBe(false)
  })

  it.each([
    { platform: 'anthropic' as GroupPlatform, clientType: 'claude' as const, app: 'claude' },
    { platform: 'gemini' as GroupPlatform, clientType: 'gemini' as const, app: 'gemini' }
  ])('does not add a model parameter for $platform imports', ({ platform, clientType, app }) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform,
        clientType
      })
    )

    expect(params.get('app')).toBe(app)
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.has('model')).toBe(false)
    expect(params.has('config')).toBe(false)
  })

  it('keeps Antigravity imports on the selected client endpoint without a model parameter', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        platform: 'antigravity',
        clientType: 'gemini'
      })
    )

    expect(params.get('app')).toBe('gemini')
    expect(params.get('endpoint')).toBe(`${baseInput.baseUrl}/antigravity`)
    expect(params.has('model')).toBe(false)
    expect(params.has('config')).toBe(false)
  })
})
