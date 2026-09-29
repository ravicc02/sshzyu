package service

import (
	"context"
	"testing"
)

// stubGuaranteeRepo 只实现首抽保底会用到的库存扣减,其余方法依赖嵌入接口
// (不会被调用,调用到即说明逻辑有误)。
type stubGuaranteeRepo struct {
	LotteryRepository
	deductOK  bool
	called    bool
	calledIDs []int64
}

func (s *stubGuaranteeRepo) DeductPrizeStock(ctx context.Context, prizeID int64) (bool, error) {
	s.called = true
	s.calledIDs = append(s.calledIDs, prizeID)
	return s.deductOK, nil
}

func guaranteePrizes() []LotteryPrize {
	return []LotteryPrize{
		{ID: 1, Name: "谢谢参与", PrizeType: LotteryPrizeTypeNone, Value: 0, Stock: -1},
		{ID: 2, Name: "$0.1 余额", PrizeType: LotteryPrizeTypeBalanceBonus, Value: 0.1, Stock: 100},
		{ID: 3, Name: "$1 余额", PrizeType: LotteryPrizeTypeBalanceBonus, Value: 1.0, Stock: 50},
		{ID: 4, Name: "$2 余额", PrizeType: LotteryPrizeTypeBalanceBonus, Value: 2.0, Stock: 20},
	}
}

func TestPickGuaranteedFirstPrize(t *testing.T) {
	repo := &stubGuaranteeRepo{deductOK: true}
	picked := pickGuaranteedFirstPrize(context.Background(), repo, guaranteePrizes())
	if picked == nil {
		t.Fatal("期望选中首抽保底 $1 奖品,实际为 nil")
	}
	if picked.ID != 3 || picked.PrizeType != LotteryPrizeTypeBalanceBonus || picked.Value != 1.0 {
		t.Fatalf("选中错误奖品: id=%d type=%s value=%f", picked.ID, picked.PrizeType, picked.Value)
	}
	if !repo.called || len(repo.calledIDs) != 1 || repo.calledIDs[0] != 3 {
		t.Fatalf("库存扣减调用不符合预期: called=%v ids=%v", repo.called, repo.calledIDs)
	}
	if picked.StockIssued != 1 {
		t.Fatalf("StockIssued 应为 1,实际 %d", picked.StockIssued)
	}
}

func TestPickGuaranteedFirstPrize_MissingPrize(t *testing.T) {
	repo := &stubGuaranteeRepo{deductOK: true}
	prizes := []LotteryPrize{
		{ID: 2, Name: "$0.1 余额", PrizeType: LotteryPrizeTypeBalanceBonus, Value: 0.1, Stock: 100},
		{ID: 4, Name: "$2 余额", PrizeType: LotteryPrizeTypeBalanceBonus, Value: 2.0, Stock: 20},
	}
	if picked := pickGuaranteedFirstPrize(context.Background(), repo, prizes); picked != nil {
		t.Fatalf("无 $1 奖品时应返回 nil 退回随机,实际选中 id=%d", picked.ID)
	}
	if repo.called {
		t.Fatal("不应触发库存扣减")
	}
}

func TestPickGuaranteedFirstPrize_OutOfStock(t *testing.T) {
	repo := &stubGuaranteeRepo{deductOK: true}
	prizes := []LotteryPrize{
		{ID: 3, Name: "$1 余额", PrizeType: LotteryPrizeTypeBalanceBonus, Value: 1.0, Stock: 0},
	}
	if picked := pickGuaranteedFirstPrize(context.Background(), repo, prizes); picked != nil {
		t.Fatalf("保底奖品无库存时应返回 nil,实际选中 id=%d", picked.ID)
	}
}

func TestPickGuaranteedFirstPrize_ConflictFallsBack(t *testing.T) {
	// 并发扣减失败: 返回 nil,由调用方退回加权随机。
	repo := &stubGuaranteeRepo{deductOK: false}
	if picked := pickGuaranteedFirstPrize(context.Background(), repo, guaranteePrizes()); picked != nil {
		t.Fatalf("扣减竞争失败应返回 nil,实际选中 id=%d", picked.ID)
	}
	if !repo.called {
		t.Fatal("应先尝试扣减库存再降级")
	}
}

func TestPickGuaranteedFirstPrize_InfiniteStock(t *testing.T) {
	repo := &stubGuaranteeRepo{deductOK: true}
	prizes := []LotteryPrize{
		{ID: 3, Name: "$1 余额", PrizeType: LotteryPrizeTypeBalanceBonus, Value: 1.0, Stock: -1},
	}
	picked := pickGuaranteedFirstPrize(context.Background(), repo, prizes)
	if picked == nil || picked.ID != 3 {
		t.Fatalf("无限库存保底奖品应直接返回,实际 picked=%v", picked)
	}
	if repo.called {
		t.Fatal("无限库存不应触发扣减")
	}
}
