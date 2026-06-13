package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

// torrentFields is the ordered d.multicall2 field set; the decoder reads rows
// positionally against this list.
var torrentFields = []string{
	"d.hash=",                // 0
	"d.name=",                // 1
	"d.size_bytes=",          // 2
	"d.completed_bytes=",     // 3
	"d.down.rate=",           // 4
	"d.up.rate=",             // 5
	"d.up.total=",            // 6
	"d.ratio=",               // 7
	"d.state=",               // 8
	"d.is_active=",           // 9
	"d.is_open=",             // 10
	"d.is_hash_checking=",    // 11
	"d.message=",             // 12
	"d.custom1=",             // 13 (label)
	"d.directory=",           // 14
	"d.peers_connected=",     // 15
	"d.peers_not_connected=", // 16
	"d.peers_complete=",      // 17
	"d.left_bytes=",          // 18
	"d.creation_date=",       // 19
	"d.size_chunks=",         // 20
	"d.completed_chunks=",    // 21
	"d.chunk_size=",          // 22
	"d.down.total=",          // 23 (cumulative downloaded; monotonic through hash-checks)
}

// Poll fetches the full torrent list for a view plus global stats in ONE batched
// SCGI round-trip. view defaults to "main".
func (c *Client) Poll(ctx context.Context, view string) ([]model.Torrent, model.Globals, error) {
	if view == "" {
		view = "main"
	}
	mcParams := make([]any, 0, len(torrentFields)+2)
	mcParams = append(mcParams, "", view)
	for _, f := range torrentFields {
		mcParams = append(mcParams, f)
	}
	// NB: the global DL/UL *rates* are summed from the per-torrent d.down.rate /
	// d.up.rate below, NOT taken from throttle.global_*.rate. The global throttle
	// rate also counts BitTorrent protocol overhead (handshakes, bitfields,
	// have/request/keepalive messages) via node_used_unthrottled, so it shows a
	// non-zero "download" even when only seeding. The per-torrent rates are fed
	// only by the chunk paths, i.e. real payload. We still pull the global
	// totals/limits, which have no per-torrent payload-only equivalent here.
	items := []BatchItem{
		{Method: "d.multicall2", Params: mcParams},
		{Method: "throttle.global_down.total", Params: []any{""}},
		{Method: "throttle.global_up.total", Params: []any{""}},
		{Method: "throttle.global_down.max_rate", Params: []any{""}},
		{Method: "throttle.global_up.max_rate", Params: []any{""}},
	}
	results, errs, err := c.Batch(ctx, items)
	if err != nil {
		return nil, model.Globals{}, err
	}
	// A per-item failure leaves its results slot nil, which asInt would silently
	// coerce to 0 — fabricated totals/limits. Surface every item's error instead.
	for i, e := range errs {
		if e != nil {
			return nil, model.Globals{}, fmt.Errorf("%s: %w", items[i].Method, e)
		}
	}

	torrents, err := decodeTorrents(results[0])
	if err != nil {
		return nil, model.Globals{}, err
	}

	g := model.Globals{
		DownTotal:    asInt(results[1]),
		UpTotal:      asInt(results[2]),
		DownLimit:    asInt(results[3]),
		UpLimit:      asInt(results[4]),
		TorrentCount: len(torrents),
	}
	// Payload-only global rates: sum the per-torrent (chunk-path) rates.
	for _, t := range torrents {
		g.DownRate += t.DownRate
		g.UpRate += t.UpRate
		if t.DownRate > 0 || t.UpRate > 0 {
			g.ActiveCount++
		}
	}
	c.enrichTrackers(ctx, torrents)
	return torrents, g, nil
}

// trackerFetchSeq numbers enrichTrackers flights so each parks a unique
// in-flight marker in the cache and never commits over another flight's work.
var trackerFetchSeq atomic.Uint64

