package server

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// "No sound?" converts the sound: the TV gets one MPEG-TS stream joined from
// Movies' segments, starting at the segment that holds the asked time; the
// choice is remembered for this TV and audio format until "original".
func TestConvertedPlayJoinsSegmentsAndIsRemembered(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(io.Discard) })
	r, f := newRigWithState(t)
	r.login()
	r.tv.mu.Lock()
	r.tv.sink = "http-get:*:video/mp2t:*,http-get:*:video/x-matroska:*"
	r.tv.mu.Unlock()

	code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID, "convert": true, "remember": true, "at": 7})
	if code != 200 || out["converted"] != true || out["resumeSeconds"] != float64(5) {
		t.Fatalf("convert: %d %v", code, out)
	}
	r.tv.mu.Lock()
	m := r.tv.media
	r.tv.mu.Unlock()
	if !m.Converted || m.MimeType != "video/mp2t" || !strings.HasSuffix(m.URL, "/movie-5.ts") || m.Duration != 13500*time.Millisecond {
		t.Fatalf("media: %+v", m)
	}
	if strings.Contains(m.URL, "tok") {
		t.Fatalf("token in the TV's URL: %s", m.URL)
	}
	resp, err := http.Get(r.srv.URL + r.streamPath())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "SEG1SEG2SEG3" || resp.Header.Get("Content-Type") != "video/mp2t" || !strings.Contains(resp.Header.Get("contentFeatures.dlna.org"), "OP=10") {
		t.Fatalf("stream: %q %v", body, resp.Header)
	}
	// The TV asks for a time in the stream it got (which starts at 5 s).
	req, _ := http.NewRequest(http.MethodGet, r.srv.URL+r.streamPath(), nil)
	req.Header.Set("TimeSeekRange.dlna.org", "npt=6.2-")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "SEG2SEG3" || resp.Header.Get("TimeSeekRange.dlna.org") != "npt=5.000-" {
		t.Fatalf("time seek: %q %v", body, resp.Header.Get("TimeSeekRange.dlna.org"))
	}
	// A seek re-sends the film from the segment that holds it.
	if code, out := r.post("/api/control", map[string]any{"renderer": lg.ID, "action": "seek", "seconds": 16}); code != 200 {
		t.Fatalf("seek: %d %v", code, out)
	}
	if !strings.HasSuffix(r.streamPath(), "/movie-15.ts") {
		t.Fatalf("after seek: %s", r.streamPath())
	}

	// Remembered, across a restart: a plain play converts again.
	r2 := r.restart(f)
	carryCookie(t, r, r2)
	if code, out := r2.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID}); code != 200 || out["converted"] != true {
		t.Fatalf("remembered: %d %v", code, out)
	}
	// "Original sound" forgets it.
	if code, out := r2.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID, "original": true, "remember": true, "force": true}); code != 200 || out["converted"] == true {
		t.Fatalf("original: %d %v", code, out)
	}
	if code, out := r2.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID, "force": true}); code != 200 || out["converted"] == true {
		t.Fatalf("after original: %d %v", code, out)
	}
	if strings.Contains(logs.String(), "tok-geheim") {
		t.Fatalf("token in the log:\n%s", logs.String())
	}
}

func TestParseNPT(t *testing.T) {
	cases := map[string]time.Duration{"npt=12.5-": 12500 * time.Millisecond, "npt=0:01:02-": 62 * time.Second, "npt=1:00:00.0-1:00:05": time.Hour}
	for in, want := range cases {
		if got, ok := parseNPT(in); !ok || got != want {
			t.Errorf("%s: %v %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", "bytes=0-", "npt=-5-", "npt=x-"} {
		if _, ok := parseNPT(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

// A segment redirect to another server is not followed: the token rides
// along with the request. The stream ends before that segment instead.
func TestConvertedSegmentRedirectStaysOnMovies(t *testing.T) {
	var hits int
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	t.Cleanup(evil.Close)
	r := newRig(t)
	r.movies.redirectAway = evil.URL + "/steal"
	r.login()
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID, "convert": true}); code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	resp, err := http.Get(r.srv.URL + r.streamPath())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "SEG0SEG1SEG2" || hits != 0 {
		t.Fatalf("body %q, other server hit %d times", body, hits)
	}
}
