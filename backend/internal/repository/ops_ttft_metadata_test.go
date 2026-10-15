package repository

import (
	"context"
	"database/sql/driver"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestOpsOverviewTTFTSamplesExcludeNonStreaming(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	for index, count := range []int{2, 13, 6, 2, 6, 2, 5} {
		cols := make([]string, count)
		vals := make([]driver.Value, count)
		for i := range cols {
			cols[i] = fmt.Sprint(i)
			vals[i] = int64(0)
		}
		if index == 0 {
			vals[0] = int64(100)
		}
		if index == 1 {
			vals[12] = int64(2)
		}
		mock.ExpectQuery("SELECT|WITH").WillReturnRows(sqlmock.NewRows(cols).AddRow(vals...))
	}
	start := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	overview, err := repo.GetDashboardOverview(context.Background(), &service.OpsDashboardFilter{StartTime: start, EndTime: start.Add(time.Hour), QueryMode: service.OpsQueryModeRaw})
	require.NoError(t, err)
	require.Equal(t, int64(100), overview.SuccessCount)
	require.Equal(t, int64(2), overview.TTFTSampleCount)
	require.False(t, overview.LatencyApproximate)
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestV2LatencyDeclaresHistogramApproximation(t *testing.T) {
	metric := latencyMetric(6100, 2, map[int64]int64{3000: 1, 5000: 1})
	require.Equal(t, "histogram_upper_bound", metric.PercentileMethod)
	require.Equal(t, int64(2), metric.SampleCount)
	require.Equal(t, int64(3000), *metric.P50Ms)
	require.Equal(t, 3050.0, *metric.AvgMs)
}
