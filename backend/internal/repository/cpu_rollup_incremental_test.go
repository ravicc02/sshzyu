package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func expectRollupLock(mock sqlmock.Sqlmock, acquired bool) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"locked"}).AddRow(acquired))
	if acquired {
		mock.ExpectExec("SET LOCAL statement_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("SET LOCAL lock_timeout").WillReturnResult(sqlmock.NewResult(0, 0))
	}
}
func TestCPURollupAdvancesOneDayWithBoundedScan(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := newDashboardAggregationRepositoryWithSQL(db)
	today := service.GroupUsageTodayStart(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	start := today.AddDate(0, 0, -2)
	earliest := start.AddDate(0, 0, -10)
	for i := 0; i < 2; i++ {
		end := start.AddDate(0, 0, 1)
		expectRollupLock(mock, true)
		mock.ExpectQuery("SELECT closed_before::text").WillReturnRows(sqlmock.NewRows([]string{"closed_before", "retained_from", "timezone_name"}).AddRow(service.GroupUsageDate(start), earliest, service.GroupUsageTimezoneName()))
		mock.ExpectQuery("SELECT MIN").WillReturnRows(sqlmock.NewRows([]string{"min"}).AddRow(earliest))
		mock.ExpectExec("DELETE FROM usage_group_daily_rollups").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("WITH source AS MATERIALIZED").WithArgs(start.UTC(), end.UTC(), service.GroupUsageTimezoneName()).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(33360))
		mock.ExpectExec("UPDATE usage_group_rollup_state").WithArgs(service.GroupUsageDate(end), earliest.UTC(), service.GroupUsageTimezoneName()).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		require.NoError(t, r.SyncGroupUsageRollups(context.Background(), today))
		t.Logf("scan_from=%s scan_until=%s scanned_rows=33360", start, end)
		start = end
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestCPURollupBusyWorkerSkipsWithoutScanning(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	expectRollupLock(mock, false)
	mock.ExpectCommit()
	require.NoError(t, newDashboardAggregationRepositoryWithSQL(db).SyncGroupUsageRollups(context.Background(), time.Now()))
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestCPURollupFailedWindowRollsBackWatermark(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	expectRollupLock(mock, true)
	mock.ExpectQuery("SELECT closed_before::text").WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()
	require.ErrorIs(t, newDashboardAggregationRepositoryWithSQL(db).SyncGroupUsageRollups(context.Background(), time.Now()), context.DeadlineExceeded)
	require.NoError(t, mock.ExpectationsWereMet())
}
