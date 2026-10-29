package server

import (
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
