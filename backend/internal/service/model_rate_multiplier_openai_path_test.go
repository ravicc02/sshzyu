package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 模型级计费倍率在 OpenAI 计费路径（OpenAIGatewayService.RecordUsage）生效的回归测试。
//
// 历史缺陷：模型级倍率最初只接入了通用计费路径 recordUsageCore，OpenAI 路径
// （openai_gateway_usage.go）漏接，导致 openai / grok 平台分组的模型级倍率线上不生效
// （2026-10-06 用真实上游调用在线上复现：分组配了模型级 0.5 / 0.6，用量记录仍按分组
// 倍率 0.25 / 0.3 计费）。
//
// openai 与 grok 平台共用本计费入口，故参数化覆盖两种平台分组。

// recordOpenAIUsageForModelRateTest 走完整 OpenAIGatewayService.RecordUsage 链路记录一次用量。
func recordOpenAIUsageForModelRateTest(
	t *testing.T,
	group *Group,
	rateRepo UserGroupRateRepository,
	model string,
) (*openAIRecordUsageLogRepoStub, *openAIRecordUsageUserRepoStub) {
	t.Helper()
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceForTest(usageRepo, userRepo, subRepo, rateRepo)

	require.NoError(t, svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "mm_openai_" + model,
			Usage:     OpenAIUsage{InputTokens: 1000, OutputTokens: 500},
			Model:     model,
			Duration:  time.Second,
		},
		APIKey:  &APIKey{ID: 11, GroupID: i64p(group.ID), Group: group},
		User:    &User{ID: 22},
		Account: &Account{ID: 33, Type: AccountTypeAPIKey},
	}))
	return usageRepo, userRepo
}

func TestOpenAIRecordUsage_ModelRateMultiplierReplacesGroupRate(t *testing.T) {
	const groupID = int64(41)
	const groupRate = 0.25
	const modelRate = 0.5

	for _, platform := range []string{PlatformOpenAI, PlatformGrok} {
		t.Run(platform, func(t *testing.T) {
			group := &Group{
				ID:             groupID,
				Platform:       platform,
				RateMultiplier: groupRate,
				// 仅为 gpt-6.1-sol 配独立倍率（隐式配置，界面不展示数值）。
				ModelPricing: []ChannelModelPricing{
					{Models: []string{"gpt-6.1-sol"}, RateMultiplier: testRatePtr(modelRate)},
				},
			}
			usageRepo, userRepo := recordOpenAIUsageForModelRateTest(t, group, nil, "gpt-6.1-sol")

			require.NotNil(t, usageRepo.lastLog)
			require.Equal(t, "gpt-6.1-sol", usageRepo.lastLog.Model)
			require.InDelta(t, modelRate, usageRepo.lastLog.RateMultiplier, 1e-12,
				"命中模型级倍率应替代分组默认倍率（线上缺陷回归）")
			// 实际扣费 = 基础成本 × 模型级倍率。
			require.InDelta(t, usageRepo.lastLog.TotalCost*modelRate, usageRepo.lastLog.ActualCost, 1e-9)
			require.InDelta(t, usageRepo.lastLog.ActualCost, userRepo.lastAmount, 1e-9)
		})
	}
}

func TestOpenAIRecordUsage_ModelRateMultiplierIsolatesOtherModels(t *testing.T) {
	const groupID = int64(42)
	const groupRate = 0.25

	group := &Group{
		ID:             groupID,
		Platform:       PlatformOpenAI,
		RateMultiplier: groupRate,
		ModelPricing: []ChannelModelPricing{
			{Models: []string{"gpt-6.1-sol"}, RateMultiplier: testRatePtr(0.5)},
		},
	}
	usageRepo, _ := recordOpenAIUsageForModelRateTest(t, group, nil, "gpt-6.1")

	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, "gpt-6.1", usageRepo.lastLog.Model)
	require.InDelta(t, groupRate, usageRepo.lastLog.RateMultiplier, 1e-12,
		"同组未命中模型不得受影响，应保持分组默认倍率")
}

func TestOpenAIRecordUsage_ModelRateMultiplierReplacesUserRate(t *testing.T) {
	const groupID = int64(43)
	userRate := 1.8

	group := &Group{
		ID:             groupID,
		Platform:       PlatformOpenAI,
		RateMultiplier: 1.0,
		ModelPricing: []ChannelModelPricing{
			{Models: []string{"gpt-6.1-sol"}, RateMultiplier: testRatePtr(0.5)},
		},
	}
	usageRepo, _ := recordOpenAIUsageForModelRateTest(t, group, &openAIUserGroupRateRepoStub{rate: &userRate}, "gpt-6.1-sol")

	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, 0.5, usageRepo.lastLog.RateMultiplier, 1e-12,
		"模型级倍率为「替代」语义：命中时连用户专属倍率也一并替代")
}

func TestOpenAIRecordUsage_ModelRateMultiplierThenPeakMultiplier(t *testing.T) {
	const groupID = int64(44)
	const modelRate = 0.5
	const peakRate = 3.0

	group := &Group{
		ID:                 groupID,
		Platform:           PlatformOpenAI,
		RateMultiplier:     1.0,
		SubscriptionType:   SubscriptionTypeSubscription,
		PeakRateEnabled:    true,
		PeakStart:          "00:00",
		PeakEnd:            "23:59",
		PeakRateMultiplier: peakRate,
		ModelPricing: []ChannelModelPricing{
			{Models: []string{"gpt-6.1-sol"}, RateMultiplier: testRatePtr(modelRate)},
		},
	}
	usageRepo, _ := recordOpenAIUsageForModelRateTest(t, group, nil, "gpt-6.1-sol")

	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, modelRate*peakRate, usageRepo.lastLog.RateMultiplier, 1e-12,
		"最终倍率 = 模型级倍率 × 高峰因子（模型级置于高峰叠加之前）")
}

func TestOpenAIRecordUsage_NoModelRateKeepsGroupRate(t *testing.T) {
	const groupID = int64(45)
	const groupRate = 0.3

	// 分组未配置任何模型级倍率：行为须与改动前完全一致。
	group := &Group{
		ID:             groupID,
		Platform:       PlatformOpenAI,
		RateMultiplier: groupRate,
	}
	usageRepo, _ := recordOpenAIUsageForModelRateTest(t, group, nil, "gpt-6.1-sol")

	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, groupRate, usageRepo.lastLog.RateMultiplier, 1e-12)
}
