package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

// tvClient is a second browser (the TV) talking to the same server.
func (r *rig) tvClient() *rig {
	jar, _ := cookiejar.New(nil)
	return &rig{t: r.t, srv: r.srv, app: r.app, movies: r.movies, tv: r.tv, client: &http.Client{Jar: jar}}
}

func (r *rig) getJSON(path string) (int, map[string]any) {
	r.t.Helper()
	resp, err := r.client.Get(r.srv.URL + path)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (r *rig) moviesLogouts() int {
	r.movies.mu.Lock()
	defer r.movies.mu.Unlock()
	n := 0
	for _, rep := range r.movies.reports {
		if strings.HasPrefix(rep, "/movies/Sessions/Logout ") {
			n++
		}
	}
	return n
}

func pairTV(t *testing.T, phone, tv *rig) {
	t.Helper()
	code, start := tv.post("/api/tv/pair/start", map[string]any{})
	if code != http.StatusOK || len(start["code"].(string)) != 6 {
		t.Fatalf("start: %d %v", code, start)
	}
	id := start["id"].(string)
	if c, out := tv.getJSON("/api/tv/pair/poll?id=" + id); c != http.StatusOK || out["pending"] != true {
		t.Fatalf("poll before approve: %d %v", c, out)
	}
	if c, _ := tv.getJSON("/api/movies"); c != http.StatusUnauthorized {
		t.Fatalf("TV without pairing sees movies: %d", c)
	}
	if c, _ := phone.post("/api/tv/pair/approve", map[string]string{"code": start["code"].(string)}); c != http.StatusOK {
		t.Fatalf("approve: %d", c)
	}
	if c, out := tv.getJSON("/api/tv/pair/poll?id=" + id); c != http.StatusOK || out["name"] != "Demo" {
		t.Fatalf("poll after approve: %d %v", c, out)
	}
	// The pairing is used up.
	if c, _ := tv.getJSON("/api/tv/pair/poll?id=" + id); c != http.StatusNotFound {
		t.Fatalf("second poll: %d", c)
	}
}

func TestPairingGivesTVASessionAndNeedsALoggedInPhone(t *testing.T) {
	phone := newRig(t)
	tv := phone.tvClient()
	// Approve needs a logged-in phone.
	if c, _ := phone.post("/api/tv/pair/approve", map[string]string{"code": "123456"}); c != http.StatusUnauthorized {
		t.Fatalf("approve without login: %d", c)
	}
	phone.login()
	if c, _ := phone.post("/api/tv/pair/approve", map[string]string{"code": "000000"}); c != http.StatusNotFound {
		t.Fatalf("wrong code: %d", c)
	}
	// A wrong code while a pairing is open must not approve it.
	_, open := tv.post("/api/tv/pair/start", map[string]any{})
	wrong := "000000"
	if open["code"] == wrong {
		wrong = "000001"
	}
	if c, _ := phone.post("/api/tv/pair/approve", map[string]string{"code": wrong}); c != http.StatusNotFound {
		t.Fatalf("wrong code with open pairing: %d", c)
	}
	if c, out := tv.getJSON("/api/tv/pair/poll?id=" + open["id"].(string)); c != http.StatusOK || out["pending"] != true {
		t.Fatalf("pairing approved by a wrong code: %d %v", c, out)
	}
	pairTV(t, phone, tv)
	if c, body := tv.get("/api/movies"); c != http.StatusOK || !strings.Contains(body, "Shelter") {
		t.Fatalf("TV movies: %d", c)
	}
}

func TestPhoneLogoutKeepsPairedTVLoggedIn(t *testing.T) {
	phone := newRig(t)
	tv := phone.tvClient()
	phone.login()
	pairTV(t, phone, tv)
	phone.post("/api/logout", map[string]any{})
	if n := phone.moviesLogouts(); n != 0 {
		t.Fatalf("Movies logout sent while the TV still uses the login: %d", n)
	}
	if c, _ := tv.get("/api/movies"); c != http.StatusOK {
		t.Fatalf("TV after phone logout: %d", c)
	}
	tv.post("/api/logout", map[string]any{})
	if n := phone.moviesLogouts(); n != 1 {
		t.Fatalf("last session out: want 1 Movies logout, got %d", n)
	}
}

func TestTVPlayStreamsReportsAndStops(t *testing.T) {
	phone := newRig(t)
	tv := phone.tvClient()
	phone.login()
	pairTV(t, phone, tv)
	// Resume point from the captured answer: Avatar at 15 s.
	if code, out := tv.post("/api/tv/play", map[string]any{"movie": "01000000000000000000000000000001", "resume": true}); code != http.StatusOK || out["resumeSeconds"] != float64(15) {
		t.Fatalf("resume play: %d %v", code, out)
	}
	// A new play replaces it; this one streams (the fake serves Shelter only).
	code, out := tv.post("/api/tv/play", map[string]any{"movie": movieID})
	if code != http.StatusOK {
		t.Fatalf("play: %d %v", code, out)
	}
	url := out["url"].(string)
	if !strings.HasPrefix(url, "/s/") || strings.Contains(url, "tok") {
		t.Fatalf("url %q", url)
	}
	resp, err := http.Head(phone.srv.URL + url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fresh play URL: %d", resp.StatusCode)
	}
	if c, _ := tv.post("/api/tv/progress", map[string]any{"play": out["play"], "position": 95.0, "paused": true}); c != http.StatusOK {
		t.Fatalf("progress: %d", c)
	}
	// Another session (the phone) may not report for the TV's play.
	if c, _ := phone.post("/api/tv/progress", map[string]any{"play": out["play"], "position": 1.0, "stopped": true}); c != http.StatusNotFound {
		t.Fatalf("foreign progress: %d", c)
	}
	tv.post("/api/tv/progress", map[string]any{"play": out["play"], "position": 120.0, "stopped": true})
	waitFor(t, "stopped report at 120 s", func() bool {
		phone.movies.mu.Lock()
		defer phone.movies.mu.Unlock()
		for _, rep := range phone.movies.reports {
			if strings.HasPrefix(rep, "/movies/Sessions/Playing/Stopped ") && strings.Contains(rep, `"PositionTicks":1200000000`) {
				return true
			}
		}
		return false
	})
	resp, _ = http.Head(phone.srv.URL + url)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("URL after stop: %d", resp.StatusCode)
	}
}

