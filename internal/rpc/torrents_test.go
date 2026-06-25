package rpc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

func TestEnrichTrackersCaching(t *testing.T) {
	var requested []string
	rows := func(url string, enabled int) json.RawMessage {
		b, _ := json.Marshal([][]any{{url, enabled}})
		return b
	}
	c := &Client{trackerCache: map[string]string{}}
	c.batch = func(_ context.Context, items []BatchItem) ([]json.RawMessage, []error, error) {
		res := make([]json.RawMessage, len(items))
		errs := make([]error, len(items))
		for i, it := range items {
			h := it.Params[0].(string)
			requested = append(requested, h)
			switch h {
			case "GOOD":
				res[i] = rows("https://good.example/announce", 1)
			case "EMPTY":
				res[i], _ = json.Marshal([][]any{}) // no trackers → host ""
			case "ERR":
				errs[i] = &Error{Code: -1, Message: "boom"}
			}
		}
		return res, errs, nil
	}

	// poll 1: all three looked up; only GOOD resolves + caches.
	tors := []model.Torrent{{Hash: "GOOD"}, {Hash: "EMPTY"}, {Hash: "ERR"}}
	c.enrichTrackers(context.Background(), tors)
	if len(requested) != 3 {
		t.Fatalf("poll1 requested %v, want all 3", requested)
	}
	if tors[0].Tracker != "good.example" {
		t.Fatalf("GOOD host = %q, want good.example", tors[0].Tracker)
	}
	if tors[1].Tracker != "" || tors[2].Tracker != "" {
		t.Fatal("EMPTY/ERR must stay blank, not a bogus value")
	}
	if _, ok := c.trackerCache["EMPTY"]; ok {
		t.Fatal("EMPTY must NOT be negative-cached (so it retries)")
	}
	if _, ok := c.trackerCache["ERR"]; ok {
		t.Fatal("ERR must NOT be cached")
	}

	// poll 2: GOOD served from cache; only EMPTY+ERR re-fetched.
	requested = nil
	tors2 := []model.Torrent{{Hash: "GOOD"}, {Hash: "EMPTY"}, {Hash: "ERR"}}
	c.enrichTrackers(context.Background(), tors2)
	for _, h := range requested {
		if h == "GOOD" {
			t.Fatal("GOOD re-fetched despite being cached")
		}
	}
	if len(requested) != 2 || tors2[0].Tracker != "good.example" {
		t.Fatalf("poll2 requested %v; GOOD tracker %q", requested, tors2[0].Tracker)
	}

	// prune: GOOD no longer present → evicted (bounds cache growth).
	c.enrichTrackers(context.Background(), []model.Torrent{{Hash: "EMPTY"}})
	if _, ok := c.trackerCache["GOOD"]; ok {
		t.Fatal("removed torrent GOOD must be pruned from the cache")
	}
}

func TestEnrichTrackersInvalidationDuringFetchWins(t *testing.T) {
	c := &Client{trackerCache: map[string]string{}}
	entered := make(chan struct{})
	release := make(chan struct{})
	c.batch = func(_ context.Context, items []BatchItem) ([]json.RawMessage, []error, error) {
		close(entered)
		<-release // hold the round-trip open while the tracker toggle lands
		rows, _ := json.Marshal([][]any{{"https://old.example/announce", 1}})
		return []json.RawMessage{rows}, []error{nil}, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.enrichTrackers(context.Background(), []model.Torrent{{Hash: "H"}})
	}()
	<-entered
	// SetTrackerEnabled's cache invalidation (detail.go) lands while the enrich
	// batch is in flight: the batch snapshot is pre-toggle data.
	c.trackerMu.Lock()
	delete(c.trackerCache, "H")
	c.trackerMu.Unlock()
	close(release)
	<-done

	// The invalidation must win: caching the pre-toggle host would serve a stale
	// primary tracker on every poll until the torrent is removed.
	c.trackerMu.Lock()
	host, ok := c.trackerCache["H"]
	c.trackerMu.Unlock()
	if ok {
		t.Fatalf("stale pre-toggle host %q resurrected after invalidation; cache must stay empty", host)
	}
}

