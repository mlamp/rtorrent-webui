package poll

import (
	"context"
	"fmt"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

// MockSource returns a Source that simulates n torrents with ~1/3 of them
// changing rates/progress each tick — for load-testing the SSE pipeline and the
// frontend table without a real swarm. Deterministic (no RNG) so runs reproduce.
func MockSource(n int) Source {
	labels := []string{"linux", "movies", "music", "science", ""}
	// Hosts (not full announce URLs): matches what the real poll path stores in
	// Torrent.Tracker after enrichment, so -mock exercises the same shape.
	trackers := []string{"bgp.technology", "empirehost.me", "tracker.opentrackr.org", "hd-space.pw"}
	chunkSizes := []int64{256 << 10, 512 << 10, 1 << 20, 2 << 20, 4 << 20} // realistic power-of-two piece sizes
	names := []string{
		"ubuntu-24.04.2-desktop-amd64", "debian-12.5.0-amd64-netinst",
		"Sintel.2010.2160p.UHD.BluRay.x265", "Big.Buck.Bunny.1080p.h264",
		"archlinux-2026.06.01-x86_64", "fedora-workstation-40-x86_64",
		"Cosmos.Laundromat.4K.AV1", "NASA.Voyager.Mission.Archive.2025",
	}
	tor := make([]model.Torrent, n)
	for i := range tor {
		size := int64(200<<20) + int64(i)*(7<<20)
		chunk := chunkSizes[i%len(chunkSizes)]
		tor[i] = model.Torrent{
			Hash:           fmt.Sprintf("%040X", i+1),
			Name:           fmt.Sprintf("%s.%05d.bin", names[i%len(names)], i),
			Size:           size,
			Completed:      int64(i%5) * (10 << 20),
			DownTotal:      int64(i%5) * (10 << 20),
			Label:          labels[i%len(labels)],
			Tracker:        trackers[i%len(trackers)],
			Status:         model.StatusSeeding,
			PeersTotal:     int64(i % 200),
			SeedsConnected: int64(i % 12),
			ChunkSize:      chunk,
			SizeChunks:     (size + chunk - 1) / chunk,
		}
		// mirror MockDetail: every third torrent has a dead backup tracker, whose
		// announce failures land in d.message as a warning (not an error status)
		if i%3 == 1 {
			tor[i].Message = "Tracker: [Could not resolve hostname]"
		}
	}
	tick := 0
	var dlTotal, ulTotal int64
	// seed plausible historical totals so the Σ line looks lived-in
	for i := range tor {
		ulTotal += tor[i].Size / 4
		dlTotal += tor[i].Size
	}
	return func(_ context.Context) ([]model.Torrent, model.Globals, error) {
		tick++
		var g model.Globals
		g.TorrentCount = n
		for i := range tor {
			t := &tor[i]
			if (i+tick)%3 == 0 { // ~1/3 of rows change this tick
				t.DownRate = int64(((i*131+tick*977)%8000)+1) << 10
				t.UpRate = int64((i*57+tick*331)%2000) << 10
				t.PeersConnected = int64((i + tick) % 40)
				t.Completed += t.DownRate
				t.DownTotal += t.DownRate // monotonic transfer counter (never collapses)
				if t.Completed >= t.Size {
					t.Completed = t.Size
					t.Status = model.StatusSeeding
					t.DownRate = 0
				} else {
					t.Status = model.StatusDownloading
				}
				if t.UpTotal += t.UpRate; t.Completed > 0 {
					t.Ratio = t.UpTotal * 1000 / t.Completed
				}
				g.ActiveCount++
			} else {
				t.DownRate = 0
				if t.Status == model.StatusSeeding {
					t.UpRate = int64((i*13+tick)%500) << 10
				} else {
					t.UpRate = 0
				}
			}
			// keep completed chunks consistent with completed bytes
			if t.Completed >= t.Size {
				t.CompletedChunks = t.SizeChunks
			} else {
				t.CompletedChunks = t.Completed / t.ChunkSize
			}
			g.DownRate += t.DownRate
			g.UpRate += t.UpRate
		}
		dlTotal += g.DownRate
		ulTotal += g.UpRate
		g.DownTotal = dlTotal
		g.UpTotal = ulTotal
		out := make([]model.Torrent, len(tor))
		copy(out, tor)
		return out, g, nil
	}
}
