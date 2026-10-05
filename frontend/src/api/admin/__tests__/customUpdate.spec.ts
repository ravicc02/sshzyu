import { beforeEach, describe, expect, it } from 'vitest'
import { canConfirmOperation, savedOperationID, operationStorageKey, latestCustomRelease, rollbackCustomRelease, clearSavedOperation, type CustomRelease, type UpdateOperation } from '../customUpdate'

const operation: UpdateOperation = {
  id: 'a'.repeat(32),
  kind: 'update',
  stage: 'ready',
  manifest_hash: 'b'.repeat(64),
  target: { version: '0.2.13-r2', repository: 'ravicc02/sshzyu', source_sha: 'c'.repeat(40), upstream: { tag: 'v0.2.13', commit: 'd'.repeat(40) }, migrations: [] },
  pending_migrations: [{ filename: '246_new.sql', checksum: 'e'.repeat(64), risk: 'backward-compatible', description: 'Add a field', non_transactional: false }]
}

describe('custom update confirmation', () => {
  beforeEach(() => localStorage.clear())
  it('requires separate downtime and migration consent', () => {
    expect(canConfirmOperation(operation, false, true)).toBe(false)
    expect(canConfirmOperation(operation, true, false)).toBe(false)
    expect(canConfirmOperation(operation, true, true)).toBe(true)
  })
  it('blocks manual and nontransactional migrations', () => {
    expect(canConfirmOperation({ ...operation, pending_migrations: [{ ...operation.pending_migrations[0], risk: 'manual' }] }, true, true)).toBe(false)
    expect(canConfirmOperation({ ...operation, pending_migrations: [{ ...operation.pending_migrations[0], non_transactional: true }] }, true, true)).toBe(false)
    expect(canConfirmOperation({ ...operation, stage: 'draining' }, true, true)).toBe(false)
    expect(canConfirmOperation(null, true, true)).toBe(false)
  })
  it('does not require migration consent when no migrations are pending', () => {
    expect(canConfirmOperation({ ...operation, pending_migrations: [] }, true, false)).toBe(true)
  })
  it('restores only validated local operation identifiers', () => {
    localStorage.setItem(operationStorageKey, '../secrets')
    expect(savedOperationID()).toBeNull()
    localStorage.setItem(operationStorageKey, operation.id)
    expect(savedOperationID()).toBe(operation.id)
  })
  it('clears only the operation being dismissed', () => {
    localStorage.setItem(operationStorageKey, operation.id)
    clearSavedOperation('b'.repeat(32))
    expect(savedOperationID()).toBe(operation.id)
    clearSavedOperation(operation.id)
    expect(savedOperationID()).toBeNull()
  })
})

function target(version: string, extra: Partial<CustomRelease> = {}): CustomRelease {
  return { release_id: 1, manifest_hash: 'a'.repeat(64), manifest: { ...operation.target, version }, ...extra }
}

describe('custom release selection', () => {
  it('selects the numerically newest custom release regardless of source order', () => {
    expect(latestCustomRelease([target('0.2.13-r9'), target('0.2.13-r3'), target('0.2.13-r10')])?.manifest.version)
      .toBe('0.2.13-r10')
  })

  it('rejects malformed and foreign-source releases', () => {
    expect(latestCustomRelease([
      target('latest'), target('0.2.13-r01'),
      { ...target('0.2.13-r10'), manifest: { ...operation.target, version: '0.2.13-r10', repository: 'Wei-Shaw/sub2api' } }
    ])).toBeUndefined()
  })

  it('selects only an older deployment-history-backed rollback target', () => {
    const deployed = target('0.2.13-r3', { rollback_available: true, installed_at: '2026-10-03T00:00:00Z' })
    expect(rollbackCustomRelease([target('0.2.13-r5'), deployed, target('0.2.13-r4', { rollback_available: true, installed_at: '2026-10-05T00:00:00Z' })], '0.2.13-r4'))
      .toEqual(deployed)
    expect(rollbackCustomRelease([target('0.2.13-r3')], '0.2.13-r4')).toBeUndefined()
  })
})
