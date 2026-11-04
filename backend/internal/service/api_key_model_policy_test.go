package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyModelRestrictionsIntersectWithGroup(t *testing.T) {
	key := &APIKey{AllowedModels: []string{"model-a", "model-c"}, Group: &Group{ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"model-a", "model-b"}}}}
	require.True(t, key.AllowsModel("model-a"))
	require.False(t, key.AllowsModel("model-b"))
	require.False(t, key.AllowsModel("model-c"))
	require.Equal(t, []string{"model-a"}, key.FilterModels([]string{"model-a", "model-b", "model-c"}))
	key.AllowedModels = nil
	require.True(t, key.AllowsModel("model-b"))
	require.False(t, key.AllowsModel("model-c"))
}

func TestAPIKeySelectedModelsNormalizeWithoutWildcards(t *testing.T) {
	models, err := normalizeAPIKeyAllowedModels([]string{" model-a ", "model-a", "MODEL-A", "model-b"})
	require.NoError(t, err)
	require.Equal(t, []string{"model-a", "model-b"}, models)
	for _, models := range [][]string{{"*"}, {"model-?"}, {""}, {"model\nunsafe"}} {
		_, err := normalizeAPIKeyAllowedModels(models)
		require.Error(t, err)
	}
	models, err = normalizeAPIKeyAllowedModels(nil)
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestAPIKeyQuotaUnitsAreMutuallyExclusive(t *testing.T) {
	require.Error(t, validateCreateAPIKeyRequest(CreateAPIKeyRequest{Quota: 1, QuotaTokens: 1000}))
	require.NoError(t, validateCreateAPIKeyRequest(CreateAPIKeyRequest{Quota: 1}))
	require.NoError(t, validateCreateAPIKeyRequest(CreateAPIKeyRequest{QuotaTokens: 1000}))
	quota := float64(1)
	tokens := int64(1000)
	require.Error(t, validateUpdateAPIKeyRequest(UpdateAPIKeyRequest{Quota: &quota, QuotaTokens: &tokens}))
	require.NoError(t, validateUpdateAPIKeyRequest(UpdateAPIKeyRequest{QuotaTokens: &tokens}))
}
