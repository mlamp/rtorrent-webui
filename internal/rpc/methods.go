package rpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
)

func (c *Client) string(ctx context.Context, method string, params ...any) (string, error) {
	res, err := c.Call(ctx, method, params...)
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(res, &s); err != nil {
		return "", fmt.Errorf("%s: want string, got %.80q", method, res)
	}
	return s, nil
}

func (c *Client) int(ctx context.Context, method string, params ...any) (int64, error) {
	res, err := c.Call(ctx, method, params...)
	if err != nil {
		return 0, err
	}
	// rtorrent may encode numerics as JSON numbers OR JSON strings, so accept both.
	var n int64
	if err := json.Unmarshal(res, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(res, &s); err == nil {
		if v, perr := strconv.ParseInt(s, 10, 64); perr == nil {
			return v, nil
		}
	}
	return 0, fmt.Errorf("%s: want int, got %.80q", method, res)
}

// ClientVersion returns rtorrent's version string.
func (c *Client) ClientVersion(ctx context.Context) (string, error) {
	return c.string(ctx, "system.client_version", "")
}

// APIVersion returns rtorrent's XML-RPC API version number.
func (c *Client) APIVersion(ctx context.Context) (int64, error) {
	return c.int(ctx, "system.api_version", "")
}

// DataURI builds the data: URI rtorrent decodes for JSON-RPC torrent uploads.
// JSON-RPC has no base64 type, so a raw .torrent rides as a base64 data: URI
// that rtorrent's core/manager.cc decode_data_uri() unpacks.
func DataURI(torrent []byte) string {
	return "data:application/x-bittorrent;base64," + base64.StdEncoding.EncodeToString(torrent)
}

// Load adds a torrent. uri may be a magnet:, http(s) URL, local path, or a
// data: URI (see DataURI). cmds are extra commands applied to the new download,
// e.g. "d.custom1.set=mylabel". start begins it now.
//
// CAUTION: rtorrent re-parses every cmd with its full command grammar (','
// splits args, ';' splits commands, and a parsed argument beginning with '$'
// is EXECUTED as a command). Any user-controlled value embedded in a cmd must
// be escaped with QuoteCommandValue and pre-screened for a leading '$'.
func (c *Client) Load(ctx context.Context, start bool, uri string, cmds ...string) error {
	method := "load.normal"
	if start {
		method = "load.start"
	}
	params := make([]any, 0, len(cmds)+2)
	params = append(params, "", uri)
	for _, cmd := range cmds {
		params = append(params, cmd)
	}
	_, err := c.Call(ctx, method, params...)
	return err
}

// Names lists torrent names in a view (default "main") — handy for quick checks.
func (c *Client) Names(ctx context.Context, view string) ([]string, error) {
	if view == "" {
		view = "main"
	}
	res, err := c.Call(ctx, "d.multicall2", "", view, "d.name=")
	if err != nil {
		return nil, err
	}
	var rows [][]string
	if err := json.Unmarshal(res, &rows); err != nil {
		return nil, fmt.Errorf("d.multicall2 decode: %w (got %.120q)", err, res)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if len(r) > 0 {
			out = append(out, r[0])
		}
	}
	return out, nil
}
