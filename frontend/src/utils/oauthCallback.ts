export interface OAuthCallbackInput {
  code: string
  state: string
}

/**
 * Accepts either a complete OAuth callback URL or a raw authorization code.
 * The caller remains responsible for falling back to the state embedded in the
 * generated authorization URL when the pasted value does not contain state.
 */
export function parseOAuthCallbackInput(value: string): OAuthCallbackInput {
  const trimmed = value.trim()
  if (!trimmed) return { code: '', state: '' }

  if (trimmed.includes('code=')) {
    try {
      const url = trimmed.includes('?')
        ? new URL(trimmed)
        : new URL(`http://localhost/callback?${trimmed.replace(/^\?/, '')}`)
      return {
        code: (url.searchParams.get('code') || '').trim(),
        state: (url.searchParams.get('state') || '').trim()
      }
    } catch {
      const code = trimmed.match(/[?&]code=([^&]+)/)?.[1] || ''
      const state = trimmed.match(/[?&]state=([^&]+)/)?.[1] || ''
      return {
        code: safeDecode(code).trim(),
        state: safeDecode(state).trim()
      }
    }
  }

  // Users often copy from the value after `code=`, producing:
  // AUTHORIZATION_CODE&scope=...&state=...
  // Recover the leading raw code and state instead of treating the whole
  // fragment as the code.
  const raw = trimmed.replace(/^\?/, '')
  const separator = raw.indexOf('&')
  if (separator > 0) {
    const leading = raw.slice(0, separator)
    if (leading && !leading.includes('=')) {
      const params = new URLSearchParams(raw.slice(separator + 1))
      return {
        code: safeDecode(leading).trim(),
        state: (params.get('state') || '').trim()
      }
    }
  }

  return { code: safeDecode(trimmed).trim(), state: '' }
}

function safeDecode(value: string): string {
  if (!value) return ''
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}
