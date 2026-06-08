// Package rpc speaks JSON-RPC 2.0 to rtorrent over an scgi.Client. rtorrent
// (>=0.16) accepts JSON natively; params[0] is the target ("" for global /
// d.multicall2, an infohash for per-download calls).
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mlamp/rtorrent-webui/internal/scgi"
)

// Client is a JSON-RPC client over SCGI.
type Client struct {
	scgi *scgi.Client

	// trackerCache memoises each torrent's primary tracker host. The announce URL
	// isn't returned by d.multicall2, so Poll enriches it with a per-hash t.multicall;
	// the result is cached (invalidated on a tracker toggle, pruned when a torrent
	// disappears) so steady-state polls do no extra work.
	trackerMu    sync.RWMutex
	trackerCache map[string]string

	// batch defaults to c.Batch; a seam so enrichTrackers can be tested without SCGI.
	batch func(context.Context, []BatchItem) ([]json.RawMessage, []error, error)
}

func New(s *scgi.Client) *Client {
	c := &Client{scgi: s, trackerCache: map[string]string{}}
	c.batch = c.Batch
	return c
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      int    `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *Error          `json:"error"`
}

// Error is a JSON-RPC error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message) }

// Call invokes a single method and returns its raw result.
func (c *Client) Call(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	if params == nil {
		params = []any{}
	}
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1})
	if err != nil {
		return nil, err
	}
	raw, err := c.scgi.Do(ctx, "application/json", body)
	if err != nil {
		return nil, err
	}
	var resp rpcResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("rpc decode: %w (body: %.200q)", err, raw)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

// BatchItem is one call in a batch.
type BatchItem struct {
	Method string
	Params []any
}

// Batch sends multiple calls in one SCGI round-trip and returns results in order.
// A per-item error is returned in the matching errs slot; transport errors abort
// the whole batch.
func (c *Client) Batch(ctx context.Context, items []BatchItem) (results []json.RawMessage, errs []error, _ error) {
	reqs := make([]rpcRequest, len(items))
	for i, it := range items {
		p := it.Params
		if p == nil {
			p = []any{}
		}
		reqs[i] = rpcRequest{JSONRPC: "2.0", Method: it.Method, Params: p, ID: i + 1}
	}
	body, err := json.Marshal(reqs)
	if err != nil {
		return nil, nil, err
	}
	raw, err := c.scgi.Do(ctx, "application/json", body)
	if err != nil {
		return nil, nil, err
	}
	var resps []rpcResponse
	if err := json.Unmarshal(raw, &resps); err != nil {
		return nil, nil, fmt.Errorf("rpc batch decode: %w (body: %.200q)", err, raw)
	}
	results = make([]json.RawMessage, len(items))
	errs = make([]error, len(items))
	for _, r := range resps {
		idx := r.ID - 1
		if idx < 0 || idx >= len(items) {
			continue
		}
		if r.Error != nil {
			errs[idx] = r.Error
		} else {
			results[idx] = r.Result
		}
	}
	return results, errs, nil
}
