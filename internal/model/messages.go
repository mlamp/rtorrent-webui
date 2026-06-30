package model

// Snapshot is sent once to each new SSE subscriber (event: snapshot).
type Snapshot struct {
	Seq      uint64    `json:"seq"`
	TS       int64     `json:"ts"`
	Globals  Globals   `json:"globals"`
	Torrents []Torrent `json:"torrents"`
}

// Delta is sent each tick after the snapshot (event: delta). Upserts are partial
// maps (full for newly-added torrents, only-changed-fields for updates); Removed
// is a list of hashes.
type Delta struct {
	Seq     uint64   `json:"seq"`
	TS      int64    `json:"ts"`
	Globals Globals  `json:"globals"`
	Upserts []any    `json:"upserts"` // full Torrent for adds, partial map for changes
	Removed []string `json:"removed"`
}

// Rtorrent reachability values for HealthMsg.Rtorrent.
const (
	RtorrentUp          = "up"
	RtorrentUnreachable = "unreachable"
)

// HealthMsg reports rtorrent reachability to browsers (event: status). Emitted on
// TRANSITION only and cached by the hub so a client joining mid-outage learns the
// current state immediately. Orthogonal to Snapshot/Delta: carries no seq/ts, so it
// never feeds the frontend staleness gauge or seq tripwire.
type HealthMsg struct {
	Rtorrent         string `json:"rtorrent"`                   // RtorrentUp | RtorrentUnreachable
	Since            int64  `json:"since"`                      // unix secs this state began
	ConsecutiveFails int    `json:"consecutiveFails,omitempty"` // debug; omitted when 0
}
