// Package model defines the wire types shared by the poller, SSE hub, and API.
package model

// Status is a UI-facing torrent state derived in Go from rtorrent's raw flags,
// so the frontend never re-derives it.
type Status string

const (
	StatusStopped     Status = "stopped"
	StatusDownloading Status = "downloading"
	StatusSeeding     Status = "seeding"
	StatusPaused      Status = "paused"
	StatusHashing     Status = "hashing"
	StatusError       Status = "error"
)

// Torrent is the per-row snapshot the table renders. done% and ETA are computed
// client-side, so they are intentionally absent here.
type Torrent struct {
	Hash           string `json:"hash"`
	Name           string `json:"name"`
	Size           int64  `json:"size"`
	Completed      int64  `json:"completed"`
	DownRate       int64  `json:"downRate"`
	UpRate         int64  `json:"upRate"`
	UpTotal        int64  `json:"upTotal"`
	Ratio          int64  `json:"ratio"` // permille (rtorrent ratio*1000)
	Status         Status `json:"status"`
	Label          string `json:"label"`
	Directory      string `json:"directory"`
	PeersConnected int64  `json:"peersConnected"`
	PeersTotal     int64  `json:"peersTotal"`
	SeedsConnected int64  `json:"seedsConnected"`
	SeedsTotal     int64  `json:"seedsTotal"`
	Tracker        string `json:"tracker"`
	Added          int64  `json:"added"`
	Message        string `json:"message"`
}

// DiffFrom returns a map of only the fields that differ from prev (plus hash),
// or nil if nothing changed. This partial-patch shape is what lets the frontend
// update single cells without re-rendering rows.
func (t Torrent) DiffFrom(prev Torrent) map[string]any {
	m := map[string]any{}
	if t.Name != prev.Name {
		m["name"] = t.Name
	}
	if t.Size != prev.Size {
		m["size"] = t.Size
	}
	if t.Completed != prev.Completed {
		m["completed"] = t.Completed
	}
	if t.DownRate != prev.DownRate {
		m["downRate"] = t.DownRate
	}
	if t.UpRate != prev.UpRate {
		m["upRate"] = t.UpRate
	}
	if t.UpTotal != prev.UpTotal {
		m["upTotal"] = t.UpTotal
	}
	if t.Ratio != prev.Ratio {
		m["ratio"] = t.Ratio
	}
	if t.Status != prev.Status {
		m["status"] = t.Status
	}
	if t.Label != prev.Label {
		m["label"] = t.Label
	}
	if t.Directory != prev.Directory {
		m["directory"] = t.Directory
	}
	if t.PeersConnected != prev.PeersConnected {
		m["peersConnected"] = t.PeersConnected
	}
	if t.PeersTotal != prev.PeersTotal {
		m["peersTotal"] = t.PeersTotal
	}
	if t.SeedsConnected != prev.SeedsConnected {
		m["seedsConnected"] = t.SeedsConnected
	}
	if t.SeedsTotal != prev.SeedsTotal {
		m["seedsTotal"] = t.SeedsTotal
	}
	if t.Tracker != prev.Tracker {
		m["tracker"] = t.Tracker
	}
	if t.Added != prev.Added {
		m["added"] = t.Added
	}
	if t.Message != prev.Message {
		m["message"] = t.Message
	}
	if len(m) == 0 {
		return nil
	}
	m["hash"] = t.Hash
	return m
}

// Globals are the bottom/top-bar stats. They ride every delta (they're tiny).
type Globals struct {
	DownRate     int64 `json:"downRate"`
	UpRate       int64 `json:"upRate"`
	DownTotal    int64 `json:"downTotal"`
	UpTotal      int64 `json:"upTotal"`
	DownLimit    int64 `json:"downLimit"` // bytes/s, 0 = unlimited
	UpLimit      int64 `json:"upLimit"`
	TorrentCount int   `json:"torrentCount"`
	ActiveCount  int   `json:"activeCount"`
}
