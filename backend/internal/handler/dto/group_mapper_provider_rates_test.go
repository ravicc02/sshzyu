package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMapperExposesCompositeProviderRates(t *testing.T) {
	group := &service.Group{
		ID:             65,
		Platform:       service.PlatformComposite,
		RateMultiplier: 0.4,
		ModelPricing: []service.ChannelModelPricing{
			{Models: []string{"deepseek-v4.1-flash"}},
			{Models: []string{"glm-5.3-flash"}},
			{Models: []string{"kimi-k3.1-pro"}},
		},
	}

	raw, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"provider_rates"`)
	require.Contains(t, string(raw), `"platform":"deepseek"`)
	require.Contains(t, string(raw), `"platform":"zhipu"`)
	require.Contains(t, string(raw), `"platform":"kimi"`)
}
