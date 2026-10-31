package server

import (
	"encoding/xml"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"movies-cast/internal/dlna"
	"movies-cast/internal/movies"
)

// TestDevServer runs the real handler against the fake Movies and the fake TV
// of the tests, so the phone page can be clicked through in a browser with a
// login (Demo or Anna, password "richtig") — without a NAS, a Movies account
// or a TV. It runs only when asked:
//
//	MC_DEV_SERVER=127.0.0.1:18097 go test -count=1 ./internal/server/ -run TestDevServer -timeout 0 -v
//
// and stops on Ctrl-C, SIGINT or SIGTERM (e2e/ starts it that way). State files live in a
// temporary directory and are gone afterwards.
func TestDevServer(t *testing.T) {
	addr := os.Getenv("MC_DEV_SERVER")
	if addr == "" {
		t.Skip("MC_DEV_SERVER not set")
	}
	// MC_DEV_LIBRARY=../../docs/screenshots/demo shows public-domain films with
	// their posters instead of the captured library (README screenshots).
	fm := &fakeMovies{library: os.Getenv("MC_DEV_LIBRARY")}
	// MC_DEV_HLS_LENGTH=94m makes "No sound?" convert a whole film (Movies
	// answers with segments for the full length) instead of the tests' 18.5 s.
	if v := os.Getenv("MC_DEV_HLS_LENGTH"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatal(err)
		}
		fm.hlsLength = d
	}
	ms := &http.Server{Handler: fm.handler(t)}
	mln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = ms.Serve(mln) }()
	defer ms.Close()

	tv := &fakeTV{state: "NO_MEDIA_PRESENT"}
	// MC_DEV_SINK=../dlna/testdata/lg-protocolinfo.xml makes the fake TV list the
	// formats of a real TV (GetProtocolInfo answer), so the format hint shows.
	if p := os.Getenv("MC_DEV_SINK"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		// Decoded as XML (entities, whitespace) like dlna.Client.Sink does.
		var env struct {
			Sink string `xml:"Body>GetProtocolInfoResponse>Sink"`
		}
		if err := xml.Unmarshal(b, &env); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if tv.sink = strings.TrimSpace(env.Sink); tv.sink == "" {
			t.Fatalf("%s: no <Sink> list", p)
		}
	}
	// The unit tests set the fake TV's state themselves; here it plays like a
	// TV: TRANSITIONING becomes PLAYING after a second, and PLAYING counts up.
	go func() {
		for range time.Tick(time.Second) {
			tv.mu.Lock()
			switch tv.state {
			case "TRANSITIONING":
				tv.state = "PLAYING"
			case "PLAYING":
				tv.pos += time.Second
			}
			tv.mu.Unlock()
		}
	}()
	state := filepath.Join(t.TempDir(), "sessions.json")
	tvs := staticRenderers{lg}
	// MC_DEV_SECOND_TV=1 adds a second TV, so "Manage TVs" can hide one, and
	// gives both documentation addresses (RFC 5737) instead of 127.0.0.x, so a
	// screen film shows LAN-like addresses.
	if os.Getenv("MC_DEV_SECOND_TV") == "1" {
		first := lg
		first.Location = "http://192.0.2.52:1795/"
		tvs = staticRenderers{first, dlna.Renderer{ID: "uuid:samsung", Name: "Samsung UE50MU6179", Location: "http://192.0.2.30:9197/"}}
	}
	app := New(Config{PublicURL: "http://" + addr, StateFile: state}, movies.New("http://"+mln.Addr().String()+"/movies", "dev"), tv, tvs)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: app.Handler()}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	t.Logf("dev server on http://%s — log in as Demo or Anna, password richtig; state in %s", ln.Addr(), filepath.Dir(state))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
