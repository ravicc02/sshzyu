import DOMPurify from 'dompurify'
import { marked } from 'marked'

export const agentStates: Record<string, { label: string; detail: string; tone: string }> = {
  queued: { label: '等待执行', detail: '请求已保存，正在等待可用执行位。', tone: 'active' },
  running: { label: '正在思考', detail: '助手正在处理你的消息，回复完成后会显示在这里。', tone: 'active' },
  waiting_user: { label: '等待你的回答', detail: '请确认上方问题，回答后继续执行本次任务。', tone: 'neutral' },
  waiting_tool: { label: '准备生成图片', detail: '正在检查生图配置、执行许可和费用。', tone: 'active' },
  executing_tool: { label: '正在生成图片', detail: '图片完成后会直接出现在对话中，你可以先停留在当前页面。', tone: 'active' },
  resuming_model: { label: '正在整理结果', detail: '已收到工具结果，正在继续处理你的请求。', tone: 'active' },
  reconciling: { label: '正在恢复结果', detail: '正在读取已保存的响应。', tone: 'warning' },
  completed: { label: '已完成', detail: '', tone: 'success' },
  partial_success: { label: '部分完成', detail: '已生成的内容已保留，后续步骤未全部完成。', tone: 'warning' },
  failed: { label: '请求未完成', detail: '请查看下方原因，再决定是否重新发送。', tone: 'error' },
  expired: { label: '执行超时', detail: '本次任务已结束，已保存的内容仍可查看。', tone: 'error' },
  cancelled: { label: '已停止', detail: '已返回的内容会保留，已发生的费用仍按实际结算。', tone: 'neutral' },
}
export function agentState(status?: string, errorCode?: string) {
  if (errorCode === 'AGENT_RECONCILIATION_TIMEOUT') return { label: status === 'partial_success' ? '部分完成' : '请求未完成', detail: '', tone: status === 'partial_success' ? 'warning' : 'error' }
  return agentStates[status || ''] || { label: '正在处理', detail: '正在获取最新状态。', tone: 'active' } }
