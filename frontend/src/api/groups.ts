/**
 * User Groups API endpoints (non-admin)
 * Handles group-related operations for regular users
 */

import { apiClient } from './client'
import type { Group } from '@/types'

/**
 * Get available groups that the current user can bind to API keys
 * This returns groups based on user's permissions:
 * - Standard groups: public (non-exclusive) or explicitly allowed
 * - Subscription groups: user has active subscription
 * @returns List of available groups
 */
export async function getAvailable(): Promise<Group[]> {
  const { data } = await apiClient.get<Group[]>('/groups/available')
  return data
}

/**
 * Get current user's custom group rate multipliers
 * @returns Map of group_id to custom rate_multiplier
 */
export async function getUserGroupRates(): Promise<Record<number, number>> {
  const { data } = await apiClient.get<Record<number, number> | null>('/groups/rates')
  return data || {}
}

/** Fetch declared group model IDs without adding built-in catalog defaults. */
export async function getModels(groupId: number, options?: { signal?: AbortSignal }): Promise<string[]> {
  const { data } = await apiClient.get<{ data: Array<{ id: string }> }>(
    `/groups/${groupId}/models`, { signal: options?.signal }
  )
  return [...new Set((data.data || []).map(model => model.id).filter(id => typeof id === 'string' && id.length > 0 && !id.includes('*') && !id.includes('?')))]
}

export const userGroupsAPI = {
  getAvailable,
  getModels,
  getUserGroupRates
}

export default userGroupsAPI
