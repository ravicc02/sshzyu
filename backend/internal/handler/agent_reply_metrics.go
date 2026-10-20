package handler

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/agent"
	"time"
)

// Reply metrics come from the authenticated user's gateway ledger. Missing
// telemetry stays absent; character counts are never presented as tokens.
func (h *AgentHandler) replyMetrics(ctx context.Context, user int64, conv string) map[string]*agent.ReplyMetrics {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result := map[string]*agent.ReplyMetrics{}
	rows, err := h.svc.Store.DB.QueryContext(ctx, `SELECT r.id,
 coalesce(sum(l.input_tokens) FILTER(WHERE o.kind='text'),0),
 coalesce(sum(l.output_tokens) FILTER(WHERE o.kind='text'),0),
 coalesce(sum(l.cache_read_tokens+l.cache_creation_tokens) FILTER(WHERE o.kind='text'),0),
 CASE WHEN count(l.duration_ms)=count(*) THEN sum(l.duration_ms) END,
 CASE WHEN count(*) FILTER(WHERE o.kind='text')=count(l.first_token_ms) FILTER(WHERE o.kind='text')
 AND count(*) FILTER(WHERE o.kind='text')=count(l.duration_ms) FILTER(WHERE o.kind='text')
 THEN sum(greatest(l.duration_ms-l.first_token_ms,0)) FILTER(WHERE o.kind='text') END
 FROM agent_runs r JOIN agent_conversations c ON c.id=r.conversation_id AND c.user_id=r.user_id AND c.deleted_at IS NULL
 JOIN agent_operations o ON o.run_id=r.id
 JOIN usage_logs l ON l.request_id=o.billing_request_id AND l.user_id=r.user_id
 AND l.api_key_id=(CASE WHEN o.kind='image' THEN r.snapshot->'config'->>'image_api_key_id' ELSE r.snapshot->'config'->>'model_api_key_id' END)::bigint
 WHERE r.user_id=$1 AND r.conversation_id=$2 GROUP BY r.id`, user, conv)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var m agent.ReplyMetrics
		if rows.Scan(&id, &m.InputTokens, &m.OutputTokens, &m.CacheTokens, &m.ElapsedMS, &m.GenerationMS) != nil {
			return map[string]*agent.ReplyMetrics{}
		}
		m.TotalTokens = m.InputTokens + m.OutputTokens + m.CacheTokens
		result[id] = &m
	}
	if rows.Err() != nil {
		return map[string]*agent.ReplyMetrics{}
	}
	return result
}
