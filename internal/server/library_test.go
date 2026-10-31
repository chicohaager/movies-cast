package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"movies-cast/internal/dlna"
	"movies-cast/internal/movies"
)

// libItems is a small Jellyfin-like library: a film, a series with two
// episodes, an album with two tracks and a photo (shapes as Jellyfin 10.11.0
// answered them on 2026-10-31; names and IDs invented).
var libItems = map[string]map[string]any{
	"F1": {"Id": "F1", "Name": "Film One", "Type": "Movie", "RunTimeTicks": 6000 * 10_000_000, "Container": "mkv",
		"MediaSources": []map[string]any{{"Id": "ms1", "Container": "mkv", "MediaStreams": []map[string]any{
			{"Type": "Video", "Codec": "h264", "Width": 1920, "Index": 0},
			{"Type": "Subtitle", "Language": "ger", "DisplayTitle": "Deutsch", "DeliveryMethod": "External", "Index": 2},
		}}}},
	"S1": {"Id": "S1", "Name": "The Show", "Type": "Series", "RecursiveItemCount": 2},
	"E1": {"Id": "E1", "Name": "Pilot", "Type": "Episode", "SeriesId": "S1", "SeriesName": "The Show", "ParentIndexNumber": 1, "IndexNumber": 1, "RunTimeTicks": 1200 * 10_000_000, "Container": "mkv"},
	"E2": {"Id": "E2", "Name": "Second", "Type": "Episode", "SeriesId": "S1", "SeriesName": "The Show", "ParentIndexNumber": 1, "IndexNumber": 2, "RunTimeTicks": 1200 * 10_000_000, "Container": "mkv"},
	"A1": {"Id": "A1", "Name": "Album", "Type": "MusicAlbum", "ChildCount": 2},
	"T1": {"Id": "T1", "Name": "Track 1", "Type": "Audio", "AlbumId": "A1", "IndexNumber": 1, "RunTimeTicks": 180 * 10_000_000, "Container": "mp3"},
	"T2": {"Id": "T2", "Name": "Track 2", "Type": "Audio", "AlbumId": "A1", "IndexNumber": 2, "RunTimeTicks": 180 * 10_000_000, "Container": "mp3"},
	"P1": {"Id": "P1", "Name": "Photo", "Type": "Photo", "Container": "jpg"},
}

type libMovies struct {
	headStatus int // answer to HEAD on /Items/ (405 Jellyfin, 404 Emby)
	mu         sync.Mutex
	fetched    []string // upstream file paths the server asked for
}

