//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyMonitorUpdateGroupLink(t *testing.T) {
	initialGroupID := int64(4)
	monitor := &ChannelMonitor{
		Provider:        MonitorProviderOpenAI,
		Endpoint:        "https://monitor.example",
		PrimaryModel:    "gpt-test",
		CheckMode:       MonitorCheckModeProbe,
		IntervalSeconds: 60,
		GroupID:         &initialGroupID,
	}

	replacement := int64(9)
	require.NoError(t, applyMonitorUpdate(monitor, ChannelMonitorUpdateParams{GroupID: &replacement}))
	require.NotNil(t, monitor.GroupID)
	require.Equal(t, int64(9), *monitor.GroupID)

	clear := int64(0)
	require.NoError(t, applyMonitorUpdate(monitor, ChannelMonitorUpdateParams{GroupID: &clear}))
	require.Nil(t, monitor.GroupID)
}
