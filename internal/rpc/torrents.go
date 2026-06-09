package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

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
	if errs[0] != nil {
		return nil, model.Globals{}, fmt.Errorf("d.multicall2: %w", errs[0])
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

// enrichTrackers fills t.Tracker with each torrent's primary tracker host.
// d.multicall2 can't return the announce URL, so we fetch t.url/t.is_enabled per
// hash — but only for hashes we haven't cached yet (the host is static), batched
// into a single SCGI round-trip. Best-effort: a failed/empty lookup is left blank
// and retried next poll, never cached as a false value.
func (c *Client) enrichTrackers(ctx context.Context, torrents []model.Torrent) {
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
		if host, ok := c.trackerCache[torrents[i].Hash]; ok {
			torrents[i].Tracker = host
		} else {
			needIdx = append(needIdx, i)
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
	if err != nil {
		return // transport error: leave blank, retry next poll
	}

	c.trackerMu.Lock()
	defer c.trackerMu.Unlock()
	for j, i := range needIdx {
		if errs[j] != nil {
			continue // failed row: leave blank, NOT cached, retried next poll
		}
		if host := primaryTrackerHost(results[j]); host != "" {
			c.trackerCache[torrents[i].Hash] = host
			torrents[i].Tracker = host
		}
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
			Hash:           asStr(r[0]),
			Name:           asStr(r[1]),
			Size:           asIntRaw(r[2]),
			Completed:      asIntRaw(r[3]),
			DownRate:       asIntRaw(r[4]),
			UpRate:         asIntRaw(r[5]),
			UpTotal:        asIntRaw(r[6]),
			Ratio:          asIntRaw(r[7]),
			Label:          asStr(r[13]),
			Directory:      asStr(r[14]),
			PeersConnected:  asIntRaw(r[15]),
			SeedsConnected:  asIntRaw(r[17]),
			Added:           asIntRaw(r[19]),
			Message:         asStr(r[12]),
			SizeChunks:      asIntRaw(r[20]),
			CompletedChunks: asIntRaw(r[21]),
			ChunkSize:       asIntRaw(r[22]),
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
	case message != "":
		return model.StatusError
	case isActive == 0:
		return model.StatusPaused
	case left == 0:
		return model.StatusSeeding
	default:
		return model.StatusDownloading
	}
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
