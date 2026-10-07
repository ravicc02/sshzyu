import type { AdminGroup, GroupPlatform } from '@/types'

/**
 * 返回某个平台 tab 下应展示的分组。
 *
 * 每个平台只展示自己 platform 的分组——聚合分组（composite）是一等平台，
 * 拥有自己的 tab，不再镜像到各个具体平台 tab。
 */
export function groupsForPlatform(allGroups: AdminGroup[], platform: GroupPlatform): AdminGroup[] {
  return allGroups.filter(g => g.platform === platform)
}