func (f *libMovies) handler() http.Handler {
	list := func(w http.ResponseWriter, ids ...string) {
		var items []map[string]any
		for _, id := range ids {
			items = append(items, libItems[id])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": len(items)})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/movies")
		switch {
		case p == "/Users/AuthenticateByName":
			_, _ = io.WriteString(w, `{"AccessToken":"tok-lib","User":{"Id":"USERID0000","Name":"Demo"}}`)
		case !strings.Contains(r.Header.Get("X-Emby-Authorization"), `Token="tok-lib"`):
			w.WriteHeader(http.StatusUnauthorized)
		case p == "/Users/USERID0000/Views":
			_, _ = io.WriteString(w, `{"Items":[{"CollectionType":"movies"},{"CollectionType":"tvshows"},{"CollectionType":"music"}]}`)
		case p == "/Shows/S1/Episodes":
			list(w, "E1", "E2")
		case p == "/Users/USERID0000/Items" && r.URL.Query().Get("ParentId") == "A1":
			list(w, "T1", "T2")
		case p == "/Users/USERID0000/Items":
			switch r.URL.Query().Get("IncludeItemTypes") {
			case "Movie,Series":
				list(w, "F1", "S1")
			case "MusicAlbum":
				list(w, "A1")
			case "Photo":
				list(w, "P1")
			default:
				list(w)
			}
		case strings.HasPrefix(p, "/Users/USERID0000/Items/"):
			it, ok := libItems[strings.TrimPrefix(p, "/Users/USERID0000/Items/")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(it)
		case p == "/Videos/F1/ms1/Subtitles/2/Stream.vtt":
			_, _ = io.WriteString(w, "WEBVTT\n\n00:00:01.000 --> 00:00:02.500\nHallo\n")
		case strings.HasPrefix(p, "/Sessions/"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasPrefix(p, "/Items/") && r.Method == http.MethodHead:
			// Jellyfin 10.11.0 answers HEAD on /Items/{id}/Download with 405,
			// Emby 4.10.2.0 with 404 (measured 2026-10-31; the LG then refused
			// the photo with UPnP 716).
			w.WriteHeader(f.headStatus)
		case strings.HasPrefix(p, "/Videos/"), strings.HasPrefix(p, "/Items/"):
			w.Header().Set("Content-Type", "image/jpeg")
			f.mu.Lock()
			f.fetched = append(f.fetched, p)
			f.mu.Unlock()
			_, _ = io.WriteString(w, "file")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func newLibRig(t *testing.T) (*rig, *libMovies) {
	fm := &libMovies{headStatus: http.StatusMethodNotAllowed}
	ms := httptest.NewServer(fm.handler())
	t.Cleanup(ms.Close)
	tv := &fakeTV{state: "NO_MEDIA_PRESENT"}
	app := New(Config{PublicURL: "http://tv-reaches-me:8097"}, movies.New(ms.URL+"/movies", "test"), tv, staticRenderers{lg})
	app.pollEvery = 5 * time.Millisecond
	app.reportEvery = 0
	srv := httptest.NewServer(app.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		if !lockWithin(&app.mu, 5*time.Second) {
			t.Error("app.mu still held at cleanup")
			return
		}
		for id := range app.plays {
			app.endPlayLocked(id)
		}
		app.mu.Unlock()
	})
	jar, _ := cookiejar.New(nil)
	r := &rig{t: t, srv: srv, app: app, tv: tv, client: &http.Client{Jar: jar}}
	r.login()
	return r, fm
}

// runToEnd lets the fake TV play the current item up to before seconds
// before its end, then stop, like a TV at the end of a file.
func runToEnd(t *testing.T, r *rig, before time.Duration) {
	t.Helper()
	r.tv.mu.Lock()
	r.tv.state, r.tv.pos = "PLAYING", r.tv.media.Duration-before
	r.tv.mu.Unlock()
	waitFor(t, "the play saw PLAYING", func() bool {
		r.app.mu.Lock()
		defer r.app.mu.Unlock()
		p := r.app.byTV[lg.ID]
		return p != nil && p.sawPlay && p.lastPos > 0
	})
	r.tv.set("STOPPED")
}

func titleOnTV(r *rig) string {
	r.tv.mu.Lock()
	defer r.tv.mu.Unlock()
	return r.tv.media.Title
}

func TestEpisodeThatRanToItsEndIsFollowedByTheNext(t *testing.T) {
	r, _ := newLibRig(t)
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "E1"}); code != 200 {
		t.Fatalf("play: %d %v", code, out)
	}
	runToEnd(t, r, 10*time.Second)
	waitFor(t, "the next episode on the TV", func() bool { return titleOnTV(r) == "Second" })
}

func TestStopOnThePhoneStartsNothing(t *testing.T) {
	r, _ := newLibRig(t)
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "E1"}); code != 200 {
		t.Fatalf("play: %d %v", code, out)
	}
	r.tv.mu.Lock()
	r.tv.state, r.tv.pos = "PLAYING", r.tv.media.Duration-5*time.Second
	r.tv.mu.Unlock()
	waitFor(t, "PLAYING seen", func() bool {
		r.app.mu.Lock()
		defer r.app.mu.Unlock()
		p := r.app.byTV[lg.ID]
		return p != nil && p.sawPlay
	})
	if code, _ := r.post("/api/control", map[string]any{"renderer": lg.ID, "action": "stop"}); code != 200 {
		t.Fatal("stop")
	}
	time.Sleep(100 * time.Millisecond)
	if got := titleOnTV(r); got != "Pilot" {
		t.Fatalf("after Stop the TV got %q", got)
	}
}

func TestAPlayStoppedInTheMiddleStartsNothing(t *testing.T) {
	r, _ := newLibRig(t)
	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "E1"})
	runToEnd(t, r, 10*time.Minute) // stopped on the TV half-way through
	time.Sleep(100 * time.Millisecond)
	if got := titleOnTV(r); got != "Pilot" {
		t.Fatalf("a play stopped half-way was followed by %q", got)
	}
}

