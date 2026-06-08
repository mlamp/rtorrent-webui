package rpc

import (
	"context"
	"encoding/json"
	"fmt"
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
	items := []BatchItem{
		{Method: "d.multicall2", Params: mcParams},
		{Method: "throttle.global_down.rate", Params: []any{""}},
		{Method: "throttle.global_up.rate", Params: []any{""}},
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
		DownRate:     asInt(results[1]),
		UpRate:       asInt(results[2]),
		DownTotal:    asInt(results[3]),
		UpTotal:      asInt(results[4]),
		DownLimit:    asInt(results[5]),
		UpLimit:      asInt(results[6]),
		TorrentCount: len(torrents),
	}
	for _, t := range torrents {
		if t.DownRate > 0 || t.UpRate > 0 {
			g.ActiveCount++
		}
	}
	return torrents, g, nil
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
			PeersConnected: asIntRaw(r[15]),
			SeedsConnected: asIntRaw(r[17]),
			Added:          asIntRaw(r[19]),
			Message:        asStr(r[12]),
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
