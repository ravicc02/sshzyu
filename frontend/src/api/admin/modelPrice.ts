/**
 * Admin Model Price API endpoints
 * 「模型价格」页：按分组列出模型并配置单模型独立计费倍率（隐式配置，界面不显示数值）。
 */

import { apiClient } from '../client'

/** 模型价格页的单个模型条目；custom_rate 表示是否已配独立倍率（界面不展示具体数值） */
export interface GroupModelPriceEntry {
  model: string
  custom_rate: boolean
}

/** 一个分组及其可配置模型清单 */
export interface GroupModelPriceGroup {
  id: number
  name: string
  platform: string
  /** 分组默认计费倍率（页面在分组名旁展示；模型级独立倍率不返回数值） */
  rate_multiplier: number
  models: GroupModelPriceEntry[]
}

export interface GroupModelPriceList {
  models: GroupModelPriceEntry[]
}

/**
 * 列出某分组下的模型及其独立倍率配置状态
 * @param groupId - 分组 ID
 */
export async function listGroupModelPrices(groupId: number): Promise<GroupModelPriceList> {
  const { data } = await apiClient.get<GroupModelPriceList>(`/admin/groups/${groupId}/model-prices`)
  return data
}

/**
 * 一次性列出所有活跃分组及其可配置模型与配置状态（「模型价格」页不按分组筛选）。
 */
export async function listAllGroupModelPrices(): Promise<GroupModelPriceGroup[]> {
  const { data } = await apiClient.get<{ groups: GroupModelPriceGroup[] }>('/admin/model-prices')
  return data.groups ?? []
}

/**
 * 设置某模型的独立计费倍率（隐式配置，替代分组默认，仅对该模型生效）
 * @param groupId - 分组 ID
 * @param model - 计费模型名
 * @param rateMultiplier - 倍率（> 0）
 */
export async function setModelRateMultiplier(
  groupId: number,
  model: string,
  rateMultiplier: number
): Promise<void> {
  await apiClient.put(
    `/admin/groups/${groupId}/model-prices/${encodeURIComponent(model)}/rate-multiplier`,
    { rate_multiplier: rateMultiplier }
  )
}

/**
 * 清除某模型的独立倍率（回落分组默认）
 * @param groupId - 分组 ID
 * @param model - 计费模型名
 */
export async function clearModelRateMultiplier(groupId: number, model: string): Promise<void> {
  await apiClient.delete(
    `/admin/groups/${groupId}/model-prices/${encodeURIComponent(model)}/rate-multiplier`
  )
}

export const modelPriceAPI = {
  listGroupModelPrices,
  listAllGroupModelPrices,
  setModelRateMultiplier,
  clearModelRateMultiplier
}

export default modelPriceAPI