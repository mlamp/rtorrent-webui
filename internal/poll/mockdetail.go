package poll

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

// MockDetail serves deterministic detail-tab data (files / peers / trackers /
// pieces) for -mock mode, so the whole detail view — including the real-bitfield
// piece map — can be driven without a live rtorrent. It mirrors MockSource's
// per-index sizing so a torrent's detail agrees with its row.
type MockDetail struct{}

func NewMockDetail() *MockDetail { return &MockDetail{} }

var mockChunkSizes = []int64{256 << 10, 512 << 10, 1 << 20, 2 << 20, 4 << 20}

// idxOf recovers the MockSource index from a "%040X" hash (which encodes i+1).
func idxOf(hash string) int {
	if v, err := strconv.ParseInt(strings.TrimLeft(hash, "0"), 16, 64); err == nil && v > 0 {
		return int(v) - 1
	}
	return 0
}

func mockSizing(i int) (size, chunk, sizeChunks int64) {
	size = int64(200<<20) + int64(i)*(7<<20)
	chunk = mockChunkSizes[i%len(mockChunkSizes)]
	sizeChunks = (size + chunk - 1) / chunk
	return
}

func (m *MockDetail) Pieces(_ context.Context, hash string) (model.Pieces, error) {
	i := idxOf(hash)
	_, chunk, sizeChunks := mockSizing(i)
	switch i % 4 {
	case 0: // complete → rtorrent's freed-bitfield sentinel
		return model.Pieces{Bitfield: "0", SizeChunks: sizeChunks, CompletedChunks: sizeChunks, ChunkSize: chunk}, nil
	case 3: // not started
		return model.Pieces{Bitfield: strings.Repeat("00", int((sizeChunks+7)/8)), SizeChunks: sizeChunks, CompletedChunks: 0, ChunkSize: chunk}, nil
	}
	// scattered partial — a realistic non-contiguous completion pattern
	bits := make([]bool, sizeChunks)
	var completed int64
	for c := int64(0); c < sizeChunks; c++ {
		if (c*2654435761+int64(i)*40503)%100 < 60 {
			bits[c] = true
			completed++
		}
	}
	return model.Pieces{Bitfield: bitsToHex(bits), SizeChunks: sizeChunks, CompletedChunks: completed, ChunkSize: chunk}, nil
}

// bitsToHex packs chunk-completion bits MSB-first per byte, matching d.bitfield.
func bitsToHex(bits []bool) string {
	b := make([]byte, (len(bits)+7)/8)
	for c, set := range bits {
		if set {
			b[c/8] |= 1 << uint(7-(c%8))
		}
	}
	return hex.EncodeToString(b)
}

func (m *MockDetail) Files(_ context.Context, hash string) ([]model.File, error) {
	i := idxOf(hash)
	names := []string{"video.mkv", "subs/english.srt", "readme.nfo", "extras/sample.mp4", "art/cover.jpg"}
	nf := 1 + i%len(names)
	out := make([]model.File, nf)
	for j := 0; j < nf; j++ {
		size := int64(50<<20) + int64(j)*(13<<20)
		sc := size >> 20
		cc := sc * int64((i+j)%5) / 4
		if cc > sc {
			cc = sc
		}
		done := 0.0
		if sc > 0 {
			done = float64(cc) / float64(sc)
		}
		out[j] = model.File{Index: j, Path: names[j%len(names)], Size: size, SizeChunks: sc, CompletedChunks: cc, Priority: j % 3, Done: done}
	}
	return out, nil
}

func (m *MockDetail) Peers(_ context.Context, hash string) ([]model.Peer, error) {
	i := idxOf(hash)
	clients := []string{"qBittorrent 4.6.2", "Deluge 2.1.1", "Transmission 4.0.5", "libtorrent 2.0.9", "rtorrent 0.16"}
	ccs := []string{"US", "DE", "SE", "NL", "JP", "FR", ""}
	np := i % 6
	out := make([]model.Peer, np)
	for j := 0; j < np; j++ {
		out[j] = model.Peer{
			Address:   fmt.Sprintf("%d.%d.%d.%d", 10+j, (i*7)%256, (j*53)%256, (i+j)%256),
			Port:      int64(6881 + j),
			Client:    clients[(i+j)%len(clients)],
			DownRate:  int64((j*131)%4000) << 10,
			UpRate:    int64((j*57)%800) << 10,
			DownTotal: int64((j*97+i*7)%900+1) << 20,
			Progress:  int64((i*j + 13) % 101),
			Encrypted: (i+j)%2 == 0,
			Incoming:  j%2 == 0,
			Snubbed:   (i+j)%5 == 0,
			Country:   ccs[(i+j)%len(ccs)],
		}
	}
	return out, nil
}

func (m *MockDetail) Trackers(_ context.Context, hash string) ([]model.Tracker, error) {
	i := idxOf(hash)
	urls := []string{
		"https://bgp.technology/announce", "https://empirehost.me/announce",
		"udp://tracker.opentrackr.org:1337/announce", "https://hd-space.pw/announce",
	}
	now := time.Now().Unix()
	out := []model.Tracker{{
		Index: 0, URL: urls[i%len(urls)], Enabled: true, Type: 1,
		LatestEvent: "completed", Success: int64(3 + i%20), SuccessAt: now - 600,
	}}
	// every third torrent carries a dead backup tracker (mirrors the real-world
	// "Tracker: [Could not resolve hostname]" case the failing-state UI exists for)
	if i%3 == 1 {
		out = append(out, model.Tracker{
			Index: 1, URL: "https://dead.invalid/announce", Enabled: true, Type: 1,
			Failed: int64(4 + i%9), FailedAt: now - 120,
		})
	}
	return out, nil
}

func (m *MockDetail) SetFilePriority(_ context.Context, _ string, _, _ int) error        { return nil }
func (m *MockDetail) SetTrackerEnabled(_ context.Context, _ string, _ int, _ bool) error { return nil }
