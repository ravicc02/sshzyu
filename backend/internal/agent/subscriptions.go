package agent

import "context"

// Short database leases bound subscriptions across processes. Expiry also
// recovers capacity when a process dies before the deferred release.
func (s *Store) Subscribe(ctx context.Context, user int64, run string) (string, error) {
	if _, err := s.Run(ctx, user, run); err != nil {
		return "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(79310254)`); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM agent_event_subscriptions WHERE expires_at<=now()`); err != nil {
		return "", err
	}
	var all, own, task int
	err = tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE user_id=$1),count(*) FILTER(WHERE run_id=$2) FROM agent_event_subscriptions`, user, run).Scan(&all, &own, &task)
	if err != nil {
		return "", err
	}
	if all >= 128 || own >= 2 || task >= 2 {
		return "", Invalid("AGENT_SUBSCRIPTION_LIMIT")
	}
	id := ID("sub")
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_event_subscriptions(id,user_id,run_id,expires_at) VALUES($1,$2,$3,now()+interval '150 seconds')`, id, user, run); err != nil {
		return "", err
	}
	return id, tx.Commit()
}
func (s *Store) Unsubscribe(ctx context.Context, id string) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM agent_event_subscriptions WHERE id=$1`, id)
}
