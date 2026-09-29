export default {
  lottery: {
    title: '抽奖管理',
    description: '查看中奖记录并调整抽奖活动参数',
    tabDraws: '中奖记录',
    tabConfig: '参数设置',
    filterUserId: '用户 ID',
    userIdLabel: '用户',
    userDeleted: '已删除',
    prizeNone: '谢谢参与',
    noDraws: '暂无抽奖记录',
    sortNewest: '最新在前',
    sortOldest: '最早在前',
    sourceLabels: {
      first: '首抽',
      threshold: '阶梯解锁',
      manual: '手动补发'
    },
    fulfillmentLabels: {
      granted: '已发放',
      pending: '发放中',
      failed: '发放失败'
    },
    retryFulfillment: '重试发放',
    retrySuccess: '发放重试成功',
    columns: {
      id: '流水号',
      user: '中奖用户',
      prize: '奖品',
      source: '来源',
      fulfillment: '发放状态',
      createdAt: '抽奖时间',
      actions: '操作'
    },
    activitySection: '活动配置',
    activityName: '活动名称',
    activityStatus: '活动状态',
    activityStatusHint: 'paused 立即暂停抽奖,ended 结束活动',
    startsAt: '开始时间',
    endsAt: '结束时间',
    rulesVersion: '规则版本',
    noActivity: '暂无活动',
    activityStatusLabels: {
      draft: '草稿',
      active: '进行中',
      paused: '已暂停',
      ended: '已结束'
    },
    prizeSection: '奖品配置',
    prizeDisabled: '已停用',
    noPrizes: '该活动暂无奖品',
    stockUnlimited: '无限',
    tierOverrideHint: '含阶梯覆盖权重',
    prizeName: '奖品名称',
    prizeType: '奖品类型',
    prizeTypeLabels: {
      none: '谢谢参与',
      balance_bonus: '余额奖励',
      quota: '使用额度'
    },
    prizeValue: '面值($)',
    prizeValueHint: '仅余额/额度类奖品生效',
    prizeWeight: '基础权重',
    prizeWeightHint: '0 表示不参与随机(未被阶梯覆盖时)',
    prizeMinTier: '最低阶梯',
    prizeMinTierHint: '低于该阶梯的用户不可中此奖',
    prizeStock: '库存',
    prizeStockHint: '-1 表示无限库存;展示为 已发放/总配额',
    prizeSortOrder: '排序',
    prizeEnabled: '启用该奖品',
    editPrizeTitle: '编辑奖品',
    saveSuccess: '保存成功,已即时生效',
    tierNames: {
      0: '青铜',
      1: '白银',
      2: '黄金',
      3: '钻石',
      4: '王者'
    },
    tierWeightsSection: '阶梯覆盖权重',
    tierWeightFallback: '回退基础权重',
    tierWeightsHint: '留空表示该阶梯回退到基础权重;填 0 表示该阶梯不可中此奖',
    adjustSection: '抽奖次数调整',
    adjustDelta: '次数增减',
    adjustApply: '执行调整',
    adjustHint: '正数补发、负数回收;用于人工补偿场景',
    adjustSuccess: '抽奖次数已调整'
  }
}
