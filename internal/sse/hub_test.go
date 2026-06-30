package sse

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHubActivityTransitions checks the basic 0->1/1->0 callback contract.
func TestHubActivityTransitions(t *testing.T) {
	h := NewHub()
	var firsts, zeros atomic.Int64
	h.OnActivity(func() { firsts.Add(1) }, func() { zeros.Add(1) })

	a := h.Subscribe()
	if got := firsts.Load(); got != 1 {
		t.Fatalf("after first Subscribe: onFirst calls = %d, want 1", got)
	}
	b := h.Subscribe()
	if got := firsts.Load(); got != 1 {
		t.Fatalf("after second Subscribe: onFirst calls = %d, want 1", got)
	}
	h.Unsubscribe(a)
	if got := zeros.Load(); got != 0 {
		t.Fatalf("with one subscriber left: onZero calls = %d, want 0", got)
	}
	h.Unsubscribe(b)
	h.Unsubscribe(b) // duplicate must be a no-op
	if got := zeros.Load(); got != 1 {
		t.Fatalf("after last Unsubscribe: onZero calls = %d, want 1", got)
	}
}

// TestHubActivitySubscribeDuringPendingOnZero deterministically reproduces the
// browser-refresh race: the old handler's Unsubscribe reaches the 1->0
// transition, then a new Subscribe arrives before the onZero callback lands.
// The hub must serialize the transition with its callback so the stale onZero
// cannot park the poller after the new client's onFirst already ran.
func TestHubActivitySubscribeDuringPendingOnZero(t *testing.T) {
	h := NewHub()
	var active atomic.Bool
	zeroEntered := make(chan struct{})
	gate := make(chan struct{})
	h.OnActivity(
		func() { active.Store(true) },
		func() {
			close(zeroEntered) // 1->0 transition reached...
			<-gate             // ...but the callback is held mid-flight
			active.Store(false)
		},
	)

	a := h.Subscribe() // 0->1, poller goes live
	unsubDone := make(chan struct{})
	go func() {
		h.Unsubscribe(a)
		close(unsubDone)
	}()
	<-zeroEntered // Unsubscribe is now stalled inside onZero

	subDone := make(chan *Subscriber, 1)
	go func() { subDone <- h.Subscribe() }()

	// Broken ordering lets Subscribe (and its onFirst) complete while the 1->0
	// callback is still pending; a serialized hub keeps it blocked until the
	// transition finishes. Cover both shapes without deadlocking either.
	var b *Subscriber
	select {
	case b = <-subDone:
	case <-time.After(200 * time.Millisecond):
	}
	close(gate)
	if b == nil {
		b = <-subDone
	}
	<-unsubDone

	if !active.Load() {
		t.Fatal("poller left idle (stale onZero ran after onFirst) with a live subscriber")
	}
	// No Unsubscribe(b) teardown: it would re-fire onZero and double-close
	// zeroEntered; the hub needs no cleanup.
}

// drainOne reads one buffered message non-blocking; ok=false if none pending.
func drainOne(sub *Subscriber) (Message, bool) {
	select {
	case m := <-sub.Ch():
		return m, true
	default:
		return Message{}, false
	}
}

// TestHubReplaysSnapshotAndStatus: a joiner gets the cached snapshot then the
// cached health frame, contents intact, in that order.
func TestHubReplaysSnapshotAndStatus(t *testing.T) {
	h := NewHub()
	snap := Message{Event: "snapshot", Data: []byte(`{"seq":7}`)}
	status := Message{Event: "status", Data: []byte(`{"rtorrent":"unreachable"}`)}
	h.SetSnapshot(snap)
	h.SetStatus(status)

	sub := h.Subscribe()
	m1, ok1 := drainOne(sub)
	m2, ok2 := drainOne(sub)
	if !ok1 || !ok2 {
		t.Fatalf("expected two buffered messages, got ok1=%v ok2=%v", ok1, ok2)
	}
	if m1.Event != "snapshot" || string(m1.Data) != string(snap.Data) {
		t.Fatalf("first message = %+v, want cached snapshot", m1)
	}
	if m2.Event != "status" || string(m2.Data) != string(status.Data) {
		t.Fatalf("second message = %+v, want cached status", m2)
	}
	if _, more := drainOne(sub); more {
		t.Fatal("more than two replayed messages")
	}
}

// TestHubStatusCacheReplacedNotAppended: SetStatus overwrites; a joiner sees
// only the latest health, exactly once.
func TestHubStatusCacheReplacedNotAppended(t *testing.T) {
	h := NewHub()
	h.SetStatus(Message{Event: "status", Data: []byte(`{"rtorrent":"up"}`)})
	h.SetStatus(Message{Event: "status", Data: []byte(`{"rtorrent":"unreachable"}`)})

	sub := h.Subscribe()
	m, ok := drainOne(sub)
	if !ok || m.Event != "status" || string(m.Data) != `{"rtorrent":"unreachable"}` {
		t.Fatalf("first message = %+v ok=%v, want latest status only", m, ok)
	}
	if _, more := drainOne(sub); more {
		t.Fatal("status cache appended instead of replaced")
	}
}

// TestHubSubscribeNoStatusWhenUnset: with no SetStatus, a joiner gets the
// snapshot and never a zero/empty status frame.
func TestHubSubscribeNoStatusWhenUnset(t *testing.T) {
	h := NewHub()
	h.SetSnapshot(Message{Event: "snapshot", Data: []byte(`{"seq":1}`)})

	sub := h.Subscribe()
	m, ok := drainOne(sub)
	if !ok || m.Event != "snapshot" {
		t.Fatalf("first message = %+v ok=%v, want snapshot", m, ok)
	}
	if extra, more := drainOne(sub); more {
		t.Fatalf("unexpected extra frame %+v (status replayed while unset)", extra)
	}
}

// TestHubActivityCallbackOrderingStress hammers the same refresh interleaving
// to catch orderings the gated test's fixed schedule might miss.
func TestHubActivityCallbackOrderingStress(t *testing.T) {
	for i := 0; i < 20000; i++ {
		h := NewHub()
		var active atomic.Bool
		h.OnActivity(func() { active.Store(true) }, func() { active.Store(false) })
		a := h.Subscribe()
		var b *Subscriber
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); h.Unsubscribe(a) }()
		go func() { defer wg.Done(); b = h.Subscribe() }()
		wg.Wait()
		if !active.Load() {
			t.Fatalf("iter %d: poller left idle while a subscriber is connected", i)
		}
		h.Unsubscribe(b)
	}
}
