package movies

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeMovies serves the captured /Items answer of Movies 0.3.0 and checks auth.
func fakeMovies(t *testing.T) (*httptest.Server, *[]string) {
	items, err := os.ReadFile("testdata/items.json")
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("X-Emby-Authorization")
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.RequestURI()+" | "+auth+" | "+string(body))
		if r.URL.Path == "/movies/Users/AuthenticateByName" {
			var in map[string]string
			_ = json.Unmarshal(body, &in)
			if in["Username"] != "demo" || in["Pw"] != "richtig" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid credentials"}`))
				return
			}
			_, _ = w.Write([]byte(`{"AccessToken":"tok-123","User":{"Id":"USERID0000","Name":"Demo"}}`))
			return
		}
		if !strings.Contains(auth, `Token="tok-123"`) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/movies/Users/USERID0000/Items":
			_, _ = w.Write(items)
		case strings.HasPrefix(r.URL.Path, "/movies/Sessions/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestLoginAndMoviesFromCapturedAnswer(t *testing.T) {
	srv, _ := fakeMovies(t)
	c := New(srv.URL+"/movies/", "test")
	if _, err := c.Login(context.Background(), "demo", "falsch", "dev1"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong password: want ErrUnauthorized, got %v", err)
	}
	s, err := c.Login(context.Background(), "demo", "richtig", "dev1")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := c.Movies(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 4 {
		t.Fatalf("got %d movies", len(ms))
	}
	var avatar Movie
	for _, m := range ms {
		if m.ID == "01000000000000000000000000000001" {
			avatar = m
		}
	}
	// Values from the captured answer: 150335840 ticks = 15 s resume, HEVC 3840 wide, E-AC3.
	if avatar.Resume != 15 || avatar.VideoCodec != "hevc" || avatar.Width != 3840 || avatar.Audio != "eac3" || !avatar.HasPoster || avatar.Container != "mkv" {
		t.Errorf("avatar = %+v", avatar)
	}
	if avatar.Size != 39673343629 {
		t.Errorf("size %d", avatar.Size)
	}
	if avatar.Runtime < 3600 {
		t.Errorf("runtime %d s", avatar.Runtime)
	}
}

func TestExpiredTokenIsUnauthorized(t *testing.T) {
	srv, _ := fakeMovies(t)
	c := New(srv.URL+"/movies", "test")
	_, err := c.Movies(context.Background(), &Session{Token: "alt", UserID: "USERID0000"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v", err)
	}
}

func TestReportSendsTicksAndSessionID(t *testing.T) {
	srv, seen := fakeMovies(t)
	c := New(srv.URL+"/movies", "test")
	s := &Session{Token: "tok-123", UserID: "USERID0000", DeviceID: "d"}
	if err := c.ReportProgress(context.Background(), s, PlayState{ItemID: "X", PlaySessionID: "ps1", Position: 90 * time.Second, Paused: true}); err != nil {
		t.Fatal(err)
	}
	got := (*seen)[len(*seen)-1]
	for _, want := range []string{"POST /movies/Sessions/Playing/Progress", `"PositionTicks":900000000`, `"PlaySessionId":"ps1"`, `"IsPaused":true`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestStreamRequestKeepsTokenOutOfURL(t *testing.T) {
	c := New("http://127.0.0.1/movies", "test")
	req, err := c.StreamRequest(context.Background(), &Session{Token: "geheim", DeviceID: "d"}, http.MethodGet, "01")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(req.URL.String(), "geheim") || !strings.Contains(req.Header.Get("X-Emby-Authorization"), `Token="geheim"`) {
		t.Errorf("url=%s header=%s", req.URL, req.Header.Get("X-Emby-Authorization"))
	}
	if req.URL.String() != "http://127.0.0.1/movies/Videos/01/stream?static=true" {
		t.Errorf("url=%s", req.URL)
	}
}

// Finding 4: the token header must not follow a redirect to another host.
func TestNoRedirectWithToken(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = leaked || strings.Contains(r.Header.Get("X-Emby-Authorization"), "geheim")
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/x", http.StatusFound)
	}))
	defer srv.Close()
	c := New(srv.URL+"/movies", "test")
	_, err := c.Movies(context.Background(), &Session{Token: "geheim", UserID: "u"})
	if leaked {
		t.Fatal("token followed the redirect")
	}
	if err == nil {
		t.Fatal("a redirect must surface as an error")
	}
}

// Captured item answer of Avatar (2026-10-28, named fields only): five text
// subtitles. They come from the item, never from PlaybackInfo: Movies answers
// every PlaybackInfo that lists the streams by starting a transcode (log line
// "[streamMedia] movie/1 transcode codec=h264", ffmpeg on the box), although
// the TV then plays the original file.
func TestSubtitlesFromItemStreams(t *testing.T) {
	it, err := os.ReadFile("testdata/item-avatar-streams.json")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+" "+r.URL.Query().Get("Fields"))
		_, _ = w.Write(it)
	}))
	defer srv.Close()
	c := New(srv.URL+"/movies", "test")
	subs, msID, listed, err := c.Subtitles(context.Background(), &Session{Token: "t", UserID: "USERID0000"}, "01000000000000000000000000000001")
	if err != nil || !listed {
		t.Fatal(err, listed)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "GET /movies/Users/USERID0000/Items/01000000000000000000000000000001 ") || !strings.Contains(got[0], "MediaStreams") {
		t.Errorf("requests %q", got)
	}
	if msID != "01000000000000000000000000000001" || len(subs) != 5 || subs[0].Index != 4 || subs[0].Language != "de" || subs[0].Label != "German" || subs[4].Index != 8 || subs[4].Language != "en" {
		t.Fatalf("got %q %+v", msID, subs)
	}
}

