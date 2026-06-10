package model

// File is one file in a torrent (on-demand detail).
type File struct {
	Index           int     `json:"index"`
	Path            string  `json:"path"`
	Size            int64   `json:"size"`
	CompletedChunks int64   `json:"completedChunks"`
	SizeChunks      int64   `json:"sizeChunks"`
	Priority        int     `json:"priority"` // 0 off, 1 normal, 2 high
	Done            float64 `json:"done"`     // 0..1
}

// Peer is one connected peer (on-demand detail).
type Peer struct {
	Address   string `json:"address"`
	Port      int64  `json:"port"`
	Client    string `json:"client"`
	DownRate  int64  `json:"downRate"`
	UpRate    int64  `json:"upRate"`
	Progress  int64  `json:"progress"` // 0..100
	Encrypted bool   `json:"encrypted"`
	Incoming  bool   `json:"incoming"`
	Snubbed   bool   `json:"snubbed"` // real rtorrent protocol state (p.is_snubbed)
	Country   string `json:"country"` // ISO-3166 alpha-2, filled by GeoIP (M5)
}

// Pieces is the on-demand per-piece completion of a torrent. Bitfield is
// rtorrent's d.bitfield hex string (MSB-first per byte; the literal "0" sentinel
// means "complete, bitfield freed"). The counts let the client decode/validate it
// and render true piece totals.
type Pieces struct {
	Bitfield        string `json:"bitfield"`
	SizeChunks      int64  `json:"sizeChunks"`
	CompletedChunks int64  `json:"completedChunks"`
	ChunkSize       int64  `json:"chunkSize"`
}

// Tracker is one tracker (on-demand detail). Failed/FailedAt/SuccessAt expose
// per-tracker announce health: rtorrent only keeps the LAST failure message
// globally (d.message, set by any tracker in the set), so these counters are
// the only way to show WHICH tracker is erroring.
type Tracker struct {
	Index       int    `json:"index"`
	URL         string `json:"url"`
	Enabled     bool   `json:"enabled"`
	Type        int    `json:"type"`
	LatestEvent string `json:"latestEvent"`
	Success     int64  `json:"success"`
	Failed      int64  `json:"failed"`
	FailedAt    int64  `json:"failedAt"`  // unix; 0 = never failed
	SuccessAt   int64  `json:"successAt"` // unix; 0 = never succeeded
}
