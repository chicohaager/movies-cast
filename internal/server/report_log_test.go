package server

import (
	"bytes"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// A stopped report that reached Movies wrote no line, so after a play nobody
// could tell whether Movies got the resume point (2026-10-29, .143).
func TestStoppedReportIsLogged(t *testing.T) {
	buf := &syncBuf{}
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	r := newRig(t)
	r.login()
	if code, out := r.post("/api/play", map[string]any{"renderer": lg.ID, "movie": movieID}); code != 200 {
		t.Fatalf("play: %d %v", code, out)
	}
	r.app.mu.Lock()
	for _, p := range r.app.plays {
		p.lastPos = 103 * time.Second
	}
	r.app.mu.Unlock()
	if code, _ := r.post("/api/control", map[string]any{"renderer": lg.ID, "action": "stop"}); code != 200 {
		t.Fatal("stop failed")
	}
	waitFor(t, "the stopped report", func() bool { return strings.Contains(buf.String(), "report stopped Shelter at 0:01:43: ok") })
}

// syncBuf is a log sink the watchers write to while the test reads it.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
