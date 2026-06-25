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
