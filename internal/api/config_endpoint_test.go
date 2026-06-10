package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mlamp/rtorrent-webui/internal/sse"
)

// /api/config must serve the instance name without touching rtorrent, so the
// brand/title load even when the daemon is down (nil rpc client here stands in
// for "unreachable").
func TestHandleConfigReturnsName(t *testing.T) {
	srv := New(sse.NewHub(), nil, "main")
	srv.SetName("TV")

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		OK   bool `json:"ok"`
		Data struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Data.Name != "TV" {
		t.Fatalf("got %+v, want ok=true name=TV", got)
	}
}
