/**
 * Admin Users API endpoints
 * Handles user management for administrators
 */

import { apiClient } from '../client'
import type { AdminUser, UpdateUserRequest, PaginatedResponse, ApiKey } from '@/types'

export interface AdminBindAuthIdentityChannelRequest {
  channel: string
  channel_app_id: string
  channel_subject: string
  metadata?: Record<string, unknown> | null
}

export interface AdminBindAuthIdentityRequest {
  provider_type: string
  provider_key: string
  provider_subject: string
  issuer?: string | null
  metadata?: Record<string, unknown> | null
  channel?: AdminBindAuthIdentityChannelRequest
}

export interface AdminBoundAuthIdentityChannel {
  channel: string
  channel_app_id: string
  channel_subject: string
  metadata: Record<string, unknown> | null
  created_at: string
  updated_at: string
}

export interface AdminBoundAuthIdentity {
  user_id: number
  provider_type: string
  provider_key: string
  provider_subject: string
  verified_at?: string | null
  issuer?: string | null
  metadata: Record<string, unknown> | null
  created_at: string
  updated_at: string
  channel?: AdminBoundAuthIdentityChannel | null
}

export interface BatchUpdateUserLimitsRequest {
  user_ids: number[]
  all?: boolean
  concurrency?: number
  rpm_limit?: number
}

export interface BatchUpdateUserLimitsResponse {
  affected: number
}

export interface BatchUpdateBalanceResponse {
  items: Array<{ user_id: number; success: boolean; error?: string; code?: string; retryable?: boolean }>
  success: number
  failed: number
}

/**
 * List all users with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters (status, role, search, attributes)
 * @param options - Optional request options (signal)
 * @returns Paginated list of users
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: 'active' | 'disabled'
    role?: 'admin' | 'user'
    search?: string
    group_name?: string         // fuzzy filter by allowed group name
    api_key_group_id?: number   // filter users by the group their API keys are bound to
    api_key_provider?: string   // filter users by provider bucket of their API keys' group platform (anthropic/openai/domestic/other)
    attributes?: Record<number, string>  // attributeId -> value
    include_subscriptions?: boolean
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<PaginatedResponse<AdminUser>> {
  // Build params with attribute filters in attr[id]=value format
  const params: Record<string, any> = {
    page,
    page_size: pageSize,
    status: filters?.status,
    role: filters?.role,
    search: filters?.search,
    group_name: filters?.group_name,
    api_key_group_id: filters?.api_key_group_id,
    api_key_provider: filters?.api_key_provider,
    include_subscriptions: filters?.include_subscriptions,
    sort_by: filters?.sort_by,
    sort_order: filters?.sort_order
  }

  // Add attribute filters as attr[id]=value
  if (filters?.attributes) {
    for (const [attrId, value] of Object.entries(filters.attributes)) {
      if (value) {
        params[`attr[${attrId}]`] = value
      }
    }
  }
  const { data } = await apiClient.get<PaginatedResponse<AdminUser>>('/admin/users', {
    params,
    signal: options?.signal
  })
  return data
}

/**
 * Get user by ID
 * @param id - User ID
 * @param includeDeleted - Whether to include soft-deleted users
 * @returns User details
 */
export async function getById(id: number, includeDeleted = false): Promise<AdminUser> {
  const url = includeDeleted ? `/admin/users/${id}?include_deleted=true` : `/admin/users/${id}`
  const { data } = await apiClient.get<AdminUser>(url)
  return data
}

/**
 * Create new user
 * @param userData - User data (email, password, etc.)
 * @returns Created user
 */
export async function create(userData: {
  email: string
  password: string
  username?: string
  notes?: string
  role?: 'admin' | 'user'
  balance?: number
  concurrency?: number
  rpm_limit?: number
  allowed_groups?: number[] | null
}): Promise<AdminUser> {
  const { data } = await apiClient.post<AdminUser>('/admin/users', userData)
  return data
}

/**
 * Update user
 * @param id - User ID
 * @param updates - Fields to update
 * @returns Updated user
 */
export async function update(id: number, updates: UpdateUserRequest): Promise<AdminUser> {
  const { data } = await apiClient.put<AdminUser>(`/admin/users/${id}`, updates)
  return data
}

/**
 * Delete user
 * @param id - User ID
 * @returns Success confirmation
 */
