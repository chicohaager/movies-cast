package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewerRelease(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.27.1", "0.27.0", true}, {"0.28.0", "0.27.9+abc", true}, {"1.0.0", "0.99.99", true},
		{"0.27.0", "0.27.0", false}, {"0.26.9", "0.27.0", false}, {"0.27.0", "dev", false}, {"x", "0.1.0", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestUpdateCheckAsksOncePerDay(t *testing.T) {
	asked := 0
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		_, _ = io.WriteString(w, `{"tag_name":"v0.28.0"}`)
	}))
	defer gh.Close()
	u := &updateCheck{url: gh.URL, every: time.Hour, hc: gh.Client()}
	for i := 0; i < 3; i++ {
		if l := u.get(context.Background()); l != "0.28.0" {
			t.Fatalf("latest %q", l)
		}
	}
	if asked != 1 {
		t.Fatalf("GitHub asked %d times, want once", asked)
	}
}
