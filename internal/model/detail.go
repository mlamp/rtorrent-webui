package model

// File is one file in a torrent (on-demand detail).
type File struct {
	Index          int     `json:"index"`
	Path           string  `json:"path"`
	Size           int64   `json:"size"`
	CompletedChunks int64  `json:"completedChunks"`
	SizeChunks     int64   `json:"sizeChunks"`
	Priority       int     `json:"priority"` // 0 off, 1 normal, 2 high
	Done           float64 `json:"done"`     // 0..1
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
	Country   string `json:"country"` // ISO-3166 alpha-2, filled by GeoIP (M5)
}

// Tracker is one tracker (on-demand detail).
type Tracker struct {
	Index       int    `json:"index"`
	URL         string `json:"url"`
	Enabled     bool   `json:"enabled"`
	Type        int    `json:"type"`
	LatestEvent string `json:"latestEvent"`
	Success     int64  `json:"success"`
}