// enrichTrackers fills t.Tracker with each torrent's primary tracker host.
// d.multicall2 can't return the announce URL, so we fetch t.url/t.is_enabled per
// hash — but only for hashes we haven't cached yet (the host is static), batched
// into a single SCGI round-trip. Best-effort: a failed/empty lookup is left blank
// and retried next poll, never cached as a false value.
//
// The batch round-trip runs without trackerMu held, so SetTrackerEnabled's cache
// invalidation can land mid-flight; writing the pre-toggle host afterwards would
// serve a stale primary tracker until the torrent is removed. Each flight
// therefore parks a marker in the cache slot while it fetches and only commits
// while its own marker is still there — a mid-flight delete (or a newer flight's
// marker) wins and the stale result is dropped.
func (c *Client) enrichTrackers(ctx context.Context, torrents []model.Torrent) {
	// A NUL byte can never start a real host, so marked slots are unambiguous.
	mark := "\x00fetch:" + strconv.FormatUint(trackerFetchSeq.Add(1), 10)

	// Prune cache entries for torrents no longer present (bounds growth on a daemon
	// that runs for weeks with add/remove churn), and collect the uncached hashes.
	present := make(map[string]struct{}, len(torrents))
	for i := range torrents {
		present[torrents[i].Hash] = struct{}{}
	}
	var needIdx []int
	c.trackerMu.Lock()
	for h := range c.trackerCache {
		if _, ok := present[h]; !ok {
			delete(c.trackerCache, h)
		}
	}
	for i := range torrents {
		host, ok := c.trackerCache[torrents[i].Hash]
		switch {
		case !ok:
			needIdx = append(needIdx, i)
			c.trackerCache[torrents[i].Hash] = mark
		case strings.HasPrefix(host, "\x00"):
			// another poll's fetch is in flight: leave blank, its result lands next poll
		default:
			torrents[i].Tracker = host
		}
	}
	c.trackerMu.Unlock()
	if len(needIdx) == 0 {
		return
	}

	items := make([]BatchItem, len(needIdx))
	for j, i := range needIdx {
		items[j] = BatchItem{Method: "t.multicall", Params: []any{torrents[i].Hash, "", "t.url=", "t.is_enabled="}}
	}
	results, errs, err := c.batch(ctx, items)

	c.trackerMu.Lock()
	defer c.trackerMu.Unlock()
	for j, i := range needIdx {
		h := torrents[i].Hash
		if c.trackerCache[h] != mark {
			continue // invalidated (or pruned) mid-flight: our snapshot is stale, drop it
		}
		host := ""
		if err == nil && errs[j] == nil {
			host = primaryTrackerHost(results[j])
		}
		if host == "" {
			// transport error / failed row / no trackers: clear the marker so the
			// hash is left blank, NOT cached, and retried next poll.
			delete(c.trackerCache, h)
			continue
		}
		c.trackerCache[h] = host
		torrents[i].Tracker = host
	}
}

// primaryTrackerHost picks the host of the first enabled tracker (falling back to
// the first tracker) from a t.multicall result of [url, is_enabled] rows. Returning
// the host (not the full announce URL) keeps passkeys out of the UI and lets the
// sidebar group torrents by tracker correctly.
func primaryTrackerHost(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var rows [][]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return ""
	}
	first := ""
	for _, r := range rows {
		if len(r) < 2 {
			continue
		}
		host := trackerHost(asStr(r[0]))
		if host == "" {
			continue
		}
		if first == "" {
			first = host
		}
		if asIntRaw(r[1]) != 0 {
			return host // prefer an enabled tracker
		}
	}
	return first
}

// trackerHost extracts the host from an announce URL ("https://t.example/announce:80"
// -> "t.example"); falls back to the raw string for unparseable inputs.
func trackerHost(announce string) string {
	if announce == "" {
		return ""
	}
	if u, err := url.Parse(announce); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return announce
}

