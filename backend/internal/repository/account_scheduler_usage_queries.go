package repository

import (
	"context"

	"github.com/lib/pq"
)

const accountSchedulerRecentModelsSQL = `
 WITH model_counts AS (
  SELECT group_id, model, COUNT(*) AS request_count
  FROM usage_logs
  WHERE group_id = ANY($1)
   AND created_at >= NOW() - INTERVAL '7 days'
   AND model <> ''
  GROUP BY group_id, model
 ), ranked_models AS (
  SELECT group_id, model, request_count,
   ROW_NUMBER() OVER (PARTITION BY group_id ORDER BY request_count DESC, model ASC) AS model_rank
  FROM model_counts
 )
 SELECT group_id, array_agg(model ORDER BY request_count DESC, model ASC)
 FROM ranked_models
 WHERE model_rank <= 5
 GROUP BY group_id
`

const accountSchedulerDispatchCountsSQL = `
 SELECT group_id, account_id, COUNT(*) AS dispatch_count
 FROM usage_logs
 WHERE group_id = ANY($1)
  AND created_at >= NOW() - INTERVAL '24 hours'
 GROUP BY group_id, account_id
`

func (r *accountSchedulerRepository) loadRecentModels(ctx context.Context, ids []int64) (map[int64][]string, error) {
	rows, err := r.db.QueryContext(ctx, accountSchedulerRecentModelsSQL, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]string, len(ids))
	for rows.Next() {
		var id int64
		var models pq.StringArray
		if err := rows.Scan(&id, &models); err != nil {
			return nil, err
		}
		out[id] = []string(models)
	}
	return out, rows.Err()
}

func (r *accountSchedulerRepository) loadDispatchCounts(ctx context.Context, ids []int64) (map[int64]map[int64]int64, error) {
	rows, err := r.db.QueryContext(ctx, accountSchedulerDispatchCountsSQL, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]map[int64]int64, len(ids))
	for rows.Next() {
		var groupID, accountID, count int64
		if err := rows.Scan(&groupID, &accountID, &count); err != nil {
			return nil, err
		}
		if out[groupID] == nil {
			out[groupID] = make(map[int64]int64)
		}
		out[groupID][accountID] = count
	}
	return out, rows.Err()
}
