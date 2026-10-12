package service

import (
	"context"
	"errors"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"math"
	"time"
)

func (s *adminServiceImpl) BatchUpdateBalance(ctx context.Context, userIDs []int64, amount float64, operation string, notes string) BatchBalanceUpdateResult {
	result := BatchBalanceUpdateResult{Items: make([]BatchBalanceUpdateItem, 0, len(userIDs))}
	seen := make(map[int64]struct{}, len(userIDs))
	valid := amount > 0 && !math.IsNaN(amount) && !math.IsInf(amount, 0) && (operation == "add" || operation == "subtract") && len(userIDs) <= 500
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		item := BatchBalanceUpdateItem{UserID: userID}
		if !valid {
			item.Code = "INVALID_BALANCE_ADJUSTMENT"
			item.Error = "Invalid balance adjustment"
		} else {
			item = s.applyBatchBalance(ctx, userID, amount, operation, notes)
		}
		if item.Success {
			result.Success++
		} else {
			result.Failed++
		}
		result.Items = append(result.Items, item)
	}
	return result
}

func (s *adminServiceImpl) applyBatchBalance(ctx context.Context, userID int64, amount float64, operation, notes string) BatchBalanceUpdateItem {
	item := BatchBalanceUpdateItem{UserID: userID, Code: "BALANCE_ADJUSTMENT_FAILED", Error: "Could not adjust this user balance", Retryable: true}
	if s.entClient == nil || s.redeemCodeRepo == nil {
		item.Retryable = false
		return item
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return item
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	delta := amount
	if operation == "subtract" {
		delta = -amount
	}
	change, err := s.userRepo.AdjustBalance(txCtx, userID, delta)
	if err != nil {
		if errors.Is(err, ErrBalanceNegative) {
			item.Code = "INSUFFICIENT_BALANCE"
			item.Error = "Insufficient balance"
		}
		if errors.Is(err, ErrUserNotFound) {
			item.Code = "USER_NOT_FOUND"
			item.Error = "User not found"
			item.Retryable = false
		}
		return item
	}
	code, err := GenerateRedeemCode()
	if err != nil {
		return item
	}
	now := time.Now().UTC()
	audit := &RedeemCode{Code: code, Type: AdjustmentTypeAdminBalance, Value: change.New - change.Old, Status: StatusUsed, UsedBy: &userID, UsedAt: &now, Notes: notes, MaxUses: 1, UsedCount: 1}
	if err = s.redeemCodeRepo.Create(txCtx, audit); err != nil {
		return item
	}
	if err = tx.Commit(); err != nil {
		item.Code = "BALANCE_UPDATE_UNCONFIRMED"
		item.Error = "Update result is unconfirmed; review balance history before retrying"
		item.Retryable = false
		s.invalidateBatchBalance(ctx, userID)
		return item
	}
	s.invalidateBatchBalance(ctx, userID)
	s.tryAccrueAffiliateRebateForAdminRecharge(ctx, userID, operation, amount)
	return BatchBalanceUpdateItem{UserID: userID, Success: true}
}

func (s *adminServiceImpl) invalidateBatchBalance(ctx context.Context, userID int64) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(cleanupCtx, userID)
	}
	if s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateUserBalance(cleanupCtx, userID)
	}
}