func decodeTorrents(raw json.RawMessage) ([]model.Torrent, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var rows [][]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("d.multicall2 decode: %w (got %.120q)", err, raw)
	}
	out := make([]model.Torrent, 0, len(rows))
	for _, r := range rows {
		if len(r) < len(torrentFields) {
			continue
		}
		left := asIntRaw(r[18])
		t := model.Torrent{
			Hash:            asStr(r[0]),
			Name:            asStr(r[1]),
			Size:            asIntRaw(r[2]),
			Completed:       asIntRaw(r[3]),
			DownRate:        asIntRaw(r[4]),
			UpRate:          asIntRaw(r[5]),
			UpTotal:         asIntRaw(r[6]),
			Ratio:           asIntRaw(r[7]),
			Label:           asStr(r[13]),
			Directory:       asStr(r[14]),
			PeersConnected:  asIntRaw(r[15]),
			SeedsConnected:  asIntRaw(r[17]),
			Added:           asIntRaw(r[19]),
			Message:         asStr(r[12]),
			SizeChunks:      asIntRaw(r[20]),
			CompletedChunks: asIntRaw(r[21]),
			ChunkSize:       asIntRaw(r[22]),
			DownTotal:       asIntRaw(r[23]),
		}
		t.PeersTotal = t.PeersConnected + asIntRaw(r[16])
		t.SeedsTotal = t.SeedsConnected
		t.Status = deriveStatus(asIntRaw(r[8]), asIntRaw(r[9]), asIntRaw(r[10]), asIntRaw(r[11]), left, t.Message)
		out = append(out, t)
	}
	return out, nil
}

func deriveStatus(state, isActive, isOpen, isHashChecking, left int64, message string) model.Status {
	switch {
	case isHashChecking != 0:
		return model.StatusHashing
	case state == 0 || isOpen == 0:
		return model.StatusStopped
	case message != "" && !isTrackerWarning(message):
		return model.StatusError
	case isActive == 0:
		return model.StatusPaused
	case left == 0:
		return model.StatusSeeding
	default:
		return model.StatusDownloading
	}
}

// isTrackerWarning matches the TRANSPORT subset of rtorrent's "Tracker: [<msg>]"
// d.message format (core/download.cc receive_tracker_msg). A transport failure
// (resolve/timeout/refused) from ANY tracker in the set puts this on the whole
// torrent — and any success clears it — so with one dead backup tracker it
// flip-flops while the torrent transfers fine on the others. That's tracker
// health, not a torrent error: the row keeps its real transfer status and shows
// the message as a warning; per-tracker failure counters in the detail view say
// which tracker is sick.
//
// A tracker REJECTION is different: an authoritative answer (unregistered
// torrent, banned passkey), typically from a single-tracker private torrent
// where nothing ever clears it — it stays an error so the ERROR filter still
// finds dead torrents. libtorrent surfaces rejections in two shapes: an HTTP
// announce response whose body carries a "failure reason" key becomes
// `Tracker: [Failure reason "..."]` (tracker_http.cc), and a UDP tracker's
// BEP-15 error packet becomes `Tracker: [tracker message: ...]`
// (tracker_udp.cc; other libtorrent revisions spell it
// `received error message: ...`).
func isTrackerWarning(message string) bool {
	const wrap = "Tracker: ["
	if !strings.HasPrefix(message, wrap) {
		return false
	}
	for _, rejection := range []string{
		`Failure reason`,          // HTTP "failure reason" body
		`tracker message:`,        // UDP error packet (libtorrent 0.16)
		`received error message:`, // UDP error packet (other libtorrent revisions)
	} {
		if strings.HasPrefix(message[len(wrap):], rejection) {
			return false
		}
	}
	return true
}

// --- tolerant value coercion (rtorrent mixes JSON numbers and strings) ---

func asStr(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	return string(raw)
}

func asIntRaw(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
	}
	return 0
}

func asInt(raw json.RawMessage) int64 { return asIntRaw(raw) }