func TestTheQueueComesBeforeTheNextEpisode(t *testing.T) {
	r, _ := newLibRig(t)
	if code, out := r.post("/api/queue", map[string]any{"renderer": lg.ID, "movie": "F1", "action": "add"}); code != 200 {
		t.Fatalf("queue add: %d %v", code, out)
	}
	if code, _ := r.post("/api/queue", map[string]any{"renderer": lg.ID, "movie": "S1", "action": "add"}); code != 400 {
		t.Fatalf("a series in the queue: %d, want 400", code)
	}
	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "E1"})
	runToEnd(t, r, 10*time.Second)
	waitFor(t, "the queued film on the TV", func() bool { return titleOnTV(r) == "Film One" })
	_, body := r.get("/api/queue?renderer=" + url.QueryEscape(lg.ID))
	if !strings.Contains(body, `"items":[]`) {
		t.Fatalf("queue after its film started: %s", body)
	}
}

func TestNoAutoplayKeepsTheNextEpisodeOff(t *testing.T) {
	r, _ := newLibRig(t)
	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "E1"})
	if code, _ := r.post("/api/queue", map[string]any{"renderer": lg.ID, "action": "noauto"}); code != 200 {
		t.Fatal("noauto")
	}
	_, body := r.get("/api/queue?renderer=" + url.QueryEscape(lg.ID))
	if !strings.Contains(body, `"autoplay":false`) {
		t.Fatalf("queue: %s", body)
	}
	runToEnd(t, r, 10*time.Second)
	time.Sleep(100 * time.Millisecond)
	if got := titleOnTV(r); got != "Pilot" {
		t.Fatalf("with autoplay off the TV got %q", got)
	}
}

func TestTheNextTrackOfAnAlbumFollows(t *testing.T) {
	r, _ := newLibRig(t)
	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "T1"})
	r.tv.mu.Lock()
	m := r.tv.media
	r.tv.mu.Unlock()
	if m.Class != "object.item.audioItem.musicTrack" || m.MimeType != "audio/mpeg" {
		t.Fatalf("track sent as %q %q", m.Class, m.MimeType)
	}
	runToEnd(t, r, 3*time.Second)
	waitFor(t, "track 2 on the TV", func() bool { return titleOnTV(r) == "Track 2" })
}

func TestAPhotoIsSentAsAnImageFromDownload(t *testing.T) {
	for _, head := range []int{http.StatusMethodNotAllowed, http.StatusNotFound} {
		t.Run(http.StatusText(head), func(t *testing.T) { photoFromDownload(t, head) })
	}
}

func photoFromDownload(t *testing.T, headStatus int) {
	r, fm := newLibRig(t)
	fm.headStatus = headStatus
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "P1"}); code != 200 {
		t.Fatalf("play photo: %d %v", code, out)
	}
	r.tv.mu.Lock()
	m := r.tv.media
	r.tv.mu.Unlock()
	if m.Class != "object.item.imageItem.photo" || m.MimeType != "image/jpeg" {
		t.Fatalf("photo sent as %q %q", m.Class, m.MimeType)
	}
	if !strings.Contains(dlna.DIDL(m), "<upnp:class>object.item.imageItem.photo</upnp:class>") {
		t.Fatalf("DIDL: %s", dlna.DIDL(m))
	}
	head, err := http.Head(r.srv.URL + strings.TrimPrefix(m.URL, "http://tv-reaches-me:8097"))
	if err != nil {
		t.Fatal(err)
	}
	head.Body.Close()
	if head.StatusCode != 200 || head.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("HEAD of the photo: %d %q (the LG asks HEAD first)", head.StatusCode, head.Header.Get("Content-Type"))
	}
	resp, err := http.Get(r.srv.URL + strings.TrimPrefix(m.URL, "http://tv-reaches-me:8097"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	fm.mu.Lock()
	defer fm.mu.Unlock()
	if resp.StatusCode != 200 || len(fm.fetched) == 0 || fm.fetched[len(fm.fetched)-1] != "/Items/P1/Download" {
		t.Fatalf("photo stream: %d, upstream %v", resp.StatusCode, fm.fetched)
	}
}

func TestASeriesIsNotSentButListsItsEpisodes(t *testing.T) {
	r, _ := newLibRig(t)
	if code, _ := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "S1"}); code != 400 {
		t.Fatalf("play series: %d, want 400", code)
	}
	code, body := r.get("/api/children/S1")
	if code != 200 || !strings.Contains(body, `"name":"Pilot"`) || !strings.Contains(body, `"episode":2`) || strings.Index(body, "Pilot") > strings.Index(body, "Second") {
		t.Fatalf("episodes: %d %s", code, body)
	}
	code, body = r.get("/api/views")
	if code != 200 || body != "{\"music\":true,\"photos\":false,\"series\":true}\n" {
		t.Fatalf("views: %d %q", code, body)
	}
	code, body = r.get("/api/library?kind=music")
	if code != 200 || !strings.Contains(body, `"type":"MusicAlbum"`) {
		t.Fatalf("music: %d %s", code, body)
	}
	if code, _ := r.get("/api/library?kind=../x"); code != 400 {
		t.Fatalf("unknown kind: %d", code)
	}
}

