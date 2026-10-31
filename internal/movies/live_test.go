package movies

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLive runs the library calls against a real server, only when asked:
//
//	MC_LIVE_URL=http://127.0.0.1:8096 MC_LIVE_USER=tester MC_LIVE_PASSWORD_FILE=<file> go test -run TestLive -v ./internal/movies/
//
// It prints counts and types, never a token. Used 2026-10-31 against
// Jellyfin 10.11.0 (test media) and Movies 0.3.0 on the owner's box.
func TestLive(t *testing.T) {
	base := os.Getenv("MC_LIVE_URL")
	if base == "" {
		t.Skip("MC_LIVE_URL not set")
	}
	pw, err := os.ReadFile(os.Getenv("MC_LIVE_PASSWORD_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := New(base, "live-test")
	s, err := c.Login(ctx, os.Getenv("MC_LIVE_USER"), strings.TrimSpace(string(pw)), "live-test-device")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() {
		if err := c.Logout(context.Background(), s); err != nil {
			t.Errorf("logout: %v", err)
		}
	}()
	views, err := c.Views(ctx, s)
	t.Logf("views %v err=%v", views, err)
	list, err := c.Movies(ctx, s)
	if err != nil {
		t.Fatalf("movies: %v", err)
	}
	for _, m := range list {
		t.Logf("%s %q genres=%v added=%q rating=%q count=%d playable=%v", m.Type, m.Name, m.Genres, m.Added, m.Rating, m.Count, m.Playable())
		if m.Type == "Series" {
			eps, err := c.Children(ctx, s, m)
			if err != nil {
				t.Fatalf("episodes: %v", err)
			}
			for _, e := range eps {
				t.Logf("  S%02dE%02d %q runtime=%ds", e.Season, e.Episode, e.Name, e.Runtime)
			}
			if len(eps) > 1 {
				n, ok, err := c.Next(ctx, s, eps[0])
				t.Logf("  next after %q: %q ok=%v err=%v", eps[0].Name, n.Name, ok, err)
			}
		}
	}
	for _, kind := range []string{"MusicAlbum", "Photo"} {
		l, err := c.Library(ctx, s, kind)
		t.Logf("%s: %d err=%v", kind, len(l), err)
		for _, m := range l {
			if m.Type == "MusicAlbum" {
				tr, err := c.Children(ctx, s, m)
				t.Logf("  album %q: %d tracks err=%v", m.Name, len(tr), err)
				if len(tr) > 1 {
					one, err := c.Movie(ctx, s, tr[0].ID)
					n, ok, err2 := c.Next(ctx, s, one)
					t.Logf("  next after track %q (album %s): %q ok=%v err=%v %v", one.Name, one.AlbumID, n.Name, ok, err, err2)
				}
			}
		}
	}
}
