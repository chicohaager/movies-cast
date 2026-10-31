package server

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"movies-cast/internal/cast"
	"movies-cast/internal/dlna"
)

// slowFinder searches for a while and stamps its stats, like the real finder.
type slowFinder struct {
	fakeFinder
	mu    sync.Mutex
	delay time.Duration
}

func (f *slowFinder) Find(context.Context, bool) ([]dlna.Renderer, error) {
	time.Sleep(f.delay)
	f.mu.Lock()
	f.stats.At = time.Now()
	f.mu.Unlock()
	return f.rs, nil
}

func (f *slowFinder) LastSearch() dlna.SearchStats {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stats
}

// mDNS runs next to the DLNA search: one after the other added 2 s to every
// "Search" on the phone (7 s measured on .143, 2026-10-31).
func TestCastSearchRunsNextToTheDLNASearch(t *testing.T) {
	inner := &slowFinder{delay: 300 * time.Millisecond}
	cr := &castRenderers{inner: inner, ttl: time.Minute,
		discover: func(context.Context, time.Duration) ([]cast.Found, error) {
			time.Sleep(300 * time.Millisecond)
			return []cast.Found{{ID: "abc", Name: "Küche", Addr: "192.0.2.9:8009"}}, nil
		}}
	t0 := time.Now()
	rs, err := cr.Find(context.Background(), true)
	took := time.Since(t0)
	if err != nil || len(rs) != 1 {
		t.Fatalf("find: %v %+v", err, rs)
	}
	if took >= 550*time.Millisecond {
		t.Fatalf("search took %v, want both at once (about 300 ms)", took)
	}
	if st := cr.LastSearch(); st.Cast != 1 || st.CastMs < 300 {
		t.Fatalf("stats %+v, want 1 Cast device and its time", st)
	}
}

// A search that ran leaves one line in the log; an answer from the cache
// leaves none.
func TestTVSearchIsLogged(t *testing.T) {
	buf := &syncBuf{}
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	r := newRig(t)
	ff := &slowFinder{fakeFinder: fakeFinder{rs: []dlna.Renderer{lg},
		stats: dlna.SearchStats{Answers: 5, Renderers: 1, SSDPMs: 2000, DescribeMs: 4100}}}
	r.app.renderers = ff
	r.login()
	if code, _ := r.get("/api/renderers?refresh=1"); code != 200 {
		t.Fatalf("renderers: %d", code)
	}
	want := "tv search: 5 answers, 1 TVs, 0 Cast, "
	if n := strings.Count(buf.String(), want); n != 1 {
		t.Fatalf("%d lines %q in the log:\n%s", n, want, buf.String())
	}
	if !strings.Contains(buf.String(), "(ssdp 2.0 s, descriptions 4.1 s, cast 0.0 s)") {
		t.Fatalf("no breakdown in the log:\n%s", buf.String())
	}
	// Same stats stamp (cache): no new line.
	ff.delay = 0
	ff.mu.Lock()
	at := ff.stats.At
	ff.mu.Unlock()
	r.app.renderers = &fakeFinder{rs: []dlna.Renderer{lg}, stats: dlna.SearchStats{At: at}}
	r.get("/api/renderers")
	if n := strings.Count(buf.String(), "tv search:"); n != 1 {
		t.Fatalf("%d search lines after a cached answer, want 1:\n%s", n, buf.String())
	}
}
