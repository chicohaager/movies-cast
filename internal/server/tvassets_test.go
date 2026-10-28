package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The TV runs Chrome 53 (user agent measured on the LG, 2026-10-28). These
// constructs came later and would leave the TV with a blank screen.
func TestTVAssetsFitChrome53(t *testing.T) {
	js, err := webFS.ReadFile("web/tv/tv.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := webFS.ReadFile("web/tv/tv.css")
	if err != nil {
		t.Fatal(err)
	}
	i18n, err := webFS.ReadFile("web/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	js = append(append(js, '\n'), i18n...) // i18n.js runs on the TV as well
	checkNone(t, "tv.js+i18n.js", string(js), map[string]string{
		`\basync\s+function|\basync\s*\(|\bawait\s`: "async/await (Chrome 55)",
		`\?\.`:                     "optional chaining (Chrome 80)",
		`\?\?`:                     "nullish coalescing (Chrome 80)",
		`\.padStart\(|\.padEnd\(`:  "padStart/padEnd (Chrome 57)",
		`Object\.(entries|values)`: "Object.entries/values (Chrome 54)",
		`\{\s*\.\.\.|,\s*\.\.\.\w`: "object/array spread in literals (object spread: Chrome 60)",
		`\.flat\(|\.flatMap\(`:     "flat/flatMap (Chrome 69)",
	})
	checkNone(t, "tv.css", string(css), map[string]string{
		`display:\s*grid`: "CSS grid (Chrome 57)",
		`\bgap:`:          "flex gap (Chrome 84)",
		`aspect-ratio`:    "aspect-ratio (Chrome 88)",
		`\binset:`:        "inset (Chrome 87)",
	})
}

func checkNone(t *testing.T, name, src string, rules map[string]string) {
	t.Helper()
	for pat, why := range rules {
		re := regexp.MustCompile(pat)
		for i, line := range strings.Split(src, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") || strings.HasPrefix(strings.TrimSpace(line), "/*") {
				continue
			}
			if re.MatchString(line) {
				t.Errorf("%s:%d uses %s: %s", name, i+1, why, strings.TrimSpace(line))
			}
		}
	}
}

// web/logo.svg is a copy of deploy/webos/logo/logo.svg, the source of the TV
// app's icons; a change to one must reach the other.
func TestLogoServedIsTheIconSource(t *testing.T) {
	src, err := os.ReadFile("../../deploy/webos/logo/logo.svg")
	if err != nil {
		t.Fatal(err)
	}
	r := newRig(t)
	resp, err := http.Get(r.srv.URL + "/logo.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/svg+xml" || !bytes.Equal(body, src) {
		t.Fatalf("GET /logo.svg: %d %q, %d bytes, equal to source: %v", resp.StatusCode, resp.Header.Get("Content-Type"), len(body), bytes.Equal(body, src))
	}
}

// The phone page can be put on the home screen: the manifest's icons exist
// with the size they claim, and the page links manifest and touch icon.
func TestManifestIconsExist(t *testing.T) {
	raw, err := webFS.ReadFile("web/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Name  string
		Icons []struct{ Src, Sizes, Type string }
	}
	if err := json.Unmarshal(raw, &m); err != nil || m.Name == "" || len(m.Icons) < 2 {
		t.Fatalf("manifest: %v %+v", err, m)
	}
	for _, ic := range append(m.Icons, struct{ Src, Sizes, Type string }{"apple-touch-icon.png", "180x180", "image/png"}) {
		f, err := webFS.Open("web/" + ic.Src)
		if err != nil {
			t.Fatalf("%s: %v", ic.Src, err)
		}
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err != nil || fmt.Sprintf("%dx%d", cfg.Width, cfg.Height) != ic.Sizes {
			t.Errorf("%s: %v, %dx%d, manifest says %s", ic.Src, err, cfg.Width, cfg.Height, ic.Sizes)
		}
	}
	page, _ := webFS.ReadFile("web/index.html")
	for _, want := range []string{`rel="manifest" href="manifest.json"`, `rel="apple-touch-icon" href="apple-touch-icon.png"`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
}
