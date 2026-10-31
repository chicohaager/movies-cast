package server

import (
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"movies-cast/internal/cast"
	"movies-cast/internal/cast/casttest"
	"movies-cast/internal/movies"
)

// A Cast device found by mDNS is listed next to the DLNA TV and plays like
// one: Play sends LOAD with the same /s/ URL, the phone's controls reach it,
// its status shows as ours, and a film that ran to its end is followed by
// the next episode.
func TestACastDevicePlaysLikeATV(t *testing.T) {
	rcv := casttest.New(t)
	fm := &libMovies{headStatus: http.StatusMethodNotAllowed}
	ms := httptest.NewServer(fm.handler())
	t.Cleanup(ms.Close)
	tv := &fakeTV{state: "NO_MEDIA_PRESENT"}
	tr, rs := CastAware(tv, staticRenderers{lg})
	ct := tr.(*castTransport)
	ct.dial = func(ctx context.Context, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
	rs.(*castRenderers).discover = func(context.Context, time.Duration) ([]cast.Found, error) {
		return []cast.Found{{ID: "abc", Name: "Wohnzimmer", Model: "Chromecast", Addr: rcv.Addr}}, nil
	}
	app := New(Config{PublicURL: "http://tv-reaches-me:8097"}, movies.New(ms.URL+"/movies", "test"), tr, rs)
	app.pollEvery = 5 * time.Millisecond
	app.reportEvery = 0
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		app.mu.Lock()
		for id := range app.plays {
			app.endPlayLocked(id)
		}
		app.mu.Unlock()
	})
	jar, _ := cookiejar.New(nil)
	r := &rig{t: t, srv: srv, app: app, tv: tv, client: &http.Client{Jar: jar}}
	r.login()

	code, body := r.get("/api/renderers")
	if code != 200 || !strings.Contains(body, `"id":"cast:abc"`) || !strings.Contains(body, `"name":"Wohnzimmer"`) || !strings.Contains(body, lg.ID) {
		t.Fatalf("renderers: %d %s", code, body)
	}
	if code, out := r.post("/api/play", map[string]any{"renderer": "cast:abc", "movie": "E1"}); code != 200 {
		t.Fatalf("play: %d %v", code, out)
	}
	content, cmds := rcv.Snapshot()
	if !strings.HasPrefix(content, "http://tv-reaches-me:8097/s/") || len(cmds) == 0 || cmds[0] != "LOAD" {
		t.Fatalf("receiver got %q %v", content, cmds)
	}
	if uri, _, _ := tv.get(); uri != "" {
		t.Fatalf("the DLNA TV got %q", uri)
	}
	waitFor(t, "status ours", func() bool {
		_, b := r.get("/api/status?renderer=" + url.QueryEscape("cast:abc"))
		return strings.Contains(b, `"ours":true`) && strings.Contains(b, `"state":"PLAYING"`) && strings.Contains(b, `"title":"Pilot"`) && strings.Contains(b, `"volume":50`)
	})
	waitFor(t, "the watcher saw PLAYING", func() bool {
		app.mu.Lock()
		defer app.mu.Unlock()
		p := app.byTV["cast:abc"]
		return p != nil && p.sawPlay
	})
	if code, _ := r.post("/api/control", map[string]any{"renderer": "cast:abc", "action": "pause"}); code != 200 {
		t.Fatal("pause")
	}
	if _, b := r.get("/api/status?renderer=" + url.QueryEscape("cast:abc")); !strings.Contains(b, `"state":"PAUSED_PLAYBACK"`) {
		t.Fatalf("after pause: %s", b)
	}
	for _, a := range []map[string]any{{"action": "seek", "seconds": 600}, {"action": "play"}} {
		a["renderer"] = "cast:abc"
		if code, out := r.post("/api/control", a); code != 200 {
			t.Fatalf("%v: %d %v", a, code, out)
		}
	}
	_, cmds = rcv.Snapshot()
	var control []string
	for _, c := range cmds {
		if c != "GET_STATUS" {
			control = append(control, c)
		}
	}
	if strings.Join(control, ",") != "LOAD,PAUSE,SEEK,PLAY" {
		t.Fatalf("commands %v", cmds)
	}
	// The episode ends on the device: the next one follows on it.
	rcv.Set("IDLE", "FINISHED", 1195)
	waitFor(t, "the next episode loaded on the Cast device", func() bool {
		c, _ := rcv.Snapshot()
		return c != content && strings.HasPrefix(c, "http://tv-reaches-me:8097/s/")
	})
}
