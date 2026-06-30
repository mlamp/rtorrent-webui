package poll

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/model"
	"github.com/mlamp/rtorrent-webui/internal/sse"
)

// errUnreachable stands in for scgi.ErrUnreachable — the poller treats every
// source error identically, so any error drives the health state machine.
var errUnreachable = errors.New("rtorrent unreachable")

func discardLogger() *log.Logger { return log.New(io.Discard, "", 0) }

// drain reads every buffered message off a subscriber without blocking.
func drain(sub *sse.Subscriber) []sse.Message {
	var msgs []sse.Message
	for {
		select {
		case m := <-sub.Ch():
			msgs = append(msgs, m)
		default:
			return msgs
		}
	}
}

// statusMsgs decodes the HealthMsg payloads of the status frames in msgs.
func statusMsgs(t *testing.T, msgs []sse.Message) []model.HealthMsg {
	t.Helper()
	var out []model.HealthMsg
	for _, m := range msgs {
		if m.Event != "status" {
			continue
		}
		var h model.HealthMsg
		if err := json.Unmarshal(m.Data, &h); err != nil {
			t.Fatalf("decode status frame: %v", err)
		}
		out = append(out, h)
	}
	return out
}

func hasEvent(msgs []sse.Message, event string) bool {
	for _, m := range msgs {
		if m.Event == event {
			return true
		}
	}
	return false
}

// failingSrc always errors; okSrc always succeeds.
func failingSrc(context.Context) ([]model.Torrent, model.Globals, error) {
	return nil, model.Globals{}, errUnreachable
}
func okSrc(context.Context) ([]model.Torrent, model.Globals, error) {
	return nil, model.Globals{}, nil
}

// TestHealthDebouncedToUnreachable: only the unreachableAfter-th consecutive
// failure emits a single status:unreachable; the prior failures stay silent.
func TestHealthDebouncedToUnreachable(t *testing.T) {
	hub := sse.NewHub()
	sub := hub.Subscribe()
	p := New(failingSrc, hub, time.Second, time.Second, discardLogger())

	for i := 0; i < unreachableAfter-1; i++ {
		p.tick()
	}
	if got := statusMsgs(t, drain(sub)); len(got) != 0 {
		t.Fatalf("emitted %d status frames before threshold, want 0", len(got))
	}
	p.tick() // crosses the threshold
	got := statusMsgs(t, drain(sub))
	if len(got) != 1 {
		t.Fatalf("at threshold: %d status frames, want 1", len(got))
	}
	if got[0].Rtorrent != model.RtorrentUnreachable {
		t.Fatalf("Rtorrent = %q, want %q", got[0].Rtorrent, model.RtorrentUnreachable)
	}
	if got[0].ConsecutiveFails != unreachableAfter {
		t.Fatalf("ConsecutiveFails = %d, want %d", got[0].ConsecutiveFails, unreachableAfter)
	}
}

// TestHealthRecoveryImmediate: a single good poll after an outage flips back to
// up at once, with the fail counter reset.
func TestHealthRecoveryImmediate(t *testing.T) {
	hub := sse.NewHub()
	sub := hub.Subscribe()
	srcErr := error(errUnreachable)
	src := func(context.Context) ([]model.Torrent, model.Globals, error) {
		return nil, model.Globals{}, srcErr
	}
	p := New(src, hub, time.Second, time.Second, discardLogger())
	for i := 0; i < unreachableAfter; i++ {
		p.tick()
	}
	drain(sub) // discard the unreachable transition

	srcErr = nil
	p.tick()
	got := statusMsgs(t, drain(sub))
	if len(got) != 1 {
		t.Fatalf("recovery: %d status frames, want 1", len(got))
	}
	if got[0].Rtorrent != model.RtorrentUp {
		t.Fatalf("Rtorrent = %q, want %q", got[0].Rtorrent, model.RtorrentUp)
	}
	if got[0].ConsecutiveFails != 0 {
		t.Fatalf("ConsecutiveFails = %d, want 0", got[0].ConsecutiveFails)
	}
}

