package poll

import (
	"context"
	"sync"
	"testing"
)

// In -mock mode main.go hands the SAME Source closure to the poller goroutine
// and to the HTTP handlers (api.SetSource), so it must be safe to call
// concurrently. Run under -race: an unguarded closure fails with a DATA RACE
// on tick/tor/dlTotal.
func TestMockSourceConcurrentUse(t *testing.T) {
	src := MockSource(50)
	var wg sync.WaitGroup
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if _, _, err := src(context.Background()); err != nil {
					t.Errorf("source returned error: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// Mock globals must use the real poll path's definition of "active"
// (rpc/torrents.go): every torrent with payload traffic counts, so g.UpRate
// never includes traffic from torrents excluded from ActiveCount.
func TestMockSourceActiveCountMatchesRealDefinition(t *testing.T) {
	src := MockSource(30)
	for tick := 1; tick <= 5; tick++ {
		torrents, g, err := src(context.Background())
		if err != nil {
			t.Fatalf("tick %d: source returned error: %v", tick, err)
		}
		want := 0
		for _, tr := range torrents {
			if tr.DownRate > 0 || tr.UpRate > 0 {
				want++
			}
		}
		if g.ActiveCount != want {
			t.Fatalf("tick %d: mock ActiveCount=%d, real-definition active=%d", tick, g.ActiveCount, want)
		}
	}
}
