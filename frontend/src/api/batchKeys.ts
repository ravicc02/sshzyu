import { apiClient } from './client'
import type { ApiKey } from '@/types'

export interface APIKeyBatchSummary {
  batch_id: string
  batch_name: string
  group_id: number | null
  group_name: string
  group_platform: string
  key_count: number
  active_key_count: number
  quota_per_key: number
  quota_total: number
  quota_used_total: number
  created_at: string
  last_used_at: string | null
}

export interface CreateAPIKeyBatchRequest {
  name?: string
  group_id: number
  count: number
  quota: number
}

export async function listBatches(options?: { signal?: AbortSignal }): Promise<{ items: APIKeyBatchSummary[] }> {
  const { data } = await apiClient.get<{ items: APIKeyBatchSummary[] }>('/batch-keys', {
    signal: options?.signal,
  })
  return data
}

export async function createBatch(payload: CreateAPIKeyBatchRequest): Promise<{ items: ApiKey[] }> {
  const { data } = await apiClient.post<{ items: ApiKey[] }>('/batch-keys', payload)
  return data
}

export async function getBatch(batchId: string, options?: { signal?: AbortSignal }): Promise<{ items: ApiKey[] }> {
  const { data } = await apiClient.get<{ items: ApiKey[] }>(`/batch-keys/${encodeURIComponent(batchId)}`, {
    signal: options?.signal,
  })
  return data
}

export const batchKeysAPI = {
  listBatches,
  createBatch,
  getBatch,
}

export default batchKeysAPI
