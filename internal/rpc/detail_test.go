package rpc

import (
	"encoding/json"
	"math"
	"testing"
)

// rawRow marshals mixed values to a multicall row. rtorrent mixes JSON numbers and
// numeric strings, so tests feed both to prove asIntRaw tolerance.
func rawRow(vals ...any) []json.RawMessage {
	out := make([]json.RawMessage, len(vals))
	for i, v := range vals {
		b, _ := json.Marshal(v)
		out[i] = b
	}
	return out
}

func TestPeerFieldsAlignment(t *testing.T) {
	if len(peerFields) != 9 {
		t.Fatalf("peerFields has %d entries; decodePeers reads r[0..8] — keep them in lockstep", len(peerFields))
	}
}

func TestDecodePeers(t *testing.T) {
	// is_encrypted=1(int), is_incoming=0(int), is_snubbed="1"(numeric string)
	rows := [][]json.RawMessage{
		rawRow("1.2.3.4", 6881, "qBittorrent 4.6", 1024, 512, 73, 1, 0, "1"),
		rawRow("5.6.7.8", "51413", "Deluge", "0", "0", "0", 0, 1, 0),
		rawRow("short", 1, "x"), // < 9 cols → dropped
	}
	got := decodePeers(rows)
	if len(got) != 2 {
		t.Fatalf("decoded %d peers, want 2 (short row dropped)", len(got))
	}
	p := got[0]
	if !p.Encrypted || p.Incoming || !p.Snubbed {
		t.Fatalf("flag decode wrong: enc=%v inc=%v snub=%v (want true/false/true)", p.Encrypted, p.Incoming, p.Snubbed)
	}
	// guards against an r[7]/r[8] swap mislabeling every peer
	if p.Incoming == p.Snubbed {
		t.Fatal("Incoming and Snubbed decoded identically — r[7]/r[8] likely swapped")
	}
	if p.Port != 6881 || p.DownRate != 1024 || p.Progress != 73 {
		t.Fatalf("numeric fields wrong: %+v", p)
	}
	if got[1].Port != 51413 { // numeric string coerced
		t.Fatalf("string port not coerced: %d", got[1].Port)
	}
}

func TestDecodeFiles(t *testing.T) {
	rows := [][]json.RawMessage{
		rawRow("video.mkv", 16<<20, 8, 16, 1),
		rawRow("empty", 0, 0, 0, 0), // SizeChunks 0 → Done must be 0, never NaN
		rawRow("short", 1),          // dropped
	}
	got := decodeFiles(rows)
	if len(got) != 2 {
		t.Fatalf("decoded %d files, want 2", len(got))
	}
	if got[0].Done != 0.5 {
		t.Fatalf("Done = %v, want 0.5", got[0].Done)
	}
	if got[0].Index != 0 || got[1].Index != 1 {
		t.Fatalf("index not positional: %d,%d", got[0].Index, got[1].Index)
	}
	if math.IsNaN(got[1].Done) || got[1].Done != 0 {
		t.Fatalf("zero-chunk Done = %v, want 0 (no NaN)", got[1].Done)
	}
}

func TestTrackerFieldsAlignment(t *testing.T) {
	if len(trackerFields) != 8 {
		t.Fatalf("trackerFields has %d entries; decodeTrackers reads r[0..7] — keep them in lockstep", len(trackerFields))
	}
}

func TestDecodeTrackers(t *testing.T) {
	rows := [][]json.RawMessage{
		rawRow("https://t.example/announce", 1, 0, 1, 7, 0, 0, 1718000000), // event 1 → completed, healthy
		rawRow("udp://b.example/x", 0, 1, 0, 0, "12", 1718000300, 0),       // event 0 → "" (UI fallback fires); failing
		rawRow("short", 1), // dropped
	}
	got := decodeTrackers(rows)
	if len(got) != 2 {
		t.Fatalf("decoded %d trackers, want 2", len(got))
	}
	if !got[0].Enabled || got[1].Enabled {
		t.Fatalf("enabled decode wrong: %v %v", got[0].Enabled, got[1].Enabled)
	}
	if got[0].LatestEvent != "completed" {
		t.Fatalf("event id 1 → %q, want completed", got[0].LatestEvent)
	}
	if got[1].LatestEvent != "" {
		t.Fatalf("event id 0 → %q, want \"\" so the UI shows working/disabled", got[1].LatestEvent)
	}
	// per-tracker health: healthy tracker has no fails, failing one decodes its
	// counter (numeric string coerced) and last-failed/last-ok timestamps
	if got[0].Failed != 0 || got[0].SuccessAt != 1718000000 {
		t.Fatalf("healthy tracker decode wrong: %+v", got[0])
	}
	if got[1].Failed != 12 || got[1].FailedAt != 1718000300 || got[1].SuccessAt != 0 {
		t.Fatalf("failing tracker decode wrong: %+v", got[1])
	}
}

func TestTrackerEvent(t *testing.T) {
	num := func(n int) json.RawMessage { b, _ := json.Marshal(n); return b }
	str := func(s string) json.RawMessage { b, _ := json.Marshal(s); return b }
	cases := []struct {
		in   json.RawMessage
		want string
	}{
		{num(0), ""}, {num(1), "completed"}, {num(2), "started"}, {num(3), "stopped"}, {num(4), "scrape"},
		{num(9), ""},                    // out-of-range
		{str("3"), "stopped"},           // numeric string
		{str("completed"), "completed"}, // already a word (older builds)
		{str("none"), ""}, {str(""), ""},
	}
	for _, c := range cases {
		if got := trackerEvent(c.in); got != c.want {
			t.Errorf("trackerEvent(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPiecesFromBatch(t *testing.T) {
	ok := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	// "0" sentinel survives, string-typed counts coerce
	p, err := piecesFromBatch(
		[]json.RawMessage{ok("0"), ok("100"), ok(100), ok(262144)},
		[]error{nil, nil, nil, nil},
	)
	if err != nil {
		t.Fatal(err)
	}
	if p.Bitfield != "0" || p.SizeChunks != 100 || p.CompletedChunks != 100 || p.ChunkSize != 262144 {
		t.Fatalf("decoded wrong: %+v", p)
	}
	// a bitfield error propagates
	if _, err := piecesFromBatch(
		[]json.RawMessage{nil, ok(1), ok(0), ok(1)},
		[]error{errBitfield, nil, nil, nil},
	); err == nil {
		t.Fatal("expected error when errs[0] is set")
	}
	// short batch is an error, not a panic
	if _, err := piecesFromBatch([]json.RawMessage{ok("0")}, []error{nil}); err == nil {
		t.Fatal("expected error on short batch result")
	}
}

var errBitfield = &Error{Code: -1, Message: "boom"}
