<template>
  <section class="agent-usage" aria-labelledby="agent-usage-heading" :aria-busy="loading">
    <header class="usage-heading"><div><h3 id="agent-usage-heading">用量统计</h3><p>全部历史 · 仅统计 Agent 调用</p></div><button type="button" class="usage-refresh" aria-label="刷新用量统计" :disabled="loading" @click="load"><Icon name="refresh" size="sm" :class="{ spinning: loading }" /><span>{{ loading ? '更新中' : '刷新' }}</span></button></header>
    <div v-if="error" class="usage-error" role="alert"><p>用量统计读取失败，请重新加载。</p><button type="button" :disabled="loading" @click="load">重新加载</button></div>
    <div v-else-if="loading && !summary" class="usage-loading" role="status"><span class="loading-line" /><span class="loading-line" /><p>正在读取用量…</p></div>
    <template v-if="summary">
      <div class="usage-totals"><div class="usage-token-total"><span>Token 总用量</span><strong><AgentTokenUsage :usage="tokenTotals" label="Token 总用量" /></strong><small>{{ tokenTotals.missing_usage_records ? '仅统计已记录用量' : '输入、输出及缓存' }}</small></div><div><span>对话费用</span><strong>{{ money(summary.text.totals.cost_usd) }}</strong><small>{{ count(summary.text.totals.requests) }} 次调用</small></div><div><span>生图费用</span><strong>{{ money(summary.image.totals.cost_usd) }}</strong><small>{{ images(summary.image.totals) }}</small></div><div><span>合计费用</span><strong>{{ money(summary.total_cost_usd) }}</strong><small>按实际记录</small></div></div>
      <p v-if="!summary.text.groups.length && !summary.image.groups.length" class="usage-empty">还没有 Agent 用量。完成对话或生图后，会按分组显示在这里。</p>
      <template v-else>
        <section class="usage-section" aria-labelledby="agent-text-usage"><div class="section-heading"><h4 id="agent-text-usage">对话用量</h4><span>{{ summary.text.groups.length }} 个分组</span></div><div v-if="summary.text.groups.length" class="usage-table-scroll"><table><caption class="sr-only">对话分组、Token 和实际费用</caption><thead><tr><th scope="col">使用过的分组</th><th scope="col">Token</th><th scope="col">实际费用</th></tr></thead><tbody><tr v-for="group in summary.text.groups" :key="group.group_id ?? 'unknown'"><th scope="row"><span>{{ group.group_name }}</span><small>{{ count(group.requests) }} 次调用</small></th><td><AgentTokenUsage :usage="group" :label="group.group_name + ' Token 用量'" /><small v-if="group.missing_usage_records && group.missing_usage_records < group.requests">已记录</small></td><td class="usage-price">{{ money(group.cost_usd) }}</td></tr></tbody></table></div><p v-else class="usage-empty">暂无对话用量。</p><p v-if="summary.text.totals.missing_usage_records" class="usage-note">部分历史调用缺少 Token 记录，已确认的费用仍计入统计。</p></section>
        <section class="usage-section" aria-labelledby="agent-image-usage"><div class="section-heading"><h4 id="agent-image-usage">生图用量</h4><span>{{ summary.image.groups.length }} 个分组</span></div><div v-if="summary.image.groups.length" class="usage-table-scroll"><table><caption class="sr-only">生图分组、张数和总费用</caption><thead><tr><th scope="col">使用过的分组</th><th scope="col">张数</th><th scope="col">总费用</th></tr></thead><tbody><tr v-for="group in summary.image.groups" :key="group.group_id ?? 'unknown'"><th scope="row"><span>{{ group.group_name }}</span><small>{{ count(group.requests) }} 次调用</small></th><td>{{ images(group) }}<small v-if="group.missing_image_records && group.missing_image_records < group.requests">已记录</small></td><td class="usage-price">{{ money(group.cost_usd) }}</td></tr></tbody></table></div><p v-else class="usage-empty">暂无生图用量。</p></section>
      </template>
      <p v-if="pending" class="usage-pending" role="status">{{ count(pending) }} 个请求仍在核对，最终用量和费用可能更新。</p>
      <footer class="usage-footer"><p>费用不含报价与未执行任务；恢复图片、沿用已有结果不会重复计入。</p><span v-if="summary.updated_at">更新于 {{ new Date(summary.updated_at).toLocaleString('zh-CN', { hour12: false }) }}</span></footer>
    </template>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import AgentTokenUsage from './AgentTokenUsage.vue'
import { agentAPI, type AgentUsageGroup, type AgentUsageSummary } from '@/api/agent'
import { useAuthStore } from '@/stores/auth'
const auth = useAuthStore(), summary = ref<AgentUsageSummary>(), loading = ref(false), error = ref(false)
let controller: AbortController | undefined, generation = 0
const pending = computed(() => (summary.value?.text.totals.pending_requests || 0) + (summary.value?.image.totals.pending_requests || 0))
const tokenTotals = computed<AgentUsageGroup>(() => {
  const text = summary.value!.text.totals, image = summary.value!.image.totals
  return { ...text, requests: text.requests + image.requests, input_tokens: text.input_tokens + image.input_tokens, output_tokens: text.output_tokens + image.output_tokens, cache_read_tokens: text.cache_read_tokens + image.cache_read_tokens, cache_creation_tokens: text.cache_creation_tokens + image.cache_creation_tokens, total_tokens: text.total_tokens + image.total_tokens, missing_usage_records: text.missing_usage_records + image.missing_usage_records }
})
const count = (value: number) => new Intl.NumberFormat('zh-CN').format(value)
function money(value: string) {
  if (!/^-?\d+(?:\.\d{1,10})?$/.test(value)) return '—'
  const [whole, fraction = ''] = value.split('.')
  return `$${whole!.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}.${fraction.replace(/0+$/, '').padEnd(4, '0')}`
}

