export default {
  lottery: {
    title: 'Lottery Management',
    description: 'Review draw records and adjust lottery settings',
    tabDraws: 'Draw Records',
    tabConfig: 'Settings',
    filterUserId: 'User ID',
    userIdLabel: 'User',
    userDeleted: 'Deleted',
    prizeNone: 'No prize',
    noDraws: 'No draw records yet',
    sortNewest: 'Newest first',
    sortOldest: 'Oldest first',
    sourceLabels: {
      first: 'First draw',
      threshold: 'Threshold',
      manual: 'Manual'
    },
    fulfillmentLabels: {
      granted: 'Granted',
      pending: 'Pending',
      failed: 'Failed'
    },
    retryFulfillment: 'Retry',
    retrySuccess: 'Fulfillment retry succeeded',
    columns: {
      id: 'ID',
      user: 'Winner',
      prize: 'Prize',
      source: 'Source',
      fulfillment: 'Fulfillment',
      createdAt: 'Drawn At',
      actions: 'Actions'
    },
    activitySection: 'Activity',
    activityName: 'Activity Name',
    activityStatus: 'Status',
    activityStatusHint: 'paused stops draws immediately; ended closes the activity',
    startsAt: 'Starts At',
    endsAt: 'Ends At',
    rulesVersion: 'Rules Version',
    noActivity: 'No activity',
    activityStatusLabels: {
      draft: 'Draft',
      active: 'Active',
      paused: 'Paused',
      ended: 'Ended'
    },
    prizeSection: 'Prizes',
    prizeDisabled: 'Disabled',
    noPrizes: 'No prizes in this activity',
    stockUnlimited: 'Unlimited',
    tierOverrideHint: 'Has tier overrides',
    prizeName: 'Prize Name',
    prizeType: 'Type',
    prizeTypeLabels: {
      none: 'No prize',
      balance_bonus: 'Balance',
      quota: 'Quota'
    },
    prizeValue: 'Value($)',
    prizeValueHint: 'Applies to balance/quota prizes only',
    prizeWeight: 'Base Weight',
    prizeWeightHint: '0 excludes the prize unless overridden by a tier weight',
    prizeMinTier: 'Min Tier',
    prizeMinTierHint: 'Users below this tier cannot win this prize',
    prizeStock: 'Stock',
    prizeStockHint: '-1 means unlimited; shown as issued/total',
    prizeSortOrder: 'Sort',
    prizeEnabled: 'Enabled',
    editPrizeTitle: 'Edit Prize',
    saveSuccess: 'Saved and effective immediately',
    tierNames: {
      0: 'Bronze',
      1: 'Silver',
      2: 'Gold',
      3: 'Diamond',
      4: 'King'
    },
    tierWeightsSection: 'Tier Weight Overrides',
    tierWeightFallback: 'Fallback to base weight',
    tierWeightsHint: 'Leave empty to fall back to the base weight; 0 excludes this tier',
    adjustSection: 'Adjust Draws',
    adjustDelta: 'Delta',
    adjustApply: 'Apply',
    adjustHint: 'Positive grants draws, negative revokes; for manual compensation',
    adjustSuccess: 'Draws adjusted'
  }
}
