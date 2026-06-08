package poll

import (
	"context"
	"io"
	"log"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/model"
	"github.com/mlamp/rtorrent-webui/internal/sse"
)

// The poller must poll slowly when idle, jump to the live cadence (and poll
// immediately) when a client connects, and slow back down when it leaves.
func TestPollerDualCadence(t *testing.T) {
	var calls atomic.Int64
	src := func(ctx context.Context) ([]model.Torrent, model.Globals, error) {
		calls.Add(1)
		return nil, model.Globals{}, nil
	}
	p := New(src, sse.NewHub(), 15*time.Millisecond, 1*time.Second, log.New(io.Discard, "", 0))
	p.Start()
	defer p.Stop()

	// idle: one immediate tick, then a long wait — count stays ~1
	time.Sleep(60 * time.Millisecond)
	if n := calls.Load(); n > 2 {
		t.Fatalf("idle cadence polled too often: %d (want ~1)", n)
	}

	// go live: should wake immediately and then poll fast
	base := calls.Load()
	p.SetActive(true)
	time.Sleep(120 * time.Millisecond)
	live := calls.Load() - base
	if live < 4 {
		t.Fatalf("live cadence too slow: %d ticks in 120ms (want >=4)", live)
	}

	// go idle: fast polling must stop (at most a tick or two before the slow wait)
	p.SetActive(false)
	afterIdle := calls.Load()
	time.Sleep(150 * time.Millisecond)
	if extra := calls.Load() - afterIdle; extra > 3 {
		t.Fatalf("kept polling fast after going idle: %d extra ticks", extra)
	}
}

// Starting is idempotent and Stop ends the loop.
func TestPollerStartIdempotentAndStop(t *testing.T) {
	var calls atomic.Int64
	src := func(ctx context.Context) ([]model.Torrent, model.Globals, error) {
		calls.Add(1)
		return nil, model.Globals{}, nil
	}
	p := New(src, sse.NewHub(), 10*time.Millisecond, 20*time.Millisecond, log.New(io.Discard, "", 0))
	p.Start()
	p.Start() // second call must be a no-op (single loop)
	p.SetActive(true)
	time.Sleep(60 * time.Millisecond)
	p.Stop()
	stopped := calls.Load()
	time.Sleep(60 * time.Millisecond)
	if grew := calls.Load() - stopped; grew > 1 {
		t.Fatalf("loop kept running after Stop: %d more ticks", grew)
	}
}
