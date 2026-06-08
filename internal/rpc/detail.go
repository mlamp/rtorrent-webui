package rpc

import (
	"context"
	"encoding/json"
	"fmt"

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

func (c *Client) Files(ctx context.Context, hash string) ([]model.File, error) {
	rows, err := multicallRows(c, ctx, "f.multicall", hash,
		"f.path=", "f.size_bytes=", "f.completed_chunks=", "f.size_chunks=", "f.priority=")
	if err != nil {
		return nil, err
	}
	out := make([]model.File, 0, len(rows))
	for i, r := range rows {
		if len(r) < 5 {
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
	return out, nil
}

func (c *Client) Peers(ctx context.Context, hash string) ([]model.Peer, error) {
	rows, err := multicallRows(c, ctx, "p.multicall", hash,
		"p.address=", "p.port=", "p.client_version=", "p.down_rate=", "p.up_rate=",
		"p.completed_percent=", "p.is_encrypted=", "p.is_incoming=")
	if err != nil {
		return nil, err
	}
	out := make([]model.Peer, 0, len(rows))
	for _, r := range rows {
		if len(r) < 8 {
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
		})
	}
	return out, nil
}

func (c *Client) Trackers(ctx context.Context, hash string) ([]model.Tracker, error) {
	rows, err := multicallRows(c, ctx, "t.multicall", hash,
		"t.url=", "t.is_enabled=", "t.type=", "t.latest_event=", "t.success_counter=")
	if err != nil {
		return nil, err
	}
	out := make([]model.Tracker, 0, len(rows))
	for i, r := range rows {
		if len(r) < 5 {
			continue
		}
		out = append(out, model.Tracker{
			Index:       i,
			URL:         asStr(r[0]),
			Enabled:     asIntRaw(r[1]) != 0,
			Type:        int(asIntRaw(r[2])),
			LatestEvent: asStr(r[3]),
			Success:     asIntRaw(r[4]),
		})
	}
	return out, nil
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
	_, err := c.Call(ctx, "t.is_enabled.set", fmt.Sprintf("%s:t%d", hash, index), v)
	return err
}
