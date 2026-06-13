// Package scgi implements a minimal SCGI client for talking to rtorrent's RPC
// listener (unix socket or TCP). rtorrent serves XML-RPC/JSON-RPC over SCGI and
// closes the connection after each response, so callers dial per request.
package scgi

import (
	"bytes"
	"fmt"
	"io"
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
// The connection is close-delimited, so a daemon that dies mid-write looks
// like a clean EOF upstream; the declared Content-Length is the only
// truncation signal, and a short body is rejected as io.ErrUnexpectedEOF.
func parseResponse(raw []byte) ([]byte, error) {
	// Split at whichever separator comes first: with bare-LF headers (the
	// lenient fallback) a CRLF-formatted body may contain "\r\n\r\n", which
	// must not be mistaken for the end of the headers.
	idx, sepLen := bytes.Index(raw, []byte("\r\n\r\n")), 4
	if lf := bytes.Index(raw, []byte("\n\n")); lf >= 0 && (idx < 0 || lf < idx) {
		idx, sepLen = lf, 2
	}
	if idx < 0 {
		return nil, fmt.Errorf("scgi: no header/body separator in %d-byte response", len(raw))
	}
	headers, body := raw[:idx], raw[idx+sepLen:]
	if want, ok := contentLength(headers); ok && len(body) < want {
		return nil, fmt.Errorf("scgi: truncated response: body %d bytes, Content-Length %d: %w",
			len(body), want, io.ErrUnexpectedEOF)
	}
	return body, nil
}

// contentLength extracts the Content-Length value from the response headers,
// if present and well-formed.
func contentLength(headers []byte) (int, bool) {
	for _, line := range bytes.Split(headers, []byte("\n")) {
		name, value, ok := bytes.Cut(bytes.TrimSuffix(line, []byte("\r")), []byte(":"))
		if !ok || !bytes.EqualFold(name, []byte("Content-Length")) {
			continue
		}
		n, err := strconv.Atoi(string(bytes.TrimSpace(value)))
		if err != nil || n < 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
