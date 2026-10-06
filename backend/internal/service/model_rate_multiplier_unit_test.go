//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 本文件补齐「模型级计费倍率」两个在 model_rate_multiplier_test.go 中未覆盖的层面：
//   1. 写入路径（service 层 SetGroupModelRateMultiplier / ClearGroupModelRateMultiplier）
//   2. 计费链路（recordUsageCore 端到端：倍率如何真正落到用量记录上）
// model_rate_multiplier_test.go 只覆盖了纯函数 applyModelRateMultiplier / groupPricingHasPricing。

// -----------------------------------------------------------------------------
// 一、写入路径：分组 × 模型的独立倍率落库
// -----------------------------------------------------------------------------

func TestAdminService_SetGroupModelRateMultiplier_IsolatesTargetModel(t *testing.T) {
	const groupID = int64(7)
	group := &Group{
		ID:       groupID,
		Platform: PlatformAnthropic,
		ModelPricing: []ChannelModelPricing{
			{Models: []string{"gpt-5"}, InputPrice: testRatePtr(3), OutputPrice: testRatePtr(6)},
			{Models: []string{"gpt-5-mini"}},
		},
	}
	groupRepo := &groupRepoStubForAdmin{getByIDByID: map[int64]*Group{groupID: group}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{groupRepo: groupRepo, authCacheInvalidator: invalidator}

	require.NoError(t, svc.SetGroupModelRateMultiplier(context.Background(), groupID, "gpt-5", 1.35))

	require.NotNil(t, groupRepo.updated, "应调用 Update 持久化")
	saved := groupRepo.updated.ModelPricing
	require.Len(t, saved, 2, "gpt-5 已存在于 ModelPricing，不应新增条目")

	byModel := map[string]*float64{}
	for i := range saved {
		for _, m := range saved[i].Models {
			byModel[m] = saved[i].RateMultiplier
		}
	}
	require.NotNil(t, byModel["gpt-5"])
	require.InDelta(t, 1.35, *byModel["gpt-5"], 1e-12)
	// 核心需求：单个模型调倍率不得影响同组其他模型。
	require.Nil(t, byModel["gpt-5-mini"], "同组其他模型的倍率必须保持未配置")

	// 原有价格配置不得被倍率写入破坏。
	require.NotNil(t, saved[0].InputPrice)
	require.InDelta(t, 3, *saved[0].InputPrice, 1e-12)
	require.NotNil(t, saved[0].OutputPrice)
	require.InDelta(t, 6, *saved[0].OutputPrice, 1e-12)

	// 鉴权缓存必须失效，否则配置在线上不生效（历史同类陷阱）。
	require.Contains(t, invalidator.groupIDs, groupID, "应失效该分组的鉴权缓存")
}

func TestAdminService_SetGroupModelRateMultiplier_AddsEntryWhenAbsent(t *testing.T) {
	const groupID = int64(8)
	group := &Group{
		ID:       groupID,
		Platform: PlatformAnthropic,
		ModelPricing: []ChannelModelPricing{
			{Models: []string{"gpt-5"}, InputPrice: testRatePtr(3)},
		},
	}
	groupRepo := &groupRepoStubForAdmin{getByIDByID: map[int64]*Group{groupID: group}}
	svc := &adminServiceImpl{groupRepo: groupRepo}

	require.NoError(t, svc.SetGroupModelRateMultiplier(context.Background(), groupID, "ds-v4-flash", 2))

	require.NotNil(t, groupRepo.updated)
	saved := groupRepo.updated.ModelPricing
	require.Len(t, saved, 2, "模型不在现有条目中时应追加一条只含倍率的条目")
	require.Equal(t, []string{"ds-v4-flash"}, saved[1].Models)
	require.NotNil(t, saved[1].RateMultiplier)
	require.InDelta(t, 2, *saved[1].RateMultiplier, 1e-12)
	// 原条目不得被牵连。
	require.Nil(t, saved[0].RateMultiplier)
	require.NotNil(t, saved[0].InputPrice)
}

func TestAdminService_SetGroupModelRateMultiplier_RejectsInvalidInput(t *testing.T) {
	const groupID = int64(10)
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{groupID: {ID: groupID, Platform: PlatformAnthropic}},
	}
	svc := &adminServiceImpl{groupRepo: groupRepo}
	ctx := context.Background()

	require.Error(t, svc.SetGroupModelRateMultiplier(ctx, groupID, "  ", 1.5), "空模型名应拒绝")
	require.Error(t, svc.SetGroupModelRateMultiplier(ctx, groupID, "m", 0), "倍率 0 应拒绝")
	require.Error(t, svc.SetGroupModelRateMultiplier(ctx, groupID, "m", -1), "负倍率应拒绝")
	require.Nil(t, groupRepo.updated, "非法输入不应写库")
}

