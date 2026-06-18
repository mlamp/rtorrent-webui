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

// /api/config advertises whether on-disk data deletion is enabled, so the SPA
// knows whether to render the "delete files" affordance. Default OFF.
func TestHandleConfigAdvertisesDeleteWithData(t *testing.T) {
	for _, tc := range []struct {
		name string
		on   bool
	}{{"default off", false}, {"enabled", true}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := New(sse.NewHub(), nil, "main")
			if tc.on {
				srv.SetDeleteWithData(true)
			}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
			var got struct {
				Data struct {
					DeleteWithData bool `json:"deleteWithData"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Data.DeleteWithData != tc.on {
				t.Fatalf("deleteWithData = %v, want %v", got.Data.DeleteWithData, tc.on)
			}
		})
	}
}
