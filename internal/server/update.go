package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Update notice (opt-in, UPDATE_CHECK=true): the status card names the newest
// release on GitHub. Off by default: it is a request from the NAS to GitHub.

const releasesURL = "https://api.github.com/repos/chicohaager/movies-cast/releases/latest"

type updateCheck struct {
	url   string
	every time.Duration
	hc    *http.Client

	mu     sync.Mutex
	at     time.Time
	latest string
}

// UpdateCheck turns the notice on (main, from UPDATE_CHECK).
func (s *Server) UpdateCheck() {
	s.update = &updateCheck{url: releasesURL, every: 24 * time.Hour, hc: &http.Client{Timeout: 5 * time.Second}}
}

// latest answers the newest release tag, asked at most once per day.
func (u *updateCheck) get(ctx context.Context) string {
	u.mu.Lock()
	if !u.at.IsZero() && time.Since(u.at) < u.every {
		l := u.latest
		u.mu.Unlock()
		return l
	}
	u.at = time.Now() // also after a failure: one try a day
	u.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.hc.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel) != nil {
		return ""
	}
	l := strings.TrimPrefix(rel.TagName, "v")
	u.mu.Lock()
	u.latest = l
	u.mu.Unlock()
	return l
}

// newer says whether release a (e.g. "0.27.1") is newer than b ("0.27.0" or
// "0.27.0+abc"); anything that is not N.N.N is not newer.
func newer(a, b string) bool {
	b, _, _ = strings.Cut(b, "+")
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	if len(pa) != 3 || len(pb) != 3 {
		return false
	}
	for i := range 3 {
		var x, y int
		if _, err := fmt.Sscan(pa[i], &x); err != nil {
			return false
		}
		if _, err := fmt.Sscan(pb[i], &y); err != nil {
			return false
		}
		if x != y {
			return x > y
		}
	}
	return false
}
