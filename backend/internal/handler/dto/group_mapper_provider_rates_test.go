package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMapperExposesCompositeProviderRates(t *testing.T) {
	channelRate := 0.4
	modelRate := 0.1
	group := &service.Group{
		ID:             65,
		Platform:       service.PlatformComposite,
		RateMultiplier: 0.4,
		ModelPricing: []service.ChannelModelPricing{
			{Models: []string{"__provider_rate__:deepseek"}, RateMultiplier: &channelRate},
			{Models: []string{"__provider_rate__:zhipu"}, RateMultiplier: &channelRate},
			{Models: []string{"__provider_rate__:kimi"}, RateMultiplier: &channelRate},
			{Models: []string{"kimi-k3.1-pro"}, RateMultiplier: &modelRate},
		},
	}

	raw, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"provider_rates"`)
	require.Contains(t, string(raw), `"platform":"deepseek"`)
	require.Contains(t, string(raw), `"platform":"zhipu"`)
	require.Contains(t, string(raw), `"platform":"kimi"`)
	require.NotContains(t, string(raw), `"rate_multiplier":0.1`)
}