func TestSilentTVAppEndsPlay(t *testing.T) {
	phone := newRig(t)
	phone.app.browserSilence = 50 * time.Millisecond
	tv := phone.tvClient()
	phone.login()
	pairTV(t, phone, tv)
	_, out := tv.post("/api/tv/play", map[string]any{"movie": movieID})
	url := out["url"].(string)
	waitFor(t, "play ended after silence", func() bool {
		resp, err := http.Head(phone.srv.URL + url)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusNotFound
	})
}

func TestPairCodeFormat(t *testing.T) {
	for range 200 {
		c := pairCode()
		if len(c) != 6 || strings.Trim(c, "0123456789") != "" {
			t.Fatalf("code %q", c)
		}
	}
}

func TestTVPlayOffersSubtitlesOnlyToItsOwnSession(t *testing.T) {
	phone := newRig(t)
	tv := phone.tvClient()
	phone.login()
	pairTV(t, phone, tv)
	code, out := tv.post("/api/tv/play", map[string]any{"movie": movieID})
	if code != http.StatusOK {
		t.Fatalf("play: %d", code)
	}
	subs, _ := out["subtitles"].([]any)
	if len(subs) != 2 {
		t.Fatalf("subtitles %v", out["subtitles"])
	}
	first := subs[0].(map[string]any)
	u := first["url"].(string)
	if first["lang"] != "en" || !strings.HasSuffix(u, "/3.vtt") || strings.Contains(u, "api_key") {
		t.Fatalf("first subtitle %v", first)
	}
	c, body := tv.get(u)
	if c != http.StatusOK || !strings.HasPrefix(body, "WEBVTT") {
		t.Fatalf("TV gets subtitle: %d %q", c, body)
	}
	if c, _ := tv.get(strings.Replace(u, "/3.vtt", "/9.vtt", 1)); c != http.StatusNotFound {
		t.Fatalf("index not offered: %d", c)
	}
	// A different session (the phone, same Movies login) must not read the TV's play.
	if c, _ := phone.get(u); c != http.StatusNotFound {
		t.Fatalf("other session: %d", c)
	}
	tv.post("/api/tv/progress", map[string]any{"play": out["play"], "position": 1.0, "stopped": true})
	if c, _ := tv.get(u); c != http.StatusNotFound {
		t.Fatalf("after stop: %d", c)
	}
	phone.movies.mu.Lock()
	defer phone.movies.mu.Unlock()
	for _, rep := range phone.movies.reports {
		if strings.HasPrefix(rep, "PlaybackInfo ") {
			t.Fatalf("asked Movies for PlaybackInfo, which starts a transcode on the box: %s", rep)
		}
	}
}

