import { apiClient } from '../client'

export interface AccountSchedulerRow {
  id: number
  name: string
  platform: string
  credential_type: string
  oauth: boolean
  available: boolean
  availability_reason: string
  testable: boolean
  dispatch_count: number
  active_dispatches: number
  degradation_rate?: number
  sample_count: number
  pass_count: number
  degraded_count: number
  error_count: number
  latest_status: string
  latest_checked_at?: string
  average_latency_ms: number
  rank: number
  score: number
  score_weight: number
  degradation_factor: number
  lowest_degradation: boolean
}

export interface AccountSchedulerRunProgress {
  group_id: number
  running: boolean
  total: number
  completed: number
  passed: number
  degraded: number
  failed: number
  skipped: number
  current_account_id?: number
  current_account_name?: string
  started_at: string
  finished_at?: string
}

export interface AccountSchedulerGroup {
  id: number
  name: string
  platform: string
  degradation_supported: boolean
  status: string
  degradation_enabled: boolean
  degradation_factor_enabled: boolean
  interval_seconds: number
  test_model: string
  test_reasoning_effort: string
  score_max?: number
  model_candidates: string[]
  oauth_first: boolean
  lowest_account_id?: number
  test_progress?: AccountSchedulerRunProgress
  accounts: AccountSchedulerRow[]
}

export interface AccountDegradationProbe {
  id: number
  account_id: number
  account_name: string
  group_id: number
  model: string
  reasoning_effort: string
  status: 'pass' | 'degraded' | 'error'
  response_text: string
  error_message: string
  latency_ms: number
  checked_at: string
}

export interface AccountSchedulerOverview {
  scoring_mode: string
  oauth_first: boolean
  priority_ignored: boolean
  advanced_enabled: boolean
  tested_groups: number
  enabled_groups: number
  total_accounts: number
  available_accounts: number
  groups: AccountSchedulerGroup[]
  recent_probes: AccountDegradationProbe[]
  generated_at: string
}

export interface AccountSchedulerSettingInput {
  group_id: number
  degradation_enabled?: boolean
  degradation_factor_enabled?: boolean
  interval_seconds?: number
  test_model?: string
  test_reasoning_effort?: string
}

export async function getOverview(): Promise<AccountSchedulerOverview> {
  const { data } = await apiClient.get<AccountSchedulerOverview>('/admin/account-scheduler/overview')
  return data
}

export async function updateSetting(
  input: AccountSchedulerSettingInput,
): Promise<void> {
  await apiClient.put('/admin/account-scheduler/settings', input)
}

export async function runNow(input: {
  group_id: number
  account_id?: number
  model?: string
  reasoning_effort?: string
}): Promise<{
  run: number
  skipped: number
  failed: number
  results: AccountDegradationProbe[]
  errors?: string[]
}> {
  const { data } = await apiClient.post('/admin/account-scheduler/run', input, {
    timeout: 5 * 60 * 1000,
  })
  return data
}

export const accountSchedulerAPI = {
  getOverview,
  updateSetting,
  runNow,
}

export default accountSchedulerAPI
