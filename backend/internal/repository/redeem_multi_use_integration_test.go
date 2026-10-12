//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcodeclaim"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type rejectedRedeemGrant struct{ service.UserRepository }

func (repo *rejectedRedeemGrant) UpdateBalance(context.Context, int64, float64) error {
	return fmt.Errorf("synthetic grant failure")
}

func newMultiRedeemFixture(t *testing.T, codeType string, maxUses int, usersCount int) (*service.RedeemService, *redeemCodeRepository, *service.RedeemCode, []*dbent.User) {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	codes := NewRedeemCodeRepository(client).(*redeemCodeRepository)
	token, err := service.GenerateRedeemCode()
	require.NoError(t, err)
	code := &service.RedeemCode{Code: token, Type: codeType, Value: 10, Status: service.StatusUnused, MaxUses: maxUses, ValidityDays: 3}
	require.NoError(t, codes.Create(ctx, code))
	users := make([]*dbent.User, 0, usersCount)
	for index := 0; index < usersCount; index++ {
		user, createErr := client.User.Create().SetEmail(fmt.Sprintf("%s-%d@example.com", token, index)).SetPasswordHash("test-only").SetConcurrency(1).Save(ctx)
		require.NoError(t, createErr)
		users = append(users, user)
	}
	t.Cleanup(func() {
		_, cleanupErr := client.RedeemCodeClaim.Delete().Where(redeemcodeclaim.RedeemCodeIDEQ(code.ID)).Exec(ctx)
		require.NoError(t, cleanupErr)
		require.NoError(t, codes.Delete(ctx, code.ID))
		for _, user := range users {
			_, cleanupErr = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id=$1", user.ID)
			require.NoError(t, cleanupErr)
		}
	})
	redeem := service.NewRedeemService(codes, NewUserRepository(client, integrationDB), nil, nil, nil, client, nil, nil)
	return redeem, codes, code, users
}

func TestMultiRedeemPersistsHistoryForEveryBeneficiary(t *testing.T) {
	for _, codeType := range []string{service.RedeemTypeBalance, service.RedeemTypeConcurrency} {
		t.Run(codeType, func(t *testing.T) {
			redeem, codes, code, users := newMultiRedeemFixture(t, codeType, 3, 2)
			for _, user := range users {
				_, err := redeem.Redeem(context.Background(), user.ID, code.Code)
				require.NoError(t, err)
				history, page, err := codes.ListByUserPaginated(context.Background(), user.ID, pagination.PaginationParams{Page: 1, PageSize: 20}, codeType)
				require.NoError(t, err)
				require.EqualValues(t, 1, page.Total)
				require.Len(t, history, 1)
				require.Equal(t, code.Code, history[0].Code)
				require.Equal(t, user.ID, *history[0].UsedBy)
				if codeType == service.RedeemTypeBalance {
					sum, sumErr := codes.SumPositiveBalanceByUser(context.Background(), user.ID)
					require.NoError(t, sumErr)
					require.Equal(t, 10.0, sum)
				}
			}
		})
	}
}

func TestMultiRedeemConcurrentClaimsCannotExceedCapacity(t *testing.T) {
	redeem, codes, code, users := newMultiRedeemFixture(t, service.RedeemTypeBalance, 2, 6)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	start := make(chan struct{})
	outcomes := make(chan error, len(users))
	var group sync.WaitGroup
	for _, user := range users {
		group.Add(1)
		go func(userID int64) {
			defer group.Done()
			<-start
			_, err := redeem.Redeem(ctx, userID, code.Code)
			outcomes <- err
		}(user.ID)
	}
	close(start)
	group.Wait()
	close(outcomes)
	success := 0
	for err := range outcomes {
		if err == nil {
			success++
		}
	}
	require.Equal(t, 2, success)
	stored, err := codes.GetByID(ctx, code.ID)
	require.NoError(t, err)
	require.Equal(t, 2, stored.UsedCount)
	total := 0.0
	for _, user := range users {
		storedUser, getErr := NewUserRepository(testEntClient(t), integrationDB).GetByID(ctx, user.ID)
		require.NoError(t, getErr)
		total += storedUser.Balance
	}
	require.Equal(t, 20.0, total)
}