func TestResumeRowInMoviesOrder(t *testing.T) {
	r := newRig(t)
	r.login()
	c, body := r.get("/api/resume")
	if c != http.StatusOK {
		t.Fatalf("resume: %d", c)
	}
	a, g, sh := strings.Index(body, "Avatar"), strings.Index(body, "Glennkill"), strings.Index(body, "Shelter")
	if sh < 0 || sh >= g || g >= a { // the fake answers reversed: Shelter, Glennkill, Avatar
		t.Fatalf("order: %s", body)
	}
}

func (r *rig) playbackInfos() int {
	r.movies.mu.Lock()
	defer r.movies.mu.Unlock()
	n := 0
	for _, rep := range r.movies.reports {
		if strings.HasPrefix(rep, "PlaybackInfo ") {
			n++
		}
	}
	return n
}

// A film whose item answer lists no subtitles (streams not read, like Shelter
// on 2026-10-28) plays without PlaybackInfo; the subtitles come on request,
// PlaybackInfo is asked once per film, and only for the session's own play.
func TestTVSubtitlesOnRequest(t *testing.T) {
	phone := newRig(t)
	tv := phone.tvClient()
	phone.login()
	pairTV(t, phone, tv)
	const unread = "01000000000000000000000000000003" // its captured item has no width and no subtitle
	code, out := tv.post("/api/tv/play", map[string]any{"movie": unread})
	if code != http.StatusOK || out["subtitlesOnRequest"] != true || len(out["subtitles"].([]any)) != 0 {
		t.Fatalf("play: %d %v", code, out)
	}
	if n := phone.playbackInfos(); n != 0 {
		t.Fatalf("play asked PlaybackInfo %d times", n)
	}
	if c, _ := phone.post("/api/tv/subs-probe", map[string]any{"play": out["play"]}); c != http.StatusNotFound {
		t.Fatalf("foreign session probe: %d", c)
	}
	c, pr := tv.post("/api/tv/subs-probe", map[string]any{"play": out["play"]})
	subs, _ := pr["subtitles"].([]any)
	if c != http.StatusOK || len(subs) != 2 || phone.playbackInfos() != 1 {
		t.Fatalf("probe: %d %v, PlaybackInfo %d", c, pr, phone.playbackInfos())
	}
	if c, body := tv.get(subs[0].(map[string]any)["url"].(string)); c != http.StatusOK || !strings.HasPrefix(body, "WEBVTT") {
		t.Fatalf("probed subtitle: %d", c)
	}
	// The same film again: listed from the cache, no second PlaybackInfo.
	_, again := tv.post("/api/tv/play", map[string]any{"movie": unread})
	if again["subtitlesOnRequest"] != false || len(again["subtitles"].([]any)) != 2 || phone.playbackInfos() != 1 {
		t.Fatalf("second play: %v, PlaybackInfo %d", again, phone.playbackInfos())
	}
}
