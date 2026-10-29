package server

import (
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The colour mode is chosen on the phone (System, Light, Dark): theme.js sets
// data-theme on <html> before style.css paints, and style.css has the dark
// colours twice — for the system mode and for a chosen "dark" — with the same
// values, and lets a chosen "light" win over a dark system.
func TestThemeChosenOnThePhone(t *testing.T) {
	r := newRig(t)
	code, js := r.get("/theme.js")
	if code != 200 || !strings.Contains(js, `"moviescast.theme"`) || !strings.Contains(js, "data-theme") {
		t.Fatalf("/theme.js: %d %.120s", code, js)
	}
	checkNone(t, "theme.js", js, map[string]string{`=>`: "arrow function", `\blet\s|\bconst\s`: "let/const"})

	html, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	jsAt, cssAt := strings.Index(page, `src="theme.js"`), strings.Index(page, `href="style.css"`)
	if jsAt < 0 || cssAt < 0 || jsAt > cssAt {
		t.Errorf("index.html must load theme.js before style.css (theme.js at %d, style.css at %d)", jsAt, cssAt)
	}
	if !strings.Contains(page, `id="theme"`) {
		t.Error(`index.html has no picker id="theme"`)
	}

	b, err := webFS.ReadFile("web/style.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	block := func(re string) string {
		m := regexp.MustCompile(re).FindStringSubmatch(css)
		if m == nil {
			t.Fatalf("style.css lacks %s", re)
		}
		return strings.Join(strings.Fields(m[1]), " ")
	}
	system := block(`(?s)@media \(prefers-color-scheme: dark\) \{\s*:root:not\(\[data-theme="light"\]\) \{(.*?)\}`)
	chosen := block(`(?s)\n:root\[data-theme="dark"\] \{(.*?)\}`)
	if system != chosen {
		t.Errorf("dark colours differ:\nsystem %s\nchosen %s", system, chosen)
	}
	if !strings.Contains(chosen, "--bg:") || !strings.Contains(chosen, "color-scheme: dark") {
		t.Errorf("dark block incomplete: %s", chosen)
	}
}

// The mode belongs to the account, not to the address: the phone reaches the
// NAS by its LAN address and by its Tailscale address, and each is its own
// localStorage (seen 2026-10-29). /api/me says the stored mode, POST
// /api/theme sets it, and it survives a restart in themes.json (0600).
func TestThemeFollowsTheAccount(t *testing.T) {
	r, f := newRigWithState(t)
	r.login()
	if code, me := r.getJSON("/api/me"); code != 200 || me["theme"] != nil {
		t.Fatalf("fresh account: %d %v (want no theme)", code, me)
	}
	if code, out := r.post("/api/theme", map[string]string{"theme": "blue"}); code != 400 {
		t.Fatalf("theme blue: %d %v", code, out)
	}
	if code, out := r.post("/api/theme", map[string]string{"theme": "dark"}); code != 200 {
		t.Fatalf("theme dark: %d %v", code, out)
	}

	// Another device of the same account, after a restart.
	other := r.restart(f)
	other.login()
	if code, me := other.getJSON("/api/me"); code != 200 || me["theme"] != "dark" {
		t.Fatalf("other device after restart: %d %v", code, me)
	}
	st, err := os.Stat(filepath.Join(filepath.Dir(f), "themes.json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("themes.json: %v %v", st, err)
	}
	// Another account keeps its own mode.
	anna := r.restart(f)
	jar, _ := cookiejar.New(nil) // restart shares the jar, and cookies are per host, not per port
	anna.client = &http.Client{Jar: jar}
	if code, out := anna.post("/api/login", map[string]string{"user": "Anna", "password": "richtig"}); code != 200 {
		t.Fatalf("login Anna: %d %v", code, out)
	}
	if _, me := anna.getJSON("/api/me"); me["name"] != "Anna" || me["theme"] != nil {
		t.Fatalf("Anna sees %v (want her own, unset mode)", me)
	}
	// "" is a choice too (follow the system), not a missing one.
	if code, _ := other.post("/api/theme", map[string]string{"theme": ""}); code != 200 {
		t.Fatalf("theme system: %d", code)
	}
	if _, me := other.getJSON("/api/me"); me["theme"] != "" {
		t.Fatalf("after System: %v", me)
	}
	// Without a login nothing is stored.
	anon := newRig(t)
	if code, _ := anon.post("/api/theme", map[string]string{"theme": "dark"}); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
}

// The language follows the account like the colour mode: /api/me says it,
// POST /api/lang sets one of the shipped languages, langs.json keeps it.
func TestLangFollowsTheAccount(t *testing.T) {
	r, f := newRigWithState(t)
	r.login()
	if _, me := r.getJSON("/api/me"); me["lang"] != nil {
		t.Fatalf("fresh account: %v (want no lang)", me)
	}
	for _, bad := range []string{"", "xx", "../en", "en.json", "EN", "de-DE"} {
		if code, out := r.post("/api/lang", map[string]string{"lang": bad}); code != 400 {
			t.Errorf("lang %q: %d %v (want 400)", bad, code, out)
		}
	}
	for _, good := range []string{"pt_BR", "fr"} {
		if code, out := r.post("/api/lang", map[string]string{"lang": good}); code != 200 {
			t.Fatalf("lang %s: %d %v", good, code, out)
		}
	}
	other := r.restart(f)
	other.login()
	if _, me := other.getJSON("/api/me"); me["lang"] != "fr" {
		t.Fatalf("other device after restart: %v", me)
	}
	st, err := os.Stat(filepath.Join(filepath.Dir(f), "langs.json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("langs.json: %v %v", st, err)
	}
	anna := r.restart(f)
	jar, _ := cookiejar.New(nil)
	anna.client = &http.Client{Jar: jar}
	if code, _ := anna.post("/api/login", map[string]string{"user": "Anna", "password": "richtig"}); code != 200 {
		t.Fatal("login Anna")
	}
	if _, me := anna.getJSON("/api/me"); me["lang"] != nil {
		t.Fatalf("Anna sees %v (want her own, unset language)", me)
	}
	anon := newRig(t)
	if code, _ := anon.post("/api/lang", map[string]string{"lang": "fr"}); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
}
