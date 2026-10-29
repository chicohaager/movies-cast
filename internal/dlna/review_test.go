package dlna

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A TV that missed a search was copied over from the cache, and the cache
// time then moved to now: switched off, it stayed listed as long as someone
// asked within the TTL. Each TV now ages out TTL after it last answered.
func TestFinderDropsATVThatStoppedAnswering(t *testing.T) {
	lgSrv := fakeDevice(t, "lg-oled55b9-desc.xml", "lg-protocolinfo.xml")
	d := strings.Replace(string(fixture(t, "lg-oled55b9-desc.xml")), "uuid:e5f75f8e", "uuid:0ff0ff8e", 1)
	p := fixture(t, "lg-protocolinfo.xml")
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write(p)
			return
		}
		_, _ = w.Write([]byte(d))
	}))
	t.Cleanup(other.Close)
	f := NewFinder(nil, true, 0, NewClient())
	f.allow = func(string) bool { return true }
	clock := time.Now()
	f.now = func() time.Time { return clock }
	answers := []string{lgSrv.URL + "/", other.URL + "/"}
	f.search = func(context.Context, time.Duration) ([]string, int, error) { return answers, len(answers), nil }
	if rs, err := f.Find(context.Background(), true); err != nil || len(rs) != 2 {
		t.Fatalf("both on: %+v %v", rs, err)
	}
	answers = answers[:1] // the second TV is switched off
	for range 6 {
		clock = clock.Add(2 * time.Minute)
		if _, err := f.Find(context.Background(), true); err != nil {
			t.Fatal(err)
		}
	}
	rs, _ := f.Find(context.Background(), true)
	if len(rs) != 1 {
		t.Fatalf("12 min after the second TV stopped answering (TTL %v): still %d listed", f.TTL, len(rs))
	}
}

// The SOAP client followed redirects: a device could send our POST on to
// any address, the NAS's own loopback services included.
func TestSOAPDoesNotFollowRedirects(t *testing.T) {
	var hit atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit.Add(1) }))
	t.Cleanup(target.Close)
	dev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/elsewhere", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(dev.Close)
	r := Renderer{Location: dev.URL + "/", AVTransport: dev.URL + "/AVTransport/control.xml"}
	if err := NewClient().Stop(context.Background(), r); err == nil {
		t.Error("Stop answered with a redirect counted as done")
	}
	if n := hit.Load(); n != 0 {
		t.Fatalf("redirect followed: %d request(s) reached the other server", n)
	}
}

// ParseFloat took "NaN", "Inf" and "1e9"; the UPnP syntax H+:MM:SS[.F+ | .F0/F1]
// allows only digits.
func TestHMSOnlyDigits(t *testing.T) {
	for _, c := range []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"0:00:NaN", 0, false},
		{"0:00:Inf", 0, false},
		{"0:00:1e9", 0, false},
		{"0:00:-5", 0, false},
		{"0:00:05.1/4", 5*time.Second + 250*time.Millisecond, true},
		{"0:00:05.1/0", 0, false},
		{"0:01:02.25", time.Minute + 2*time.Second + 250*time.Millisecond, true},
	} {
		got, err := ParseHMS(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("ParseHMS(%q) = %v, %v; want %v ok=%v", c.in, got, err, c.want, c.ok)
		}
	}
}
