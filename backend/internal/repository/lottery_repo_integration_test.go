//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

// LotteryRepoSuite 覆盖抽奖链路的幂等、并发库存、权限与阶梯阈值。
// 使用 testcontainers 提供的真实 Postgres(见 integration_harness_test.go)。
type LotteryRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *lotteryRepository
}

func TestLotteryRepoSuite(t *testing.T) {
	suite.Run(t, new(LotteryRepoSuite))
}

func (s *LotteryRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.client = testEntClient(s.T())
	s.repo = NewLotteryRepository(s.client).(*lotteryRepository)
}

// --- fixtures ---

func (s *LotteryRepoSuite) createUser(email string, status string) *dbent.User {
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetStatus(status).
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return u
}

// createActiveActivity 创建活动与 4 个默认奖品,返回活动。
func (s *LotteryRepoSuite) createActiveActivity(name string) *dbent.LotteryActivity {
	activity, err := s.client.LotteryActivity.Create().
		SetName(name).
		SetStatus(service.LotteryActivityStatusActive).
		Save(s.ctx)
	s.Require().NoError(err, "create activity")

	for _, p := range []struct {
		nm     string
		typ    string
		value  float64
		weight int
		stock  int
	}{
		{"谢谢参与", service.LotteryPrizeTypeNone, 0, 70, -1},
		{"$0.1", service.LotteryPrizeTypeBalanceBonus, 0.1, 20, 100},
	} {
		_, err := s.client.LotteryPrize.Create().
			SetActivityID(activity.ID).
			SetName(p.nm).
			SetPrizeType(p.typ).
			SetValue(p.value).
			SetWeight(p.weight).
			SetStock(p.stock).
			Save(s.ctx)
		s.Require().NoError(err, "create prize")
	}
	return activity
}

func (s *LotteryRepoSuite) newService() *service.LotteryService {
	return service.NewLotteryService(s.repo, NewUserRepository(s.client, integrationDB), s.client, nil)
}

func (s *LotteryRepoSuite) TestActivityTiersRequireValidPrizePool() {
	activity, err := s.client.LotteryActivity.Create().
		SetName("档位配置测试").
		SetStatus(service.LotteryActivityStatusActive).
		Save(s.ctx)
	s.Require().NoError(err)
	prize, err := s.client.LotteryPrize.Create().
		SetActivityID(activity.ID).
		SetName("谢谢参与").
		SetPrizeType(service.LotteryPrizeTypeNone).
		SetWeight(100).
		SetTierWeights(map[string]int{"5": 0}).
		Save(s.ctx)
	s.Require().NoError(err)

	svc := s.newService()
	mode := service.LotteryTierModeCustom
	definitions := append(service.DefaultLotteryTierConfig().TierDefinitions(),
		service.LotteryTierDefinition{Name: "新增档", ThresholdCents: 20500})
	err = svc.AdminUpdateActivity(s.ctx, activity.ID, service.LotteryActivityUpdateInput{
		TierMode: &mode, TierDefinitions: &definitions,
	})
	s.Require().Error(err, "新增档位奖池为零时必须拒绝保存")
	unchanged, err := s.repo.GetActivityByID(s.ctx, activity.ID)
	s.Require().NoError(err)
	s.Equal(service.LotteryTierModeFixed, unchanged.TierConfig().Mode, "失败时不得部分保存档位")
	s.Equal(activity.RulesVersion, unchanged.RulesVersion, "失败时不得递增规则版本")

	// 先为尚未开放的档位预配概率，再更新档位；两个保存过程均保持有效奖池。
	err = svc.AdminUpdatePrizeWeights(s.ctx, activity.ID, []service.LotteryPrizeWeightUpdate{{
		ID: prize.ID, Weight: 100, TierWeights: map[string]int{"5": 100}, Enabled: true, MinTier: 0,
	}})
	s.Require().NoError(err)
	err = svc.AdminUpdateActivity(s.ctx, activity.ID, service.LotteryActivityUpdateInput{
		TierMode: &mode, TierDefinitions: &definitions,
	})
	s.Require().NoError(err)
	updated, err := s.repo.GetActivityByID(s.ctx, activity.ID)
	s.Require().NoError(err)
	s.Equal(int64(6), updated.TierConfig().DisplayTierCount())

	empty := []service.LotteryTierDefinition{}
	err = svc.AdminUpdateActivity(s.ctx, activity.ID, service.LotteryActivityUpdateInput{
		TierMode: &mode, TierDefinitions: &empty,
	})
	s.Require().Error(err, "显式提交空自定义档位必须被拒绝")
}

