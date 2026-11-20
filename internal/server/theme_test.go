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

// The phone page and the TV page are always dark in Raumklang's colours
// (owner, 2026-11-20): no colour-mode picker, no theme.js, no light block, and
// both pages and the manifest carry the same near-black and gold.
func TestPageIsAlwaysDark(t *testing.T) {
	r := newRig(t)
	if code, _ := r.get("/theme.js"); code != http.StatusNotFound {
		t.Errorf("/theme.js: %d (want 404, the picker is gone)", code)
	}
	read := func(name string) string {
		b, err := webFS.ReadFile("web/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	page := read("index.html")
	for _, gone := range []string{`id="theme"`, `theme.js`, `data-i18n="theme.`} {
		if strings.Contains(page, gone) {
			t.Errorf("index.html still has %s", gone)
		}
	}
	if !strings.Contains(page, `<meta name="theme-color" content="#0c0d0f">`) {
		t.Error("index.html: theme-color is not Raumklang's #0c0d0f")
	}
	css, tv := read("style.css"), read("tv/tv.css")
	for _, gone := range []string{"data-theme", "prefers-color-scheme", "color-scheme: light"} {
		if strings.Contains(css, gone) {
			t.Errorf("style.css still has %q", gone)
		}
	}
	root := regexp.MustCompile(`(?s)^:root \{(.*?)\n\}`).FindStringSubmatch(strings.TrimLeft(css, "\n"))
	if root == nil {
		t.Fatal("style.css does not start with a :root block")
	}
	for name, want := range map[string]string{"style.css": root[1], "tv/tv.css": tv} {
		for _, token := range []string{"--bg: #0c0d0f", "--text: #f3f1ec", "--accent: #dcbf8f"} {
			if !strings.Contains(want, token) {
				t.Errorf("%s lacks %s", name, token)
			}
		}
	}
	if !strings.Contains(root[1], "color-scheme: dark") {
		t.Error("style.css :root lacks color-scheme: dark")
	}
	if m := read("manifest.json"); !strings.Contains(m, `"theme_color": "#0c0d0f"`) || !strings.Contains(m, `"background_color": "#0c0d0f"`) {
		t.Errorf("manifest.json colours: %.200s", m)
	}
	for _, f := range []string{"de.json", "en.json", "zh_CN.json"} {
		if strings.Contains(read("i18n/"+f), `"theme.`) {
			t.Errorf("i18n/%s still has theme.* texts", f)
		}
	}
}

// The colour mode is no longer a setting of the account: POST /api/theme is
// gone, and a themes.json left by an earlier version is not read into /api/me.
func TestThemeSettingIsGone(t *testing.T) {
	r, f := newRigWithState(t)
	if err := os.WriteFile(filepath.Join(filepath.Dir(f), "themes.json"), []byte(`{"USERID0000":"light"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	other := r.restart(f)
	other.login()
	if code, out := other.post("/api/theme", map[string]string{"theme": "dark"}); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/theme: %d %v (want 404/405)", code, out)
	}
	if code, me := other.getJSON("/api/me"); code != 200 || me["theme"] != nil {
		t.Fatalf("/api/me: %d %v (want no theme)", code, me)
	}
}

// The language follows the account: /api/me says it,
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
