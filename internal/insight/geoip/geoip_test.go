package geoip

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Run with: GEOIP_DB=/path/to.mmdb go test ./internal/insight/geoip/
func TestCountryLookup(t *testing.T) {
	path := os.Getenv("GEOIP_DB")
	if path == "" {
		t.Skip("set GEOIP_DB to run")
	}
	r, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if got := r.Country("8.8.8.8"); got != "US" {
		t.Errorf("Country(8.8.8.8) = %q, want US", got)
	}
	if got := r.Country("1.1.1.1"); got == "" {
		t.Error("Country(1.1.1.1) = empty, want a code")
	}
	if got := r.Country("not-an-ip"); got != "" {
		t.Errorf("Country(not-an-ip) = %q, want empty", got)
	}
}

// writeTestMMDB writes a minimal GeoLite2-Country mmdb that maps every IP to
// US, so tests can run without a licensed database.
func writeTestMMDB(t *testing.T) string {
	t.Helper()
	var b []byte
	appendStr := func(s string) { // mmdb UTF-8 string, lengths <= 28 only
		b = append(b, byte(0x40+len(s)))
		b = append(b, s...)
	}
	// Search tree: one 32-bit-record node; both branches point at the sole
	// data record (value = node_count + 16-byte separator + offset 0 = 17).
	b = append(b, 0x00, 0x00, 0x00, 0x11, 0x00, 0x00, 0x00, 0x11)
	b = append(b, make([]byte, 16)...) // data section separator
	// Data section: {"country": {"iso_code": "US"}}
	b = append(b, 0xE1)
	appendStr("country")
	b = append(b, 0xE1)
	appendStr("iso_code")
	appendStr("US")
	// Metadata: marker followed by a 9-entry map.
	b = append(b, 0xAB, 0xCD, 0xEF)
	b = append(b, "MaxMind.com"...)
	b = append(b, 0xE9)
	appendStr("binary_format_major_version")
	b = append(b, 0xA1, 0x02) // uint16 2
	appendStr("binary_format_minor_version")
	b = append(b, 0xA0) // uint16 0
	appendStr("build_epoch")
	b = append(b, 0x00, 0x02) // uint64 0
	appendStr("database_type")
	appendStr("GeoLite2-Country")
	appendStr("description")
	b = append(b, 0xE0) // empty map
	appendStr("ip_version")
	b = append(b, 0xA1, 0x06) // uint16 6
	appendStr("languages")
	b = append(b, 0x00, 0x04) // empty array
	appendStr("node_count")
	b = append(b, 0xC1, 0x01) // uint32 1
	appendStr("record_size")
	b = append(b, 0xA1, 0x20) // uint16 32

	path := filepath.Join(t.TempDir(), "test.mmdb")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCountryFixtureLookup(t *testing.T) {
	r, err := New(writeTestMMDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if got := r.Country("8.8.8.8"); got != "US" {
		t.Errorf("Country(8.8.8.8) = %q, want US", got)
	}
}

func TestCountryAfterClose(t *testing.T) {
	r, err := New(writeTestMMDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := r.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
	if got := r.Country("8.8.8.8"); got != "" {
		t.Errorf("Country after Close = %q, want empty", got)
	}
}

// TestCountryConcurrentWithClose verifies that Close fully synchronizes with
// in-flight Country calls: the lookup must happen under the read lock, and
// Close must nil out the inner reader so later calls return "". Run with
// -race; the unsynchronized variant is a use-after-close on the mmdb.
func TestCountryConcurrentWithClose(t *testing.T) {
	g, err := New(writeTestMMDB(t))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				g.Country("8.8.8.8")
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	for closed := false; !closed; {
		select {
		case <-done:
			closed = true
		default:
			if err := g.Close(); err != nil {
				t.Fatalf("Close() = %v", err)
			}
		}
	}
	if got := g.Country("8.8.8.8"); got != "" {
		t.Errorf("Country after Close = %q, want empty", got)
	}
}
