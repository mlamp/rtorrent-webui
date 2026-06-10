// Package system reads cheap host metrics from /proc (CPU, load, memory) and
// combines them with per-tick torrent/global figures into a gauge map for the
// history store. It holds the previous /proc/stat snapshot so CPU can be derived
// as a delta between ticks.
//
// NOTE on topology: the webui typically runs in its own container, so /proc here
// reflects the HOST (procfs stat/loadavg/meminfo aren't namespaced) — these are
// host-wide figures, not the rtorrent process specifically.
//
// The Collector is only ever called from the single poll loop, so it needs no
// locking. Any /proc read/parse failure omits just that metric — never panics.
package system

import (
	"os"
	"strconv"
	"strings"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

type Collector struct {
	prevBusy, prevTotal uint64
	havePrev            bool
}

func New() *Collector { return &Collector{} }

// Collect returns the gauge map for this tick. cpu/load/mem come from /proc;
// peers/sess_* come from the tick's torrents+globals.
func (c *Collector) Collect(torrents []model.Torrent, g model.Globals) map[string]int64 {
	m := make(map[string]int64, 8)

	// CPU permille of busy time since the last tick (omitted on the first tick).
	if busy, total, ok := parseStat(readFile("/proc/stat")); ok {
		if v, ok2 := c.cpuDelta(busy, total); ok2 {
			m["cpu"] = v
		}
	}
	// Load average ×100.
	if l1, l5, l15, ok := parseLoadavg(readFile("/proc/loadavg")); ok {
		m["load1"], m["load5"], m["load15"] = l1, l5, l15
	}
	// Memory used permille.
	if used, ok := parseMeminfo(readFile("/proc/meminfo")); ok {
		m["mem"] = used
	}
	// Peers: sum of connected peers across torrents (already polled — free).
	var peers int64
	for i := range torrents {
		peers += torrents[i].PeersConnected
	}
	m["peers"] = peers
	// Session totals straight from rtorrent (reset to 0 on restart — acceptable).
	m["sess_down"] = g.DownTotal
	m["sess_up"] = g.UpTotal
	return m
}

// cpuDelta turns the current busy/total jiffy counters into a 0..1000 permille
// utilization vs the previous snapshot, then stores the snapshot. The first call
// (or a counter reset) returns ok=false so no bogus point is recorded.
func (c *Collector) cpuDelta(busy, total uint64) (int64, bool) {
	prevBusy, prevTotal, have := c.prevBusy, c.prevTotal, c.havePrev
	c.prevBusy, c.prevTotal, c.havePrev = busy, total, true
	if !have || total <= prevTotal || busy < prevBusy {
		return 0, false
	}
	v := int64((busy - prevBusy) * 1000 / (total - prevTotal))
	if v < 0 {
		v = 0
	} else if v > 1000 {
		v = 1000
	}
	return v, true
}

func readFile(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return b
}

// parseStat sums the aggregate "cpu" line of /proc/stat into busy/total jiffies.
// Fields: user nice system idle iowait irq softirq steal [guest guest_nice].
// total = sum of all; idle = idle + iowait; busy = total - idle.
func parseStat(b []byte) (busy, total uint64, ok bool) {
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || f[0] != "cpu" {
			continue
		}
		var idle uint64
		for i := 1; i < len(f); i++ {
			v, err := strconv.ParseUint(f[i], 10, 64)
			if err != nil {
				return 0, 0, false
			}
			total += v
			if i == 4 || i == 5 { // idle, iowait
				idle += v
			}
		}
		if total == 0 {
			return 0, 0, false
		}
		return total - idle, total, true
	}
	return 0, 0, false
}

// parseLoadavg reads the first three floats of /proc/loadavg, each ×100.
func parseLoadavg(b []byte) (l1, l5, l15 int64, ok bool) {
	f := strings.Fields(string(b))
	if len(f) < 3 {
		return 0, 0, 0, false
	}
	p := func(s string) (int64, bool) {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v < 0 {
			return 0, false
		}
		return int64(v*100 + 0.5), true
	}
	var ok1, ok5, ok15 bool
	l1, ok1 = p(f[0])
	l5, ok5 = p(f[1])
	l15, ok15 = p(f[2])
	return l1, l5, l15, ok1 && ok5 && ok15
}

// parseMeminfo computes used-memory permille = (MemTotal-MemAvailable)/MemTotal.
func parseMeminfo(b []byte) (used int64, ok bool) {
	var total, avail uint64
	var haveTotal, haveAvail bool
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			if v, err := strconv.ParseUint(f[1], 10, 64); err == nil {
				total, haveTotal = v, true
			}
		case "MemAvailable:":
			if v, err := strconv.ParseUint(f[1], 10, 64); err == nil {
				avail, haveAvail = v, true
			}
		}
		if haveTotal && haveAvail {
			break
		}
	}
	if !haveTotal || !haveAvail || total == 0 || avail > total {
		return 0, false
	}
	return int64((total - avail) * 1000 / total), true
}
