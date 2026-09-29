/**
 * Admin Lottery API endpoints
 * 管理端抽奖接口: 中奖记录查询 / 活动与奖品参数调整 / 发放重试 / 次数补发
 * 后端接口一览(internal/server/routes/admin.go registerLotteryRoutes):
 * - GET  /admin/lottery/draws?user_id=&page=&page_size=&sort_order=
 * - GET  /admin/lottery/activities
 * - GET  /admin/lottery/prizes?activity_id=
 * - PUT  /admin/lottery/activity/:id
 * - PUT  /admin/lottery/prizes/:id
 * - POST /admin/lottery/draws/:id/retry-fulfillment
 * - POST /admin/lottery/users/:id/adjust
 */

import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'

/** 管理端中奖记录附带的中奖人标识(后端 dto.LotteryDrawUserRef) */
export interface AdminLotteryDrawUser {
  id: number
  email: string
  username: string
  deleted: boolean
}

/** 管理端抽奖流水(后端 dto.LotteryDraw) */
export interface AdminLotteryDraw {
  id: number
  user_id: number
  activity_id: number
  prize_id: number | null
  prize_name: string
  prize_type: 'none' | 'balance_bonus' | 'quota'
  prize_value: number
  rules_version: number
  source: string
  balance_spent_at_draw: number
  fulfillment_status: 'granted' | 'pending' | 'failed'
  fulfilled_at: string | null
  fulfillment_error?: string | null
  created_at: string
  /** 中奖人摘要,后端单条查询失败时缺省 */
  user?: AdminLotteryDrawUser
}

/** 管理端活动(后端 service.LotteryActivity 原样) */
export interface AdminLotteryActivity {
  id: number
  name: string
  status: 'draft' | 'active' | 'paused' | 'ended'
  rules_version: number
  starts_at: string | null
  ends_at: string | null
  created_at: string
  updated_at: string
}

/** 管理端奖品原始配置(后端 dto.LotteryAdminPrize,未做概率换算) */
export interface AdminLotteryPrize {
  id: number
  activity_id: number
  name: string
  prize_type: 'none' | 'balance_bonus' | 'quota'
  value: number
  weight: number
  /** 可中该奖的最低阶梯: 0 青铜 / 1 白银 / 2 黄金 / 3 钻石 / 4 王者 */
  min_tier: number
  /** 按阶梯覆盖权重, key 为 "0".."4"; 缺失回退 weight */
  tier_weights: Record<string, number>
  /** 总配额, -1 表示无限 */
  stock: number
  /** 已发放数量, 可用量 = stock - stock_issued */
  stock_issued: number
  enabled: boolean
  sort_order: number
}

export interface AdminUpdateActivityRequest {
  name?: string
  status?: 'draft' | 'active' | 'paused' | 'ended'
  starts_at?: string | null
  ends_at?: string | null
}

export interface AdminUpdatePrizeRequest {
  name?: string
  prize_type?: 'none' | 'balance_bonus' | 'quota'
  value?: number
  weight?: number
  min_tier?: number
  tier_weights?: Record<string, number>
  stock?: number
  enabled?: boolean
  sort_order?: number
}

/**
 * 分页查询中奖/抽奖流水(管理端)
 * @param userId 按用户过滤(可空)
 * @param sortOrder 排序: desc 最新在前(默认) / asc 最早在前,按记录 ID 即时间序
 */
export async function listDraws(
  page = 1,
  pageSize = 20,
  userId?: number,
  sortOrder: 'asc' | 'desc' = 'desc',
  options?: { signal?: AbortSignal }
): Promise<BasePaginationResponse<AdminLotteryDraw>> {
  const { data } = await apiClient.get<BasePaginationResponse<AdminLotteryDraw>>(
    '/admin/lottery/draws',
    {
      params: {
        page,
        page_size: pageSize,
        sort_order: sortOrder,
        ...(userId ? { user_id: userId } : {})
      },
      signal: options?.signal
    }
  )
  return data
}

/** 查询全部活动 */
export async function getActivities(options?: {
  signal?: AbortSignal
}): Promise<{ activities: AdminLotteryActivity[] }> {
  const { data } = await apiClient.get<{ activities: AdminLotteryActivity[] }>(
    '/admin/lottery/activities',
    { signal: options?.signal }
  )
  return data
}

/** 查询指定活动的奖品原始配置(含停用) */
export async function getPrizes(
  activityId: number,
  options?: { signal?: AbortSignal }
): Promise<{ prizes: AdminLotteryPrize[] }> {
  const { data } = await apiClient.get<{ prizes: AdminLotteryPrize[] }>('/admin/lottery/prizes', {
    params: { activity_id: activityId },
    signal: options?.signal
  })
  return data
}

/** 更新活动配置(局部更新,只传需要修改的字段) */
export async function updateActivity(
  id: number,
  request: AdminUpdateActivityRequest
): Promise<{ updated: boolean }> {
  const { data } = await apiClient.put<{ updated: boolean }>(`/admin/lottery/activity/${id}`, request)
  return data
}

/** 更新奖品配置(局部更新,只传需要修改的字段) */
export async function updatePrize(
  id: number,
  request: AdminUpdatePrizeRequest
): Promise<{ updated: boolean }> {
  const { data } = await apiClient.put<{ updated: boolean }>(`/admin/lottery/prizes/${id}`, request)
  return data
}

/** 重试失败/待处理的奖品发放(granted 不可重试) */
export async function retryFulfillment(
  drawId: number
): Promise<{ draw: AdminLotteryDraw }> {
  const { data } = await apiClient.post<{ draw: AdminLotteryDraw }>(
    `/admin/lottery/draws/${drawId}/retry-fulfillment`
  )
  return data
}

/** 补发(正数)/回收(负数)指定用户的抽奖次数 */
export async function adjustDraws(
  userId: number,
  delta: number
): Promise<{ adjusted: boolean }> {
  const { data } = await apiClient.post<{ adjusted: boolean }>(
    `/admin/lottery/users/${userId}/adjust`,
    { delta }
  )
  return data
}

export const adminLotteryAPI = {
  listDraws,
  getActivities,
  getPrizes,
  updateActivity,
  updatePrize,
  retryFulfillment,
  adjustDraws
}

export default adminLotteryAPI
