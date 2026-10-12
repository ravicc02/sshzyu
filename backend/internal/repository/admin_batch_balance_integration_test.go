//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type batchBalanceAuditFailure struct{ service.RedeemCodeRepository }

func (repo *batchBalanceAuditFailure) Create(context.Context, *service.RedeemCode) error {
	return errors.New("synthetic audit failure")
}

func TestAdminBatchBalancePartialResultsAndAtomicAudit(t *testing.T) {
	_, codes, _, users := newMultiRedeemFixture(t, service.RedeemTypeBalance, 1, 2)
	ctx := context.Background()
	client := testEntClient(t)
	userRepo := NewUserRepository(client, integrationDB)
	require.NoError(t, userRepo.UpdateBalance(ctx, users[0].ID, 20))
	require.NoError(t, userRepo.UpdateBalance(ctx, users[1].ID, 2))
	t.Cleanup(func() {
		for _, user := range users {
			_, err := integrationDB.ExecContext(ctx, "DELETE FROM redeem_codes WHERE used_by=$1", user.ID)
			require.NoError(t, err)
		}
	})
	admin := service.NewAdminService(nil, userRepo, nil, nil, nil, nil, codes, nil, nil, nil, nil, nil, nil, client, nil, nil, nil, nil, nil, nil, nil, nil, nil).(service.BatchBalanceAdminService)
	result := admin.BatchUpdateBalance(ctx, []int64{users[0].ID, users[0].ID, users[1].ID}, 5, "subtract", "test batch deduction")
	require.Equal(t, 1, result.Success)
	require.Equal(t, 1, result.Failed)
	require.Len(t, result.Items, 2)
	first, err := userRepo.GetByID(ctx, users[0].ID)
	require.NoError(t, err)
	require.Equal(t, 15.0, first.Balance)
	second, err := userRepo.GetByID(ctx, users[1].ID)
	require.NoError(t, err)
	require.Equal(t, 2.0, second.Balance)
	history, err := codes.ListByUser(ctx, users[0].ID, 20)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, -5.0, history[0].Value)
	brokenAdmin := service.NewAdminService(nil, userRepo, nil, nil, nil, nil, &batchBalanceAuditFailure{RedeemCodeRepository: codes}, nil, nil, nil, nil, nil, nil, client, nil, nil, nil, nil, nil, nil, nil, nil, nil).(service.BatchBalanceAdminService)
	broken := brokenAdmin.BatchUpdateBalance(ctx, []int64{users[0].ID}, 7, "add", "audit must be atomic")
	require.Equal(t, 1, broken.Failed)
	first, err = userRepo.GetByID(ctx, users[0].ID)
	require.NoError(t, err)
	require.Equal(t, 15.0, first.Balance)
}
