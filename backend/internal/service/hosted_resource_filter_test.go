//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminAccountOwnershipFilter(t *testing.T) {
	for _, tc := range []struct {
		ownership string
		hosted    bool
		enabled   bool
	}{
		{"", false, true},
		{"standard", false, true},
		{"hosted", true, true},
		{"all", false, false},
	} {
		t.Run(tc.ownership, func(t *testing.T) {
			ctx, err := WithAccountOwnershipFilter(context.Background(), tc.ownership)
			require.NoError(t, err)
			hosted, enabled := HostedResourceFilterFromContext(ctx)
			require.Equal(t, tc.hosted, hosted)
			require.Equal(t, tc.enabled, enabled)
		})
	}
	_, err := WithAccountOwnershipFilter(context.Background(), "other")
	require.Error(t, err)
	_, enabled := HostedResourceFilterFromContext(context.Background())
	require.False(t, enabled, "internal scheduling must not inherit the admin filter")
}
