// Package sse provides a server-sent-events hub that fans out poller messages to
// all connected browsers from a single shared poll loop.
package sse

import (
	"sync"
)

// Message is a pre-encoded SSE event.
type Message struct {
	Event string
	Data  []byte
}

// Subscriber is one connected SSE client.
type Subscriber struct {
	ch     chan Message
	closed chan struct{}
	once   sync.Once
}

// Ch is the stream of messages to write to the client.
func (s *Subscriber) Ch() <-chan Message { return s.ch }

// Closed fires when the hub drops this subscriber (e.g. slow consumer).
func (s *Subscriber) Closed() <-chan struct{} { return s.closed }

func (s *Subscriber) kill() { s.once.Do(func() { close(s.closed) }) }

// Hub fans messages out to subscribers and remembers the latest snapshot so a
// new subscriber gets full state immediately.
type Hub struct {
	mu         sync.RWMutex
	subs         map[*Subscriber]struct{}
	latestSnap   *Message
	latestStatus *Message // cached health (event: status) for new joiners
	onFirst      func()
	onZero       func()
}

func NewHub() *Hub {
	return &Hub{subs: make(map[*Subscriber]struct{})}
}

// OnActivity registers callbacks for subscriber-count 0->1 and 1->0, used to
// start/stop the poller on demand. Callbacks run synchronously under the hub
// lock so transitions stay strictly ordered; they must not call back into the
// Hub.
func (h *Hub) OnActivity(first, zero func()) {
	h.mu.Lock()
	h.onFirst, h.onZero = first, zero
	h.mu.Unlock()
}

// SetSnapshot stores the latest snapshot message for new joiners.
func (h *Hub) SetSnapshot(m Message) {
	h.mu.Lock()
	mm := m
	h.latestSnap = &mm
	h.mu.Unlock()
}

// SetStatus caches the latest health message so new subscribers learn current
// rtorrent reachability on connect, not only on the next transition.
func (h *Hub) SetStatus(m Message) {
	h.mu.Lock()
	mm := m
	h.latestStatus = &mm
	h.mu.Unlock()
}

// Subscribe registers a client; it immediately receives the latest snapshot.
func (h *Hub) Subscribe() *Subscriber {
	s := &Subscriber{ch: make(chan Message, 16), closed: make(chan struct{})}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	if h.latestSnap != nil {
		select {
		case s.ch <- *h.latestSnap:
		default:
		}
	}
	if h.latestStatus != nil {
		select {
		case s.ch <- *h.latestStatus:
		default:
		}
	}
	// Invoked under h.mu: a callback decided outside the lock could land out of
	// order (e.g. a stale onZero after a newer onFirst, parking the poller while
	// a client is connected).
	if len(h.subs) == 1 && h.onFirst != nil {
		h.onFirst()
	}
	h.mu.Unlock()
	return s
}

// Unsubscribe removes a client.
func (h *Hub) Unsubscribe(s *Subscriber) {
	h.mu.Lock()
	if _, ok := h.subs[s]; !ok {
		h.mu.Unlock()
		return
	}
	delete(h.subs, s)
	// Under h.mu for the same ordering reason as Subscribe.
	if len(h.subs) == 0 && h.onZero != nil {
		h.onZero()
	}
	h.mu.Unlock()
}

// Broadcast sends to all subscribers without blocking; a full buffer (slow
// consumer) kills that subscriber so its handler returns and the browser
// reconnects with a fresh snapshot.
func (h *Hub) Broadcast(m Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subs {
		select {
		case s.ch <- m:
		default:
			s.kill()
		}
	}
}
