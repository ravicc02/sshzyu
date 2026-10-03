import { apiClient } from '../client'
import type { VersionInfo } from './system'

export interface ReleaseMigration {
  filename: string
  checksum: string
  risk: 'backward-compatible' | 'manual'
  description: string
  non_transactional: boolean
}

export interface CustomManifest {
  version: string
  repository: string
  source_sha: string
  upstream: { tag: string; commit: string }
  migrations: ReleaseMigration[]
}

export interface CustomRelease {
  manifest_hash: string
  manifest: CustomManifest
  release_id: number
}

export interface UpdateOperation {
  id: string
  kind: 'update' | 'rollback'
  stage: string
  manifest_hash: string
  target: CustomManifest
  pending_migrations: ReleaseMigration[]
  error?: string
}

export const operationStorageKey = 'sshzy-custom-update-operation'
export const terminalStages = new Set(['completed', 'rolled_back', 'failed', 'cancelled', 'manual_intervention'])

export function canConfirmOperation(operation: UpdateOperation | null, downtime: boolean, migrations: boolean): boolean {
  return !!operation && operation.stage === 'ready' && downtime &&
    (operation.pending_migrations.length === 0 || migrations) &&
    operation.pending_migrations.every(item => item.risk === 'backward-compatible' && !item.non_transactional)
}

export function savedOperationID(): string | null {
  const id = localStorage.getItem(operationStorageKey)
  return id && /^[a-f0-9]{32}$/.test(id) ? id : null
}

export async function listCustomReleases(): Promise<CustomRelease[]> {
  const { data } = await apiClient.get<{ releases: CustomRelease[] }>('/admin/system/releases')
  return data.releases
}

export async function prepareCustomUpdate(target: CustomRelease, kind: 'update' | 'rollback' = 'update'): Promise<UpdateOperation> {
  const { data } = await apiClient.post<UpdateOperation>(
    kind === 'rollback' ? '/admin/system/updates/rollback' : '/admin/system/updates/prepare',
    { version: target.manifest.version, manifest_hash: target.manifest_hash },
    { headers: { 'Idempotency-Key': crypto.randomUUID() }, timeout: 120000 }
  )
  localStorage.setItem(operationStorageKey, data.id)
  return data
}

export async function getCustomOperation(id: string): Promise<UpdateOperation> {
  const { data } = await apiClient.get<UpdateOperation>(`/admin/system/updates/operations/${id}`)
  return data
}

export async function activateCustomUpdate(operation: UpdateOperation): Promise<UpdateOperation> {
  const { data } = await apiClient.post<UpdateOperation>(
    `/admin/system/updates/operations/${operation.id}/activate`,
    {
      manifest_hash: operation.manifest_hash,
      confirm_downtime: true,
      confirm_migrations: operation.pending_migrations.map(item => item.filename)
    }
  )
  return data
}

export async function cancelCustomUpdate(operation: UpdateOperation): Promise<UpdateOperation> {
  const { data } = await apiClient.post<UpdateOperation>(`/admin/system/updates/operations/${operation.id}/cancel`)
  return data
}

export type CustomVersionInfo = VersionInfo & {
  installation_mode?: string
  update_source?: string
  check_status?: string
  can_update?: boolean
  commit?: string
  upstream_version?: string
  custom_release?: CustomRelease
  official_notice?: { version: string; has_update: boolean; url: string }
}