func TestMultiRedeemEmailAndAccountDeduplication(t *testing.T) {
	redeem, codes, code, users := newMultiRedeemFixture(t, service.RedeemTypeBalance, 3, 2)
	ctx := context.Background()
	_, err := redeem.Redeem(ctx, users[0].ID, code.Code)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE users SET email=$1 WHERE id=$2", "changed-"+users[0].Email, users[0].ID)
	require.NoError(t, err)
	_, err = redeem.Redeem(ctx, users[0].ID, code.Code)
	require.ErrorIs(t, err, service.ErrRedeemCodeAlreadyClaimed)
	_, err = integrationDB.ExecContext(ctx, "UPDATE users SET email=UPPER($1) WHERE id=$2", users[0].Email, users[1].ID)
	require.NoError(t, err)
	_, err = redeem.Redeem(ctx, users[1].ID, code.Code)
	require.ErrorIs(t, err, service.ErrRedeemCodeAlreadyClaimed)
	stored, err := codes.GetByID(ctx, code.ID)
	require.NoError(t, err)
	require.Equal(t, 1, stored.UsedCount)
}

func TestMultiRedeemGrantFailureRollsBackClaim(t *testing.T) {
	redeem, codes, code, users := newMultiRedeemFixture(t, service.RedeemTypeBalance, 2, 1)
	ctx := context.Background()
	client := testEntClient(t)
	broken := service.NewRedeemService(codes, &rejectedRedeemGrant{UserRepository: NewUserRepository(client, integrationDB)}, nil, nil, nil, client, nil, nil)
	_, err := broken.Redeem(ctx, users[0].ID, code.Code)
	require.Error(t, err)
	stored, err := codes.GetByID(ctx, code.ID)
	require.NoError(t, err)
	require.Zero(t, stored.UsedCount)
	count, err := client.RedeemCodeClaim.Query().Where(redeemcodeclaim.RedeemCodeIDEQ(code.ID)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	_, err = redeem.Redeem(ctx, users[0].ID, code.Code)
	require.NoError(t, err)
}

func TestMultiRedeemSubscriptionPerUser(t *testing.T) {
	_, codes, code, users := newMultiRedeemFixture(t, service.RedeemTypeSubscription, 2, 2)
	ctx := context.Background()
	client := testEntClient(t)
	group, err := client.Group.Create().SetName(code.Code).SetSubscriptionType("subscription").Save(ctx)
	require.NoError(t, err)
	_, err = client.RedeemCode.UpdateOneID(code.ID).SetGroupID(group.ID).Save(ctx)
	require.NoError(t, err)
	subRepo := NewUserSubscriptionRepository(client)
	subscriptions := service.NewSubscriptionService(NewGroupRepository(client, integrationDB), subRepo, nil, client, nil)
	defer subscriptions.Stop()
	redeem := service.NewRedeemService(codes, NewUserRepository(client, integrationDB), subscriptions, nil, nil, client, nil, nil)
	t.Cleanup(func() {
		_, err := client.RedeemCodeClaim.Delete().Where(redeemcodeclaim.RedeemCodeIDEQ(code.ID)).Exec(ctx)
		require.NoError(t, err)
		require.NoError(t, codes.Delete(ctx, code.ID))
		for _, user := range users {
			_, err = integrationDB.ExecContext(ctx, "DELETE FROM user_subscriptions WHERE user_id=$1", user.ID)
			require.NoError(t, err)
		}
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id=$1", group.ID)
		require.NoError(t, err)
	})
	for _, user := range users {
		_, err = redeem.Redeem(ctx, user.ID, code.Code)
		require.NoError(t, err)
		sub, getErr := subRepo.GetByUserIDAndGroupID(ctx, user.ID, group.ID)
		require.NoError(t, getErr)
		require.Equal(t, service.SubscriptionStatusActive, sub.Status)
		history, _, getErr := codes.ListByUserPaginated(ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 20}, service.RedeemTypeSubscription)
		require.NoError(t, getErr)
		require.Len(t, history, 1)
		require.Equal(t, group.ID, *history[0].GroupID)
	}
}
