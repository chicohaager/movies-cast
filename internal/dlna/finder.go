package dlna

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Finder caches renderers found by SSDP plus fixed entries (for networks
// where multicast does not reach the container). An entry is a description
// URL or just a host: the LG moved its description from port 1795 to 1626
// between two starts (measured 2026-10-28), so a host is asked by SSDP
// unicast for its current URL, and the last answer is kept for a TV that
// misses a search. Only renderers that accept video are kept; Sonos speakers
// answer SSDP for AVTransport too (measured: 5 on this LAN).
type Finder struct {
	Static []string // description URLs
	Hosts  []string // hosts whose URL SSDP finds
	SSDP   bool
	Port   int // local SSDP port; fixed so a firewall can allow the answers
	Wait   time.Duration
	TTL    time.Duration
	HTTP   *http.Client
	search func(context.Context, time.Duration) ([]string, error)
	video  func(context.Context, Renderer) (bool, error)
	// allow filters locations that came from SSDP; Static is the operator's
	// own configuration and is not filtered.
	allow func(string) bool

	mu      sync.Mutex
	at      time.Time
	cached  []Renderer
	lastLoc map[string]string // host → last description URL SSDP gave for it
}

// NewFinder splits entries into description URLs (with "://") and hosts.
func NewFinder(entries []string, ssdp bool, port int, c *Client) *Finder {
	var static, hosts []string
	for _, e := range entries {
		if strings.Contains(e, "://") {
			static = append(static, e)
		} else {
			hosts = append(hosts, e)
		}
	}
	f := &Finder{
		Static: static, Hosts: hosts, SSDP: ssdp, Port: port, Wait: 2 * time.Second, TTL: 5 * time.Minute,
		HTTP: &http.Client{
			Timeout:       4 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		video:   c.PlaysVideo,
		allow:   func(loc string) bool { return LANLocation(loc, OwnIPs()) },
		lastLoc: map[string]string{},
	}
	f.search = func(ctx context.Context, wait time.Duration) ([]string, error) {
		return Search(ctx, wait, f.Port, f.Hosts)
	}
	return f
}

func hostOf(loc string) string {
	u, err := url.Parse(loc)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Find returns the cached list, or searches when it is stale or refresh is set.
//
// The search runs on its own context: the result is shared by every client,
// so one browser closing its tab must not cut the search short (that cached
// an empty list for five minutes, found in the E2E run 2026-10-28). An empty
// result is never cached.
func (f *Finder) Find(_ context.Context, refresh bool) ([]Renderer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !refresh && len(f.cached) > 0 && time.Since(f.at) < f.TTL {
		return f.cached, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.Wait+15*time.Second)
	defer cancel()

	locs := append([]string(nil), f.Static...)
	var errs []error
	if f.SSDP || len(f.Hosts) > 0 {
		found, err := f.search(ctx, f.Wait)
		if err != nil {
			errs = append(errs, err)
		}
		answered := map[string]bool{}
		for _, loc := range found {
			h := hostOf(loc)
			entry := slices.Contains(f.Hosts, h)
			if !f.SSDP && !entry {
				continue
			}
			if !f.allow(loc) {
				log.Printf("ssdp: ignoring location %q (not a private LAN address of another host)", loc)
				continue
			}
			locs = append(locs, loc)
			if entry {
				f.lastLoc[h], answered[h] = loc, true
			}
		}
		for _, h := range f.Hosts {
			if !answered[h] && f.lastLoc[h] != "" {
				locs = append(locs, f.lastLoc[h])
			}
		}
	}
	type result struct {
		r   Renderer
		ok  bool
		err error
	}
	seen := map[string]bool{}
	results := make(chan result)
	n := 0
	for _, loc := range locs {
		if seen[loc] {
			continue
		}
		seen[loc] = true
		n++
		go func(loc string) {
			r, err := FetchRenderer(ctx, f.HTTP, loc)
			if err != nil {
				results <- result{err: err}
				return
			}
			video, verr := f.video(ctx, r)
			if verr != nil {
				// Unknown is kept: hiding a TV because one query failed is worse
				// than listing a speaker.
				log.Printf("renderer %s (%s): protocol info: %v", r.Name, loc, verr)
				video = true
			}
			results <- result{r: r, ok: video}
		}(loc)
	}
	byID := map[string]Renderer{}
	for range n {
		res := <-results
		switch {
		case res.err != nil:
			errs = append(errs, res.err)
		case res.ok:
			byID[res.r.ID] = res.r
		}
	}
	// A TV that missed this one search stays listed until its cache entry ages out.
	if time.Since(f.at) < f.TTL {
		for _, r := range f.cached {
			if _, ok := byID[r.ID]; !ok {
				byID[r.ID] = r
			}
		}
	}
	out := make([]Renderer, 0, len(byID))
	for _, r := range byID {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > 0 {
		f.cached, f.at = out, time.Now()
	}
	return out, errors.Join(errs...)
}
