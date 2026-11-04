package timezone

import (
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestLoadLocationReturnsOneParsedLocation(t *testing.T) {
	first, err := LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := LoadLocation("Asia/Shanghai")
			require.NoError(t, err)
			require.Same(t, first, got)
		}()
	}
	wg.Wait()
	_, err = LoadLocation("Invalid/Location")
	require.Error(t, err)
	got, err := LoadLocation("Local")
	require.NoError(t, err)
	require.Same(t, time.Local, got)
}
