package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/insight/history"
	"github.com/mlamp/rtorrent-webui/internal/model"
	"github.com/mlamp/rtorrent-webui/internal/sse"
)

// /api/history must return the windowed grid contract {start,end,step,first,points}
// the charts depend on: step = the chosen tier bucket, a now-anchored window, the
// first-seen timestamp, and a dense (zero-filled) points list. Regression guard for
// the wire shape and for the no-spurious-spike behaviour at series start.
func TestHandleHistoryWireShape(t *testing.T) {
	st, err := history.New(t.TempDir() + "/h.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().Unix()
	// First sample already large (100 MiB session total), then +1 MiB/s — the grid
	// must show ~1 MiB/s, never a 100 MiB/s delta-from-zero spike.
	st.Sample(nil, model.Globals{DownTotal: 100 << 20}, now-3)
	st.Sample(nil, model.Globals{DownTotal: 101 << 20}, now-2)
	st.Sample(nil, model.Globals{DownTotal: 102 << 20}, now-1)

	srv := New(sse.NewHub(), nil, "main")
	srv.SetHistory(st)
	rec := httptest.NewRecorder()
	// a short range keeps the raw grid under the decimation target, so the real
	// per-second rate is exact (not averaged away)
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/history?range=30s", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		OK   bool `json:"ok"`
		Data struct {
			Start  int64 `json:"start"`
			End    int64 `json:"end"`
			Step   int64 `json:"step"`
			First  int64 `json:"first"`
			Points []struct {
				T    int64 `json:"t"`
				Down int64 `json:"down"`
				Up   int64 `json:"up"`
			} `json:"points"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK {
		t.Fatal("ok=false")
	}
	d := got.Data
	if d.Step != 1 {
		t.Fatalf("step = %d, want 1 (raw tier for 30s)", d.Step)
	}
	if d.End-d.Start != 30 {
		t.Fatalf("window = %ds, want 30", d.End-d.Start)
	}
	if d.First != now-3 {
		t.Fatalf("first = %d, want %d", d.First, now-3)
	}
	if len(d.Points) == 0 {
		t.Fatal("no points")
	}
	var maxDown int64
	for _, p := range d.Points {
		if p.Down > maxDown {
			maxDown = p.Down
		}
	}
	if maxDown != 1<<20 { // exact real rate — not 0, and NOT a ~100 MiB/s delta-from-zero spike
		t.Fatalf("max down rate = %d B/s, want exactly %d (1 MiB/s, no spurious spike)", maxDown, 1<<20)
	}
}

func TestParseRange(t *testing.T) {
	cases := map[string]int64{
		"15m":     900,
		"1h":      3600,
		"6h":      21600,
		"24h":     86400,
		"7d":      604800,
		"1w":      604800,
		"3mo":     3 * 30 * 86400,
		"1y":      31536000, // the regression: must NOT fall through to 3600
		"2y":      2 * 31536000,
		"":        3600, // bad inputs default to 1h
		"garbage": 3600,
		"-5h":     3600,
		"0d":      3600,
	}
	for in, want := range cases {
		if got := parseRange(in); got != want {
			t.Errorf("parseRange(%q) = %d, want %d", in, got, want)
		}
	}
}
