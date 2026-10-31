package movies

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// pagingServer serves n films; honour=false answers every request with the
// first page, like a server that ignores StartIndex.
func pagingServer(t *testing.T, n int, honour bool) (*Client, *Session, *int) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/Items") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		requests++
		start, _ := strconv.Atoi(r.URL.Query().Get("StartIndex"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("Limit"))
		if !honour {
			start = 0
		}
		if limit == 0 {
			limit = n // no Limit: the whole library in one answer
		}
		var items []map[string]any
		for i := start; i < n && i < start+limit; i++ {
			items = append(items, map[string]any{"Id": fmt.Sprintf("%032d", i), "Name": fmt.Sprintf("Film %d", i), "Type": "Movie"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": n})
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "test"), &Session{UserID: "U", Token: "t"}, &requests
}

func TestMoviesReadsEveryPage(t *testing.T) {
	c, s, requests := pagingServer(t, 450, true)
	ms, err := c.Movies(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 450 || ms[449].Name != "Film 449" {
		t.Fatalf("got %d films", len(ms))
	}
	if *requests != 3 {
		t.Errorf("%d requests for 450 films in pages of %d, want 3", *requests, pageSize)
	}
}

func TestMoviesStopsWhenPagingIsIgnored(t *testing.T) {
	c, s, requests := pagingServer(t, 450, false)
	ms, err := c.Movies(context.Background(), s)
	if err == nil {
		t.Fatal("no error for a server that ignores StartIndex")
	}
	if len(ms) != pageSize {
		t.Errorf("got %d films, want the %d of the first page, none twice", len(ms), pageSize)
	}
	if *requests > 2 {
		t.Errorf("%d requests, want it to stop after the repeated page", *requests)
	}
}
