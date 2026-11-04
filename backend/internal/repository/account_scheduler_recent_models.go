package repository

import (
	"context"
	"sync/atomic"
	"time"
)

// Write invalidations are coalesced for at most one minute. An INSERT must not
// turn the next one-second scheduler/UI poll into a seven-day history scan.
var accountSchedulerUsageEpoch atomic.Uint64

func (r *accountSchedulerRepository) cachedRecentModels(now time.Time) (map[int64][]string, bool) {
	r.recentMu.Lock()
	defer r.recentMu.Unlock()
	age := now.Sub(r.recentAt)
	valid := r.recentModels != nil && age < 5*time.Minute && (r.recentEpoch == accountSchedulerUsageEpoch.Load() || age < time.Minute)
	return r.recentModels, valid
}

func (r *accountSchedulerRepository) loadRecentModels(ctx context.Context) (map[int64][]string, error) {
	if cached, ok := r.cachedRecentModels(time.Now()); ok {
		return cached, nil
	}
	value, err, _ := r.recentRefresh.Do("recent-models", func() (any, error) {
		if cached, ok := r.cachedRecentModels(time.Now()); ok {
			return cached, nil
		}
		epoch := accountSchedulerUsageEpoch.Load()
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		rows, err := r.db.QueryContext(queryCtx, `
   WITH counts AS (
    SELECT group_id,model,COUNT(*) AS n FROM usage_logs
    WHERE group_id IS NOT NULL AND created_at >= NOW()-INTERVAL '7 days' AND COALESCE(model,'') <> ''
    GROUP BY group_id,model
   ), ranked AS (SELECT group_id,model,ROW_NUMBER() OVER(PARTITION BY group_id ORDER BY n DESC,model) AS rank FROM counts)
   SELECT group_id,model FROM ranked WHERE rank <= 5 ORDER BY group_id,rank
  `)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		models := make(map[int64][]string)
		for rows.Next() {
			var id int64
			var model string
			if err := rows.Scan(&id, &model); err != nil {
				return nil, err
			}
			models[id] = append(models[id], model)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		r.recentMu.Lock()
		r.recentModels = models
		r.recentAt = time.Now()
		r.recentEpoch = epoch
		r.recentMu.Unlock()
		return models, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(map[int64][]string), nil
}
