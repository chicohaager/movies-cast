package server

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"movies-cast/internal/movies"
)

// Over HTTPS the session cookie is __Host-mc_s with Secure (roadmap 21: no
// other app on the same NAS address receives it); over HTTP it stays mc_s,
// which a browser accepts there.
func TestSessionCookieOverHTTPSIsHostBound(t *testing.T) {
	fm := &libMovies{headStatus: http.StatusMethodNotAllowed}
	ms := httptest.NewServer(fm.handler())
	defer ms.Close()
	app := New(Config{}, movies.New(ms.URL+"/movies", "test"), &fakeTV{state: "NO_MEDIA_PRESENT"}, staticRenderers{lg})
	for _, tc := range []struct {
		name   string
		srv    *httptest.Server
		cookie string
		secure bool
	}{
		{"https", httptest.NewTLSServer(app.Handler()), "__Host-mc_s", true},
		{"http", httptest.NewServer(app.Handler()), "mc_s", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.srv.Close()
			jar, _ := cookiejar.New(nil)
			c := tc.srv.Client()
			c.Jar = jar
			c.Timeout = 5 * time.Second
			req, _ := http.NewRequest(http.MethodPost, tc.srv.URL+"/api/login", strings.NewReader(`{"user":"Demo","password":"x"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			var got *http.Cookie
			for _, ck := range resp.Cookies() {
				got = ck
			}
			if resp.StatusCode != 200 || got == nil || got.Name != tc.cookie || got.Secure != tc.secure || !got.HttpOnly || got.Path != "/" || got.Domain != "" {
				t.Fatalf("login: %d cookie %+v, want %s secure=%v", resp.StatusCode, got, tc.cookie, tc.secure)
			}
			// The session works with that cookie.
			r2, err := c.Get(tc.srv.URL + "/api/me")
			if err != nil {
				t.Fatal(err)
			}
			r2.Body.Close()
			if r2.StatusCode != 200 {
				t.Fatalf("/api/me with the cookie: %d", r2.StatusCode)
			}
		})
	}
}
