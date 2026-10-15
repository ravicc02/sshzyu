package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOpsUserAPIKeyGroupsMetadataOnly(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}
	mock.ExpectQuery(`SELECT k.id, k.user_id, k.group_id, COALESCE`).WithArgs(pq.Array([]int64{1, 2})).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "group_id", "name"}).AddRow(100, 1, 2, "GPT mixed").AddRow(101, 2, 78, "DS v4"))
	keys, err := repo.ListOpsUserAPIKeyGroups(context.Background(), []int64{1, 2})
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.Equal(t, "GPT mixed", keys[0].GroupName)
	require.Equal(t, int64(2), keys[1].UserID)
	empty, err := repo.ListOpsUserAPIKeyGroups(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, empty)
	require.NoError(t, mock.ExpectationsWereMet())
}
