package repository

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const schedulerUsageRefreshTimeout = 5 * time.Second

type schedulerUsageEntry[T any] struct {
	value     T
	expiresAt time.Time
}

// schedulerUsageCache only retains historical usage aggregates. Account status,
// group visibility, model allowlists and prices are always read from the database.
// A single bounded refresh prevents concurrent panel requests multiplying scans.
type schedulerUsageCache[T any] struct {
	mu      sync.Mutex
	entries map[int64]schedulerUsageEntry[T]
	refresh singleflight.Group
	now     func() time.Time
	kind    string
}

func (c *schedulerUsageCache[T]) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *schedulerUsageCache[T]) lookup(ids []int64) (map[int64]T, []int64) {
	now := c.clock()
	out := make(map[int64]T, len(ids))
	missing := make([]int64, 0)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range ids {
		entry, ok := c.entries[id]
		if ok && now.Before(entry.expiresAt) {
			out[id] = entry.value
		} else {
			missing = append(missing, id)
		}
	}
	return out, missing
}

func (c *schedulerUsageCache[T]) get(
	ctx context.Context, ids []int64, ttl time.Duration,
	load func(context.Context, []int64) (map[int64]T, error),
) (map[int64]T, error) {
	ids = schedulerUsageGroupIDs(ids)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[int64]T{}, nil
	}
	if ttl <= 0 {
		return load(ctx, ids)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out, missing := c.lookup(ids)
		if len(missing) == 0 {
			return out, nil
		}
		result := c.refresh.DoChan("usage", func() (any, error) {
			_, missing := c.lookup(ids)
			if len(missing) == 0 {
				return nil, nil
			}
			// A caller leaving must not cancel other requests sharing the refresh.
			refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), schedulerUsageRefreshTimeout)
			defer cancel()
			started := time.Now()
			values, err := load(refreshCtx, missing)
			elapsed := time.Since(started)
			if elapsed >= time.Second {
				slog.Warn("account_scheduler.slow_usage_refresh", "kind", c.kind, "group_count", len(missing), "duration_ms", elapsed.Milliseconds(), "success", err == nil)
			}
			if err == nil {
				err = refreshCtx.Err()
			}
			if err != nil {
				return nil, err
			}
			now := c.clock()
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.entries == nil {
				c.entries = make(map[int64]schedulerUsageEntry[T])
			}
			for id, entry := range c.entries {
				if !now.Before(entry.expiresAt) {
					delete(c.entries, id)
				}
			}
			for _, id := range missing {
				c.entries[id] = schedulerUsageEntry[T]{value: values[id], expiresAt: now.Add(ttl)}
			}
			return nil, nil
		})
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case loaded := <-result:
			if loaded.Err != nil {
				return nil, loaded.Err
			}
		}
		// Another batch may have held the flight. Recheck our own group IDs.
	}
}

func schedulerUsageGroupIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
