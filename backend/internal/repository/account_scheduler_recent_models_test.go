package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestAccountSchedulerRecentModelsSingleflightAndWriteInvalidation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &accountSchedulerRepository{db: db}
	mock.ExpectQuery("WITH counts AS").WillDelayFor(20 * time.Millisecond).WillReturnRows(sqlmock.NewRows([]string{"group_id", "model"}).AddRow(1, "model-a"))
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			models, err := r.loadRecentModels(context.Background())
			require.NoError(t, err)
			require.Equal(t, []string{"model-a"}, models[1])
		}()
	}
	wg.Wait()
	accountSchedulerUsageEpoch.Add(1)
	_, ok := r.cachedRecentModels(time.Now())
	require.True(t, ok, "write invalidations must coalesce for one minute")
	r.recentMu.Lock()
	r.recentAt = time.Now().Add(-time.Minute)
	r.recentMu.Unlock()
	mock.ExpectQuery("WITH counts AS").WillReturnRows(sqlmock.NewRows([]string{"group_id", "model"}).AddRow(1, "model-b"))
	models, err := r.loadRecentModels(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"model-b"}, models[1])
	require.NoError(t, mock.ExpectationsWereMet())
}