// --- 库存原子扣减 ---

func (s *LotteryRepoSuite) TestDeductStockConcurrentExactlyStockTimes() {
	activity := s.createActiveActivity("库存并发测试")
	prizes, err := s.repo.ListPrizesByActivity(s.ctx, activity.ID, true)
	s.Require().NoError(err)

	// 限定库存的奖品
	limited, err := s.client.LotteryPrize.Create().
		SetActivityID(activity.ID).
		SetName("限定奖品").
		SetPrizeType(service.LotteryPrizeTypeBalanceBonus).
		SetValue(0.1).
		SetWeight(1).
		SetStock(5).
		Save(s.ctx)
	s.Require().NoError(err)

	const workers = 20
	var wg sync.WaitGroup
	successCh := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, derr := s.repo.DeductPrizeStock(s.ctx, limited.ID)
			if derr == nil {
				successCh <- ok
			}
		}()
	}
	wg.Wait()
	close(successCh)

	success := 0
	for ok := range successCh {
		if ok {
			success++
		}
	}
	s.Equal(5, success, "库存为 5 时恰好只应扣减成功 5 次")
	_ = prizes
}

// --- 抽奖流水幂等唯一约束 ---

func (s *LotteryRepoSuite) TestCreateDrawIdempotencyConflict() {
	user := s.createUser("lottery-idem@test.local", service.StatusActive)
	record := &service.LotteryDrawRecord{
		UserID:         user.ID,
		ActivityID:     0,
		PrizeName:      "谢谢参与",
		PrizeType:      service.LotteryPrizeTypeNone,
		RulesVersion:   1,
		Source:         service.LotteryDrawSourceFirst,
		IdempotencyKey: "key-dup-1",
	}
	// activity 外键需要真实活动
	activity := s.createActiveActivity("幂等活动")
	record.ActivityID = activity.ID

	s.Require().NoError(s.repo.CreateDraw(s.ctx, record))
	conflict := &service.LotteryDrawRecord{
		UserID:         user.ID,
		ActivityID:     activity.ID,
		PrizeName:      "谢谢参与",
		PrizeType:      service.LotteryPrizeTypeNone,
		RulesVersion:   1,
		Source:         service.LotteryDrawSourceFirst,
		IdempotencyKey: "key-dup-1",
	}
	err := s.repo.CreateDraw(s.ctx, conflict)
	s.Require().ErrorIs(err, service.ErrLotteryDrawIdempotencyConflict, "同 key 重复写入必须返回幂等冲突")

	// 计数仍为 1
	count, err := s.repo.CountDraws(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(1, count, "幂等冲突后流水数量不变")
}

// --- 完整 Draw 流程: 首抽 + 幂等 + 权限 + 阈值 ---

func (s *LotteryRepoSuite) TestDrawFirstDrawAndIdempotentRetry() {
	activity := s.createActiveActivity("首抽测试")
	_ = activity
	svc := s.newService()
	user := s.createUser("lottery-first@test.local", service.StatusActive)

	outcome, err := svc.Draw(s.ctx, user.ID, "retry-key-1")
	s.Require().NoError(err, "首抽应成功")
	s.NotNil(outcome.Record)
	s.Equal(service.LotteryFulfillmentGranted, outcome.Record.FulfillmentStatus)
	s.Equal(int64(0), outcome.RemainingDraws, "首抽后剩余 0 次")

	// 同幂等键重试: 返回同一条流水,不重复扣次数
	retry, err := svc.Draw(s.ctx, user.ID, "retry-key-1")
	s.Require().NoError(err, "同 key 重试应返回已有结果")
	s.Equal(outcome.Record.ID, retry.Record.ID, "重试返回同一条流水")

	// 不同 key 但无剩余次数: 拒绝
	_, err = svc.Draw(s.ctx, user.ID, "retry-key-2")
	s.Require().ErrorIs(err, service.ErrLotteryNoDrawsLeft, "无剩余次数应拒绝")

	count, err := s.repo.CountDraws(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(1, count, "全程只应有一条流水")
}

func (s *LotteryRepoSuite) TestDrawDeniedForDisabledUser() {
	s.createActiveActivity("禁用用户测试")
	svc := s.newService()
	disabled := s.createUser("lottery-disabled@test.local", service.StatusDisabled)

	_, err := svc.Draw(s.ctx, disabled.ID, "disabled-key-1")
	s.Require().ErrorIs(err, service.ErrLotteryUserNotEligible, "禁用账号不能抽奖")

	_, err = svc.GetUserStatus(s.ctx, disabled.ID)
	s.Require().ErrorIs(err, service.ErrLotteryUserNotEligible, "禁用账号不能获得首抽资格")
}

func (s *LotteryRepoSuite) TestThresholdUnlockViaSpendOffset() {
	s.createActiveActivity("阈值测试")
	svc := s.newService()
	user := s.createUser("lottery-threshold@test.local", service.StatusActive)

	// 初始: 仅首抽 1 次
	status, err := svc.GetUserStatus(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(int64(1), status.AvailableDraws)
	s.Equal(int64(0), status.ThresholdEntitlement)

	// 偏移 $4.99: 不解锁
	s.Require().NoError(svc.AdminSetSpendOffset(s.ctx, user.ID, 499))
	status, err = svc.GetUserStatus(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(int64(1), status.AvailableDraws, "$4.99 不应解锁第 2 抽")

	// 偏移 $5: 解锁第 2 抽
	s.Require().NoError(svc.AdminSetSpendOffset(s.ctx, user.ID, 500))
	status, err = svc.GetUserStatus(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(int64(2), status.AvailableDraws, "$5 应解锁第 2 抽")
	s.Equal(int64(1), status.ThresholdEntitlement)

	// 偏移 $14.99: 仍 2 次
	s.Require().NoError(svc.AdminSetSpendOffset(s.ctx, user.ID, 1499))
	status, err = svc.GetUserStatus(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(int64(2), status.AvailableDraws, "$14.99 不应解锁第 3 抽")

	// 偏移 $15: 3 次; $25: 4 次
	s.Require().NoError(svc.AdminSetSpendOffset(s.ctx, user.ID, 1500))
	status, err = svc.GetUserStatus(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(int64(3), status.AvailableDraws, "$15 应解锁第 3 抽")

	s.Require().NoError(svc.AdminSetSpendOffset(s.ctx, user.ID, 2500))
	status, err = svc.GetUserStatus(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(int64(4), status.AvailableDraws, "$25 应解锁第 4 抽")

	// 重复设置同一偏移(模拟重复同步): 状态不变,不重复发放
	s.Require().NoError(svc.AdminSetSpendOffset(s.ctx, user.ID, 2500))
	statusAgain, err := svc.GetUserStatus(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(status.AvailableDraws, statusAgain.AvailableDraws, "重复同步不得重复增加次数")

	// 抽掉 4 次: 阶梯来源 first -> threshold -> threshold -> manual(此处为 threshold+first)
	consumed := 0
	for i := 0; i < 4; i++ {
		_, err := svc.Draw(s.ctx, user.ID, fmt.Sprintf("threshold-key-%d", i))
		if err == nil {
			consumed++
		}
	}
	s.Equal(4, consumed, "4 次可用次数应可连续抽完")
	_, err = svc.Draw(s.ctx, user.ID, "threshold-key-extra")
	s.Require().ErrorIs(err, service.ErrLotteryNoDrawsLeft)
}

func (s *LotteryRepoSuite) TestConcurrentDrawsDoNotDoubleSpend() {
	s.createActiveActivity("并发测试")
	svc := s.newService()
	user := s.createUser("lottery-concurrent@test.local", service.StatusActive)

	const goroutines = 12
	var wg sync.WaitGroup
	results := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// 每个协程用独立 key,只允许 1 次成功(用户只有首抽 1 次)
			_, err := svc.Draw(s.ctx, user.ID, fmt.Sprintf("concurrent-key-%d", n))
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)

	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	s.Equal(1, success, "并发 12 个不同 key 请求,只有 1 次应成功(仅首抽次数)")

	count, err := s.repo.CountDraws(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Equal(1, count, "并发后流水只有 1 条")
}
