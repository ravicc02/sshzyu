import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('studio entry experience', () => {
  it('does not automatically open the entry notice but preserves operation confirmations', () => {
    const source = readFileSync(new URL('./App.tsx', import.meta.url), 'utf8')
    expect(source).not.toContain('useEntryNotice')
    expect(source).toContain('<ConfirmDialog />')
    expect(source).toContain('useDockerApiUrlMigrationNotice()')
  })
})
