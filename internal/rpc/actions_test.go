package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/scgi"
)

// scgiCall is one JSON-RPC call as received by the fake daemon.
type scgiCall struct {
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
	ID     int               `json:"id"`
}

// fakeSCGI starts a minimal SCGI JSON-RPC responder on a loopback listener and
// returns a Client wired to it plus an accessor for the calls received, in
// order. handle maps one call to its result; a non-nil *Error becomes a
// JSON-RPC error object for that id (a per-item error inside a batch).
func fakeSCGI(t *testing.T, handle func(scgiCall) (any, *Error)) (*Client, func() []scgiCall) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	var mu sync.Mutex
	var calls []scgiCall

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				raw, err := io.ReadAll(conn) // client half-closes after writing
				if err != nil {
					return
				}
				reqs, batch := decodeSCGICalls(scgiBody(raw))
				resps := make([]map[string]any, len(reqs))
				mu.Lock()
				for i, call := range reqs {
					calls = append(calls, call)
					resps[i] = map[string]any{"jsonrpc": "2.0", "id": call.ID}
					if res, rerr := handle(call); rerr != nil {
						resps[i]["error"] = rerr
					} else {
						resps[i]["result"] = res
					}
				}
				mu.Unlock()
				var out []byte
				if batch {
					out, _ = json.Marshal(resps)
				} else {
					out, _ = json.Marshal(resps[0])
				}
				fmt.Fprintf(conn, "Status: 200 OK\r\nContent-Type: application/json\r\n\r\n%s", out)
			}(conn)
		}
	}()

	c := New(scgi.New(ln.Addr().String(), 4, time.Second, 10*time.Second))
	return c, func() []scgiCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]scgiCall(nil), calls...)
	}
}

// scgiBody strips the SCGI netstring framing ("<len>:<NUL-headers>,<body>").
func scgiBody(raw []byte) []byte {
	colon := bytes.IndexByte(raw, ':')
	if colon < 0 {
		return nil
	}
	n, err := strconv.Atoi(string(raw[:colon]))
	if err != nil || colon+1+n+1 > len(raw) {
		return nil
	}
	return raw[colon+1+n+1:]
}

// decodeSCGICalls decodes a JSON-RPC request body: a single object (Call) or an
// array (Batch). The bool reports whether it was a batch.
func decodeSCGICalls(body []byte) ([]scgiCall, bool) {
	trim := bytes.TrimSpace(body)
	if len(trim) > 0 && trim[0] == '[' {
		var reqs []scgiCall
		_ = json.Unmarshal(trim, &reqs)
		return reqs, true
	}
	var req scgiCall
	_ = json.Unmarshal(trim, &req)
	return []scgiCall{req}, false
}

func TestPauseClearsStartedStateBeforeClose(t *testing.T) {
	c, calls := fakeSCGI(t, func(scgiCall) (any, *Error) { return 0, nil })
	if err := c.Pause(context.Background(), "HASH"); err != nil {
		t.Fatal(err)
	}
	sent := calls()
	if len(sent) == 0 {
		t.Fatal("Pause sent no commands")
	}
	// A bare d.close leaves the download visible in the daemon's 'started' view
	// with the persisted started-state (d.state=1) intact: a later d.start is a
	// no-op (View::set_visible early-returns for already-visible downloads, so
	// the resume event never fires) and the daemon silently resumes the torrent
	// on restart. Pause must move it to the stopped view (d.stop / d.try_close)
	// before/along with closing the files.
	if m := sent[0].Method; m != "d.stop" && m != "d.try_close" {
		t.Fatalf("first command = %q, want d.stop or d.try_close (started-state must be cleared before closing)", m)
	}
	if m := sent[len(sent)-1].Method; m != "d.close" && m != "d.try_close" {
		t.Fatalf("last command = %q, want the files closed (d.close or d.try_close)", m)
	}
	for _, call := range sent {
		if len(call.Params) == 0 || asStr(call.Params[0]) != "HASH" {
			t.Fatalf("%s targeted params %v, want the infohash first", call.Method, call.Params)
		}
	}
}
