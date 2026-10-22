import { apiClient } from '../client'
export interface MonitorGroup { id: number; name: string }
export interface MonitorAccount { id: number; name: string; platform: string; status: string }
export interface MonitorCandidate { id: string; base_url: string; provider_type: string; accounts: MonitorAccount[]; groups: MonitorGroup[]; monitored: boolean }
export interface MonitorSite { id: string; base_url: string; provider_type: 'sub2api'|'newapi'; account_id: number; display_name: string; enabled: boolean; warning_enabled: boolean; threshold: string|null; recharge_url: string; has_access_token: boolean; upstream_user_id: string; amount: string|null; currency: string; status: string; error_code: string; last_success_at: string|null; last_attempt_at: string|null; next_check_at: string|null; alert_active: boolean; revision: number; groups: MonitorGroup[] }
export interface MonitorSiteInput { candidate_id?: string; provider_type: 'sub2api'|'newapi'; account_id: number; display_name: string; enabled: boolean; warning_enabled: boolean; threshold: string|null; recharge_url: string; access_token?: string; clear_access_token?: boolean; upstream_user_id: string; revision?: number }
export interface MonitorEvent {id:number;site_id:string|null;kind:string;level:string;message:string;details:Record<string,unknown>;created_at:string}
export interface MonitorEvents {items:MonitorEvent[];total:number;page:number;page_size:number}
export interface MonitorChannel {app_id:string;chat_id:string;has_secret:boolean;revision:number}
export interface MonitorDelivery {id:string;status:string;message_id:string;error_code:string;is_test:boolean}
const base='/admin/upstream-monitor'
export const upstreamMonitorAPI={
 sites:async(signal?:AbortSignal)=>(await apiClient.get<MonitorSite[]>(base+'/sites',{signal})).data,
 candidates:async(signal?:AbortSignal)=>(await apiClient.get<MonitorCandidate[]>(base+'/candidates',{signal})).data,
 discover:async(ids:string[])=>(await apiClient.post<MonitorCandidate[]>(base+'/discovery',{candidate_ids:ids})).data,
 add:async(input:MonitorSiteInput)=>(await apiClient.post<MonitorSite>(base+'/sites',input)).data,
 update:async(id:string,input:MonitorSiteInput)=>(await apiClient.patch<MonitorSite>(base+'/sites/'+encodeURIComponent(id),input)).data,
 remove:async(site:MonitorSite)=>{await apiClient.delete(base+'/sites/'+encodeURIComponent(site.id),{params:{revision:site.revision}})},
 refresh:async(id:string)=>(await apiClient.post<MonitorSite>(base+'/sites/'+encodeURIComponent(id)+'/refresh')).data,
 events:async(params:{site_id?:string;kind?:string;page?:number},signal?:AbortSignal)=>(await apiClient.get<MonitorEvents>(base+'/events',{params,signal})).data,
 channel:async(signal?:AbortSignal)=>(await apiClient.get<MonitorChannel>(base+'/feishu',{signal})).data,
 saveChannel:async(input:{app_id:string;app_secret:string;chat_id:string;revision:number})=>(await apiClient.put<MonitorChannel>(base+'/feishu',input)).data,
 test:async(request_id:string,site_id?:string)=>(await apiClient.post<MonitorDelivery>(base+'/feishu/test',{request_id,site_id:site_id||''})).data,
}
export function monitorError(error:unknown):string {
 const e=error as {response?:{data?:{message?:string}};message?:string}
 const code=e.response?.data?.message||e.message||''
 const messages:Record<string,string>={MONITOR_UNAVAILABLE:'监控服务尚未启用。',MONITOR_AUTH_REQUIRED:'查询凭证未通过上游鉴权。',MONITOR_ACCOUNT_CREDENTIAL_REQUIRED:'需配置 New API 账户访问令牌；当前 key 未开放总余额读取。',MONITOR_API_REJECTED:'上游拒绝了查询，请检查账户访问令牌与用户 ID。',MONITOR_ACCOUNT_BALANCE_NOT_EXPOSED:'上游返回的不是账户总余额，未纳入预警。',MONITOR_ENDPOINT_UNAVAILABLE:'该类型的余额接口未找到。',MONITOR_BALANCE_MISSING:'响应中缺少有效总余额字段。',MONITOR_CURRENCY_UNCONFIRMED:'尚未确认上游额度的金额换算规则。',MONITOR_CURRENCY_MISSING:'上游没有返回可确认的余额币种。',MONITOR_CONFIG_CHANGED:'配置已发生变化，请刷新后再操作。',MONITOR_ALREADY_ADDED:'该上游已加入监控。',MONITOR_THRESHOLD_REQUIRED:'开启预警前请填写阈值。',MONITOR_INVALID_AMOUNT:'请输入有效金额，最多保留 12 位小数。',MONITOR_QUERY_BUSY:'该上游正在查询，请稍后刷新。',MONITOR_FEISHU_SECRET_REQUIRED:'首次配置或更换应用时需填写 App Secret。',MONITOR_INVALID_FEISHU_CONFIG:'请检查 App ID、群 ID 和应用密钥格式。',MONITOR_BALANCE_STALE:'请先成功刷新该上游的总余额。',MONITOR_NETWORK_ERROR:'连接上游失败，将按退避策略重试。',MONITOR_INVALID_RECHARGE_URL:'请输入有效的站点或充值地址，链接中不可携带密钥。',MONITOR_PROXY_UNAVAILABLE:'账号代理配置不可用。',MONITOR_FEISHU_NOT_CONFIGURED:'请先完成飞书通知设置。',MONITOR_ACCOUNT_BINDING_CHANGED:'账号或 Base URL 已变化，请重新选择查询凭证。'}
 return messages[code]||(code.startsWith('FEISHU_API_')?'飞书拒绝请求：'+code:code.startsWith('MONITOR_')?'操作未完成：'+code:'操作未完成，请稍后重试。')
}