func TestSubtitleRequestURL(t *testing.T) {
	c := New("http://127.0.0.1/movies", "test")
	req, _ := c.SubtitleRequest(context.Background(), &Session{Token: "geheim"}, "A", "B", 3)
	if req.URL.String() != "http://127.0.0.1/movies/Videos/A/B/Subtitles/3/Stream.vtt" || strings.Contains(req.URL.String(), "geheim") {
		t.Fatalf("url %s", req.URL)
	}
}

// Captured Resume answer: Avatar, Glennkill, Shelter in that order.
func TestResumeKeepsMoviesOrder(t *testing.T) {
	rs, err := os.ReadFile("testdata/resume.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(rs) }))
	defer srv.Close()
	ms, err := New(srv.URL+"/movies", "t").Resume(context.Background(), &Session{Token: "t", UserID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 3 || ms[0].Name != "Avatar: Fire and Ash" || ms[0].Resume != 203 || ms[2].Name != "Shelter" || ms[2].Resume != 33 {
		t.Fatalf("got %+v", ms)
	}
}

// Image subtitles (PGS on Blu-ray rips) cannot become WebVTT; Movies marks
// them other than "External". None is in the captured answer, so this case
// is written by hand.
func TestSubtitlesSkipImageTracks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"MediaSources":[{"Id":"M","MediaStreams":[
{"Type":"Subtitle","Index":2,"Codec":"PGSSUB","Language":"de","DeliveryMethod":"Encode"},
{"Type":"Subtitle","Index":3,"Codec":"srt","Language":"en","DeliveryMethod":"External"}]}]}`)
	}))
	defer srv.Close()
	subs, _, _, err := New(srv.URL+"/movies", "t").Subtitles(context.Background(), &Session{Token: "t", UserID: "u"}, "X")
	if err != nil || len(subs) != 1 || subs[0].Index != 3 {
		t.Fatalf("got %+v %v", subs, err)
	}
}

// Shelter's item answer (2026-10-28) listed no subtitle and no video width,
// while its PlaybackInfo listed en and fr: such an answer is not "listed".
func TestSubtitlesNotListedWhenStreamsAreUnread(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"Id":"S","MediaSources":[{"Id":"S","MediaStreams":[{"Type":"Video","Codec":"hevc"},{"Type":"Audio","Codec":"ac3"}]}]}`)
	}))
	defer srv.Close()
	subs, _, listed, err := New(srv.URL+"/movies", "t").Subtitles(context.Background(), &Session{Token: "t", UserID: "u"}, "S")
	if err != nil || listed || len(subs) != 0 {
		t.Fatalf("listed=%v subs=%+v err=%v", listed, subs, err)
	}
}

// Captured PlaybackInfo of Shelter: two text subtitles, en and fr.
func TestProbedSubtitlesAskPlaybackInfo(t *testing.T) {
	pi, err := os.ReadFile("testdata/playbackinfo-shelter.json")
	if err != nil {
		t.Fatal(err)
	}
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Method + " " + r.URL.RequestURI()
		_, _ = w.Write(pi)
	}))
	defer srv.Close()
	subs, msID, err := New(srv.URL+"/movies", "t").ProbedSubtitles(context.Background(), &Session{Token: "t", UserID: "USERID0000"}, "01000000000000000000000000000006")
	if err != nil {
		t.Fatal(err)
	}
	if got != "POST /movies/Items/01000000000000000000000000000006/PlaybackInfo?UserId=USERID0000" {
		t.Errorf("request %q", got)
	}
	if msID != "01000000000000000000000000000006" || len(subs) != 2 || subs[0].Language != "en" || subs[1].Language != "fr" {
		t.Fatalf("got %q %+v", msID, subs)
	}
}
