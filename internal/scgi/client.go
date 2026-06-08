package scgi

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// Client is a concurrency-bounded SCGI client. rtorrent closes the socket after
// each response, so every call dials a fresh connection; the semaphore bounds how
// many hit rtorrent's single RPC thread at once.
type Client struct {
	network string // "unix" or "tcp"
	address string
	sem     chan struct{}
	timeout time.Duration
}

// New builds a client. addr forms:
//
//	/path/to.sock  or  unix:/path        -> unix socket
//	host:port      or  tcp://host:port   -> tcp
func New(addr string, maxInflight int, timeout time.Duration) *Client {
	network, address := parseAddr(addr)
	if maxInflight <= 0 {
		maxInflight = 8
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		network: network,
		address: address,
		sem:     make(chan struct{}, maxInflight),
		timeout: timeout,
	}
}

func parseAddr(addr string) (network, address string) {
	switch {
	case strings.HasPrefix(addr, "unix:"):
		return "unix", strings.TrimPrefix(addr, "unix:")
	case strings.HasPrefix(addr, "tcp://"):
		return "tcp", strings.TrimPrefix(addr, "tcp://")
	case strings.HasPrefix(addr, "/"):
		return "unix", addr
	default:
		return "tcp", addr
	}
}

// Addr returns a human-readable "network:address" for logs.
func (c *Client) Addr() string { return c.network + ":" + c.address }

// Do sends one SCGI request and returns the response body.
func (c *Client) Do(ctx context.Context, contentType string, body []byte) ([]byte, error) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, c.network, c.address)
	if err != nil {
		return nil, fmt.Errorf("scgi dial %s: %w", c.Addr(), err)
	}
	defer conn.Close()

	deadline := time.Now().Add(c.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	if _, err := conn.Write(encodeRequest(contentType, body)); err != nil {
		return nil, fmt.Errorf("scgi write %s: %w", c.Addr(), err)
	}
	// Signal end-of-request; rtorrent half-closes after it responds.
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}

	raw, err := io.ReadAll(conn)
	if err != nil {
		return nil, fmt.Errorf("scgi read %s: %w", c.Addr(), err)
	}
	return parseResponse(raw)
}
