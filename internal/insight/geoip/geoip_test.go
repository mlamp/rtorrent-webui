package geoip

import (
	"os"
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
