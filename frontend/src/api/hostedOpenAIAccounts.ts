import { apiClient } from './client'

export interface HostedQuotaWindow {
  used_percent: number
  reset_at?: string
  window_minutes?: number
}

export interface HostedQuotaSnapshot {
  plan_type?: string
  account_type: string
  five_hour?: HostedQuotaWindow
  weekly?: HostedQuotaWindow
  fetched_at?: string
  error?: string
}

export interface HostedOpenAIAccount {
  id: number
  user_id: number
  account_id: number
  group_id: number
  subscription_id?: number | null
  proxy_id?: number | null
  account_name: string
  model: string
  models: string[]
  supports_messages: boolean
  status: 'active' | 'suspended' | 'archived'
  account_email: string
  plan_type: string
  last_quota_refreshed_at?: string | null
  created_at: string
  updated_at: string
  user_name: string
  user_email: string
  group_name: string
  proxy_name: string
  account_status: string
  quota: HostedQuotaSnapshot
}

export interface HostedProxyOption {
  id: number
  name: string
}

export interface HostedOpenAIOptions {
  proxies: HostedProxyOption[]
  suggested_models: string[]
}

export interface HostedAuthURLResult {
  auth_url: string
  session_id: string
}

export interface HostedCreatePayload {
  session_id: string
  callback_url?: string
  code?: string
  state?: string
  redirect_uri?: string
  account_name: string
  models: string[]
  model?: string
  proxy_id?: number | null
  supports_messages: boolean
}

export interface HostedConfigPayload {
  account_name?: string
  models?: string[]
  model?: string
  proxy_id?: number | null
  supports_messages?: boolean
}

export async function listHostedOpenAIAccounts(): Promise<HostedOpenAIAccount[]> {
  const { data } = await apiClient.get<HostedOpenAIAccount[]>('/hosted-openai-accounts')
  return data
}

export async function getHostedOpenAIOptions(): Promise<HostedOpenAIOptions> {
  const { data } = await apiClient.get<HostedOpenAIOptions>('/hosted-openai-accounts/options')
  return data
}

export async function generateHostedOpenAIAuthURL(payload: {
  proxy_id?: number | null
  redirect_uri?: string
}): Promise<HostedAuthURLResult> {
  const { data } = await apiClient.post<HostedAuthURLResult>('/hosted-openai-accounts/auth-url', payload)
  return data
}

export async function createHostedOpenAIAccount(payload: HostedCreatePayload): Promise<HostedOpenAIAccount> {
  const { data } = await apiClient.post<HostedOpenAIAccount>('/hosted-openai-accounts', payload)
  return data
}

export async function updateHostedOpenAIAccount(
  id: number,
  payload: HostedConfigPayload
): Promise<HostedOpenAIAccount> {
  const { data } = await apiClient.put<HostedOpenAIAccount>(`/hosted-openai-accounts/${id}`, payload)
  return data
}

export async function refreshHostedOpenAIQuota(id: number): Promise<HostedOpenAIAccount> {
  const { data } = await apiClient.post<HostedOpenAIAccount>(`/hosted-openai-accounts/${id}/refresh-quota`)
  return data
}

export async function reauthorizeHostedOpenAIAccount(
  id: number,
  payload: Pick<HostedCreatePayload, 'session_id' | 'callback_url' | 'code' | 'state' | 'redirect_uri'>
): Promise<HostedOpenAIAccount> {
  const { data } = await apiClient.post<HostedOpenAIAccount>(`/hosted-openai-accounts/${id}/reauthorize`, payload)
  return data
}

export async function listAdminHostedOpenAIAccounts(filters?: {
  search?: string
  status?: string
}): Promise<HostedOpenAIAccount[]> {
  const { data } = await apiClient.get<HostedOpenAIAccount[]>('/admin/hosted-openai-accounts', {
    params: filters
  })
  return data
}

export async function updateAdminHostedOpenAIAccount(
  id: number,
  payload: HostedConfigPayload
): Promise<HostedOpenAIAccount> {
  const { data } = await apiClient.put<HostedOpenAIAccount>(`/admin/hosted-openai-accounts/${id}`, payload)
  return data
}

export async function refreshAdminHostedOpenAIQuota(id: number): Promise<HostedOpenAIAccount> {
  const { data } = await apiClient.post<HostedOpenAIAccount>(`/admin/hosted-openai-accounts/${id}/refresh-quota`)
  return data
}

export async function setAdminHostedOpenAIStatus(
  id: number,
  status: HostedOpenAIAccount['status']
): Promise<HostedOpenAIAccount> {
  const { data } = await apiClient.post<HostedOpenAIAccount>(`/admin/hosted-openai-accounts/${id}/status`, {
    status
  })
  return data
}
