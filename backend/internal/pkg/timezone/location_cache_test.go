package timezone

import (
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestLoadLocationCachesSuccessfulLoads(t *testing.T) {
	first, err := LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	second, err := LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	require.Same(t, first, second)
}

func TestLoadLocationConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	locations := make([]*time.Location, 32)
	for i := range locations {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var err error
			locations[i], err = LoadLocation("America/New_York")
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	for _, loc := range locations {
		require.Same(t, locations[0], loc)
	}
	winter := time.Date(2026, 1, 15, 12, 0, 0, 0, locations[0])
	_, winterOffset := winter.Zone()
	summer := time.Date(2026, 7, 15, 12, 0, 0, 0, locations[0])
	_, summerOffset := summer.Zone()
	require.Equal(t, -5*3600, winterOffset)
	require.Equal(t, -4*3600, summerOffset)
}

func TestLoadLocationPreservesSpecialNamesAndErrors(t *testing.T) {
	for _, name := range []string{"", "UTC", "Local"} {
		want, err := time.LoadLocation(name)
		require.NoError(t, err)
		got, err := LoadLocation(name)
		require.NoError(t, err)
		require.Same(t, want, got)
	}
	for _, name := range []string{"Invalid/Timezone", "../Asia/Shanghai", " Asia/Shanghai "} {
		_, err := LoadLocation(name)
		require.Error(t, err)
	}
}

func BenchmarkLoadLocation(b *testing.B) {
	b.Run("cached", func(b *testing.B) {
		_, _ = LoadLocation("Asia/Shanghai")
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, _ = LoadLocation("Asia/Shanghai")
		}
	})
	b.Run("uncached", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = time.LoadLocation("Asia/Shanghai")
		}
	})
}
