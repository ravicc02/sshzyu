//go:build unit

package repository

import (
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

func TestEntToServiceMonitorPreservesGroupID(t *testing.T) {
	groupID := int64(15)
	monitor := entToServiceMonitor(&dbent.ChannelMonitor{GroupID: &groupID})
	require.NotNil(t, monitor)
	require.NotNil(t, monitor.GroupID)
	require.Equal(t, groupID, *monitor.GroupID)
}