// Display legacy generic names consistently without rewriting conversation data.
export function agentConfigCardTitle(title?: string): string {
  const value = title?.trim()
  return !value || ['图片生成方案', '视觉方案', '未命名方案'].includes(value) ? '配置卡' : value
}
export function agentError(code?: string, failure?: { message: string; status_code?: number; upstream_status_code?: number }) {
  if (failure?.message) {
    const status = failure.upstream_status_code || failure.status_code
    return `${status && status >= 400 ? `HTTP ${status} · ` : ''}${failure.message}`
  }
  const errors: Record<string, string> = {
    AGENT_WORK_PLAN_INVALID: '配置需包含1至8项完整内容及有效的规格、参考图。',
    AGENT_WORK_REVIEW_REQUIRED: '配置已变化或报价过期，请重新保存核价后确认。',
    AGENT_IMAGE_KEY_REQUIRED: '请在设置中单独选择生图key和模型，再保存核价。',
    AGENT_UPSTREAM_REJECTED: '上游接口拒绝了本次请求。',
    AGENT_RESPONSE_INCOMPLETE: '上游响应中断，未收到完整结果。',
    AGENT_RESPONSE_TOO_LARGE: '上游响应超过大小限制。',
    AGENT_BILLING_UNCONFIRMED: '接口调用已结束，但费用记录暂未返回。',
    AGENT_UPSTREAM_CONNECTION_FAILED: '上游连接失败，请检查当前账号的出站代理或网络。请求未发送到模型服务，无需等待核对。',
    AGENT_MODEL_FIRST_RESPONSE_TIMEOUT: '模型在 60 秒内未返回内容，本次请求已超时结束。',
    AGENT_MODEL_IDLE_TIMEOUT: '模型超过 60 秒未返回新内容，本次请求已超时结束。',
    AGENT_MODEL_TIMEOUT: '模型调用超过 120 秒，本次请求已超时结束。',
    AGENT_IMAGE_TIMEOUT: '图片生成超过 10 分钟，本次请求已超时结束。',
    AGENT_RUN_TIMEOUT: '本次任务已超时结束，已返回的内容会保留。',
    AGENT_REQUEST_CANCELLED: '本次请求已停止。',
    AGENT_QUESTION_TIMEOUT: '等待回答已超时，本次任务已结束，可以发送新请求。',
    AGENT_INVALID_ANSWER: '请填写必要信息，并检查回答长度。',
    AGENT_QUESTION_LIMIT: '本次澄清次数已达上限，请把具体需求整理后重新发送。',
    AGENT_FILE_SIZE_LIMIT: '文件不能为空，且不能超过 10 MiB。',
    AGENT_MODEL_VISION_UNSUPPORTED: '当前对话模型不支持图片输入，请切换到支持识图的模型。',
    AGENT_VISION_IMAGE_TOO_LARGE: '图片尺寸过大，请缩小或裁剪后重新上传。',
    AGENT_INVALID_IMAGE_REFERENCE: '图片引用无效，请重新上传图片。',
    AGENT_RETRY_NOT_LATEST: '此回复之后已有新消息，请重试最新一轮。',
    AGENT_RETRY_UNSETTLED: '原调用未取得完整结果，无法直接重试。请发送新消息明确发起新的请求。',
    AGENT_RETRY_CONTEXT_UNAVAILABLE: '该历史请求缺少可恢复的上下文，请重新发送消息。',
    AGENT_INVALID_FILE: '文件内容与格式不符，或文件已损坏，请检查后重新上传。',
    AGENT_UNSUPPORTED_FILE: '暂不支持此格式，请使用图片、PDF、文本或 DOCX/XLSX/PPTX 文件。',
    AGENT_MODEL_INCOMPLETE: '模型未完整返回，本阶段未完成。',
    AGENT_TOOL_NOT_ALLOWED: '当前未启用此工具，无法执行。',
    AGENT_DISK_PRESSURE: 'Agent 所在磁盘空间不足，请稍后再试；已有结果与费用记录会保留。',
    AGENT_STORAGE_CAPACITY: 'Agent 存储空间已达上限，请等待过期内容清理后重试。',
    AGENT_STORAGE_UNAVAILABLE: 'Agent 存储暂时不可用，请稍后再试。',
    AGENT_STORAGE_QUOTA_EXCEEDED: 'Agent 存储额度不足，无法保存或预留结果空间；已有文件和费用记录已保留。',
    AGENT_IMAGE_RESULT_TOO_LARGE: '上游图片超过单图大小限制，已保留响应和费用记录，不会自动重新生成。',
    AGENT_IMAGE_RESULT_INVALID: '上游返回的图片格式无效，已保留响应和费用记录，不会自动重新生成。',
    AGENT_IMAGE_RECOVERY_UNAVAILABLE: '没有可恢复的已保存图片响应，本次操作不会重新生成图片。',
    AGENT_RESULT_RECOVERY_PENDING: '未能恢复已保存的图片，请稍后手动重试恢复。',
    AGENT_IMAGE_AUTHORIZATION_REQUIRED: '请明确需要生成或修改的图片，状态查询不会发起新的生图请求。',
    AGENT_ASSET_STORAGE_FAILED: '接口已返回图片，但文件保存失败。',
    AGENT_RESULT_STORAGE_FAILED: '无法读取已保存的响应文件。',
    AGENT_GATEWAY_REJECTED: '请求在提交前被平台拒绝，请检查模型、权限或额度。',
    AGENT_WORKER_INTERRUPTED: '执行进程中断，本次请求已结束。',
    AGENT_BUDGET_EXCEEDED: '此历史任务曾被旧预算规则中断，请重新核价后继续。',
    AGENT_UNBOUNDED_PRICING: '当前模型无法预估费用，请在设置中更换模型或检查价卡。',
    AGENT_WORK_ITEMS_FAILED: '部分图片未生成完成，可在对应内容下单独重试。',
    AGENT_PRICING_CHANGED: '选中账号的价格高于预估，本次调用已停止。',
    AGENT_PRICING_UNAVAILABLE: '当前模型缺少可用价卡，请检查分组或渠道价格。',
    AGENT_IMAGE_FIXED_PRICE_REQUIRED: '当前生图模型缺少按张价格，请选择其他生图模型。',
    UPSTREAM_RESULT_UNKNOWN: '未收到完整响应，本次请求已结束。',
    AGENT_RECONCILIATION_TIMEOUT: '请求未完成，本次调用已结束。',
    AGENT_RUN_BUSY: '你还有一个任务正在执行，请先查看该对话。',
    AGENT_MODEL_NOT_ALLOWED: '当前 API key 不支持所选模型，请检查设置。',
    AGENT_ACTIVE_RUN_EXISTS: '你还有一个任务正在执行，请先查看该对话。',
  }
  return errors[code || ''] || (code ? `错误代码：${code}` : '')
}
// Model output is untrusted. Disallow remote media, raw interactive HTML and
// styling. Links cannot open a new tab or carry script URLs after sanitization.
export function renderAgentMarkdown(text: string): string {
  return DOMPurify.sanitize(marked.parse(text, { async: false, breaks: true }), {
    ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'del', 'ul', 'ol', 'li', 'blockquote', 'pre', 'code', 'h1', 'h2', 'h3', 'h4', 'hr', 'a', 'table', 'thead', 'tbody', 'tr', 'th', 'td'],
    ALLOWED_ATTR: ['href', 'title', 'start'],
  })
}

// Use one vocabulary for stage headings and individual tool rows.
export function agentToolLabel(name: string): string {
  return ({ ask_user: '询问用户', generate_image: '生成图片', save_work_plan: '保存草稿', update_draft: '更新草稿', present_review: '展示配置卡', inspect_assets: '查看素材', get_job: '查看任务', finish_task: '结束任务' } as Record<string, string>)[name] || name || '工具调用'
}
