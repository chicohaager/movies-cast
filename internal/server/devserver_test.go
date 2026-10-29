package server

import (
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"testing"
	"time"

	"movies-cast/internal/movies"
)

// TestDevServer runs the real handler against the fake Movies and the fake TV
// of the tests, so the phone page can be clicked through in a browser with a
// login (Demo or Anna, password "richtig") — without a NAS, a Movies account
// or a TV. It runs only when asked:
//
//	MC_DEV_SERVER=127.0.0.1:18097 go test ./internal/server/ -run TestDevServer -timeout 0 -v
//
// and stops on Ctrl-C (or SIGINT to the test binary). State files live in a
// temporary directory and are gone afterwards.
func TestDevServer(t *testing.T) {
	addr := os.Getenv("MC_DEV_SERVER")
	if addr == "" {
		t.Skip("MC_DEV_SERVER not set")
	}
	fm := &fakeMovies{}
	ms := &http.Server{Handler: fm.handler(t)}
	mln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = ms.Serve(mln) }()
	defer ms.Close()

	tv := &fakeTV{state: "NO_MEDIA_PRESENT"}
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
	app := New(Config{PublicURL: "http://" + addr, StateFile: state}, movies.New("http://"+mln.Addr().String()+"/movies", "dev"), tv, staticRenderers{lg})
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: app.Handler()}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	t.Logf("dev server on http://%s — log in as Demo or Anna, password richtig; state in %s", ln.Addr(), filepath.Dir(state))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}
