package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *opsRepository) ListOpsUserAPIKeyGroups(ctx context.Context, userIDs []int64) ([]service.OpsUserAPIKeyGroup, error) {
	result := []service.OpsUserAPIKeyGroup{}
	if len(userIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT k.id, k.user_id, k.group_id, COALESCE(g.name, '')
 FROM api_keys k LEFT JOIN groups g ON g.id=k.group_id
 WHERE k.user_id=ANY($1) AND k.group_id IS NOT NULL AND k.deleted_at IS NULL
 ORDER BY k.user_id, k.id`, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item service.OpsUserAPIKeyGroup
		if err := rows.Scan(&item.APIKeyID, &item.UserID, &item.GroupID, &item.GroupName); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