function images(group: AgentUsageGroup) { return group.requests > 0 && group.missing_image_records === group.requests ? '未记录' : `${count(group.image_count)} 张` }
async function load() {
  const token = ++generation, owner = auth.user?.id
  controller?.abort(); controller = new AbortController(); const request = controller
  if (!owner) { summary.value = undefined; loading.value = false; return }
  loading.value = true; error.value = false
  try { const value = await agentAPI.usage(request.signal); if (token === generation && owner === auth.user?.id && !request.signal.aborted) summary.value = value }
  catch { if (token === generation && owner === auth.user?.id && !request.signal.aborted) error.value = true }
  finally { if (token === generation) loading.value = false }
}
watch(() => auth.user?.id, () => { summary.value = undefined; void load() }, { immediate: true })
onBeforeUnmount(() => { ++generation; controller?.abort() })
</script>
<style scoped>
/* finesse · component: AgentUsage · register=product · tokens: inherited */
.agent-usage { color:var(--settings-ink,var(--workspace-ink)); min-height:390px; }
.usage-heading,.section-heading { display:flex; justify-content:space-between; align-items:center; gap:12px; }.usage-heading { margin-bottom:22px; }.usage-heading h3 { font-size:15px; font-weight:600; }.usage-heading p,.section-heading > span { font-size:11px; color:var(--settings-muted,var(--workspace-muted)); margin-top:5px; }
.usage-refresh,.usage-error button { display:inline-flex; align-items:center; justify-content:center; gap:6px; min-height:36px; min-width:68px; padding:7px 10px; border-radius:7px; color:var(--settings-ink,var(--workspace-ink)); background:var(--settings-soft,var(--workspace-soft)); font-size:12px; white-space:nowrap; }.usage-refresh:disabled,.usage-error button:disabled { opacity:.5; cursor:not-allowed; }.usage-refresh:active:not(:disabled),.usage-error button:active:not(:disabled) { transform:translateY(1px); }
.usage-totals { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:16px; padding-bottom:22px; border-bottom:1px solid var(--settings-line,var(--workspace-divider)); }.usage-totals > div { display:grid; align-content:start; gap:8px; min-width:0; }.usage-totals > div > span { color:var(--settings-muted,var(--workspace-muted)); font-size:11px; }.usage-totals strong { font-size:18px; font-weight:600; font-variant-numeric:tabular-nums; letter-spacing:-.02em; overflow-wrap:anywhere; }.usage-totals small { color:var(--settings-muted,var(--workspace-muted)); font-size:10px; }
.usage-section { margin-top:24px; }.section-heading { margin-bottom:10px; }.section-heading h4 { font-size:13px; font-weight:550; }.usage-table-scroll { overflow-x:auto; }table { width:100%; min-width:365px; table-layout:fixed; font-size:12px; }th,td { padding:12px 8px; border-bottom:1px solid var(--settings-line,var(--workspace-divider)); text-align:right; vertical-align:top; font-variant-numeric:tabular-nums; }thead th { color:var(--settings-muted,var(--workspace-muted)); font-size:10px; font-weight:500; }th:first-child { padding-left:0; width:46%; text-align:left; }tbody th { font-weight:500; overflow-wrap:anywhere; }th:last-child,td:last-child { padding-right:0; }.usage-price { white-space:nowrap; }td small,th small { display:block; color:var(--settings-muted,var(--workspace-muted)); font-size:10px; font-weight:400; margin-top:5px; }

.usage-empty,.usage-note,.usage-footer,.usage-pending { color:var(--settings-muted,var(--workspace-muted)); font-size:11px; line-height:1.8; }.usage-empty { padding:24px 0; }.usage-note { margin-top:9px; }.usage-pending { margin-top:18px; padding:10px 12px; border-radius:7px; background:var(--settings-soft,var(--workspace-soft)); }.usage-footer { margin-top:24px; }.usage-footer > span { display:block; margin-top:8px; font-size:10px; }
.usage-loading { min-height:170px; padding-top:18px; color:var(--settings-muted,var(--workspace-muted)); font-size:12px; }.loading-line { display:block; height:14px; margin-bottom:16px; width:80%; border-radius:5px; background:var(--settings-soft,var(--workspace-soft)); }.loading-line:nth-child(2) { width:55%; }.usage-error { display:flex; flex-wrap:wrap; gap:12px; align-items:center; padding:12px; margin-bottom:18px; border:1px solid var(--settings-line,var(--workspace-divider)); border-radius:8px; font-size:12px; }.usage-error p { flex:1; color:var(--settings-ink,var(--workspace-ink)); }
:where(button,summary):focus-visible { outline:2px solid var(--settings-accent,var(--workspace-accent)); outline-offset:3px; }
@media(hover:hover) { .usage-refresh:hover:not(:disabled),.usage-error button:hover:not(:disabled) { background:color-mix(in srgb,var(--settings-accent,var(--workspace-accent)) 10%,var(--settings-soft,var(--workspace-soft))); } }
.spinning { animation:usage-spin 1s linear infinite; }@keyframes usage-spin { to { transform:rotate(360deg); } }@media(prefers-reduced-motion:reduce) { .spinning { animation:none; } }
@media(max-width:640px) { .usage-totals { grid-template-columns:repeat(2,minmax(0,1fr)); gap:18px 12px; }.usage-totals strong { font-size:15px; }.usage-heading { margin-bottom:18px; } }
</style>
