package server

import (
	"net/http"
	"net/http/cookiejar"
	"sync"
	"testing"
	"time"
)

// Guesses sent at the same time all passed the check before the first
// failure was recorded: the limit held for serial attempts only.
func TestParallelLoginGuessesAreThrottled(t *testing.T) {
	r := newRig(t)
	r.movies.loginDelay = 200 * time.Millisecond
	var wg sync.WaitGroup
	for range 3 * loginFailLimit {
		wg.Go(func() {
			r.post("/api/login", map[string]string{"user": "Demo", "password": "falsch"})
		})
	}
	wg.Wait()
	r.movies.mu.Lock()
	n := r.movies.wrongLogins
	r.movies.mu.Unlock()
	if n > loginFailLimit {
		t.Fatalf("%d of %d parallel guesses reached Movies, limit %d", n, 3*loginFailLimit, loginFailLimit)
	}
}

// The server keeps a daily-used session alive (sessionMaxAge after the last
// request), so the cookie must slide too; it was set once at login with a
// fixed Max-Age and the browser dropped it after 30 days of daily use.
func TestSessionCookieSlidesWithUse(t *testing.T) {
	r := newRig(t)
	r.login()
	start := time.Now()
	r.app.mu.Lock()
	r.app.now = func() time.Time { return start.Add(2 * seenSaveEvery) }
	r.app.mu.Unlock()
	resp, err := r.client.Get(r.srv.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var got *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == cookieName {
			got = c
		}
	}
	if got == nil || got.MaxAge != int(sessionMaxAge/time.Second) || !got.HttpOnly || got.SameSite != http.SameSiteStrictMode {
		t.Fatalf("after %v of use: cookie %+v, want it issued again with Max-Age %d", 2*seenSaveEvery, got, int(sessionMaxAge/time.Second))
	}
	// Not on every request: only once per seenSaveEvery.
	resp, err = r.client.Get(r.srv.URL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(resp.Cookies()) != 0 {
		t.Fatalf("cookie issued again on the next request: %v", resp.Cookies())
	}
}

// An IP that failed once and never came back stayed in loginFails for good.
func TestOldLoginFailuresAreSwept(t *testing.T) {
	r := newRig(t)
	if code, _ := r.post("/api/login", map[string]string{"user": "Demo", "password": "falsch"}); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	r.app.mu.Lock()
	r.app.loginFails["192.0.2.7"] = []time.Time{time.Now().Add(-2 * loginFailWindow)}
	r.app.mu.Unlock()
	r.client.Jar, _ = cookiejar.New(nil)
	r.login()
	r.app.mu.Lock()
	_, stale := r.app.loginFails["192.0.2.7"]
	r.app.mu.Unlock()
	if stale {
		t.Fatal("an old failure of another IP is still kept after a login")
	}
}

// A second draw that hit an open code was used without checking it again.
func TestPairCodesAreUniqueAmongOpenPairings(t *testing.T) {
	r := newRig(t)
	draws := []string{"111111", "111111", "111111", "222222"}
	r.app.mu.Lock()
	r.app.newPairCode = func() string { c := draws[0]; draws = draws[1:]; return c }
	r.app.mu.Unlock()
	tv := r.tvClient()
	_, first := tv.post("/api/tv/pair/start", map[string]any{})
	_, second := tv.post("/api/tv/pair/start", map[string]any{})
	if first["code"] != "111111" || second["code"] != "222222" {
		t.Fatalf("codes %v and %v, want 111111 and 222222", first["code"], second["code"])
	}
}

// pair/start needs no login; one client could hold all open pairings, and
// nobody else could pair a TV.
func TestPairStartIsLimitedPerClient(t *testing.T) {
	r := newRig(t)
	tv := r.tvClient()
	for i := range maxPairPerIP {
		if code, out := tv.post("/api/tv/pair/start", map[string]any{}); code != 200 {
			t.Fatalf("pairing %d: %d %v", i, code, out)
		}
	}
	if code, _ := tv.post("/api/tv/pair/start", map[string]any{}); code != 429 {
		t.Fatalf("pairing %d from the same client: %d, want 429", maxPairPerIP+1, code)
	}
}

// A handler that panicked under app.mu left it locked, and the rig's cleanup
// waited for it until go test's -timeout (600 s) instead of failing at once.
func TestRigCleanupGivesUpOnAHeldLock(t *testing.T) {
	var mu sync.Mutex
	mu.Lock()
	got := make(chan bool, 1)
	go func() { got <- lockWithin(&mu, 50*time.Millisecond) }()
	select {
	case ok := <-got:
		if ok {
			t.Fatal("lockWithin took a lock that is held")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lockWithin still waits for a held lock after 2 s")
	}
	mu.Unlock()
	if !lockWithin(&mu, 50*time.Millisecond) {
		t.Fatal("lockWithin did not take a free lock")
	}
}