func TestSourceNamesTheAccountsServer(t *testing.T) {
	info := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jf/System/Info/Public":
			_, _ = io.WriteString(w, `{"ProductName":"Jellyfin Server","Version":"10.11.0"}`)
		case "/emby/System/Info/Public": // Emby 4.10.2.0 sends no ProductName
			_, _ = io.WriteString(w, `{"Version":"4.10.2.0"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	up := httptest.NewServer(info)
	defer up.Close()
	for base, want := range map[string]string{
		"/movies": `{"zimaos":true}`,
		"/jf":     `{"product":"Jellyfin Server","zimaos":false}`,
		"/emby":   `{"product":"Emby Server","zimaos":false}`,
	} {
		app := New(Config{}, movies.New(up.URL+base, "test"), &fakeTV{}, staticRenderers{})
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/source", nil))
		if got := strings.TrimSpace(rec.Body.String()); rec.Code != 200 || got != want {
			t.Errorf("%s: %d %s, want %s", base, rec.Code, got, want)
		}
	}
}

func TestADLNAPlayCarriesTheChosenSubtitleAsSRT(t *testing.T) {
	r, _ := newLibRig(t)
	code, body := r.get("/api/subtitles/F1")
	if code != 200 || !strings.Contains(body, `"label":"Deutsch"`) || !strings.Contains(body, `"index":2`) {
		t.Fatalf("subtitles: %d %s", code, body)
	}
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "F1", "sub": 2}); code != 200 {
		t.Fatalf("play: %d %v", code, out)
	}
	r.tv.mu.Lock()
	m := r.tv.media
	r.tv.mu.Unlock()
	if !strings.HasSuffix(m.SubtitleURL, "/sub.srt") || strings.TrimSuffix(m.SubtitleURL, "sub.srt") != strings.TrimSuffix(m.URL, "movie.mkv") {
		t.Fatalf("subtitle URL %q next to %q", m.SubtitleURL, m.URL)
	}
	didl := dlna.DIDL(m)
	if !strings.Contains(didl, `<sec:CaptionInfoEx sec:type="srt">`+m.SubtitleURL) || !strings.Contains(didl, `protocolInfo="http-get:*:text/srt:*"`) {
		t.Fatalf("DIDL: %s", didl)
	}
	resp, err := http.Get(r.srv.URL + strings.TrimPrefix(m.SubtitleURL, "http://tv-reaches-me:8097"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != "1\n00:00:01,000 --> 00:00:02,500\nHallo\n\n" {
		t.Fatalf("sub.srt: %d %q", resp.StatusCode, b)
	}
	// The film's own fetch tells a Samsung where the subtitle is.
	req, _ := http.NewRequest(http.MethodHead, r.srv.URL+strings.TrimPrefix(m.URL, "http://tv-reaches-me:8097"), nil)
	req.Header.Set("getcaptionInfo.sec", "1")
	h, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	h.Body.Close()
	if h.Header.Get("CaptionInfo.sec") != m.SubtitleURL {
		t.Fatalf("CaptionInfo.sec = %q", h.Header.Get("CaptionInfo.sec"))
	}
	// Without a choice: no subtitle.
	r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "F1"})
	r.tv.mu.Lock()
	m = r.tv.media
	r.tv.mu.Unlock()
	if m.SubtitleURL != "" || strings.Contains(dlna.DIDL(m), "CaptionInfo") {
		t.Fatalf("no subtitle chosen, TV got %q", m.SubtitleURL)
	}
}
