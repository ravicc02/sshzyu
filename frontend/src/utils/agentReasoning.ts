export const agentReasoningEfforts = ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const
export type AgentReasoningEffort = typeof agentReasoningEfforts[number]
export const agentReasoningLabels: Record<AgentReasoningEffort, string> = { none: '关闭', minimal: '极低', low: '低', medium: '中', high: '高', xhigh: '超高', max: '最高' }
