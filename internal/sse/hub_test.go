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
