package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 模型级计费倍率在批量生图计费路径（BatchImagePublicService.resolvePricingSnapshot）生效的
// 回归测试。批量生图是独立于 token 计费的异步路径，同样必须支持模型级倍率，且与主路径同源：
// 「替代」语义；分组独立图片倍率优先于模型级倍率（对齐 computePeakAwareMultipliers 的
// resolveImageRateMultiplier）。

type batchImageGroupRepoForRateTest struct{ group *Group }

func (s *batchImageGroupRepoForRateTest) GetByIDLite(context.Context, int64) (*Group, error) {
	return s.group, nil
}

func TestBatchImageResolvePricingSnapshot_ModelRateMultiplier(t *testing.T) {
	const groupID = int64(77)
	groupRate := 0.2
	modelRate := 0.5
	imagePrice := 0.1
	batchDiscount := 1.0

	newGroup := func() *Group {
		return &Group{
			ID:                           groupID,
			Platform:                     PlatformGemini,
			AllowBatchImageGeneration:    true,
			RateMultiplier:               groupRate,
			ImagePrice1K:                 &imagePrice,
			BatchImageDiscountMultiplier: batchDiscount,
			ModelPricing: []ChannelModelPricing{
				{Models: []string{"gemini-2.5-flash-image"}, RateMultiplier: testRatePtr(modelRate)},
			},
		}
	}

	submit := func(t *testing.T, group *Group, model string) *BatchImagePricingSnapshot {
		t.Helper()
		svc := &BatchImagePublicService{GroupRepo: &batchImageGroupRepoForRateTest{group: group}}
		gid := groupID
		snap, err := svc.resolvePricingSnapshot(context.Background(),
			BatchImageOwner{UserID: 1, GroupID: &gid},
			BatchImageSubmitRequest{
				Model:     model,
				ImageSize: "1K",
				Items:     []BatchImageSubmitItem{{CustomID: "a", Prompt: "x"}},
			},
			BatchImageProviderGeminiAPI, nil)
		require.NoError(t, err)
		return snap
	}

	t.Run("命中模型级倍率则替代分组默认倍率", func(t *testing.T) {
		snap := submit(t, newGroup(), "gemini-2.5-flash-image")
		require.InDelta(t, modelRate, snap.GroupRateMultiplier, 1e-12,
			"批量生图须按模型级倍率而非分组默认倍率计价")
		require.InDelta(t, imagePrice*modelRate*batchDiscount, snap.BillableUnitPrice, 1e-12)
		// 展示用快照不含模型级覆盖：用量记录据此展示，模型级倍率保持隐式。
		require.InDelta(t, groupRate, snap.GroupRateMultiplierWithoutModel, 1e-12,
			"展示用倍率不得包含模型级覆盖")
	})

	t.Run("同组未命中模型仍用分组默认倍率", func(t *testing.T) {
		snap := submit(t, newGroup(), "gemini-2.5-pro-image")
		require.InDelta(t, groupRate, snap.GroupRateMultiplier, 1e-12)
		require.InDelta(t, groupRate, snap.GroupRateMultiplierWithoutModel, 1e-12)
	})

	t.Run("分组独立图片倍率优先于模型级倍率", func(t *testing.T) {
		group := newGroup()
		group.ImageRateIndependent = true
		group.ImageRateMultiplier = 0.9
		snap := submit(t, group, "gemini-2.5-flash-image")
		require.InDelta(t, 0.9, snap.GroupRateMultiplier, 1e-12,
			"与主路径一致：分组独立图片倍率覆盖模型级倍率")
		// 独立图片倍率本身不受模型级影响，展示值与计费值一致。
		require.InDelta(t, 0.9, snap.GroupRateMultiplierWithoutModel, 1e-12)
	})
}
