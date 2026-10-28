package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"movies-cast/internal/movies"
)

// restart starts a second server on the same state file and fake Movies,
// keeping the clients' cookies: what a container restart looks like.
func (r *rig) restart(stateFile string) *rig {
	r.t.Helper()
	app := New(Config{PublicURL: "http://tv-reaches-me:8097", StateFile: stateFile}, r.app.movies, r.tv, staticRenderers{lg})
	app.pollEvery = 5 * time.Millisecond
	srv := httptest.NewServer(app.Handler())
	r.t.Cleanup(srv.Close)
	// Cookies are per host; keep the same jar by pointing at the new URL with the old cookies.
	return &rig{t: r.t, srv: srv, app: app, movies: r.movies, tv: r.tv, client: r.client}
}

func newRigWithState(t *testing.T) (*rig, string) {
	r := newRig(t)
	f := filepath.Join(t.TempDir(), "sessions.json")
	r.app.cfg.StateFile = f
	return r, f
}

func carryCookie(t *testing.T, from, to *rig) {
	t.Helper()
	fu, _ := http.NewRequest(http.MethodGet, from.srv.URL, nil)
	tu, _ := http.NewRequest(http.MethodGet, to.srv.URL, nil)
	to.client.Jar.SetCookies(tu.URL, from.client.Jar.Cookies(fu.URL))
}

func TestSessionSurvivesRestartAndFileIs0600(t *testing.T) {
	r, f := newRigWithState(t)
	r.login()
	st, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode %o", st.Mode().Perm())
	}
	r2 := r.restart(f)
	carryCookie(t, r, r2)
	if c, body := r2.get("/api/movies"); c != http.StatusOK || !strings.Contains(body, "Shelter") {
		t.Fatalf("after restart: %d", c)
	}
}

func TestPairedTVSharesLoginAfterRestart(t *testing.T) {
	phone, f := newRigWithState(t)
	tv := phone.tvClient()
	phone.login()
	pairTV(t, phone, tv)
	phone2 := phone.restart(f)
	tv2 := &rig{t: t, srv: phone2.srv, app: phone2.app, movies: phone2.movies, tv: phone2.tv, client: tv.client}
	carryCookie(t, phone, phone2)
	carryCookie(t, tv, tv2)
	phone2.app.mu.Lock()
	var mss []*movies.Session
	for _, s := range phone2.app.sessions {
		mss = append(mss, s.ms)
	}
	phone2.app.mu.Unlock()
	if len(mss) != 2 || mss[0] != mss[1] {
		t.Fatalf("want 2 sessions sharing one Movies login, got %d sessions, shared=%v", len(mss), len(mss) == 2 && mss[0] == mss[1])
	}
	phone2.post("/api/logout", map[string]any{})
	if n := phone2.moviesLogouts(); n != 0 {
		t.Fatalf("phone logout after restart logged the TV out at Movies: %d", n)
	}
	if c, _ := tv2.get("/api/movies"); c != http.StatusOK {
		t.Fatalf("TV after restart + phone logout: %d", c)
	}
}

func TestLogoutRemovesTokenFromFile(t *testing.T) {
	r, f := newRigWithState(t)
	r.login()
	if b, _ := os.ReadFile(f); !strings.Contains(string(b), "tok-geheim") {
		t.Fatalf("token not stored: %s", b)
	}
	r.post("/api/logout", map[string]any{})
	if b, _ := os.ReadFile(f); strings.Contains(string(b), "tok-geheim") {
		t.Fatalf("token still in file after logout")
	}
}

func TestExpiredStoredSessionIsDropped(t *testing.T) {
	f := filepath.Join(t.TempDir(), "s.json")
	old := time.Now().Add(-sessionMaxAge - time.Hour).Format(time.RFC3339)
	if err := os.WriteFile(f, []byte(`[{"id":"a","token":"t","userId":"u","lastSeen":"`+old+`"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	app := New(Config{StateFile: f}, movies.New("http://127.0.0.1:1/movies", "t"), &fakeTV{}, staticRenderers{})
	if len(app.sessions) != 0 {
		t.Fatalf("expired session loaded")
	}
}
