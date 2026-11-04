package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func schedulerGroupRows(allowlist string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "platform", "status", "models"}).AddRow(1, "one", "openai", "active", allowlist)
}

func TestAccountSchedulerListGroupsCachesOnlyHistoricalModels(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountSchedulerRepository{db: db}
	mock.ExpectQuery("SELECT g.id,.*FROM groups g").WithArgs("active").WillReturnRows(schedulerGroupRows(`["allowed-a"]`))
	mock.ExpectQuery("WITH model_counts AS").WithArgs("{1}").WillReturnRows(sqlmock.NewRows([]string{"group_id", "models"}).AddRow(1, "{recent-a,recent-b}"))
	groups, err := repo.ListGroups(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"allowed-a", "recent-a", "recent-b"}, groups[0].ModelCandidates)
	groups[0].ModelCandidates[1] = "mutated"
	mock.ExpectQuery("SELECT g.id,.*FROM groups g").WithArgs("active").WillReturnRows(schedulerGroupRows(`["allowed-new"]`))
	groups, err = repo.ListGroups(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"allowed-new", "recent-a", "recent-b"}, groups[0].ModelCandidates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountSchedulerListGroupsOnlyLoadsSelectedGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountSchedulerRepository{db: db}
	got, err := repo.ListGroupsByIDs(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
	mock.ExpectQuery("SELECT g.id,.*g.id = ANY").WithArgs("active", "{1}").WillReturnRows(schedulerGroupRows("[]"))
	mock.ExpectQuery("WITH model_counts AS").WithArgs("{1}").WillReturnRows(sqlmock.NewRows([]string{"group_id", "models"}))
	got, err = repo.ListGroupsByIDs(context.Background(), []int64{1})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func schedulerAccountRows(status string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "group_id", "name", "platform", "type", "status", "schedulable", "priority", "group_priority", "concurrency", "rate_multiplier", "rate_limit_reset_at", "overload_until", "temp_unschedulable_until", "temp_unschedulable_reason", "expires_at", "auto_pause_on_expired"}).AddRow(7, 1, "account", "openai", "apikey", status, true, 0, 0, 3, nil, nil, nil, nil, "", nil, false)
}

func TestAccountSchedulerListGroupAccountsKeepsStatusFreshAndCachesCounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountSchedulerRepository{db: db}
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	repo.dispatchCounts.now = func() time.Time { return now }
	for i, status := range []string{"active", "disabled", "active"} {
		mock.ExpectQuery("SELECT a.id,.*FROM account_groups").WithArgs("{1}").WillReturnRows(schedulerAccountRows(status))
		if i != 1 {
			mock.ExpectQuery("SELECT group_id, account_id, COUNT").WithArgs("{1}").WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "dispatch_count"}).AddRow(1, 7, 42+i))
		}
		got, err := repo.ListGroupAccounts(context.Background(), []int64{1})
		require.NoError(t, err)
		require.Equal(t, status, got[0].Status)
		if i == 2 {
			require.EqualValues(t, 44, got[0].DispatchCount)
		} else {
			require.EqualValues(t, 42, got[0].DispatchCount)
		}
		if i == 1 {
			now = now.Add(time.Minute)
		}
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountSchedulerListGroupAccountsEmptyAndFailedQueries(t *testing.T) {
	t.Run("no groups", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo := &accountSchedulerRepository{db: db}
		got, err := repo.ListGroupAccounts(context.Background(), nil)
		require.NoError(t, err)
		require.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("no accounts", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo := &accountSchedulerRepository{db: db}
		mock.ExpectQuery("SELECT a.id,.*FROM account_groups").WithArgs("{1}").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		got, err := repo.ListGroupAccounts(context.Background(), []int64{1})
		require.NoError(t, err)
		require.Empty(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("metadata failure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo := &accountSchedulerRepository{db: db}
		want := errors.New("metadata unavailable")
		mock.ExpectQuery("SELECT a.id,.*FROM account_groups").WithArgs("{1}").WillReturnError(want)
		_, err = repo.ListGroupAccounts(context.Background(), []int64{1})
		require.ErrorIs(t, err, want)
		require.NoError(t, mock.ExpectationsWereMet())
	})
	t.Run("aggregate failure is retried", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		repo := &accountSchedulerRepository{db: db}
		want := errors.New("aggregate unavailable")
		mock.ExpectQuery("SELECT a.id,.*FROM account_groups").WithArgs("{1}").WillReturnRows(schedulerAccountRows("active"))
		mock.ExpectQuery("SELECT group_id, account_id, COUNT").WithArgs("{1}").WillReturnError(want)
		_, err = repo.ListGroupAccounts(context.Background(), []int64{1})
		require.ErrorIs(t, err, want)
		mock.ExpectQuery("SELECT a.id,.*FROM account_groups").WithArgs("{1}").WillReturnRows(schedulerAccountRows("disabled"))
		mock.ExpectQuery("SELECT group_id, account_id, COUNT").WithArgs("{1}").WillReturnRows(sqlmock.NewRows([]string{"group_id", "account_id", "dispatch_count"}))
		got, err := repo.ListGroupAccounts(context.Background(), []int64{1})
		require.NoError(t, err)
		require.Zero(t, got[0].DispatchCount)
		require.Equal(t, "disabled", got[0].Status)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestAccountSchedulerListGroupsDoesNotCachePartialAggregate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &accountSchedulerRepository{db: db}
	want := errors.New("interrupted rows")
	mock.ExpectQuery("SELECT g.id,.*FROM groups g").WithArgs("active").WillReturnRows(schedulerGroupRows("[]"))
	mock.ExpectQuery("WITH model_counts AS").WithArgs("{1}").WillReturnRows(sqlmock.NewRows([]string{"group_id", "models"}).AddRow(1, "{partial}").RowError(0, want))
	_, err = repo.ListGroups(context.Background())
	require.ErrorIs(t, err, want)
	mock.ExpectQuery("SELECT g.id,.*FROM groups g").WithArgs("active").WillReturnRows(schedulerGroupRows("[]"))
	mock.ExpectQuery("WITH model_counts AS").WithArgs("{1}").WillReturnRows(sqlmock.NewRows([]string{"group_id", "models"}).AddRow(1, "{complete}"))
	got, err := repo.ListGroups(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"complete"}, got[0].ModelCandidates)
	require.NoError(t, mock.ExpectationsWereMet())
}
