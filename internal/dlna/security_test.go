package dlna

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// One host on the LAN answering a search thousands of times, each with
// another path, became thousands of description fetches per search.
func TestSearchTakesFewLocationsPerHost(t *testing.T) {
	tv, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tv.Close()
	old := multicastTarget
	multicastTarget = tv.LocalAddr().String()
	defer func() { multicastTarget = old }()
	go func() {
		buf := make([]byte, 2048)
		_, from, err := tv.ReadFrom(buf)
		if err != nil {
			return
		}
		for i := range 200 {
			_, _ = tv.WriteTo(fmt.Appendf(nil, "HTTP/1.1 200 OK\r\nLOCATION: http://127.0.0.1:1795/d%d.xml\r\n\r\n", i), from)
		}
	}()
	locs, answers, err := Search(context.Background(), 400*time.Millisecond, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) > maxLocationsPerHost {
		t.Fatalf("%d locations from one host (of %d answers), want at most %d", len(locs), answers, maxLocationsPerHost)
	}
}

// Every location was fetched at once, each read up to 1 MB of description
// and 1 MB of protocol info.
func TestFinderFetchesFewDescriptionsAtOnce(t *testing.T) {
	d, p := fixture(t, "lg-oled55b9-desc.xml"), fixture(t, "lg-protocolinfo.xml")
	var mu sync.Mutex
	open, most := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write(p)
			return
		}
		mu.Lock()
		open++
		most = max(most, open)
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		open--
		mu.Unlock()
		_, _ = w.Write([]byte(strings.Replace(string(d), "uuid:e5f75f8e", "uuid:"+strings.Trim(r.URL.Path, "/"), 1)))
	}))
	t.Cleanup(srv.Close)
	f := NewFinder(nil, true, 0, NewClient())
	f.allow = func(string) bool { return true }
	var locs []string
	for i := range 40 {
		locs = append(locs, fmt.Sprintf("%s/x%d", srv.URL, i))
	}
	f.search = func(context.Context, time.Duration) ([]string, int, error) { return locs, len(locs), nil }
	if _, err := f.Find(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if most > maxFetches {
		t.Fatalf("%d description fetches at once, want at most %d", most, maxFetches)
	}
}

// A device name goes into log lines; a newline in it forged a line.
func TestDescriptionDropsControlCharacters(t *testing.T) {
	d := strings.Replace(string(fixture(t, "lg-oled55b9-desc.xml")), "<friendlyName>", "<friendlyName>evil\n2026/10/29 movies-cast: fake&#13;\t ", 1)
	r, err := ParseDescription("http://192.0.2.5:1795/", []byte(d))
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(r.Name, "\n\r\t") {
		t.Fatalf("name keeps control characters: %q", r.Name)
	}
}
