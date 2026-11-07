//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBatchAPIKeyName(t *testing.T) {
	require.Equal(t, "十月渠道-001", batchAPIKeyName("十月渠道", 1, 12))
	require.Equal(t, "十月渠道-012", batchAPIKeyName("十月渠道", 12, 12))
}

func TestTruncateBatchNameReservesSuffix(t *testing.T) {
	name := truncateBatchName(strings.Repeat("批", 120), 50)
	require.Len(t, []rune(name), 96)
	require.Equal(t, 100, len([]rune(batchAPIKeyName(name, 50, 50))))
}

func TestValidateBatchQuotaSelection(t *testing.T) {
	require.NoError(t, validateBatchQuotaSelection(1, 0))
	require.NoError(t, validateBatchQuotaSelection(0, 1_000_000))
	require.Error(t, validateBatchQuotaSelection(0, 0))
	require.Error(t, validateBatchQuotaSelection(1, 1_000_000))
}
