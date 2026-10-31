package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"movies-cast/internal/dlna"
)

// The TV app on an Android device asks to play in Kodi. The test client runs
// on 127.0.0.1, where the rig's TV (lg) has its description URL: that is
// "Kodi answering on the app's address", so the film goes by DLNA.
func TestKodiOnTheAppsAddressGetsTheFilmByDLNA(t *testing.T) {
	r, _ := newLibRig(t)
	code, out := r.post("/api/tv/kodi", map[string]any{"movie": "E1"})
	if code != 200 || out["mode"] != "dlna" {
		t.Fatalf("kodi: %d %v", code, out)
	}
	if got := titleOnTV(r); got != "Pilot" {
		t.Fatalf("the renderer on the app's address got %q", got)
	}
}

// Without a renderer there (Kodi's UPnP off), the app gets a stream URL for
// an intent to Kodi, on the address it used, and the URL streams.
func TestKodiWithoutUPnPGetsAStreamURLForAnIntent(t *testing.T) {
	r, _ := newLibRig(t)
	far := dlna.Renderer{ID: "uuid:far", Name: "Elsewhere", Location: "http://192.0.2.40:1400/"}
	r.app.renderers = staticRenderers{far}
	code, out := r.post("/api/tv/kodi", map[string]any{"movie": "F1"})
	if code != 200 || out["mode"] != "intent" || out["mime"] != "video/x-matroska" || out["title"] != "Film One" {
		t.Fatalf("kodi: %d %v", code, out)
	}
	u, _ := out["url"].(string)
	if !strings.HasPrefix(u, r.srv.URL+"/s/") || !strings.HasSuffix(u, "/movie.mkv") {
		t.Fatalf("url %q, want one on %s", u, r.srv.URL)
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != "file" {
		t.Fatalf("stream: %d %q", resp.StatusCode, b)
	}
	if uri, _, _ := r.tv.get(); uri != "" {
		t.Fatalf("a DLNA TV got %q", uri)
	}
	code, out = r.post("/api/tv/kodi", map[string]any{"movie": "S1"})
	if code != 400 {
		b, _ := json.Marshal(out)
		t.Fatalf("a series to Kodi: %d %s", code, b)
	}
}
