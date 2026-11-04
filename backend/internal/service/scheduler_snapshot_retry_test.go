package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSchedulerFullRebuildBackoff(t *testing.T) {
	svc := &SchedulerSnapshotService{}
	now := time.Unix(1000, 0)
	svc.fullRebuildNow = func() time.Time { return now }
	calls := 0
	failure := errors.New("database timeout")
	run := func() error { calls++; return failure }
	for i, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, time.Minute, time.Minute} {
		require.ErrorIs(t, svc.coalesceFullRebuild(run), failure)
		require.Equal(t, i+1, calls)
		t.Logf("failure=%d retry_after=%s", i+1, want)
		require.Equal(t, want, schedulerFullRebuildRetryDelay(i+1))
		require.ErrorIs(t, svc.coalesceFullRebuild(run), failure)
		require.Equal(t, i+1, calls, "cooldown must not query database")
		now = svc.fullRebuildRetryAt
	}
	require.NoError(t, svc.coalesceFullRebuild(func() error { return nil }))
	require.Zero(t, svc.fullRebuildFailures)
	require.True(t, svc.fullRebuildRetryAt.IsZero())
}

func TestSchedulerIdleOutboxPollDelay(t *testing.T) {
	for i, want := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second} {
		require.Equal(t, want, schedulerIdlePollDelay(time.Second, uint32(i)))
	}
	require.Equal(t, 10*time.Second, schedulerIdlePollDelay(10*time.Second, 10))
}

type deadlineSnapshotCache struct{ SchedulerCache }

func (*deadlineSnapshotCache) ListBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestSchedulerFullRebuildDeadlineEntersBackoff(t *testing.T) {
	svc := NewSchedulerSnapshotService(&deadlineSnapshotCache{}, nil, nil, nil, nil)
	svc.fullRebuildTimeout = 10 * time.Millisecond
	started := time.Now()
	require.ErrorIs(t, svc.triggerFullRebuild("test"), context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)
	require.Equal(t, 1, svc.fullRebuildFailures)
	require.True(t, svc.fullRebuildRetryAt.After(time.Now()))
}
