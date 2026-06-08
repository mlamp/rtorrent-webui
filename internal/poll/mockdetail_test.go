package poll

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/bits"
	"testing"

	"github.com/mlamp/rtorrent-webui/internal/api"
)

// Pin the seam: MockDetail must satisfy api.DetailRPC, so a signature change to the
// interface fails `go test ./internal/...`, not only `go build ./cmd/...`.
var _ api.DetailRPC = (*MockDetail)(nil)

func TestIdxOfRoundTrips(t *testing.T) {
	for i := 0; i < 50; i++ {
		hash := fmt.Sprintf("%040X", i+1)
		if got := idxOf(hash); got != i {
			t.Fatalf("idxOf(%q) = %d, want %d", hash, got, i)
		}
	}
}

// The mock bitfield must be internally consistent: its popcount equals
// CompletedChunks, it's the right length for SizeChunks, and the complete case
// uses the "0" sentinel. This is exactly what the frontend decoder relies on.
func TestMockDetailPiecesConsistent(t *testing.T) {
	md := NewMockDetail()
	sawSentinel, sawPartial, sawEmpty := false, false, false
	for i := 0; i < 16; i++ {
		hash := fmt.Sprintf("%040X", i+1)
		p, err := md.Pieces(context.Background(), hash)
		if err != nil {
			t.Fatal(err)
		}
		if p.SizeChunks <= 0 || p.ChunkSize <= 0 {
			t.Fatalf("i=%d bad sizing: %+v", i, p)
		}
		if p.CompletedChunks < 0 || p.CompletedChunks > p.SizeChunks {
			t.Fatalf("i=%d completed out of range: %+v", i, p)
		}
		if p.Bitfield == "0" { // complete sentinel
			sawSentinel = true
			if p.CompletedChunks != p.SizeChunks {
				t.Fatalf("i=%d sentinel but completed(%d) != size(%d)", i, p.CompletedChunks, p.SizeChunks)
			}
			continue
		}
		rawBits, err := hex.DecodeString(p.Bitfield)
		if err != nil {
			t.Fatalf("i=%d bitfield not hex: %v", i, err)
		}
		if want := int((p.SizeChunks + 7) / 8); len(rawBits) != want {
			t.Fatalf("i=%d bitfield bytes = %d, want %d", i, len(rawBits), want)
		}
		var pop int64
		for _, b := range rawBits {
			pop += int64(bits.OnesCount8(b))
		}
		if pop != p.CompletedChunks {
			t.Fatalf("i=%d popcount %d != completedChunks %d", i, pop, p.CompletedChunks)
		}
		if pop == 0 {
			sawEmpty = true
		} else {
			sawPartial = true
		}
	}
	if !sawSentinel || !sawPartial || !sawEmpty {
		t.Fatalf("coverage gap: sentinel=%v partial=%v empty=%v", sawSentinel, sawPartial, sawEmpty)
	}
}

func TestBitsToHexMSBFirst(t *testing.T) {
	// chunk 0 is the most-significant bit of byte 0.
	got := bitsToHex([]bool{true, false, false, false, false, false, false, false})
	if got != "80" {
		t.Fatalf("bitsToHex(chunk0 set) = %q, want 80", got)
	}
	got = bitsToHex([]bool{false, false, false, false, false, false, false, true, true})
	if got != "0180" { // bit7 of byte0, then bit7(MSB) of byte1
		t.Fatalf("bitsToHex = %q, want 0180", got)
	}
}
