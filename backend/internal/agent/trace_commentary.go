package agent

import "context"

// Rehydrate only public assistant commentary on old, completed tool handoffs.
// Never export protocol history, encrypted reasoning or raw provider responses.
func (s *Service) PopulateTraceCommentary(ctx context.Context, user int64, conv string, traces ...[]TraceStep) {
	missing := map[string]bool{}
	for _, trace := range traces {
		for _, step := range trace {
			if step.Kind == "reasoning" && step.Status == "completed" && step.Commentary == "" {
				missing[step.ID] = true
			}
		}
	}
	if len(missing) == 0 {
		return
	}
	rows, err := s.Store.DB.QueryContext(ctx, `SELECT o.id FROM agent_operations o JOIN agent_runs r ON r.id=o.run_id JOIN agent_conversations c ON c.id=r.conversation_id WHERE r.user_id=$1 AND r.conversation_id=$2 AND c.user_id=$1 AND c.deleted_at IS NULL AND o.kind='text' AND o.status='succeeded'`, user, conv)
	if err != nil {
		return
	}
	allowed := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return
		}
		if missing[id] {
			allowed = append(allowed, id)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return
	}
	text := map[string]string{}
	for _, id := range allowed {
		out, e := s.readOutcome(id)
		if e != nil {
			continue
		}
		response, e := ParseResponse(out.Body)
		if e == nil && len(response.Calls) > 0 && response.Text != "" {
			text[id] = response.Text
		}
	}
	for _, trace := range traces {
		for i := range trace {
			if trace[i].Commentary == "" {
				trace[i].Commentary = text[trace[i].ID]
			}
		}
	}
}
