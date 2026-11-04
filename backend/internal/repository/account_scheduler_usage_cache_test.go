package repository

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerUsageCacheFreshAndExpired(t *testing.T) {
	var cache schedulerUsageCache[[]string]
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	calls := 0
	loader := func(_ context.Context, ids []int64) (map[int64][]string, error) {
		calls++
		require.Equal(t, []int64{1, 2}, ids)
		return map[int64][]string{1: {"model-a"}}, nil
	}
	got, err := cache.get(context.Background(), []int64{2, 1, 2}, time.Minute, loader)
	require.NoError(t, err)
	require.Equal(t, []string{"model-a"}, got[1])
	require.Contains(t, got, int64(2))
	_, err = cache.get(context.Background(), []int64{1, 2}, time.Minute, loader)
	require.NoError(t, err)
	require.Equal(t, 1, calls, "empty groups must also be cached")
	now = now.Add(time.Minute)
	_, err = cache.get(context.Background(), []int64{1, 2}, time.Minute, loader)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestSchedulerUsageCacheCoalescesConcurrentLoads(t *testing.T) {
	var cache schedulerUsageCache[int64]
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	loader := func(context.Context, []int64) (map[int64]int64, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return map[int64]int64{1: 42}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := cache.get(context.Background(), []int64{1}, time.Minute, loader)
			if err != nil || got[1] != 42 {
				t.Errorf("got %v, error %v", got, err)
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	require.EqualValues(t, 1, calls.Load())
}

func TestSchedulerUsageCacheCancellationDoesNotCancelSharedRefresh(t *testing.T) {
	var cache schedulerUsageCache[int64]
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	loader := func(ctx context.Context, _ []int64) (map[int64]int64, error) {
		close(started)
		select {
		case <-release:
			return map[int64]int64{1: 7}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _, err := cache.get(ctx, []int64{1}, time.Minute, loader); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	close(release)
	got, err := cache.get(context.Background(), []int64{1}, time.Minute, loader)
	require.NoError(t, err)
	require.EqualValues(t, 7, got[1])
}

func TestSchedulerUsageCacheDoesNotCacheFailures(t *testing.T) {
	var cache schedulerUsageCache[int64]
	calls := 0
	want := errors.New("database unavailable")
	loader := func(context.Context, []int64) (map[int64]int64, error) {
		calls++
		if calls == 1 {
			return nil, want
		}
		return map[int64]int64{1: 9}, nil
	}
	_, err := cache.get(context.Background(), []int64{1}, time.Minute, loader)
	require.ErrorIs(t, err, want)
	got, err := cache.get(context.Background(), []int64{1}, time.Minute, loader)
	require.NoError(t, err)
	require.EqualValues(t, 9, got[1])
	require.Equal(t, 2, calls)
}

func TestSchedulerUsageCacheLoadsOnlyMissingGroups(t *testing.T) {
	var cache schedulerUsageCache[int64]
	var batches [][]int64
	loader := func(_ context.Context, ids []int64) (map[int64]int64, error) {
		batches = append(batches, append([]int64(nil), ids...))
		out := map[int64]int64{}
		for _, id := range ids {
			out[id] = id
		}
		return out, nil
	}
	_, err := cache.get(context.Background(), []int64{1}, time.Minute, loader)
	require.NoError(t, err)
	got, err := cache.get(context.Background(), []int64{2, 1, 3}, time.Minute, loader)
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, [][]int64{{1}, {2, 3}}, batches)
}

func TestSchedulerUsageCacheEmptyAndCancelledRequests(t *testing.T) {
	var cache schedulerUsageCache[int64]
	loader := func(context.Context, []int64) (map[int64]int64, error) {
		t.Fatal("loader must not run")
		return nil, nil
	}
	got, err := cache.get(context.Background(), nil, time.Minute, loader)
	require.NoError(t, err)
	require.Empty(t, got)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = cache.get(ctx, []int64{1}, time.Minute, loader)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSchedulerUsageCacheNonpositiveTTLDoesNotSpin(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		var cache schedulerUsageCache[int64]
		calls := 0
		loader := func(context.Context, []int64) (map[int64]int64, error) { calls++; return map[int64]int64{1: 3}, nil }
		got, err := cache.get(context.Background(), []int64{1}, ttl, loader)
		require.NoError(t, err)
		require.EqualValues(t, 3, got[1])
		require.Equal(t, 1, calls)
	}
}

func TestSchedulerUsageCacheOverlappingGroupsRemainComplete(t *testing.T) {
	var cache schedulerUsageCache[int64]
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	loader := func(_ context.Context, ids []int64) (map[int64]int64, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		out := map[int64]int64{}
		for _, id := range ids {
			out[id] = id * 10
		}
		return out, nil
	}
	first := make(chan error, 1)
	go func() {
		got, err := cache.get(context.Background(), []int64{1}, time.Minute, loader)
		if err == nil && got[1] != 10 {
			err = errors.New("first group missing")
		}
		first <- err
	}()
	<-started
	second := make(chan error, 1)
	go func() {
		got, err := cache.get(context.Background(), []int64{1, 2}, time.Minute, loader)
		if err == nil && (got[1] != 10 || got[2] != 20) {
			err = errors.New("overlapping group missing")
		}
		second <- err
	}()
	close(release)
	require.NoError(t, <-first)
	require.NoError(t, <-second)
	require.EqualValues(t, 2, calls.Load())
}

func TestSchedulerUsageCacheBoundsLoaderContext(t *testing.T) {
	var cache schedulerUsageCache[int64]
	loader := func(ctx context.Context, _ []int64) (map[int64]int64, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > schedulerUsageRefreshTimeout {
			return nil, errors.New("unbounded aggregate refresh")
		}
		return map[int64]int64{1: 1}, nil
	}
	_, err := cache.get(context.Background(), []int64{1}, time.Minute, loader)
	require.NoError(t, err)
}
