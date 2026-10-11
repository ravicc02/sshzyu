//go:build unit

package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestBatchImageDownloadLimiter_AcquireDenyReleaseAndTTL(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	limiter := &batchImageDownloadLimiter{
		rdb:          rdb,
		activePrefix: defaultBatchImageDownloadActivePrefix,
		maxActive:    1,
		ttl:          time.Minute,
	}

	permit, err := limiter.Acquire(ctx, "11", "zip")
	require.NoError(t, err)
	require.NotNil(t, permit)
	require.True(t, mr.TTL(limiter.activeKey("11")) > 0)

	_, err = limiter.Acquire(ctx, "11", "zip")
	require.ErrorIs(t, err, service.ErrBatchImageDownloadLimited)

	require.NoError(t, permit.Release(ctx))
	require.NoError(t, permit.Release(ctx))
	require.False(t, mr.Exists(limiter.activeKey("11")))

	permit, err = limiter.Acquire(ctx, "11", "zip")
	require.NoError(t, err)
	require.NotNil(t, permit)
}

func TestBatchImageDownloadLimiter_ReleaseAfterRequestEnds(t *testing.T) {
	for _, scenario := range []string{"cancelled", "deadline_exceeded"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			limiter := &batchImageDownloadLimiter{
				rdb: client, activePrefix: defaultBatchImageDownloadActivePrefix,
				maxActive: 2, ttl: time.Minute,
			}
			first, err := limiter.Acquire(ctx, "11", "zip")
			require.NoError(t, err)
			second, err := limiter.Acquire(ctx, "11", "item")
			require.NoError(t, err)
			_, err = limiter.Acquire(ctx, "11", "zip")
			require.ErrorIs(t, err, service.ErrBatchImageDownloadLimited)

			endedCtx, cancel := context.WithCancel(ctx)
			if scenario == "deadline_exceeded" {
				cancel()
				endedCtx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
			}
			cancel()
			require.Error(t, endedCtx.Err())
			require.NoError(t, first.Release(endedCtx))
			current, err := server.Get(limiter.activeKey("11"))
			require.NoError(t, err)
			require.Equal(t, "1", current)
			require.NoError(t, first.Release(endedCtx))
			current, err = server.Get(limiter.activeKey("11"))
			require.NoError(t, err)
			require.Equal(t, "1", current)

			third, err := limiter.Acquire(ctx, "11", "zip")
			require.NoError(t, err)
			_, err = limiter.Acquire(ctx, "11", "item")
			require.ErrorIs(t, err, service.ErrBatchImageDownloadLimited)
			require.NoError(t, second.Release(endedCtx))
			require.NoError(t, third.Release(endedCtx))
			require.False(t, server.Exists(limiter.activeKey("11")))
		})
	}
}

func TestBatchImageDownloadLimiter_DisconnectedClientCanDownloadAgain(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	limiter := &batchImageDownloadLimiter{
		rdb: redisClient, activePrefix: defaultBatchImageDownloadActivePrefix,
		maxActive: 1, ttl: time.Minute,
	}
	released := make(chan error, 2)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		permit, err := limiter.Acquire(request.Context(), "11", "zip")
		if err != nil {
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer func() { released <- permit.Release(request.Context()) }()
		_, _ = io.WriteString(writer, "download")
		writer.(http.Flusher).Flush()
		if request.URL.Path == "/interrupt" {
			<-request.Context().Done()
		}
	}))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/interrupt", nil)
	require.NoError(t, err)
	response, err := httpServer.Client().Do(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	buffer := make([]byte, 1)
	_, err = response.Body.Read(buffer)
	require.NoError(t, err)
	cancel()
	require.NoError(t, response.Body.Close())
	select {
	case err := <-released:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("download permit was not released after client disconnect")
	}

	response, err = httpServer.Client().Get(httpServer.URL + "/retry")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "download", string(body))
	select {
	case err := <-released:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("download permit was not released after successful download")
	}
	require.False(t, redisServer.Exists(limiter.activeKey("11")))
}

func TestBatchImageDownloadLimiter_ConfigAndUserIsolation(t *testing.T) {
	ctx := context.Background()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	defaults := NewBatchImageDownloadLimiter(client, nil).(*batchImageDownloadLimiter)
	require.Equal(t, defaultBatchImageDownloadConcurrency, defaults.maxActive)
	require.Equal(t, defaultBatchImageDownloadActiveTTL, defaults.ttl)
	unset := NewBatchImageDownloadLimiter(client, &config.Config{}).(*batchImageDownloadLimiter)
	require.Equal(t, defaults.maxActive, unset.maxActive)
	require.Equal(t, defaults.ttl, unset.ttl)
	limiter := NewBatchImageDownloadLimiter(client, &config.Config{
		BatchImage: config.BatchImageConfig{MaxDownloadConcurrencyPerUser: 1, MaxDownloadDurationSeconds: 30},
	}).(*batchImageDownloadLimiter)
	require.Equal(t, 1, limiter.maxActive)
	require.Equal(t, 30*time.Second, limiter.ttl)
	first, err := limiter.Acquire(ctx, "11", "zip")
	require.NoError(t, err)
	require.Equal(t, 30*time.Second, server.TTL(limiter.activeKey("11")))
	second, err := limiter.Acquire(ctx, "22", "item")
	require.NoError(t, err)
	_, err = limiter.Acquire(ctx, "11", "item")
	require.ErrorIs(t, err, service.ErrBatchImageDownloadLimited)
	require.NoError(t, first.Release(ctx))
	require.True(t, server.Exists(limiter.activeKey("22")))
	require.NoError(t, second.Release(ctx))

	var absent *batchImageDownloadLimiter
	_, err = absent.Acquire(ctx, "11", "zip")
	require.ErrorIs(t, err, service.ErrBatchImageDownloadLimited)
	_, err = (&batchImageDownloadLimiter{}).Acquire(ctx, "11", "zip")
	require.ErrorIs(t, err, service.ErrBatchImageDownloadLimited)
	var absentPermit *batchImageDownloadPermit
	require.NoError(t, absentPermit.Release(ctx))
	require.NoError(t, (&batchImageDownloadPermit{}).Release(ctx))
	require.NoError(t, (&batchImageDownloadPermit{rdb: client}).Release(ctx))
}
