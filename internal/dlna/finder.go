package dlna

import (
	"context"
	"errors"
	"log"
	"net"
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
	search func(context.Context, time.Duration) ([]string, int, error)
	video  func(context.Context, Renderer) (bool, error)
	// allow filters locations that came from SSDP; Static is the operator's
	// own configuration and is not filtered.
	allow func(string) bool

	mu      sync.Mutex
	at      time.Time
	now     func() time.Time     // time.Now; a test clock
	seenAt  map[string]time.Time // renderer ID → when it last answered a search
	cached  []Renderer
	lastLoc map[string]string // host → last description URL SSDP gave for it
	conn    net.PacketConn    // SSDP socket, open for good when Port is fixed
	extra   []string          // hosts added in the app (SetExtraHosts)
	known   []string          // description URLs of TVs seen before (SetKnown)
	last    SearchStats
}

// SearchStats describes the last search, for the status page and for a
// hint that says why the list is empty.
type SearchStats struct {
	At        time.Time `json:"at"`
	Answers   int       `json:"answers"`   // SSDP datagrams received, from any device
	Renderers int       `json:"renderers"` // video renderers listed
	Err       string    `json:"error,omitempty"`
	Port      int       `json:"port"`     // local SSDP port, 0 = random
	PortHeld  bool      `json:"portHeld"` // the fixed port is bound now
}

// LastSearch returns the stats of the last search.
func (f *Finder) LastSearch() SearchStats {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.last
	st.Port, st.PortHeld = f.Port, f.conn != nil
	return st
}

// SetExtraHosts sets the hosts added in the app; they are searched like
// RENDERERS hosts. The next Find searches anew.
func (f *Finder) SetExtraHosts(hosts []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.extra = append([]string(nil), hosts...)
	f.at = time.Time{}
}

// SetKnown sets the description URLs of TVs seen before. A TV that misses
// the search is asked there directly: the Samsung UE50MU6179 answered its
// description URL in 0.2–0.5 s while a search a minute before had not found
// it (measured 2026-10-28). A known URL that does not answer is no error —
// that TV is off.
func (f *Finder) SetKnown(locs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.known = append([]string(nil), locs...)
}

// hosts is RENDERERS hosts plus app hosts. Caller holds f.mu.
func (f *Finder) hosts() []string {
	out := append([]string(nil), f.Hosts...)
	for _, h := range f.extra {
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
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
		now:     time.Now,
		seenAt:  map[string]time.Time{},
	}
	f.search = func(ctx context.Context, wait time.Duration) ([]string, int, error) {
		// caller holds f.mu
		if f.Port == 0 {
			return Search(ctx, wait, 0, f.hosts())
		}
		if f.conn == nil {
			c, err := ListenSSDP(ctx, f.Port)
			if err != nil {
				return nil, 0, err
			}
			f.conn = c
		}
		locs, n, err := SearchOn(f.conn, wait, f.hosts())
		if err != nil {
			f.conn.Close()
			f.conn = nil
		}
		return locs, n, err
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

// Open binds the fixed SSDP port right away, so the firewall sees the port
// before the first search (otherwise that search's answers can arrive before
// ZFW has opened it). No-op without a fixed port.
func (f *Finder) Open(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Port == 0 || f.conn != nil {
		return nil
	}
	c, err := ListenSSDP(ctx, f.Port)
	if err != nil {
		return err
	}
	f.conn = c
	return nil
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
	if !refresh && len(f.cached) > 0 && f.now().Sub(f.at) < f.TTL {
		return f.cached, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.Wait+15*time.Second)
	defer cancel()

	locs := append([]string(nil), f.Static...)
	var errs []error
	hosts := f.hosts()
	answers := 0
	if f.SSDP || len(hosts) > 0 {
		found, n, err := f.search(ctx, f.Wait)
		answers = n
		if err != nil {
			errs = append(errs, err)
		}
		answered := map[string]bool{}
		for _, loc := range found {
			h := hostOf(loc)
			entry := slices.Contains(hosts, h)
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
		for _, h := range hosts {
			if !answered[h] && f.lastLoc[h] != "" {
				locs = append(locs, f.lastLoc[h])
			}
		}
	}
	type result struct {
		r     Renderer
		ok    bool
		err   error
		known bool
	}
	seen := map[string]bool{}
	fromSearch := map[string]bool{}
	for _, loc := range locs {
		fromSearch[hostOf(loc)] = true
	}
	var knownLocs []string
	for _, loc := range f.known {
		if !fromSearch[hostOf(loc)] {
			knownLocs = append(knownLocs, loc)
		}
	}
	results := make(chan result)
	n := 0
	for i, loc := range append(locs, knownLocs...) {
		if seen[loc] {
			continue
		}
		seen[loc] = true
		n++
		known := i >= len(locs)
		go func(loc string) {
			r, err := FetchRenderer(ctx, f.HTTP, loc)
			if err != nil {
				results <- result{err: err, known: known}
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
		case res.err != nil && res.known: // off
		case res.err != nil:
			errs = append(errs, res.err)
		case res.ok:
			byID[res.r.ID] = res.r
			f.seenAt[res.r.ID] = f.now()
		}
	}
	// A TV that missed this one search stays listed until TTL after it last
	// answered. Not after the cache time: that moved on with every search,
	// and a TV switched off stayed listed as long as someone kept asking.
	for _, r := range f.cached {
		if _, ok := byID[r.ID]; !ok && f.now().Sub(f.seenAt[r.ID]) < f.TTL {
			byID[r.ID] = r
		}
	}
	for id := range f.seenAt {
		if _, ok := byID[id]; !ok {
			delete(f.seenAt, id)
		}
	}
	out := make([]Renderer, 0, len(byID))
	for _, r := range byID {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > 0 {
		f.cached, f.at = out, f.now()
	}
	err := errors.Join(errs...)
	f.last = SearchStats{At: time.Now(), Answers: answers, Renderers: len(out)}
	if err != nil {
		f.last.Err = err.Error()
	}
	return out, err
}
