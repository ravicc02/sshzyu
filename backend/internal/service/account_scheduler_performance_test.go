package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type schedulerPerformanceRepo struct {
	AccountSchedulerRepository
	settings    map[int64]AccountSchedulerGroupSetting
	settingsErr error
	groups      []AccountSchedulerGroup
	allCalls    int
	selected    []int64
}

func (r *schedulerPerformanceRepo) ListGroups(context.Context) ([]AccountSchedulerGroup, error) {
	r.allCalls++
	return r.groups, nil
}
func (r *schedulerPerformanceRepo) ListGroupsByIDs(_ context.Context, ids []int64) ([]AccountSchedulerGroup, error) {
	r.selected = append([]int64(nil), ids...)
	return r.groups, nil
}
func (r *schedulerPerformanceRepo) ListGroupSettings(context.Context) (map[int64]AccountSchedulerGroupSetting, error) {
	return r.settings, r.settingsErr
}
func (r *schedulerPerformanceRepo) ListGroupAccounts(context.Context, []int64) ([]AccountSchedulerAccount, error) {
	return nil, nil
}
func (r *schedulerPerformanceRepo) ListDegradationStats(context.Context, time.Time) ([]AccountDegradationStat, error) {
	return nil, nil
}

func TestAccountSchedulerDisabledProbesSkipGroupUsageQueries(t *testing.T) {
	for _, settings := range []map[int64]AccountSchedulerGroupSetting{nil, {1: {GroupID: 1, DegradationEnabled: false}}} {
		repo := &schedulerPerformanceRepo{settings: settings}
		svc := NewAccountSchedulerService(repo, nil, nil, nil)
		got, err := svc.RunScheduled(context.Background())
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Zero(t, repo.allCalls)
		require.Empty(t, repo.selected)
	}
}
func TestAccountSchedulerScheduledQueriesOnlyEnabledGroups(t *testing.T) {
	repo := &schedulerPerformanceRepo{settings: map[int64]AccountSchedulerGroupSetting{1: {GroupID: 1, DegradationEnabled: true}, 2: {GroupID: 2}}, groups: []AccountSchedulerGroup{{ID: 1, Platform: PlatformOpenAI, Status: StatusActive}}}
	svc := NewAccountSchedulerService(repo, nil, nil, nil)
	_, err := svc.RunScheduled(context.Background())
	require.NoError(t, err)
	require.Equal(t, []int64{1}, repo.selected)
	require.Zero(t, repo.allCalls)
}
func TestAccountSchedulerSettingsFailureSkipsGroupUsageQueries(t *testing.T) {
	want := errors.New("settings unavailable")
	repo := &schedulerPerformanceRepo{settingsErr: want}
	svc := NewAccountSchedulerService(repo, nil, nil, nil)
	_, err := svc.RunScheduled(context.Background())
	require.ErrorIs(t, err, want)
	require.Zero(t, repo.allCalls)
}
