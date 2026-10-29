package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The TV hands the stream URL to anyone who asks it over SOAP
// (GetPositionInfo), and /s/ needed nothing but that URL: any host on the
// LAN could watch along. The first address that fetches a stream keeps it.
func TestStreamStaysWithTheFirstFetcher(t *testing.T) {
	r := newRig(t)
	r.login()
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID}); code != 200 {
		t.Fatalf("play: %d %v", code, out)
	}
	path := r.streamPath()
	fetch := func(from string) int {
		req := httptest.NewRequest(http.MethodHead, path, nil)
		req.RemoteAddr = from
		w := httptest.NewRecorder()
		r.app.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if c := fetch("192.0.2.10:40000"); c != 200 {
		t.Fatalf("the TV's first fetch: %d", c)
	}
	if c := fetch("192.0.2.10:40001"); c != 200 {
		t.Fatalf("the TV again, other port: %d", c)
	}
	if c := fetch("192.0.2.66:50000"); c != http.StatusForbidden {
		t.Fatalf("another host with the URL: %d, want 403", c)
	}
}
