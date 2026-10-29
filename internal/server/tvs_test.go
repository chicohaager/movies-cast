package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"movies-cast/internal/dlna"
)

// fakeFinder is a Finder with fixed answers and stats.
type fakeFinder struct {
	rs    []dlna.Renderer
	stats dlna.SearchStats
	extra []string
	known []string
}

func (f *fakeFinder) Find(context.Context, bool) ([]dlna.Renderer, error) { return f.rs, nil }
func (f *fakeFinder) LastSearch() dlna.SearchStats                        { return f.stats }
func (f *fakeFinder) SetExtraHosts(h []string)                            { f.extra = h }
func (f *fakeFinder) SetKnown(l []string)                                 { f.known = l }

var samsung = dlna.Renderer{ID: "uuid:samsung", Name: "[TV] Samsung 6 Series (50)", Location: "http://127.0.0.2:9197/dmr"}

// A name and "hidden" set in the app change the list the phone gets, and
// survive a restart (tvs.json next to the sessions).
func TestTVLabelAndHideSurviveRestart(t *testing.T) {
	r, f := newRigWithState(t)
	r.app.renderers = staticRenderers{lg, samsung}
	r.login()
	if code, out := r.post("/api/tvs", map[string]any{"id": lg.ID, "label": "Wohnzimmer"}); code != 200 {
		t.Fatalf("label: %d %v", code, out)
	}
	if code, out := r.post("/api/tvs", map[string]any{"id": samsung.ID, "hidden": true}); code != 200 {
		t.Fatalf("hide: %d %v", code, out)
	}
	r2 := r.restart(f)
	r2.app.renderers = staticRenderers{lg, samsung}
	carryCookie(t, r, r2)
	code, body := r2.get("/api/renderers")
	var list []rendererOut
	_ = json.Unmarshal([]byte(body), &list)
	if code != 200 || len(list) != 1 || list[0].Name != "Wohnzimmer" || list[0].ID != lg.ID {
		t.Fatalf("after restart: %d %s", code, body)
	}
	// The management list still shows the hidden one, with its own name.
	_, body = r2.get("/api/tvs")
	if !strings.Contains(body, `"hidden":true`) || !strings.Contains(body, "Samsung 6 Series") {
		t.Fatalf("/api/tvs: %s", body)
	}
	if code, _ := r2.post("/api/tvs", map[string]any{"id": lg.ID, "label": strings.Repeat("x", 41)}); code != 400 {
		t.Fatalf("41-character name: %d", code)
	}
}

// An empty list says why: no answer at all points at the firewall; answers
// without a TV, or every TV hidden, say that.
func TestEmptyTVListExplainsWhy(t *testing.T) {
	r := newRig(t)
	ff := &fakeFinder{stats: dlna.SearchStats{Answers: 0, Port: 1901}}
	r.app.renderers = ff
	r.login()
	code, body := r.get("/api/renderers")
	if code != 502 || !strings.Contains(body, "UDP 1901") {
		t.Fatalf("no answers: %d %s", code, body)
	}
	ff.stats.Answers = 7
	if _, body = r.get("/api/renderers"); !strings.Contains(body, "7 Geräte") {
		t.Fatalf("answers without TV: %s", body)
	}
	ff.rs = []dlna.Renderer{lg}
	r.post("/api/tvs", map[string]any{"id": lg.ID, "hidden": true})
	if _, body = r.get("/api/renderers"); !strings.Contains(body, "ausgeblendet") {
		t.Fatalf("all hidden: %s", body)
	}
}

// A TV added by address reaches the finder; addresses outside the own LAN
// or of this machine are refused.
func TestAddTVByHost(t *testing.T) {
	r := newRig(t)
	ff := &fakeFinder{rs: []dlna.Renderer{lg}}
	r.app.renderers = ff
	r.login()
	if code, out := r.post("/api/tvs/hosts", map[string]any{"host": "10.0.0.86"}); code != 200 {
		t.Fatalf("add: %d %v", code, out)
	}
	if len(ff.extra) != 1 || ff.extra[0] != "10.0.0.86" {
		t.Fatalf("finder got %v", ff.extra)
	}
	for _, bad := range []string{"8.8.8.8", "127.0.0.1", "tv.local", "fe80::1", ""} {
		if code, _ := r.post("/api/tvs/hosts", map[string]any{"host": bad}); code != 400 {
			t.Errorf("host %q accepted", bad)
		}
	}
	if !validTVHost("10.0.0.86", nil) || validTVHost("10.0.0.143", []net.IP{net.ParseIP("10.0.0.143")}) {
		t.Error("validTVHost: own address must be refused, LAN address accepted")
	}
	if code, _ := r.post("/api/tvs/hosts", map[string]any{"host": "10.0.0.86", "remove": true}); code != 200 || len(ff.extra) != 0 {
		t.Fatalf("remove: %d %v", code, ff.extra)
	}
}

// The status page reports Movies and the last search.
func TestSystemStatus(t *testing.T) {
	r := newRig(t)
	r.app.renderers = &fakeFinder{stats: dlna.SearchStats{At: time.Unix(1, 0), Answers: 9, Renderers: 2, Port: 1901, PortHeld: true}}
	r.login()
	code, body := r.get("/api/system")
	var out struct {
		MoviesOK bool             `json:"moviesOK"`
		Search   dlna.SearchStats `json:"search"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	if code != http.StatusOK || out.Search.Answers != 9 || !out.Search.PortHeld {
		t.Fatalf("%d %s", code, body)
	}
	if !out.MoviesOK {
		t.Fatalf("moviesOK false against the fake Movies: %s", body)
	}
}
