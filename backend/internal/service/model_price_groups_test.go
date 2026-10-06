//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAdminService_ListAllGroupModelPrices 覆盖「模型价格」页的「全部列出」接口：
//   - 列出所有活跃分组（不再按分组筛选）；
//   - 模型清单只取分组实际配置的模型（可调度账号 model_mapping），
//     不含平台默认模型兜底，也不再用 ModelPricing ∪ ModelAllowlist 拼凑；
//   - CustomRate 反映该模型是否已配独立倍率，且不泄露倍率数值。
func TestAdminService_ListAllGroupModelPrices(t *testing.T) {
	accountRepo := &accountRepoStubForCompositeModelsList{
		accounts: []Account{
			{
				ID:       1,
				Platform: PlatformOpenAI,
				Credentials: map[string]any{
					"model_mapping": map[string]any{"gpt-custom": "gpt-5"},
				},
			},
		},
	}
	groupRepo := &groupRepoStubForAdmin{
		getByIDByID: map[int64]*Group{
			7: {ID: 7, Platform: PlatformOpenAI},
		},
		activeGroups: []Group{
			{
				ID:             7,
				Name:           "默认",
				Platform:       PlatformOpenAI,
				RateMultiplier: 1.2,
				// 仅为 gpt-custom 配独立倍率：它应显示 custom，同组其他模型不受影响。
				ModelPricing: []ChannelModelPricing{
					{Models: []string{"gpt-custom"}, RateMultiplier: testRatePtr(1.5)},
				},
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: accountRepo, groupRepo: groupRepo}

	groups, err := svc.ListAllGroupModelPrices(context.Background())
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, int64(7), groups[0].ID)
	require.Equal(t, "默认", groups[0].Name)
	require.Equal(t, PlatformOpenAI, groups[0].Platform)
	require.Equal(t, 1.2, groups[0].RateMultiplier, "分组默认倍率应回传，供页面在分组名旁展示")

	byModel := make(map[string]bool, len(groups[0].Models))
	for _, m := range groups[0].Models {
		byModel[m.Model] = m.CustomRate
	}

	// 只列出分组实际配置的模型：账号 model_mapping 的键，不含平台默认模型兜底。
	require.Len(t, groups[0].Models, 1, "只应列出分组实际配置的模型，不应并入平台默认模型")
	require.Equal(t, "gpt-custom", groups[0].Models[0].Model)

	// 来自账号 model_mapping 的模型在列，且已配独立倍率。
	require.Contains(t, byModel, "gpt-custom")
	require.True(t, byModel["gpt-custom"], "gpt-custom 应标记为已配独立倍率")
}
