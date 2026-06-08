// Package poll runs the single shared poll loop: each tick it pulls the torrent
// list + globals from a Source, computes a delta vs the previous snapshot, and
// publishes both to the SSE hub. One loop serves all browsers (rtorrent load is
// O(1) in browser count).
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

// Source produces the current torrent list + globals for one tick.
type Source func(ctx context.Context) ([]model.Torrent, model.Globals, error)

type Poller struct {
	src      Source
	hub      *sse.Hub
	interval time.Duration
	log      *log.Logger

	mu      sync.Mutex
	running bool
	stop    chan struct{}

	prev map[string]model.Torrent
	seq  uint64
}

func New(src Source, hub *sse.Hub, interval time.Duration, logger *log.Logger) *Poller {
	if interval <= 0 {
		interval = time.Second
	}
	return &Poller{src: src, hub: hub, interval: interval, log: logger}
}

// Start begins the loop (idempotent). Called when the first SSE client connects.
func (p *Poller) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return
	}
	p.running = true
	p.stop = make(chan struct{})
	p.prev = nil
	go p.loop(p.stop)
	p.log.Printf("poller started (interval=%s)", p.interval)
}

// Stop halts the loop (idempotent). Called when the last SSE client disconnects.
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	p.running = false
	close(p.stop)
	p.log.Printf("poller stopped (no subscribers)")
}

func (p *Poller) loop(stop chan struct{}) {
	p.tick()
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			p.tick()
		}
	}
}

func (p *Poller) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), p.interval+5*time.Second)
	torrents, globals, err := p.src(ctx)
	cancel()
	if err != nil {
		p.log.Printf("poll error: %v", err)
		return
	}

	p.seq++
	ts := time.Now().Unix()
	isFirst := p.prev == nil

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

func mustJSON(l *log.Logger, v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		l.Printf("json marshal: %v", err)
		return []byte("{}")
	}
	return b
}
