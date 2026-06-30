// Package poll runs the single shared poll loop: each tick it pulls the torrent
// list + globals from a Source, computes a delta vs the previous snapshot, and
// publishes both to the SSE hub. One loop serves all browsers (rtorrent load is
// O(1) in browser count).
//
// The loop is always running but dual-cadence: while a browser is watching it
// polls at the live interval (snappy UI + history); when nobody is connected it
// drops to a slow idle interval purely to keep recording history. Because the
// history store keeps cumulative counters, the coarse idle samples still yield
// correct totals — only sub-interval burst shape is lost. The moment a client
// connects we poll immediately so its first view is fresh, not up-to-idle stale.
package poll

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/model"
	"github.com/mlamp/rtorrent-webui/internal/sse"
)

// pollTimeout bounds one multicall round-trip (independent of cadence).
const pollTimeout = 15 * time.Second

// unreachableAfter is how many consecutive failed polls before we signal rtorrent
// unreachable. Debounces restart-loop flapping; recovery is immediate.
const unreachableAfter = 3

// Source produces the current torrent list + globals for one tick.
type Source func(ctx context.Context) ([]model.Torrent, model.Globals, error)

// Sink receives each tick's data (e.g. the history sampler).
type Sink func(torrents []model.Torrent, g model.Globals, ts int64)

type Poller struct {
	src  Source
	hub  *sse.Hub
	live time.Duration // cadence while a client is watching
	idle time.Duration // background cadence for history when nobody is
	log  *log.Logger
	sink Sink

	mu      sync.Mutex
	active  bool // at least one SSE client connected
	started bool
	done    chan struct{}
	wake    chan struct{} // nudges the loop to poll now (on idle->live)

	prev map[string]model.Torrent
	seq  uint64

	consecFails int    // consecutive poll failures (reset on success)
	health      string // "" unknown | model.RtorrentUp | model.RtorrentUnreachable
}

func New(src Source, hub *sse.Hub, live, idle time.Duration, logger *log.Logger) *Poller {
	if live <= 0 {
		live = time.Second
	}
	if idle < live {
		idle = live // idle is the *slower* cadence; never faster than live
	}
	return &Poller{
		src: src, hub: hub, live: live, idle: idle, log: logger,
		done: make(chan struct{}),
		wake: make(chan struct{}, 1),
	}
}

// SetSink installs a per-tick data sink (call before Start).
func (p *Poller) SetSink(s Sink) { p.sink = s }

// Start launches the perpetual loop (idempotent). Call once at startup; it polls
// at the idle cadence until a client connects.
func (p *Poller) Start() {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.mu.Unlock()
	go p.run()
	p.log.Printf("poller running (live=%s, idle/background=%s)", p.live, p.idle)
}

// SetActive switches cadence. Wire to the hub: true on the first client, false
// when the last disconnects. Going live nudges an immediate poll so the joining
// client sees current state rather than a stale idle sample.
func (p *Poller) SetActive(active bool) {
	p.mu.Lock()
	changed := p.active != active
	p.active = active
	p.mu.Unlock()
	if !changed {
		return
	}
	if active {
		p.log.Printf("poller: live (%s) — client connected", p.live)
		select {
		case p.wake <- struct{}{}:
		default:
		}
	} else {
		p.log.Printf("poller: idle (%s background, history only) — no clients", p.idle)
	}
}

// Stop halts the loop (graceful shutdown / tests).
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.done:
	default:
		close(p.done)
	}
}

func (p *Poller) curInterval() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active {
		return p.live
	}
	return p.idle
}

func (p *Poller) run() {
	for {
		p.tick()
		select {
		case <-p.done:
			return
		case <-p.wake: // bumped to live — poll now
		case <-time.After(p.curInterval()):
		}
	}
}

func (p *Poller) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), pollTimeout)
	torrents, globals, err := p.src(ctx)
	cancel()
	if err != nil {
		p.log.Printf("poll error: %v", err)
		p.consecFails++
		// Transition to unreachable only after the debounce; idempotent thereafter.
		if p.health != model.RtorrentUnreachable && p.consecFails >= unreachableAfter {
			p.setHealth(model.RtorrentUnreachable)
		}
		return
	}
	// Success: recovery is immediate, counter resets. The first-ever success also
	// publishes the initial "up" (health == "") so joiners have a cached status.
	p.consecFails = 0
	if p.health != model.RtorrentUp {
		p.setHealth(model.RtorrentUp)
	}

	p.seq++
	ts := time.Now().Unix()
	isFirst := p.prev == nil

	if p.sink != nil {
		p.sink(torrents, globals, ts)
	}

	next := make(map[string]model.Torrent, len(torrents))
	upserts := make([]any, 0)
	for _, t := range torrents {
		next[t.Hash] = t
		if prev, ok := p.prev[t.Hash]; ok {
			if d := t.DiffFrom(prev); d != nil {
				upserts = append(upserts, d)
			}
		} else {
			upserts = append(upserts, t)
		}
	}
	var removed []string
	for h := range p.prev {
		if _, ok := next[h]; !ok {
			removed = append(removed, h)
		}
	}
	p.prev = next

	snap := model.Snapshot{Seq: p.seq, TS: ts, Globals: globals, Torrents: torrents}
	snapMsg := sse.Message{Event: "snapshot", Data: mustJSON(p.log, snap)}
	p.hub.SetSnapshot(snapMsg)

	if isFirst {
		p.hub.Broadcast(snapMsg)
	} else {
		delta := model.Delta{Seq: p.seq, TS: ts, Globals: globals, Upserts: upserts, Removed: removed}
		p.hub.Broadcast(sse.Message{Event: "delta", Data: mustJSON(p.log, delta)})
	}
}

// setHealth records a reachability transition, caches it on the hub for new
// joiners, then broadcasts it. Called only from tick().
func (p *Poller) setHealth(state string) {
	p.health = state
	msg := sse.Message{Event: "status", Data: mustJSON(p.log, model.HealthMsg{
		Rtorrent:         state,
		Since:            time.Now().Unix(),
		ConsecutiveFails: p.consecFails,
	})}
	p.hub.SetStatus(msg) // cache first so a Subscribe racing the Broadcast still gets it
	p.hub.Broadcast(msg)
	p.log.Printf("rtorrent health: %s (consecutiveFails=%d)", state, p.consecFails)
}

func mustJSON(l *log.Logger, v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		l.Printf("json marshal: %v", err)
		return []byte("{}")
	}
	return b
}