func TestTrackerHost(t *testing.T) {
	cases := map[string]string{
		"https://bgp.technology/announce":                    "bgp.technology",
		"udp://tracker.opentrackr.org:1337/announce":         "tracker.opentrackr.org",
		"https://user:pass@hd-space.pw/announce?passkey=abc": "hd-space.pw", // passkey/userinfo stripped
		"":          "",
		"not-a-url": "not-a-url", // unparseable falls back to raw
	}
	for in, want := range cases {
		if got := trackerHost(in); got != want {
			t.Errorf("trackerHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrimaryTrackerHost(t *testing.T) {
	raw := func(rows [][]any) json.RawMessage {
		b, _ := json.Marshal(rows)
		return b
	}
	// prefers the ENABLED tracker even when it isn't first
	if got := primaryTrackerHost(raw([][]any{
		{"https://disabled.example/announce", 0},
		{"https://enabled.example/announce", 1},
	})); got != "enabled.example" {
		t.Errorf("enabled-preference: got %q, want enabled.example", got)
	}
	// falls back to the first tracker when none are enabled
	if got := primaryTrackerHost(raw([][]any{
		{"udp://first.example:80/x", 0},
		{"https://second.example/y", 0},
	})); got != "first.example" {
		t.Errorf("first-fallback: got %q, want first.example", got)
	}
	// empty / nil never panic and yield ""
	if got := primaryTrackerHost(raw([][]any{})); got != "" {
		t.Errorf("empty rows: got %q, want \"\"", got)
	}
	if got := primaryTrackerHost(nil); got != "" {
		t.Errorf("nil: got %q, want \"\"", got)
	}
}

func TestDecodeTorrentsChunks(t *testing.T) {
	row := make([]any, len(torrentFields))
	for i := range row {
		row[i] = 0
	}
	row[0] = "ABC"       // hash
	row[1] = "name"      // name
	row[2] = 1048576     // size
	row[3] = 524288      // completed
	row[20] = 16         // size_chunks
	row[21] = 8          // completed_chunks
	row[22] = 65536      // chunk_size
	row[23] = 9876543210 // down.total (cumulative downloaded)
	b, _ := json.Marshal([][]any{row})

	got, err := decodeTorrents(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1", len(got))
	}
	if got[0].SizeChunks != 16 || got[0].CompletedChunks != 8 || got[0].ChunkSize != 65536 {
		t.Fatalf("chunk fields decoded wrong: %+v", got[0])
	}
	if got[0].DownTotal != 9876543210 {
		t.Fatalf("DownTotal = %d, want 9876543210 (d.down.total at field 23)", got[0].DownTotal)
	}
}

func TestPollSurfacesThrottleItemErrors(t *testing.T) {
	row := make([]any, len(torrentFields))
	for i := range row {
		row[i] = 0
	}
	row[0], row[1] = "ABC", "name"
	c, _ := fakeSCGI(t, func(call scgiCall) (any, *Error) {
		switch call.Method {
		case "d.multicall2":
			return [][]any{row}, nil
		case "t.multicall":
			return [][]any{}, nil // tracker enrichment is best-effort, not under test
		default: // the four throttle.* globals fail as per-item errors
			return nil, &Error{Code: -506, Message: "method not found"}
		}
	})
	_, g, err := c.Poll(context.Background(), "")
	// Batch delivers per-item failures in errs with the matching result nil;
	// swallowing them coerces the totals/limits to fabricated zeros (session
	// counters reset, limits shown as unlimited) with nothing logged. Poll must
	// surface the failure instead.
	if err == nil {
		t.Fatalf("Poll() err = nil with all throttle.* items failing; globals fabricated as %+v", g)
	}
	if !strings.Contains(err.Error(), "throttle.global_down.total") {
		t.Fatalf("Poll() err = %q, want it to name the failed item", err)
	}
}

func TestDeriveStatus(t *testing.T) {
	cases := []struct {
		name                                       string
		state, isActive, isOpen, isHashCheck, left int64
		message                                    string
		want                                       model.Status
	}{
		{"downloading", 1, 1, 1, 0, 100, "", model.StatusDownloading},
		{"seeding", 1, 1, 1, 0, 0, "", model.StatusSeeding},
		{"stopped", 0, 0, 0, 0, 100, "", model.StatusStopped},
		{"hashing wins", 1, 1, 1, 1, 100, "", model.StatusHashing},
		{"real message is an error", 1, 1, 1, 0, 100, "Could not create download: invalid bencode", model.StatusError},
		// a tracker failure from ANY tracker in the set lands in d.message and
		// flip-flops as other trackers succeed — it must NOT error the torrent
		{"tracker message keeps transfer status", 1, 1, 1, 0, 100, "Tracker: [Could not resolve hostname]", model.StatusDownloading},
		{"tracker message while seeding", 1, 1, 1, 0, 0, "Tracker: [Timeout was reached]", model.StatusSeeding},
		{"tracker message while inactive", 1, 0, 1, 0, 100, "Tracker: [Timeout was reached]", model.StatusPaused},
		// a tracker REJECTION (announce answered, body carries a failure reason —
		// unregistered torrent, banned passkey) is authoritative and typically
		// permanent on single-tracker private torrents: it stays an error
		{"tracker rejection is an error", 1, 1, 1, 0, 0, `Tracker: [Failure reason "Unregistered torrent"]`, model.StatusError},
		// a UDP tracker's rejection (BEP-15 error packet) is just as authoritative
		// as the HTTP "Failure reason" wrapper — libtorrent 0.16 surfaces it as
		// "tracker message: ...", other revisions as "received error message: ..."
		{"udp tracker rejection is an error", 1, 1, 1, 0, 100, `Tracker: [tracker message: unregistered torrent]`, model.StatusError},
		{"udp tracker rejection (alt spelling) is an error", 1, 1, 1, 0, 100, `Tracker: [received error message: unregistered torrent]`, model.StatusError},
	}
	for _, c := range cases {
		if got := deriveStatus(c.state, c.isActive, c.isOpen, c.isHashCheck, c.left, c.message); got != c.want {
			t.Errorf("%s: deriveStatus = %q, want %q", c.name, got, c.want)
		}
	}
}
