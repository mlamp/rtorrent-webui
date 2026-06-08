// Package scgi implements a minimal SCGI client for talking to rtorrent's RPC
// listener (unix socket or TCP). rtorrent serves XML-RPC/JSON-RPC over SCGI and
// closes the connection after each response, so callers dial per request.
package scgi

import (
	"bytes"
	"fmt"
	"strconv"
)

// encodeRequest builds an SCGI request: a netstring of NUL-delimited headers
// followed by the body. The first header MUST be CONTENT_LENGTH; we always send
// SCGI=1 and CONTENT_TYPE so rtorrent doesn't have to sniff.
func encodeRequest(contentType string, body []byte) []byte {
	var h bytes.Buffer
	writeNul := func(s string) { h.WriteString(s); h.WriteByte(0) }
	writeNul("CONTENT_LENGTH")
	writeNul(strconv.Itoa(len(body)))
	writeNul("SCGI")
	writeNul("1")
	writeNul("CONTENT_TYPE")
	writeNul(contentType)

	var out bytes.Buffer
	out.Grow(h.Len() + len(body) + 16)
	out.WriteString(strconv.Itoa(h.Len()))
	out.WriteByte(':')
	out.Write(h.Bytes())
	out.WriteByte(',')
	out.Write(body)
	return out.Bytes()
}

// parseResponse strips the CGI-style response headers ("Status: 200 OK\r\n
// Content-Type: ...\r\nContent-Length: ...\r\n\r\n") and returns the body.
func parseResponse(raw []byte) ([]byte, error) {
	sep := []byte("\r\n\r\n")
	idx := bytes.Index(raw, sep)
	if idx < 0 {
		sep = []byte("\n\n")
		idx = bytes.Index(raw, sep)
	}
	if idx < 0 {
		return nil, fmt.Errorf("scgi: no header/body separator in %d-byte response", len(raw))
	}
	return raw[idx+len(sep):], nil
}
