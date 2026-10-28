package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"movies-cast/internal/dlna"
	"movies-cast/internal/movies"
)

// fakeMovies answers like Movies 0.3.0 (captured /Items answer) and records
// play-state reports and the auth header of stream requests.
type fakeMovies struct {
	mu      sync.Mutex
	reports []string
	streams []string
}

const movieID = "01000000000000000000000000000006"
const fileBytes = "0123456789abcdefghij"

func (f *fakeMovies) handler(t *testing.T) http.Handler {
	items, err := os.ReadFile("../movies/testdata/items.json")
	if err != nil {
		t.Fatal(err)
	}
	var all struct{ Items []json.RawMessage }
	if err := json.Unmarshal(items, &all); err != nil {
		t.Fatal(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("X-Emby-Authorization")
		if r.URL.Path == "/movies/Users/AuthenticateByName" {
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["Pw"] != "richtig" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"AccessToken":"tok-geheim","User":{"Id":"USERID0000","Name":"Demo"}}`)
			return
		}
		if !strings.Contains(auth, `Token="tok-geheim"`) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/movies/Users/USERID0000/Items":
			_, _ = w.Write(items)
		case r.URL.Path == "/movies/Users/USERID0000/Items/Resume":
			// Reversed, so that "keeps Movies' order" differs from "sorted by name"
			// (the captured answer happens to be alphabetical).
			b, _ := os.ReadFile("../movies/testdata/resume.json")
			var res struct{ Items []json.RawMessage }
			_ = json.Unmarshal(b, &res)
			for i, j := 0, len(res.Items)-1; i < j; i, j = i+1, j-1 {
				res.Items[i], res.Items[j] = res.Items[j], res.Items[i]
			}
			_ = json.NewEncoder(w).Encode(res)
		case strings.HasPrefix(r.URL.Path, "/movies/Users/USERID0000/Items/"):
			id := strings.TrimPrefix(r.URL.Path, "/movies/Users/USERID0000/Items/")
			for _, raw := range all.Items {
				if strings.Contains(string(raw), `"Id": "`+id+`"`) {
					if id == movieID && strings.Contains(r.URL.Query().Get("Fields"), "MediaStreams") {
						// Shelter's two text subtitles (en, fr), from the captured answer.
						var it, pi map[string]any
						_ = json.Unmarshal(raw, &it)
						b, _ := os.ReadFile("../movies/testdata/playbackinfo-shelter.json")
						_ = json.Unmarshal(b, &pi)
						it["MediaSources"] = pi["MediaSources"]
						_ = json.NewEncoder(w).Encode(it)
						return
					}
					_, _ = w.Write(raw)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/movies/Videos/"+movieID+"/stream" && r.Header.Get("Range") == "bytes=0-":
			// endless body, like a 10 GB film
			w.Header().Set("Content-Type", "video/x-matroska")
			w.WriteHeader(http.StatusPartialContent)
			for r.Context().Err() == nil {
				if _, err := w.Write(make([]byte, 1024)); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				time.Sleep(2 * time.Millisecond)
			}
		case r.URL.Path == "/movies/Videos/"+movieID+"/stream":
			f.mu.Lock()
			f.streams = append(f.streams, r.Method+" "+r.URL.RequestURI()+" range="+r.Header.Get("Range"))
			f.mu.Unlock()
			http.ServeContent(w, r, "movie.mkv", time.Time{}, strings.NewReader(fileBytes))
		case strings.HasSuffix(r.URL.Path, "/PlaybackInfo"):
			// Movies starts a transcode on every PlaybackInfo; movies-cast must not ask.
			f.mu.Lock()
			f.reports = append(f.reports, "PlaybackInfo "+r.URL.Path)
			f.mu.Unlock()
			b, _ := os.ReadFile("../movies/testdata/playbackinfo-shelter.json")
			_, _ = w.Write(b)
		case strings.HasPrefix(r.URL.Path, "/movies/Videos/") && strings.Contains(r.URL.Path, "/Subtitles/"): // any film and index: the server must do the filtering
			_, _ = io.WriteString(w, "WEBVTT\n\n00:45.943 --> 00:47.838\n[suspenseful music fades in]\n")
		case strings.HasPrefix(r.URL.Path, "/movies/Sessions/"):
			body, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.reports = append(f.reports, r.URL.Path+" "+string(body))
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// fakeTV is a renderer whose state the test drives.
type fakeTV struct {
	onPlay func()
	down   bool
	sink   string // GetProtocolInfo sink; empty = accepts everything
	mu     sync.Mutex
	uri    string
	state  string
	pos    time.Duration
	seeks  []time.Duration
	calls  []string
}

func (f *fakeTV) rec(c string) { f.mu.Lock(); f.calls = append(f.calls, c); f.mu.Unlock() }
func (f *fakeTV) SetURI(_ context.Context, _ dlna.Renderer, m dlna.Media) error {
	f.mu.Lock()
	f.uri, f.state = m.URL, "STOPPED"
	f.mu.Unlock()
	f.rec("SetURI")
	return nil
}
func (f *fakeTV) Play(context.Context, dlna.Renderer) error {
	if f.onPlay != nil {
		f.onPlay()
	}
	f.set("TRANSITIONING")
	f.rec("Play")
	return nil
}
func (f *fakeTV) Pause(context.Context, dlna.Renderer) error {
	f.set("PAUSED_PLAYBACK")
	f.rec("Pause")
	return nil
}
func (f *fakeTV) Stop(context.Context, dlna.Renderer) error {
	f.set("STOPPED")
	f.rec("Stop")
	return nil
}
func (f *fakeTV) Seek(_ context.Context, _ dlna.Renderer, p time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state != "PLAYING" && f.state != "PAUSED_PLAYBACK" {
		// The LG does not answer Seek while stopped; model that as an error.
		return context.DeadlineExceeded
	}
	f.seeks = append(f.seeks, p)
	f.pos = p
	return nil
}
func (f *fakeTV) Status(context.Context, dlna.Renderer) (dlna.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return dlna.Status{}, context.DeadlineExceeded
	}
	return dlna.Status{State: f.state, Position: f.pos, Duration: 2 * time.Hour, TrackURI: f.uri}, nil
}
func (f *fakeTV) Volume(context.Context, dlna.Renderer) (int, error) {
	return 0, &dlna.Fault{Code: 606, Description: "Action not authorized"}
}
func (f *fakeTV) SetVolume(context.Context, dlna.Renderer, int) error { return nil }
func (f *fakeTV) Sink(context.Context, dlna.Renderer) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sink == "" {
		return "http-get:*:*:*", nil
	}
	return f.sink, nil
}
func (f *fakeTV) set(s string) { f.mu.Lock(); f.state = s; f.mu.Unlock() }
func (f *fakeTV) get() (string, string, []time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uri, f.state, append([]time.Duration(nil), f.seeks...)
}

type staticRenderers []dlna.Renderer

func (s staticRenderers) Find(context.Context, bool) ([]dlna.Renderer, error) { return s, nil }

var lg = dlna.Renderer{ID: "uuid:lg", Name: "[LG] webOS TV OLED55B97LA", Location: "http://127.0.0.1:1795/"}

type rig struct {
	t      *testing.T
	srv    *httptest.Server
	app    *Server
	movies *fakeMovies
	tv     *fakeTV
	client *http.Client
}

func newRig(t *testing.T) *rig {
	fm := &fakeMovies{}
	ms := httptest.NewServer(fm.handler(t))
	t.Cleanup(ms.Close)
	tv := &fakeTV{state: "NO_MEDIA_PRESENT"}
	app := New(Config{PublicURL: "http://tv-reaches-me:8097"}, movies.New(ms.URL+"/movies", "test"), tv, staticRenderers{lg})
	app.pollEvery = 5 * time.Millisecond
	app.reportEvery = 0
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() { // stop watchers before the fake Movies goes away
		app.mu.Lock()
		for id := range app.plays {
			app.endPlayLocked(id)
		}
		app.mu.Unlock()
	})
	jar, _ := cookiejar.New(nil)
	return &rig{t: t, srv: srv, app: app, movies: fm, tv: tv, client: &http.Client{Jar: jar}}
}

func (r *rig) post(path string, body any, hdr ...string) (int, map[string]any) {
	r.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, r.srv.URL+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	u, _ := url.Parse(r.srv.URL)
	req.Header.Set("Origin", "http://"+u.Host)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := r.client.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (r *rig) get(path string) (int, string) {
	r.t.Helper()
	resp, err := r.client.Get(r.srv.URL + path)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (r *rig) login() {
	r.t.Helper()
	if code, out := r.post("/api/login", map[string]string{"user": "Demo", "password": "richtig"}); code != 200 {
		r.t.Fatalf("login: %d %v", code, out)
	}
}

// streamPath turns the URL the TV got into a path on the test server.
func (r *rig) streamPath() string {
	uri, _, _ := r.tv.get()
	return strings.TrimPrefix(uri, "http://tv-reaches-me:8097")
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestLoginRequiredAndWrongPassword(t *testing.T) {
	r := newRig(t)
	if code, _ := r.get("/api/movies"); code != 401 {
		t.Fatalf("movies without login: %d", code)
	}
	if code, _ := r.post("/api/login", map[string]string{"user": "Demo", "password": "falsch"}); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	r.login()
	code, body := r.get("/api/movies")
	if code != 200 || !strings.Contains(body, "Shelter") || strings.Contains(body, "tok-geheim") {
		t.Fatalf("movies: %d %s", code, body)
	}
}

func TestCrossOriginPostIsRefused(t *testing.T) {
	r := newRig(t)
	if code, _ := r.post("/api/login", map[string]string{"user": "Demo", "password": "richtig"}, "Origin", "http://evil.example"); code != 403 {
		t.Fatalf("foreign Origin: %d", code)
	}
	if code, _ := r.post("/api/login", map[string]string{"user": "Demo", "password": "richtig"}, "Origin", "", "Sec-Fetch-Site", "cross-site"); code != 403 {
		t.Fatalf("Sec-Fetch-Site cross-site: %d", code)
	}
	if code, _ := r.post("/api/login", map[string]string{"user": "Demo", "password": "richtig"}, "Content-Type", "application/x-www-form-urlencoded"); code != 415 {
		t.Fatalf("form post: %d", code)
	}
	// Same-origin with the headers a browser really sends.
	if code, _ := r.post("/api/login", map[string]string{"user": "Demo", "password": "richtig"}, "Sec-Fetch-Site", "same-origin"); code != 200 {
		t.Fatalf("same-origin: %d", code)
	}
}

func TestPlayGivesTVATokenFreeURLThatStreamsWithRange(t *testing.T) {
	r := newRig(t)
	r.login()
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID}); code != 200 {
		t.Fatalf("play: %d %v", code, out)
	}
	uri, _, _ := r.tv.get()
	if !strings.HasPrefix(uri, "http://tv-reaches-me:8097/s/") || !strings.HasSuffix(uri, "/movie.mkv") || strings.Contains(uri, "tok-geheim") || strings.Contains(uri, "api_key") {
		t.Fatalf("TV got %q", uri)
	}
	req, _ := http.NewRequest(http.MethodGet, r.srv.URL+r.streamPath(), nil)
	req.Header.Set("Range", "bytes=5-9")
	resp, err := http.DefaultClient.Do(req) // the TV has no cookie
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(b) != "56789" || resp.Header.Get("Content-Range") != "bytes 5-9/20" {
		t.Fatalf("stream: %d %q %q", resp.StatusCode, b, resp.Header.Get("Content-Range"))
	}
	r.movies.mu.Lock()
	last := r.movies.streams[len(r.movies.streams)-1]
	r.movies.mu.Unlock()
	if last != "GET /movies/Videos/"+movieID+"/stream?static=true range=bytes=5-9" {
		t.Fatalf("upstream got %q", last)
	}
	// Status reports our title, never the TV's URI.
	code, body := r.get("/api/status?renderer=" + url.QueryEscape(lg.ID))
	if code != 200 || !strings.Contains(body, `"title":"Shelter"`) || strings.Contains(body, "/s/") {
		t.Fatalf("status: %d %s", code, body)
	}
}

func TestStreamURLDiesOnStopNewPlayAndLogout(t *testing.T) {
	r := newRig(t)
	r.login()
	streamCode := func(p string) int {
		resp, err := http.Head(r.srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID})
	first := r.streamPath()
	if c := streamCode(first); c != 200 {
		t.Fatalf("fresh URL: %d", c)
	}
	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID})
	second := r.streamPath()
	if c := streamCode(first); c != 404 || first == second {
		t.Fatalf("old URL after new play: %d", c)
	}
	if code, _ := r.post("/api/control", map[string]any{"renderer": lg.ID, "action": "stop"}); code != 200 {
		t.Fatal("stop failed")
	}
	if c := streamCode(second); c != 404 {
		t.Fatalf("URL after stop: %d", c)
	}

	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID})
	third := r.streamPath()
	r.post("/api/logout", map[string]any{})
	if c := streamCode(third); c != 404 {
		t.Fatalf("URL after logout: %d", c)
	}
	if c := streamCode("/s/" + strings.Repeat("0", 32) + "/movie.mkv"); c != 404 {
		t.Fatalf("made-up ID: %d", c)
	}
}

func TestResumeSeeksOnceAfterPlayingAndReportsStop(t *testing.T) {
	r := newRig(t)
	r.login()
	// Shelter has no resume point in the capture; use Avatar's 15 s.
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "01000000000000000000000000000001", "resume": true}); code != 200 || out["resumeSeconds"] != float64(15) {
		t.Fatalf("play: %d %v", code, out)
	}
	time.Sleep(30 * time.Millisecond) // still TRANSITIONING: no seek yet
	if _, _, seeks := r.tv.get(); len(seeks) != 0 {
		t.Fatalf("seeked before PLAYING: %v", seeks)
	}
	r.tv.set("PLAYING")
	waitFor(t, "resume seek", func() bool { _, _, s := r.tv.get(); return len(s) == 1 })
	time.Sleep(30 * time.Millisecond)
	if _, _, seeks := r.tv.get(); len(seeks) != 1 || seeks[0] != 15*time.Second {
		t.Fatalf("seeks = %v", seeks)
	}
	p := r.streamPath()
	r.tv.set("STOPPED") // stopped with the TV remote
	waitFor(t, "stopped report", func() bool {
		r.movies.mu.Lock()
		defer r.movies.mu.Unlock()
		for _, rep := range r.movies.reports {
			if strings.HasPrefix(rep, "/movies/Sessions/Playing/Stopped ") && strings.Contains(rep, `"PositionTicks":150000000`) {
				return true
			}
		}
		return false
	})
	resp, _ := http.Head(r.srv.URL + p)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("URL after TV stop: %d", resp.StatusCode)
	}
}

// Finding 1: a running transfer ends when the play is revoked.
func TestStopCutsARunningStream(t *testing.T) {
	r := newRig(t)
	r.login()
	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID})
	req, _ := http.NewRequest(http.MethodGet, r.srv.URL+r.streamPath(), nil)
	req.Header.Set("Range", "bytes=0-")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadFull(resp.Body, make([]byte, 4096)); err != nil {
		t.Fatalf("stream did not start: %v", err)
	}
	r.post("/api/control", map[string]any{"renderer": lg.ID, "action": "stop"})
	done := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, resp.Body); done <- err }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream kept running after stop")
	}
}

// Finding 2: a TV that stops answering ends the play and reports the stop.
func TestUnreachableTVEndsPlay(t *testing.T) {
	r := newRig(t)
	r.app.unreachable = 50 * time.Millisecond
	r.login()
	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID})
	p := r.streamPath()
	r.tv.set("PLAYING")
	waitFor(t, "first progress report", func() bool {
		r.movies.mu.Lock()
		defer r.movies.mu.Unlock()
		for _, rep := range r.movies.reports {
			if strings.HasPrefix(rep, "/movies/Sessions/Playing/Progress ") {
				return true
			}
		}
		return false
	})
	r.tv.mu.Lock()
	r.tv.down = true // switched off with the remote
	r.tv.mu.Unlock()
	waitFor(t, "stream URL revoked", func() bool {
		resp, err := http.Head(r.srv.URL + p)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusNotFound
	})
	waitFor(t, "stopped report", func() bool {
		r.movies.mu.Lock()
		defer r.movies.mu.Unlock()
		for _, rep := range r.movies.reports {
			if strings.HasPrefix(rep, "/movies/Sessions/Playing/Stopped ") {
				return true
			}
		}
		return false
	})
}

// Finding 5: a play revoked while SetURI/Play were on the wire gets no watcher.
func TestPlayRevokedDuringStartGetsNoWatcher(t *testing.T) {
	r := newRig(t)
	r.login()
	r.tv.onPlay = func() {
		r.app.mu.Lock()
		if p := r.app.byTV[lg.ID]; p != nil {
			r.app.endPlayLocked(p.id)
		}
		r.app.mu.Unlock()
	}
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID}); code != 409 {
		t.Fatalf("want 409, got %d %v", code, out)
	}
	time.Sleep(50 * time.Millisecond)
	r.movies.mu.Lock()
	defer r.movies.mu.Unlock()
	for _, rep := range r.movies.reports {
		if strings.HasPrefix(rep, "/movies/Sessions/Playing ") {
			t.Fatalf("watcher started for a dead play: %s", rep)
		}
	}
}

// Finding 6: wrong passwords are throttled per client, sessions are bounded.
func TestLoginThrottleAndSessionCap(t *testing.T) {
	r := newRig(t)
	for i := 0; i < loginFailLimit; i++ {
		if code, _ := r.post("/api/login", map[string]string{"user": "Demo", "password": "falsch"}); code != 401 {
			t.Fatalf("attempt %d: %d", i, code)
		}
	}
	if code, _ := r.post("/api/login", map[string]string{"user": "Demo", "password": "richtig"}); code != 429 {
		t.Fatalf("after %d failures: want 429, got %d", loginFailLimit, code)
	}
	r.app.mu.Lock()
	r.app.loginFails = map[string][]time.Time{}
	r.app.mu.Unlock()
	for i := 0; i < maxSessions+5; i++ {
		r.client.Jar, _ = cookiejar.New(nil)
		r.login()
	}
	r.app.mu.Lock()
	n := len(r.app.sessions)
	r.app.mu.Unlock()
	if n > maxSessions {
		t.Fatalf("%d sessions kept", n)
	}
}

// The test film is HEVC in MKV (Shelter). A TV whose list names Matroska but
// no HEVC gets a hint; "force" sends anyway; after the TV played it for
// provenAfter, the hint is gone for that TV and format.
func TestPlayHintsUnlistedFormatAndLearnsFromAPlay(t *testing.T) {
	r := newRig(t)
	r.login()
	r.tv.mu.Lock()
	r.tv.sink = "http-get:*:video/x-matroska:*,http-get:*:video/mp4:*"
	r.tv.mu.Unlock()
	code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID})
	if code != 409 || fmt.Sprint(out["unlisted"]) != "[HEVC (H.265)]" || !strings.Contains(fmt.Sprint(out["errorEn"]), "HEVC") {
		t.Fatalf("hint: %d %v", code, out)
	}
	if uri, _, _ := r.tv.get(); uri != "" {
		t.Fatalf("TV got %q before the user chose to send anyway", uri)
	}
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID, "force": true}); code != 200 {
		t.Fatalf("force: %d %v", code, out)
	}
	r.tv.mu.Lock()
	r.tv.state, r.tv.pos = "PLAYING", provenAfter+time.Second
	r.tv.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for {
		r.app.mu.Lock()
		n := len(r.app.proven)
		r.app.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a play of 11 s was not remembered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID}); code != 200 {
		t.Fatalf("after a proven play: %d %v", code, out)
	}
}

// A proven format survives a restart (formats.json next to the sessions).
func TestProvenFormatSurvivesRestart(t *testing.T) {
	r, f := newRigWithState(t)
	r.login()
	r.tv.mu.Lock()
	r.tv.sink = "http-get:*:video/x-matroska:*"
	r.tv.mu.Unlock()
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID, "force": true}); code != 200 {
		t.Fatalf("force: %d %v", code, out)
	}
	r.tv.mu.Lock()
	r.tv.state, r.tv.pos = "PLAYING", provenAfter+time.Second
	r.tv.mu.Unlock()
	formats := filepath.Join(filepath.Dir(f), "formats.json")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if b, err := os.ReadFile(formats); err == nil && strings.Contains(string(b), "|mkv|hevc") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("formats.json not written")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if fi, _ := os.Stat(formats); fi.Mode().Perm() != 0o600 {
		t.Errorf("formats.json mode %v, want 0600", fi.Mode().Perm())
	}
	r2 := r.restart(f)
	carryCookie(t, r, r2)
	if code, out := r2.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID}); code != 200 {
		t.Fatalf("after restart: %d %v", code, out)
	}
}
