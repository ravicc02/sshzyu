<template>
  <section id="agent-workspace-resources" class="workspace-resources" aria-label="工作区内容">
    <p class="resource-intro">当前对话的方案配置与生成结果</p>
    <section class="resource-group" aria-labelledby="workspace-plans-heading">
      <header><h4 id="workspace-plans-heading">方案配置</h4><span>{{ plans.length }}</span></header>
      <div v-if="plans.length" class="resource-list">
        <button v-for="plan in plans" :key="plan.id" type="button" class="resource-plan" @click="emit('plan', plan.id)">
          <span class="resource-kind"><Icon name="document" size="sm" /></span>
          <span class="resource-copy"><strong>{{ plan.title || '未命名方案' }}</strong><small>{{ plan.items.length }} 项 · v{{ plan.revision }} · {{ planLabel(plan.status) }}</small></span>
          <Icon name="chevronRight" size="sm" />
        </button>
      </div>
      <p v-else class="resource-empty">还没有方案草稿，可在对话中与 Agent 一起创建。</p>
    </section>
    <section class="resource-group" aria-labelledby="workspace-images-heading">
      <header><h4 id="workspace-images-heading">生成图片</h4><span>{{ images.length }}</span></header>
      <div v-if="images.length" class="resource-images">
        <button v-for="(image, index) in images" :key="image.id" v-asset-visible="image.id" type="button" class="resource-image" :aria-label="`查看生成图片 ${index + 1}`" @click="emit('image', image.id)">
          <span class="resource-thumbnail"><img v-if="image.url" :src="image.url" :alt="image.filename || `生成图片 ${index + 1}`" loading="lazy" @error="emit('imageError', image.id)" /><Icon v-else name="photograph" size="lg" /><span v-if="image.status === 'expired' || image.content_available === false" class="resource-image-state">已过期</span><span v-else-if="image.failed" class="resource-image-state">暂时无法读取</span></span>
          <span class="resource-image-name">{{ image.filename || `生成图片 ${index + 1}` }}</span>
        </button>
      </div>
      <p v-else class="resource-empty">生成的图片会保存在这里，点击即可查看。</p>
    </section>
  </section>
</template>
<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import { useAgentAssetVisibility } from '@/composables/useAgentAssetVisibility'
import type { AgentWorkPlan } from '@/api/agent'
import type { AgentAttachmentView } from '@/utils/agentAttachments'
defineProps<{ plans: AgentWorkPlan[]; images: AgentAttachmentView[] }>()
const emit = defineEmits<{ request: [id: string]; plan: [id: string]; image: [id: string]; imageError: [id: string] }>()
const vAssetVisible = useAgentAssetVisibility(id => emit('request', id))
function planLabel(status: AgentWorkPlan['status']) { return ({ draft: '草稿', ready: '待批准', submitted: '已提交', superseded: '历史版本' })[status] }
</script>
<style scoped>
/* finesse · component: AgentWorkspaceResources · register=product · tokens: inherited */
.workspace-resources { min-width:0; color:var(--workspace-ink); }.resource-intro { color:var(--workspace-muted); font-size:11px; line-height:1.7; margin-bottom:22px; }.resource-group + .resource-group { margin-top:26px; padding-top:20px; border-top:1px solid var(--workspace-divider); }.resource-group > header { display:flex; align-items:center; gap:8px; margin-bottom:12px; }.resource-group h4 { font-size:12px; font-weight:550; }.resource-group header > span { font-size:10px; color:var(--workspace-muted); font-variant-numeric:tabular-nums; }.resource-list { display:grid; gap:7px; }.resource-plan { display:flex; align-items:center; width:100%; gap:11px; padding:12px 10px; border:1px solid var(--workspace-divider); border-radius:9px; text-align:left; }.resource-kind { display:flex; flex-shrink:0; align-items:center; justify-content:center; width:31px; height:35px; border-radius:6px; color:var(--workspace-muted); background:var(--workspace-soft); }.resource-copy { display:grid; gap:6px; flex:1; min-width:0; }.resource-copy strong { overflow:hidden; white-space:nowrap; text-overflow:ellipsis; font-size:12px; font-weight:500; }.resource-copy small { font-size:10px; color:var(--workspace-muted); }.resource-plan > svg { flex-shrink:0; color:var(--workspace-muted); }.resource-empty { color:var(--workspace-muted); font-size:11px; line-height:1.9; padding:17px 12px; background:var(--workspace-soft); border-radius:8px; }
.resource-images { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:12px; }.resource-image { text-align:left; min-width:0; border-radius:8px; overflow:hidden; border:1px solid var(--workspace-divider); }.resource-thumbnail { display:flex; align-items:center; justify-content:center; position:relative; aspect-ratio:4/3; background:var(--workspace-soft); color:var(--workspace-muted); }.resource-thumbnail img { width:100%; height:100%; object-fit:contain; position:absolute; }.resource-image-name { display:block; padding:9px 8px; color:var(--workspace-muted); font-size:10px; overflow:hidden; white-space:nowrap; text-overflow:ellipsis; }.resource-image-state { position:absolute; bottom:6px; left:6px; padding:3px 5px; border-radius:4px; background:var(--workspace-surface); font-size:9px; }
button:focus-visible { outline:2px solid var(--workspace-accent); outline-offset:3px; }button:active { transform:translateY(1px); }
@media(hover:hover) { .resource-plan:hover,.resource-image:hover { border-color:var(--workspace-accent); background:var(--workspace-soft); } }
</style>
