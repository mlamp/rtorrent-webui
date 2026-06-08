// Command scgi-probe exercises the SCGI/JSON-RPC transport against a live
// rtorrent: prints version info, adds a magnet and a base64 data: URI .torrent,
// and confirms they appear via d.multicall2. This de-risks the two unknowns —
// SCGI framing and the JSON-RPC torrent-upload path — before the HTTP layer.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/rpc"
	"github.com/mlamp/rtorrent-webui/internal/scgi"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:5000", "rtorrent SCGI address (host:port or /path.sock)")
	seed := flag.Int("seed", 0, "also add N synthetic torrents (for UI/scroll testing)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cl := rpc.New(scgi.New(*addr, 8, 10*time.Second))
	fmt.Printf("probing rtorrent SCGI at %s\n", *addr)

	ver, err := cl.ClientVersion(ctx)
	check("system.client_version", err)
	fmt.Printf("  client_version = %s\n", ver)

	api, err := cl.APIVersion(ctx)
	check("system.api_version", err)
	fmt.Printf("  api_version    = %d\n", api)

	// magnet add (no base64 needed)
	magnet := "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=probe-magnet"
	check("load magnet", cl.Load(ctx, false, magnet))

	// data: URI .torrent add — the de-risked JSON-RPC upload path
	tor := makeTestTorrent("probe.bin")
	check("load data-uri torrent", cl.Load(ctx, false, rpc.DataURI(tor), "d.custom1.set=probe"))

	time.Sleep(400 * time.Millisecond) // let rtorrent register the adds

	names, err := cl.Names(ctx, "main")
	check("d.multicall2", err)
	fmt.Printf("  torrents (%d): %v\n", len(names), names)

	if !contains(names, "probe.bin") {
		fmt.Println("FAIL: data-uri torrent 'probe.bin' did not appear in the list")
		os.Exit(1)
	}
	fmt.Println("PASS: SCGI framing + JSON-RPC + data-uri upload all verified")

	if *seed > 0 {
		base := []string{
			"ubuntu-24.04.2-desktop-amd64.iso", "debian-12.5.0-amd64-netinst.iso",
			"Sintel.2010.2160p.UHD.BluRay.x265.mkv", "Big.Buck.Bunny.1080p.h264.mp4",
			"archlinux-2026.06.01-x86_64.iso", "fedora-workstation-40-x86_64.iso",
			"Cosmos.Laundromat.4K.AV1.mkv", "NASA.Voyager.Mission.Archive.2025.tar",
		}
		lbl := []string{"linux", "movies", "music", "science"}
		ok := 0
		for i := 0; i < *seed; i++ {
			nm := fmt.Sprintf("%s.%04d", base[i%len(base)], i)
			if err := cl.Load(ctx, false, rpc.DataURI(makeTestTorrent(nm)), "d.custom1.set="+lbl[i%len(lbl)]); err != nil {
				fmt.Printf("seed %d failed: %v\n", i, err)
				break
			}
			ok++
		}
		fmt.Printf("seeded %d synthetic torrents\n", ok)
	}
}

func check(what string, err error) {
	if err != nil {
		fmt.Printf("FAIL: %s: %v\n", what, err)
		os.Exit(1)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// makeTestTorrent bencodes a minimal valid single-file torrent (trackerless).
// rtorrent loads the metadata and lists it (hash-check of the absent data is
// irrelevant to the add path we're verifying). Info keys are in sorted order.
func makeTestTorrent(name string) []byte {
	pieces := make([]byte, 20) // one zero-filled SHA1 piece
	var b bytes.Buffer
	b.WriteString("d4:infod")
	b.WriteString("6:lengthi1e")
	fmt.Fprintf(&b, "4:name%d:%s", len(name), name)
	b.WriteString("12:piece lengthi16384e")
	fmt.Fprintf(&b, "6:pieces%d:", len(pieces))
	b.Write(pieces)
	b.WriteString("ee")
	return b.Bytes()
}
