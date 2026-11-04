package service

import (
	"context"
	"errors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type opsWindowRepo struct {
	OpsRepository
	calls int
	fail  bool
}

func (r *opsWindowRepo) GetLatestHourlyBucketStart(context.Context) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (r *opsWindowRepo) UpsertHourlyMetrics(context.Context, time.Time, time.Time) error {
	r.calls++
	if r.fail {
		return errors.New("timeout")
	}
	return nil
}
func (r *opsWindowRepo) UpsertJobHeartbeat(context.Context, *OpsUpsertJobHeartbeatInput) error {
	return nil
}
func TestOpsHourlyAggregationRunsOnlyWhenWindowAdvances(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	repo := &opsWindowRepo{}
	svc := NewOpsAggregationService(repo, nil, nil, client, nil)
	svc.aggregateHourly()
	svc.aggregateHourly()
	require.Equal(t, 1, repo.calls)
	svc.hourlyCompletedEnd = svc.hourlyCompletedEnd.Add(-time.Hour)
	repo.fail = true
	svc.aggregateHourly()
	svc.aggregateHourly()
	require.Equal(t, 3, repo.calls, "failed window must remain retryable")
	repo.fail = false
	svc.aggregateHourly()
	svc.aggregateHourly()
	require.Equal(t, 4, repo.calls)
}
