//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
	"testing"
)

type opsActiveGroupsRepoStub struct {
	OpsRepository
	keys    []OpsUserAPIKeyGroup
	err     error
	queried []int64
}

func (r *opsActiveGroupsRepoStub) ListOpsUserAPIKeyGroups(_ context.Context, ids []int64) ([]OpsUserAPIKeyGroup, error) {
	r.queried = append(r.queried, ids...)
	return r.keys, r.err
}

type opsActiveGroupsUsersStub struct {
	UserRepository
	users []User
}

func (r opsActiveGroupsUsersStub) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _ UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return r.users, nil, nil
}

func TestOpsUserConcurrencyActiveGroups(t *testing.T) {
	repo := &opsActiveGroupsRepoStub{keys: []OpsUserAPIKeyGroup{
		{APIKeyID: 100, UserID: 1, GroupID: 2, GroupName: "GPT mixed"},
		{APIKeyID: 101, UserID: 1, GroupID: 2, GroupName: "GPT mixed"},
		{APIKeyID: 102, UserID: 1, GroupID: 78, GroupName: "DS v4"},
		{APIKeyID: 103, UserID: 1, GroupID: 4, GroupName: "Idle group"},
	}}
	cache := &stubConcurrencyCacheForTest{usersLoadBatch: map[int64]*UserLoadInfo{1: {CurrentConcurrency: 4}, 2: {WaitingCount: 2}}, apiKeyConcurrency: map[int64]int{100: 1, 101: 2, 102: 1}}
	svc := &OpsService{opsRepo: repo, userRepo: opsActiveGroupsUsersStub{users: []User{{ID: 1, Concurrency: 5}, {ID: 2, Concurrency: 5}, {ID: 3, Concurrency: 5}}}, concurrencyService: NewConcurrencyService(cache)}
	result, at, err := svc.GetUserConcurrencyStats(context.Background())
	require.NoError(t, err)
	require.NotNil(t, at)
	require.Len(t, result, 2)
	require.Equal(t, []int64{1}, repo.queried)
	require.Equal(t, int64(4), result[1].CurrentInUse)
	require.Equal(t, []UserConcurrencyGroupInfo{{GroupID: 2, GroupName: "GPT mixed", CurrentInUse: 3}, {GroupID: 78, GroupName: "DS v4", CurrentInUse: 1}}, result[1].ActiveGroups)
	require.Empty(t, result[2].ActiveGroups, "waiting for a user slot is not an active API key request")
	// Finishing requests must remove group labels even if a global load sample lags.
	cache.apiKeyConcurrency = map[int64]int{}
	result, _, err = svc.GetUserConcurrencyStats(context.Background())
	require.NoError(t, err)
	require.Empty(t, result[1].ActiveGroups)
}
func TestOpsUserConcurrencyActiveGroupsMetadataFailureIsBestEffort(t *testing.T) {
	repo := &opsActiveGroupsRepoStub{err: errors.New("metadata unavailable")}
	svc := &OpsService{opsRepo: repo, concurrencyService: NewConcurrencyService(&stubConcurrencyCacheForTest{})}
	users := map[int64]*UserConcurrencyInfo{1: {UserID: 1, CurrentInUse: 2}}
	svc.fillUserActiveGroups(context.Background(), users)
	require.Equal(t, int64(2), users[1].CurrentInUse)
	require.NotNil(t, users[1].ActiveGroups)
	require.Empty(t, users[1].ActiveGroups)
}