func TestAdminService_ClearGroupModelRateMultiplier_RemovesOnlyRate(t *testing.T) {
	const groupID = int64(9)
	group := &Group{
		ID:       groupID,
		Platform: PlatformAnthropic,
		ModelPricing: []ChannelModelPricing{
			// 倍率 + 价格：清倍率后条目应保留（价格还在）。
			{Models: []string{"gpt-5"}, InputPrice: testRatePtr(3), RateMultiplier: testRatePtr(1.35)},
			// 只有倍率：清倍率后条目应整体移除，不留空壳。
			{Models: []string{"ds-v4-flash"}, RateMultiplier: testRatePtr(2)},
			// 无关模型：不受影响。
			{Models: []string{"gpt-5-mini"}, InputPrice: testRatePtr(1)},
		},
	}
	groupRepo := &groupRepoStubForAdmin{getByIDByID: map[int64]*Group{groupID: group}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{groupRepo: groupRepo, authCacheInvalidator: invalidator}

	require.NoError(t, svc.ClearGroupModelRateMultiplier(context.Background(), groupID, "gpt-5"))
	require.NoError(t, svc.ClearGroupModelRateMultiplier(context.Background(), groupID, "ds-v4-flash"))

	require.NotNil(t, groupRepo.updated)
	saved := groupRepo.updated.ModelPricing
	require.Len(t, saved, 2, "只含倍率的空壳条目应被移除")

	require.Equal(t, []string{"gpt-5"}, saved[0].Models)
	require.Nil(t, saved[0].RateMultiplier, "倍率应被清除")
	require.NotNil(t, saved[0].InputPrice, "价格必须保留")

	require.Equal(t, []string{"gpt-5-mini"}, saved[1].Models)
	require.Contains(t, invalidator.groupIDs, groupID)
}

// -----------------------------------------------------------------------------
// 二、计费链路：recordUsageCore 端到端
// -----------------------------------------------------------------------------

// TestRecordUsage_ModelRateMultiplierReplacesGroupRate 走完整 RecordUsage 链路，
// 验证「替代」语义：命中模型级倍率的请求按该倍率计费，未命中的同组模型仍走分组默认倍率。
func TestRecordUsage_ModelRateMultiplierReplacesGroupRate(t *testing.T) {
	const groupID = int64(21)
	const groupRate = 1.0
	const modelRate = 1.35

	usage := ClaudeUsage{InputTokens: 1000, OutputTokens: 500}

	newSvc := func() (*GatewayService, *openAIRecordUsageLogRepoStub, *openAIRecordUsageUserRepoStub) {
		usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
		userRepo := &openAIRecordUsageUserRepoStub{}
		svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
		svc.resolver = NewModelPricingResolver(nil, svc.billingService)
		return svc, usageRepo, userRepo
	}

	record := func(t *testing.T, svc *GatewayService, model string) {
		t.Helper()
		group := &Group{
			ID:             groupID,
			Platform:       PlatformAnthropic,
			RateMultiplier: groupRate,
			// 仅为 gpt-5.1 配独立倍率（隐式配置，界面不展示数值）。
			ModelPricing: []ChannelModelPricing{
				{Models: []string{"gpt-5.1"}, RateMultiplier: testRatePtr(modelRate)},
			},
		}
		require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{
			Result: &ForwardResult{
				RequestID: "mm_" + model,
				Model:     model,
				Usage:     usage,
				Duration:  time.Second,
			},
			APIKey:  &APIKey{ID: 1, GroupID: i64p(groupID), Group: group},
			User:    &User{ID: 2},
			Account: &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
		}))
	}

	t.Run("命中模型级倍率则替代分组默认倍率", func(t *testing.T) {
		svc, usageRepo, userRepo := newSvc()
		record(t, svc, "gpt-5.1")

		require.NotNil(t, usageRepo.lastLog)
		require.Equal(t, "gpt-5.1", usageRepo.lastLog.Model)
		require.InDelta(t, modelRate, usageRepo.lastLog.RateMultiplier, 1e-12,
			"应使用模型级倍率 1.35 而非分组默认 1.0")
		// 实际扣费 = 基础成本 × 模型级倍率。
		require.InDelta(t, usageRepo.lastLog.TotalCost*modelRate, usageRepo.lastLog.ActualCost, 1e-9)
		require.InDelta(t, usageRepo.lastLog.ActualCost, userRepo.lastAmount, 1e-9)
	})

	t.Run("同组未命中模型仍用分组默认倍率", func(t *testing.T) {
		svc, usageRepo, _ := newSvc()
		record(t, svc, "gpt-5")

		require.NotNil(t, usageRepo.lastLog)
		require.Equal(t, "gpt-5", usageRepo.lastLog.Model)
		require.InDelta(t, groupRate, usageRepo.lastLog.RateMultiplier, 1e-12,
			"同组其他模型不得受影响，应保持分组默认倍率")
	})

	t.Run("未配置独立倍率的分组行为与改动前一致", func(t *testing.T) {
		svc, usageRepo, _ := newSvc()
		group := &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: groupRate}
		require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{
			Result: &ForwardResult{
				RequestID: "mm_none",
				Model:     "gpt-5.1",
				Usage:     usage,
				Duration:  time.Second,
			},
			APIKey:  &APIKey{ID: 1, GroupID: i64p(groupID), Group: group},
			User:    &User{ID: 2},
			Account: &Account{ID: 3, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
		}))
		require.NotNil(t, usageRepo.lastLog)
		require.InDelta(t, groupRate, usageRepo.lastLog.RateMultiplier, 1e-12)
	})
}