export async function deleteUser(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/users/${id}`)
  return data
}

/**
 * Update user balance
 * @param id - User ID
 * @param balance - New balance
 * @param operation - Operation type ('set', 'add', 'subtract')
 * @param notes - Optional notes for the balance adjustment
 * @returns Updated user
 */
export async function updateBalance(
  id: number,
  balance: number,
  operation: 'set' | 'add' | 'subtract' = 'set',
  notes?: string
): Promise<AdminUser> {
  const { data } = await apiClient.post<AdminUser>(`/admin/users/${id}/balance`, {
    balance,
    operation,
    notes: notes || ''
  })
  return data
}

/**
 * Update user concurrency
 * @param id - User ID
 * @param concurrency - New concurrency limit
 * @returns Updated user
 */
export async function updateConcurrency(id: number, concurrency: number): Promise<AdminUser> {
  return update(id, { concurrency })
}

/** Overwrite concurrency and/or RPM limits for multiple users in one request. */
export async function batchUpdateLimits(
  request: BatchUpdateUserLimitsRequest
): Promise<BatchUpdateUserLimitsResponse> {
  const { data } = await apiClient.post<BatchUpdateUserLimitsResponse>(
    '/admin/users/batch-limits',
    request
  )
  return data
}

export interface BatchUpdateBalanceRequest {
  user_ids: number[]
  amount: number
  operation: 'add' | 'subtract'
  notes?: string
}

const balanceAttemptKeys = new Map<string, { key: string; createdAt: number }>()

async function balanceStorageKey(signature: string): Promise<string> {
  if (globalThis.crypto?.subtle) {
    const digest = await globalThis.crypto.subtle.digest('SHA-256', new TextEncoder().encode(signature))
    return 'sshzy:batch-balance:' + Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('')
  }
  let hash = 2166136261
  for (const character of signature) hash = Math.imul(hash ^ character.charCodeAt(0), 16777619) >>> 0
  return 'sshzy:batch-balance:' + hash.toString(16)
}

export async function batchUpdateBalance(request: BatchUpdateBalanceRequest, operationKey?: string): Promise<BatchUpdateBalanceResponse> {
  const normalized = { ...request, user_ids: [...new Set(request.user_ids)].sort((left,right) => left-right), notes: request.notes?.trim() || '' }
  let owner = 'unknown'
  try { owner = String(JSON.parse(globalThis.localStorage?.getItem('auth_user') || 'null')?.id ?? owner) } catch { owner = 'unknown' }
  const signature = JSON.stringify({ owner, request: normalized })
  const storageKey = await balanceStorageKey(signature)
  let attempt = balanceAttemptKeys.get(signature)
  if (!attempt) {
    try {
      const stored = JSON.parse(globalThis.sessionStorage?.getItem(storageKey) || 'null')
      if (stored && typeof stored.key === 'string' && typeof stored.createdAt === 'number') attempt = stored
    } catch { attempt = undefined }
  }
  if (attempt && Date.now() - attempt.createdAt > 23*60*60*1000) throw new Error('The previous balance request is unconfirmed and its safe retry window expired. Review balance history before making another adjustment.')
  if (!attempt) attempt = { key: operationKey || 'balance-' + (globalThis.crypto?.randomUUID?.() || Date.now().toString(36) + '-' + Math.random().toString(36).slice(2)), createdAt: Date.now() }
  balanceAttemptKeys.set(signature, attempt)
  try { globalThis.sessionStorage?.setItem(storageKey, JSON.stringify(attempt)) } catch { balanceAttemptKeys.set(signature, attempt) }
  const { data } = await apiClient.post<BatchUpdateBalanceResponse>('/admin/users/batch-balance', normalized, { headers: { 'Idempotency-Key': attempt.key }, timeout: 180000 })
  balanceAttemptKeys.delete(signature)
  try { globalThis.sessionStorage?.removeItem(storageKey) } catch { balanceAttemptKeys.delete(signature) }
  return data
}

/**
 * Toggle user status
 * @param id - User ID
 * @param status - New status
 * @returns Updated user
 */
export async function toggleStatus(id: number, status: 'active' | 'disabled'): Promise<AdminUser> {
  return update(id, { status })
}

/**
 * Get user's API keys
 * @param id - User ID
 * @returns List of user's API keys
 */
export async function getUserApiKeys(id: number): Promise<PaginatedResponse<ApiKey>> {
  const { data } = await apiClient.get<PaginatedResponse<ApiKey>>(`/admin/users/${id}/api-keys`)
  return data
}

/**
 * Get user's usage statistics
 * @param id - User ID
 * @param period - Time period
 * @returns User usage statistics
 */
export async function getUserUsageStats(
  id: number,
  period: string = 'month'
): Promise<{
  total_requests: number
  total_cost: number
  total_tokens: number
}> {
  const { data } = await apiClient.get<{
    total_requests: number
    total_cost: number
    total_tokens: number
  }>(`/admin/users/${id}/usage`, {
    params: { period }
  })
  return data
}

/**
 * Balance history item returned from the API
 */
export interface BalanceHistoryItem {
  id: number
  code: string
  type: string
  value: number
  status: string
  used_by: number | null
  used_at: string | null
  created_at: string
  group_id: number | null
  validity_days: number
  notes: string
  user?: { id: number; email: string } | null
  group?: { id: number; name: string } | null
}

// Balance history response extends pagination with total_recharged summary
export interface BalanceHistoryResponse extends PaginatedResponse<BalanceHistoryItem> {
  total_recharged: number
}

/**
 * Get user's balance/concurrency change history
 * @param id - User ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @param type - Optional type filter (balance, affiliate_balance, admin_balance, concurrency, admin_concurrency, subscription)
 * @returns Paginated balance history with total_recharged
 */
export async function getUserBalanceHistory(
  id: number,
  page: number = 1,
  pageSize: number = 20,
  type?: string
): Promise<BalanceHistoryResponse> {
  const params: Record<string, any> = { page, page_size: pageSize }
  if (type) params.type = type
  const { data } = await apiClient.get<BalanceHistoryResponse>(
    `/admin/users/${id}/balance-history`,
    { params }
  )
  return data
}

/**
 * Replace user's exclusive group
 * @param userId - User ID
 * @param oldGroupId - Current group ID to replace
 * @param newGroupId - New group ID to replace with
 * @returns Number of migrated keys
 */
export async function replaceGroup(
  userId: number,
  oldGroupId: number,
  newGroupId: number
): Promise<{ migrated_keys: number }> {
  const { data } = await apiClient.post<{ migrated_keys: number }>(
    `/admin/users/${userId}/replace-group`,
    { old_group_id: oldGroupId, new_group_id: newGroupId }
  )
  return data
}

export async function bindUserAuthIdentity(
  userId: number,
  input: AdminBindAuthIdentityRequest
): Promise<AdminBoundAuthIdentity> {
  const { data } = await apiClient.post<AdminBoundAuthIdentity>(
    `/admin/users/${userId}/auth-identities`,
    input
  )
  return data
}

/**
 * Platform quota types
 */
// Keep aligned with backend/internal/service/domain_constants.go AllowedQuotaPlatforms.
export const PLATFORM_QUOTA_PLATFORMS = [
  'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
  'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe',
] as const
export type PlatformQuotaPlatform = typeof PLATFORM_QUOTA_PLATFORMS[number]
export type PlatformQuotaWindow = 'daily' | 'weekly' | 'monthly'

export interface PlatformQuotaItem {
  platform: PlatformQuotaPlatform
  daily_limit_usd: number | null
  weekly_limit_usd: number | null
  monthly_limit_usd: number | null
  daily_usage_usd: number
  weekly_usage_usd: number
  monthly_usage_usd: number
  daily_window_start?: string | null
  weekly_window_start?: string | null
  monthly_window_start?: string | null
  daily_window_resets_at?: string | null
  weekly_window_resets_at?: string | null
  monthly_window_resets_at?: string | null
}

export interface PlatformQuotaUpdateItem {
  platform: PlatformQuotaPlatform
  daily_limit_usd: number | null
  weekly_limit_usd: number | null
  monthly_limit_usd: number | null
}

export interface PlatformQuotasResponse {
  platform_quotas: PlatformQuotaItem[]
}

/**
 * Get user's platform quotas
 */
export async function getPlatformQuotas(id: number): Promise<PlatformQuotasResponse> {
  const { data } = await apiClient.get<PlatformQuotasResponse>(
    `/admin/users/${id}/platform-quotas`
  )
  return data
}

/**
 * Replace user's platform quotas (全量替换)
 */
export async function updatePlatformQuotas(
  id: number,
  quotas: PlatformQuotaUpdateItem[]
): Promise<PlatformQuotasResponse> {
  const { data } = await apiClient.put<PlatformQuotasResponse>(
    `/admin/users/${id}/platform-quotas`,
    { quotas }
  )
  return data
}

/**
 * Reset a single (platform, window) usage immediately
 */
export async function resetPlatformQuotaWindow(
  id: number,
  platform: PlatformQuotaPlatform,
  window: PlatformQuotaWindow
): Promise<PlatformQuotasResponse> {
  const { data } = await apiClient.post<PlatformQuotasResponse>(
    `/admin/users/${id}/platform-quotas/reset`,
    { platform, window }
  )
  return data
}

export const usersAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteUser,
  updateBalance,
  updateConcurrency,
  batchUpdateLimits,
  batchUpdateBalance,
  toggleStatus,
  getUserApiKeys,
  getUserUsageStats,
  getUserBalanceHistory,
  replaceGroup,
  bindUserAuthIdentity,
  getPlatformQuotas,
  updatePlatformQuotas,
  resetPlatformQuotaWindow,
}

export default usersAPI
