package scgi

import (
	"errors"
	"io"
	"testing"
)

// parseResponse must split headers from body at whichever separator the
// backend emitted first (CRLF per rtorrent, bare LF per the lenient fallback)
// and reject bodies shorter than the declared Content-Length — the connection
// is close-delimited, so a daemon crash mid-write looks like a clean EOF and
// that header is the only truncation signal we get.
func TestParseResponse(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantBody string
		wantErr  error // non-nil: returned error must errors.Is-match this
		anyErr   bool  // an error is required but its identity is open
	}{
		{
			name:     "crlf headers",
			raw:      "Status: 200 OK\r\nContent-Type: application/json\r\nContent-Length: 8\r\n\r\n{\"ok\":1}",
			wantBody: `{"ok":1}`,
		},
		{
			name:     "lf headers",
			raw:      "Status: 200 OK\nContent-Type: application/json\nContent-Length: 8\n\n{\"ok\":1}",
			wantBody: `{"ok":1}`,
		},
		{
			name:     "lf headers with crlf-crlf inside the body",
			raw:      "Status: 200 OK\nContent-Type: text/xml\nContent-Length: 17\n\n<data>\r\n\r\n</data>",
			wantBody: "<data>\r\n\r\n</data>",
		},
		{
			name:    "truncated body vs declared content-length",
			raw:     "Status: 200 OK\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{\"ok\":1234",
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:    "content-length header name is case-insensitive",
			raw:     "Status: 200 OK\r\ncontent-length: 1000\r\n\r\nshort",
			wantErr: io.ErrUnexpectedEOF,
		},
		{
			name:     "missing content-length stays lenient",
			raw:      "Status: 200 OK\r\nContent-Type: application/json\r\n\r\n{\"ok\":1}",
			wantBody: `{"ok":1}`,
		},
		{
			name:   "no separator",
			raw:    "Status: 200 OK\r\nContent-Type: application/json\r\n",
			anyErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := parseResponse([]byte(tt.raw))
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want errors.Is(%v)", err, tt.wantErr)
				}
			case tt.anyErr:
				if err == nil {
					t.Fatalf("expected an error, got body %q", body)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if string(body) != tt.wantBody {
					t.Fatalf("body = %q, want %q", body, tt.wantBody)
				}
			}
		})
	}
}
