package handler

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type agentUsageGroup struct {
	GroupID             *int64 `json:"group_id,omitempty"`
	GroupName           string `json:"group_name,omitempty"`
	Requests            int64  `json:"requests"`
	InputTokens         int64  `json:"input_tokens"`
	OutputTokens        int64  `json:"output_tokens"`
	CacheReadTokens     int64  `json:"cache_read_tokens"`
	CacheCreationTokens int64  `json:"cache_creation_tokens"`
	TotalTokens         int64  `json:"total_tokens"`
	ImageCount          int64  `json:"image_count"`
	CostUSD             string `json:"cost_usd"`
	PendingRequests     int64  `json:"pending_requests"`
	MissingUsageRecords int64  `json:"missing_usage_records"`
	MissingImageRecords int64  `json:"missing_image_records"`
}
type agentUsageSection struct {
	Groups []agentUsageGroup `json:"groups"`
	Totals agentUsageGroup   `json:"totals"`
}
type agentUsageSummary struct {
	TotalCostUSD string            `json:"total_cost_usd"`
	Scope        string            `json:"scope"`
	Text         agentUsageSection `json:"text"`
	Image        agentUsageSection `json:"image"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

func emptyAgentUsageSection() agentUsageSection {
	return agentUsageSection{Groups: []agentUsageGroup{}, Totals: agentUsageGroup{CostUSD: "0.0000000000"}}
}

// A single read snapshot of the owner's Agent operations. Billing identities
// select at most one owned gateway row per operation; recovered assets and
// inherited work results never introduce another operation or charge.
const agentUsageQuery = `WITH owned_operations AS (
 SELECT o.*,r.user_id,r.snapshot,
 CASE WHEN o.kind='image' THEN (r.snapshot->'config'->>'image_api_key_id')::bigint
 ELSE (r.snapshot->'config'->>'model_api_key_id')::bigint END AS bound_key_id
 FROM agent_operations o JOIN agent_runs r ON r.id=o.run_id
 JOIN agent_conversations c ON c.id=r.conversation_id AND c.user_id=r.user_id
 WHERE r.user_id=$1 AND o.kind IN ('text','image')
), usage_rows AS (
 SELECT o.kind,coalesce(l.group_id,NULLIF((CASE WHEN o.kind='image'
 THEN o.snapshot->>'image_group_id' ELSE o.snapshot->>'model_group_id' END)::bigint,0)) AS group_id,
 l.id AS usage_id,coalesce(l.input_tokens,0)::bigint AS input_tokens,
 coalesce(l.output_tokens,0)::bigint AS output_tokens,
 coalesce(l.cache_read_tokens,0)::bigint AS cache_read_tokens,
 coalesce(l.cache_creation_tokens,0)::bigint AS cache_creation_tokens,
 CASE WHEN o.kind='image' THEN greatest(coalesce(l.image_count,0),CASE WHEN a.present THEN 1 ELSE 0 END) ELSE 0 END AS image_count,
 coalesce(l.actual_cost,o.cost,0) AS cost,
 o.status IN ('dispatching','accepted','result_unknown') AS pending,
 o.kind='image' AND l.id IS NULL AND NOT a.present AS missing_image
 FROM owned_operations o
 LEFT JOIN LATERAL (
 SELECT l.id,l.group_id,l.input_tokens,l.output_tokens,l.cache_read_tokens,l.cache_creation_tokens,l.image_count,l.actual_cost
 FROM usage_logs l WHERE l.request_id=o.billing_request_id AND l.user_id=o.user_id AND l.api_key_id=o.bound_key_id
 ORDER BY l.id DESC LIMIT 1
 ) l ON true
 CROSS JOIN LATERAL (SELECT EXISTS(SELECT 1 FROM agent_assets a WHERE a.operation_id=o.id AND a.user_id=o.user_id) AS present) a
 WHERE l.id IS NOT NULL OR o.status IN ('dispatching','accepted','result_unknown','succeeded') OR coalesce(o.cost,0)>0
)
 SELECT u.kind,u.group_id,g.name,count(*),
 coalesce(sum(u.input_tokens),0),coalesce(sum(u.output_tokens),0),
 coalesce(sum(u.cache_read_tokens),0),coalesce(sum(u.cache_creation_tokens),0),
 coalesce(sum(u.image_count),0),coalesce(sum(u.cost),0)::text,
 count(*) FILTER(WHERE u.pending),count(*) FILTER(WHERE u.usage_id IS NULL),count(*) FILTER(WHERE u.missing_image)
 FROM usage_rows u LEFT JOIN groups g ON g.id=u.group_id
 GROUP BY u.kind,u.group_id,g.name ORDER BY u.kind,sum(u.cost) DESC,u.group_id NULLS LAST`

func (h *AgentHandler) usageSummary(ctx context.Context, user int64) (agentUsageSummary, error) {
	out := agentUsageSummary{Scope: "all_agent_history", TotalCostUSD: "0.0000000000", Text: emptyAgentUsageSection(), Image: emptyAgentUsageSection(), UpdatedAt: time.Now().UTC()}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := h.svc.Store.DB.QueryContext(ctx, agentUsageQuery, user)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var group sql.NullInt64
		var name sql.NullString
		var item agentUsageGroup
		if err = rows.Scan(&kind, &group, &name, &item.Requests, &item.InputTokens, &item.OutputTokens, &item.CacheReadTokens, &item.CacheCreationTokens, &item.ImageCount, &item.CostUSD, &item.PendingRequests, &item.MissingUsageRecords, &item.MissingImageRecords); err != nil {
			return out, err
		}
		cost, e := decimal.NewFromString(item.CostUSD)
		if e != nil {
			return out, e
		}
		item.CostUSD = cost.StringFixed(10)
		if group.Valid {
			v := group.Int64
			item.GroupID = &v
		}
		item.GroupName = name.String
		if item.GroupName == "" {
			if group.Valid {
				item.GroupName = fmt.Sprintf("历史分组 #%d", group.Int64)
			} else {
				item.GroupName = "未记录分组"
			}
		}
		item.TotalTokens = item.InputTokens + item.OutputTokens + item.CacheReadTokens + item.CacheCreationTokens
		section := &out.Text
		if kind == "image" {
			section = &out.Image
		}
		section.Groups = append(section.Groups, item)
		total := &section.Totals
		total.Requests += item.Requests
		total.InputTokens += item.InputTokens
		total.OutputTokens += item.OutputTokens
		total.CacheReadTokens += item.CacheReadTokens
		total.CacheCreationTokens += item.CacheCreationTokens
		total.TotalTokens += item.TotalTokens
		total.ImageCount += item.ImageCount
		total.PendingRequests += item.PendingRequests
		total.MissingUsageRecords += item.MissingUsageRecords
		total.MissingImageRecords += item.MissingImageRecords
		previous, e := decimal.NewFromString(total.CostUSD)
		if e != nil {
			return out, e
		}
		total.CostUSD = previous.Add(cost).StringFixed(10)
		all, e := decimal.NewFromString(out.TotalCostUSD)
		if e != nil {
			return out, e
		}
		out.TotalCostUSD = all.Add(cost).StringFixed(10)
	}
	return out, rows.Err()
}
func (h *AgentHandler) Usage(c *gin.Context) {
	user, ok := h.user(c)
	if !ok {
		return
	}
	if !h.ready {
		c.JSON(503, gin.H{"code": "AGENT_USAGE_UNAVAILABLE", "message": "用量统计尚未就绪，请稍后刷新。"})
		return
	}
	result, err := h.usageSummary(c.Request.Context(), user)
	if err != nil {
		c.JSON(503, gin.H{"code": "AGENT_USAGE_UNAVAILABLE", "message": "用量统计读取失败，请稍后刷新。"})
		return
	}
	c.JSON(200, result)
}
