/**
 * Lottery API endpoints
 * 用户端抽奖接口: 活动/状态/抽奖/记录/奖励
 */

import { apiClient } from './client'

export interface LotteryPrize {
  id: number
  name: string
  prize_type: 'none' | 'balance_bonus' | 'quota'
  value: number
  weight: number
  min_tier: number
  tier_weights?: Record<string, number>
  stock: number
  stock_issued: number
  probability: number
}

/** 阶梯内单奖品概率 */
export interface LotteryTierPrizeChance {
  prize_id: number
  name: string
  value: number
  prize_type: 'none' | 'balance_bonus' | 'quota'
  probability: number
}

/** 单个阶梯的奖池视图 */
export interface LotteryTierView {
  tier: number
  name: string
  threshold: number
  prizes: LotteryTierPrizeChance[]
}

export interface LotteryActivity {
  id: number
  name: string
  status: 'draft' | 'active' | 'paused' | 'ended'
  rules_version: number
  starts_at: string | null
  ends_at: string | null
  is_open: boolean
  server_time: string
  prizes: LotteryPrize[]
  tiers: LotteryTierView[]
}

export interface LotteryStatus {
  activity_open: boolean
  available_draws: number
  used_draws: number
  first_draw_granted: boolean
  threshold_entitlement: number
  manual_adjustment: number
  balance_spent: number
  next_threshold: number
  current_tier: number
  tier_name: string
  rules_version: number
}

export interface LotteryDrawRecord {
  id: number
  user_id: number
  activity_id: number
  prize_id: number | null
  prize_name: string
  prize_type: 'none' | 'balance_bonus' | 'quota'
  prize_value: number
  rules_version: number
  source: 'first' | 'threshold' | 'manual'
  balance_spent_at_draw: number
  fulfillment_status: 'pending' | 'granted' | 'failed'
  fulfilled_at: string | null
  fulfillment_error?: string
  created_at: string
}

export interface LotteryDrawResult {
  draw: LotteryDrawRecord
  remaining_draws: number
}

export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

/**
 * 获取当前活动与奖品展示
 */
export async function getActivity(): Promise<LotteryActivity> {
  const { data } = await apiClient.get<LotteryActivity>('/lottery/activity')
  return data
}

/**
 * 获取用户抽奖状态(次数/消耗/下一阈值)
 */
export async function getStatus(): Promise<LotteryStatus> {
  const { data } = await apiClient.get<LotteryStatus>('/lottery/status')
  return data
}

/**
 * 执行一次抽奖
 * @param idempotencyKey 幂等键(同一请求重试必须复用,防止重复扣次数)
 */
export async function draw(idempotencyKey: string): Promise<LotteryDrawResult> {
  const { data } = await apiClient.post<LotteryDrawResult>('/lottery/draw', {
    idempotency_key: idempotencyKey
  })
  return data
}

/**
 * 分页获取抽奖流水
 */
export async function getRecords(page = 1, pageSize = 20): Promise<Paginated<LotteryDrawRecord>> {
  const { data } = await apiClient.get<Paginated<LotteryDrawRecord>>('/lottery/records', {
    params: { page, page_size: pageSize }
  })
  return data
}

/**
 * 分页获取中奖记录(不含"谢谢参与")
 */
export async function getRewards(page = 1, pageSize = 20): Promise<Paginated<LotteryDrawRecord>> {
  const { data } = await apiClient.get<Paginated<LotteryDrawRecord>>('/lottery/rewards', {
    params: { page, page_size: pageSize }
  })
  return data
}

export const lotteryAPI = {
  getActivity,
  getStatus,
  draw,
  getRecords,
  getRewards
}

export default lotteryAPI
