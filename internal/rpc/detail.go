package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

func multicallRows(c *Client, ctx context.Context, method, hash string, fields ...string) ([][]json.RawMessage, error) {
	params := make([]any, 0, len(fields)+2)
	params = append(params, hash, "")
	for _, f := range fields {
		params = append(params, f)
	}
	res, err := c.Call(ctx, method, params...)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, nil
	}
	var rows [][]json.RawMessage
	if err := json.Unmarshal(res, &rows); err != nil {
		return nil, fmt.Errorf("%s decode: %w (got %.120q)", method, err, res)
	}
	return rows, nil
}

// Request field lists. The decoders below read rows POSITIONALLY against these,
// so the order here is the contract — keep the two in lockstep (the decode tests
// pin the alignment).
var (
	fileFields    = []string{"f.path=", "f.size_bytes=", "f.completed_chunks=", "f.size_chunks=", "f.priority="}
	peerFields    = []string{"p.address=", "p.port=", "p.client_version=", "p.down_rate=", "p.up_rate=", "p.completed_percent=", "p.is_encrypted=", "p.is_incoming=", "p.is_snubbed=", "p.down_total="}
	trackerFields = []string{"t.url=", "t.is_enabled=", "t.type=", "t.latest_event=", "t.success_counter=", "t.failed_counter=", "t.failed_time_last=", "t.success_time_last="}
)

func (c *Client) Files(ctx context.Context, hash string) ([]model.File, error) {
	rows, err := multicallRows(c, ctx, "f.multicall", hash, fileFields...)
	if err != nil {
		return nil, err
	}
	return decodeFiles(rows), nil
}

func decodeFiles(rows [][]json.RawMessage) []model.File {
	out := make([]model.File, 0, len(rows))
	for i, r := range rows {
		if len(r) < len(fileFields) {
			continue
		}
		f := model.File{
			Index:           i,
			Path:            asStr(r[0]),
			Size:            asIntRaw(r[1]),
			CompletedChunks: asIntRaw(r[2]),
			SizeChunks:      asIntRaw(r[3]),
			Priority:        int(asIntRaw(r[4])),
		}
		if f.SizeChunks > 0 {
			f.Done = float64(f.CompletedChunks) / float64(f.SizeChunks)
		}
		out = append(out, f)
	}
	return out
}

func (c *Client) Peers(ctx context.Context, hash string) ([]model.Peer, error) {
	rows, err := multicallRows(c, ctx, "p.multicall", hash, peerFields...)
	if err != nil {
		return nil, err
	}
	return decodePeers(rows), nil
}

func decodePeers(rows [][]json.RawMessage) []model.Peer {
	out := make([]model.Peer, 0, len(rows))
	for _, r := range rows {
		if len(r) < len(peerFields) {
			continue
		}
		out = append(out, model.Peer{
			Address:   asStr(r[0]),
			Port:      asIntRaw(r[1]),
			Client:    asStr(r[2]),
			DownRate:  asIntRaw(r[3]),
			UpRate:    asIntRaw(r[4]),
			Progress:  asIntRaw(r[5]),
			Encrypted: asIntRaw(r[6]) != 0,
			Incoming:  asIntRaw(r[7]) != 0,
			Snubbed:   asIntRaw(r[8]) != 0,
			DownTotal: asIntRaw(r[9]),
		})
	}
	return out
}

func (c *Client) Trackers(ctx context.Context, hash string) ([]model.Tracker, error) {
	rows, err := multicallRows(c, ctx, "t.multicall", hash, trackerFields...)
	if err != nil {
		return nil, err
	}
	return decodeTrackers(rows), nil
}

func decodeTrackers(rows [][]json.RawMessage) []model.Tracker {
	out := make([]model.Tracker, 0, len(rows))
	for i, r := range rows {
		if len(r) < len(trackerFields) {
			continue
		}
		out = append(out, model.Tracker{
			Index:       i,
			URL:         asStr(r[0]),
			Enabled:     asIntRaw(r[1]) != 0,
			Type:        int(asIntRaw(r[2])),
			LatestEvent: trackerEvent(r[3]),
			Success:     asIntRaw(r[4]),
			Failed:      asIntRaw(r[5]),
			FailedAt:    asIntRaw(r[6]),
			SuccessAt:   asIntRaw(r[7]),
		})
	}
	return out
}

// trackerEvent turns rtorrent's t.latest_event into a word. libtorrent returns an
// INTEGER event id (Tracker::event_enum: 0 none, 1 completed, 2 started, 3 stopped,
// 4 scrape), so decoding it as a string would surface a bare digit — and id 0
// (the steady state) as the literal "0", which is truthy and defeats the UI's
// "working/disabled" fallback. We map ids to words and return "" for none. Older
// builds that already hand back a word pass through (with "none" normalised to "").
func trackerEvent(raw json.RawMessage) string {
	s := strings.TrimSpace(asStr(raw))
	if s == "" {
		return ""
	}
	if n, err := strconv.Atoi(s); err == nil {
		switch n {
		case 1:
			return "completed"
		case 2:
			return "started"
		case 3:
			return "stopped"
		case 4:
			return "scrape"
		default:
			return "" // 0 / unknown → no event, let the UI fall back to working/disabled
		}
	}
	if w := strings.ToLower(s); w != "none" {
		return w
	}
	return ""
}

// Pieces returns the real per-piece completion (bitfield) plus chunk counts for a
// torrent — the source of truth for the detail PIECES map. d.bitfield is a hex
// string (or the "0" sentinel = complete). Fetched only when a detail panel opens.
func (c *Client) Pieces(ctx context.Context, hash string) (model.Pieces, error) {
	results, errs, err := c.Batch(ctx, []BatchItem{
		{Method: "d.bitfield", Params: []any{hash}},
		{Method: "d.size_chunks", Params: []any{hash}},
		{Method: "d.completed_chunks", Params: []any{hash}},
		{Method: "d.chunk_size", Params: []any{hash}},
	})
	if err != nil {
		return model.Pieces{}, err
	}
	return piecesFromBatch(results, errs)
}

func piecesFromBatch(results []json.RawMessage, errs []error) (model.Pieces, error) {
	if len(results) < 4 || len(errs) < 4 {
		return model.Pieces{}, fmt.Errorf("pieces: short batch result")
	}
	if errs[0] != nil {
		return model.Pieces{}, fmt.Errorf("d.bitfield: %w", errs[0])
	}
	return model.Pieces{
		Bitfield:        asStr(results[0]),
		SizeChunks:      asIntRaw(results[1]),
		CompletedChunks: asIntRaw(results[2]),
		ChunkSize:       asIntRaw(results[3]),
	}, nil
}

// SetFilePriority sets a file's priority (0 off, 1 normal, 2 high) and applies it.
func (c *Client) SetFilePriority(ctx context.Context, hash string, index, prio int) error {
	if _, err := c.Call(ctx, "f.priority.set", fmt.Sprintf("%s:f%d", hash, index), prio); err != nil {
		return err
	}
	_, err := c.Call(ctx, "d.update_priorities", hash)
	return err
}

// SetTrackerEnabled enables/disables a tracker by index.
func (c *Client) SetTrackerEnabled(ctx context.Context, hash string, index int, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	if _, err := c.Call(ctx, "t.is_enabled.set", fmt.Sprintf("%s:t%d", hash, index), v); err != nil {
		return err
	}
	// The cached primary-tracker host is derived from enabled-state; invalidate it
	// so the next poll re-resolves which tracker is primary.
	c.trackerMu.Lock()
	delete(c.trackerCache, hash)
	c.trackerMu.Unlock()
	return nil
}
