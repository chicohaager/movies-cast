package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"movies-cast/internal/dlna"
)

// An invented, locally administered MAC address.
const testMAC = "02:00:00:00:00:01"

type wakeRec struct {
	mu    sync.Mutex
	calls []string // "mac host"
}

func (w *wakeRec) fn(_ context.Context, mac, host string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, mac+" "+host)
	return nil
}

func (w *wakeRec) n() int { w.mu.Lock(); defer w.mu.Unlock(); return len(w.calls) }

func wakeRig(t *testing.T) (*rig, string, *fakeFinder, *wakeRec) {
	r, f := newRigWithState(t)
	ff := &fakeFinder{rs: []dlna.Renderer{lg}}
	wr := &wakeRec{}
	r.app.renderers = ff
	r.app.macOf = func(h string) string {
		if h == lg.Host() {
			return testMAC
		}
		return ""
	}
	r.app.wake = wr.fn
	r.app.alive = func(context.Context, dlna.Renderer) bool { return true }
	r.app.wakeEvery = time.Millisecond
	r.login()
	return r, f, ff, wr
}

// A TV seen once with its MAC is still offered when it is off (it answers
// no search then), after a restart too; waking it sends the packet to its
// MAC and address and returns once the TV answers again.
func TestOffTVIsListedAndWoken(t *testing.T) {
	r, f, ff, wr := wakeRig(t)
	if code, body := r.get("/api/renderers"); code != 200 || strings.Contains(body, `"off"`) {
		t.Fatalf("TV on: %d %s", code, body)
	}
	ff.rs = nil // standby
	r2 := r.restart(f)
	r2.app.renderers, r2.app.wake, r2.app.wakeEvery = ff, wr.fn, time.Millisecond
	r2.app.macOf = func(string) string { return "" }
	carryCookie(t, r, r2)
	code, body := r2.get("/api/renderers")
	var list []rendererOut
	_ = json.Unmarshal([]byte(body), &list)
	if code != 200 || len(list) != 1 || list[0].ID != lg.ID || !list[0].Off {
		t.Fatalf("TV off: %d %s", code, body)
	}
	// The TV answers after the third packet.
	r2.app.alive = func(context.Context, dlna.Renderer) bool {
		if wr.n() >= 3 {
			ff.rs = []dlna.Renderer{lg}
			return true
		}
		return false
	}
	ff.rs = []dlna.Renderer{lg} // stale cache entry: listed, but the description does not answer
	code, out := r2.post("/api/tvs/wake", map[string]any{"id": lg.ID})
	if code != 200 || out["ok"] != true {
		t.Fatalf("wake: %d %v", code, out)
	}
	if wr.n() != 3 || wr.calls[0] != testMAC+" "+lg.Host() {
		t.Fatalf("packets: %v", wr.calls)
	}
}

func TestWakeGivesUpAndSaysWhy(t *testing.T) {
	r, _, ff, _ := wakeRig(t)
	r.get("/api/renderers")
	ff.rs = nil
	r.app.alive = func(context.Context, dlna.Renderer) bool { return false }
	r.app.wakeWait = 20 * time.Millisecond
	code, out := r.post("/api/tvs/wake", map[string]any{"id": lg.ID})
	if code != 504 || out["key"] != "wake.noAnswer" {
		t.Fatalf("no answer: %d %v", code, out)
	}
	if code, out := r.post("/api/tvs/wake", map[string]any{"id": "uuid:never-seen"}); code != 404 {
		t.Fatalf("unknown TV: %d %v", code, out)
	}
}

// A TV that cannot be reached (off) is not a format it failed to play: no
// "failed before" afterwards, and the answer offers to wake it.
func TestUnreachableTVIsNotAFailedFormat(t *testing.T) {
	r, _, _, _ := wakeRig(t)
	r.get("/api/renderers")
	r.tv.mu.Lock()
	r.tv.setErr = &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: no route to host")}
	r.tv.mu.Unlock()
	code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "01000000000000000000000000000001", "force": true})
	if code != 502 || out["wake"] != true || out["key"] != "srv:the TV is off or not reachable" {
		t.Fatalf("off: %d %v", code, out)
	}
	r.tv.mu.Lock()
	r.tv.setErr = nil
	r.tv.mu.Unlock()
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": "01000000000000000000000000000001", "force": false}); out["key"] == "format.failedBefore" {
		t.Fatalf("marked as failed format: %d %v", code, out)
	}
}

// Off TVs came from ranging over a map: with two of them the list on the
// phone changed its order from one call to the next.
func TestOffTVsKeepTheirOrder(t *testing.T) {
	r, _, ff, _ := wakeRig(t)
	ff.rs = nil
	r.login()
	r.app.mu.Lock()
	if r.app.prefs.Known == nil {
		r.app.prefs.Known = map[string]knownTV{}
	}
	for _, n := range []string{"Kitchen", "Bedroom", "Attic", "Living room", "Office"} {
		r.app.prefs.Known["uuid:"+n] = knownTV{Name: n, Host: "192.0.2.9", MAC: testMAC}
	}
	r.app.mu.Unlock()
	var first string
	for i := range 30 {
		code, body := r.get("/api/renderers")
		if code != 200 {
			t.Fatalf("list: %d %s", code, body)
		}
		if i == 0 {
			first = body
		} else if body != first {
			t.Fatalf("call %d gave another order:\n%s\nfirst:\n%s", i, body, first)
		}
	}
	if a, b := strings.Index(first, "Attic"), strings.Index(first, "Office"); a < 0 || b < a {
		t.Fatalf("off TVs not by name: %s", first)
	}
}
