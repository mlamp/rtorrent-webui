package scgi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// ErrUnreachable wraps a dial failure that survived the connect-retry budget:
// the rtorrent daemon isn't listening on the socket (down, restarting, or
// crash-looping). Callers map this to a transient "reconnecting" state rather
// than a hard error — rtorrent restarts in ~1s, so a brief blip should not
// surface to the user.
var ErrUnreachable = errors.New("rtorrent unreachable")

// connectBudget bounds the total time spent retrying a refused/failed dial. It
// is long enough to ride a fast daemon restart (~1s) but short enough to fail
// fast and report "unreachable" when the daemon is genuinely down. A var (not a
// const) only so tests can shrink it; production never changes it.
var connectBudget = 4 * time.Second

// Client is a concurrency-bounded SCGI client. rtorrent closes the socket after
// each response, so every call dials a fresh connection; the semaphore bounds how
// many hit rtorrent's single RPC thread at once.
type Client struct {
	network     string // "unix" or "tcp"
	address     string
	sem         chan struct{}
	dialTimeout time.Duration // per-attempt connect timeout (fail fast if down)
	readTimeout time.Duration // whole request after connect (nginx scgi_read_timeout parity)
}

// New builds a client. addr forms:
//
//	/path/to.sock  or  unix:/path        -> unix socket
//	host:port      or  tcp://host:port   -> tcp
//
// dialTimeout bounds a single connect attempt; readTimeout bounds the request
// once connected (set it generously — large d.multicall replies are slow, and
// abandoning one mid-flight is what trips rtorrent's scgi thread).
func New(addr string, maxInflight int, dialTimeout, readTimeout time.Duration) *Client {
	network, address := parseAddr(addr)
	if maxInflight <= 0 {
		maxInflight = 8
	}
	if dialTimeout <= 0 {
		dialTimeout = 3 * time.Second
	}
	if readTimeout <= 0 {
		readTimeout = 60 * time.Second
	}
	return &Client{
		network:     network,
		address:     address,
		sem:         make(chan struct{}, maxInflight),
		dialTimeout: dialTimeout,
		readTimeout: readTimeout,
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

// dial connects, retrying a failed/refused dial with exponential backoff until
// connectBudget (or the caller's ctx deadline) is exhausted. This rides over a
// fast daemon restart so it never surfaces; a genuinely-down daemon exhausts the
// budget and returns ErrUnreachable.
func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	giveUp := time.Now().Add(connectBudget)
	if dl, ok := ctx.Deadline(); ok && dl.Before(giveUp) {
		giveUp = dl
	}

	backoff := 100 * time.Millisecond
	var lastErr error
	for {
		dialer := net.Dialer{Timeout: c.dialTimeout}
		conn, err := dialer.DialContext(ctx, c.network, c.address)
		if err == nil {
			return conn, nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// No room left in the budget for another backoff+attempt.
		if !time.Now().Add(backoff).Before(giveUp) {
			break
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if backoff < time.Second {
			backoff *= 2
		}
	}
	// %w on ErrUnreachable so callers can errors.Is() it; lastErr for the detail.
	return nil, fmt.Errorf("scgi dial %s: %w (%v)", c.Addr(), ErrUnreachable, lastErr)
}

// Do sends one SCGI request and returns the response body.
func (c *Client) Do(ctx context.Context, contentType string, body []byte) ([]byte, error) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	deadline := time.Now().Add(c.readTimeout)
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