// TestHealthFirstSuccessEmitsUp: the very first successful tick publishes both a
// cached status:up and the initial snapshot.
func TestHealthFirstSuccessEmitsUp(t *testing.T) {
	hub := sse.NewHub()
	sub := hub.Subscribe()
	p := New(okSrc, hub, time.Second, time.Second, discardLogger())
	p.tick()
	msgs := drain(sub)
	if !hasEvent(msgs, "snapshot") {
		t.Fatal("first success did not broadcast a snapshot")
	}
	got := statusMsgs(t, msgs)
	if len(got) != 1 || got[0].Rtorrent != model.RtorrentUp {
		t.Fatalf("status frames = %+v, want exactly one up", got)
	}
}

// TestHealthTransitionsIdempotent: repeated states never re-emit.
func TestHealthTransitionsIdempotent(t *testing.T) {
	hub := sse.NewHub()
	sub := hub.Subscribe()
	srcErr := error(errUnreachable)
	src := func(context.Context) ([]model.Torrent, model.Globals, error) {
		return nil, model.Globals{}, srcErr
	}
	p := New(src, hub, time.Second, time.Second, discardLogger())
	for i := 0; i < unreachableAfter+5; i++ {
		p.tick()
	}
	if got := statusMsgs(t, drain(sub)); len(got) != 1 {
		t.Fatalf("many failures emitted %d status frames, want 1", len(got))
	}
	srcErr = nil
	p.tick()
	if got := statusMsgs(t, drain(sub)); len(got) != 1 || got[0].Rtorrent != model.RtorrentUp {
		t.Fatalf("first success: %+v, want one up", got)
	}
	p.tick()
	p.tick()
	if got := statusMsgs(t, drain(sub)); len(got) != 0 {
		t.Fatalf("further successes emitted %d status frames, want 0", len(got))
	}
}

// TestHealthCachedForLateJoiner: a browser that connects mid-outage immediately
// learns rtorrent is unreachable from the hub's cached status (the HARD
// new-joiner-while-down constraint).
func TestHealthCachedForLateJoiner(t *testing.T) {
	hub := sse.NewHub()
	sub := hub.Subscribe()
	p := New(failingSrc, hub, time.Second, time.Second, discardLogger())
	for i := 0; i < unreachableAfter; i++ {
		p.tick()
	}
	drain(sub)

	late := hub.Subscribe()
	got := statusMsgs(t, drain(late))
	if len(got) != 1 || got[0].Rtorrent != model.RtorrentUnreachable {
		t.Fatalf("late joiner got %+v, want cached unreachable", got)
	}
}

// TestHealthFlapToleratedBelowThreshold: alternating short failures never reach
// the threshold, so the dot never goes unreachable.
func TestHealthFlapToleratedBelowThreshold(t *testing.T) {
	hub := sse.NewHub()
	sub := hub.Subscribe()
	srcErr := error(nil)
	src := func(context.Context) ([]model.Torrent, model.Globals, error) {
		return nil, model.Globals{}, srcErr
	}
	p := New(src, hub, time.Second, time.Second, discardLogger())

	var ups, unreach int
	for cycle := 0; cycle < 4; cycle++ {
		srcErr = errUnreachable
		p.tick()
		p.tick() // only 2 fails — below unreachableAfter=3
		srcErr = nil
		p.tick()
	}
	for _, h := range statusMsgs(t, drain(sub)) {
		switch h.Rtorrent {
		case model.RtorrentUp:
			ups++
		case model.RtorrentUnreachable:
			unreach++
		}
	}
	if unreach != 0 {
		t.Fatalf("transient flapping signalled unreachable %d times, want 0", unreach)
	}
	if ups != 1 {
		t.Fatalf("emitted up %d times, want exactly 1 (idempotent)", ups)
	}
}

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
