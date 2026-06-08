// Package geoip resolves peer IPs to country codes from a MaxMind/DB-IP mmdb.
// The database is not shipped (licensing); a path is supplied via config.
package geoip

import (
	"net/netip"
	"sync"

	geoip2 "github.com/oschwald/geoip2-golang/v2"
)

type Reader struct {
	mu sync.RWMutex
	r  *geoip2.Reader
}

func New(path string) (*Reader, error) {
	r, err := geoip2.Open(path)
	if err != nil {
		return nil, err
	}
	return &Reader{r: r}, nil
}

// Country returns the ISO-3166 alpha-2 code for an IP, or "" if unknown.
func (g *Reader) Country(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}
	g.mu.RLock()
	r := g.r
	g.mu.RUnlock()
	if r == nil {
		return ""
	}
	rec, err := r.Country(addr)
	if err != nil || rec == nil {
		return ""
	}
	return rec.Country.ISOCode
}

func (g *Reader) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.r != nil {
		return g.r.Close()
	}
	return nil
}