// TestRecordUsage_ModelRateMultiplierAppliesAcrossPlatforms 断言模型级倍率的匹配与应用
// 与分组平台无关：通用计费路径（recordUsageCore）服务的每一种平台分组，命中模型级倍率时
// 都必须生效，不存在「某些平台漏接」的旁路。
//
// openai / grok 走 OpenAIGatewayService，另有
// model_rate_multiplier_openai_path_test.go 参数化覆盖，本测试不重复。
func TestRecordUsage_ModelRateMultiplierAppliesAcrossPlatforms(t *testing.T) {
	const groupID = int64(22)
	const groupRate = 0.4
	const modelRate = 0.9

	platforms := []string{
		PlatformAnthropic,
		PlatformGemini,
		PlatformAntigravity,
		PlatformKimi,
		PlatformZhipu,
		PlatformDeepseek,
		PlatformMiniMax,
		PlatformTypeSafe,
		PlatformOpenCodeGo,
		PlatformComposite,
	}

	for _, platform := range platforms {
		t.Run(platform, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newGatewayRecordUsageServiceForTest(usageRepo, userRepo, &openAIRecordUsageSubRepoStub{})
			svc.resolver = NewModelPricingResolver(nil, svc.billingService)

			group := &Group{
				ID:             groupID,
				Platform:       platform,
				RateMultiplier: groupRate,
				ModelPricing: []ChannelModelPricing{
					{Models: []string{"gpt-5.1"}, RateMultiplier: testRatePtr(modelRate)},
				},
			}
			require.NoError(t, svc.RecordUsage(context.Background(), &RecordUsageInput{
				Result: &ForwardResult{
					RequestID: "mm_platform_" + platform,
					Model:     "gpt-5.1",
					Usage:     ClaudeUsage{InputTokens: 1000, OutputTokens: 500},
					Duration:  time.Second,
				},
				APIKey:  &APIKey{ID: 1, GroupID: i64p(groupID), Group: group},
				User:    &User{ID: 2},
				Account: &Account{ID: 3, Platform: platform, Type: AccountTypeAPIKey},
			}))

			require.NotNil(t, usageRepo.lastLog)
			require.InDelta(t, modelRate, usageRepo.lastLog.RateMultiplier, 1e-12,
				"平台 %s 的模型级倍率须生效", platform)
		})
	}
}
