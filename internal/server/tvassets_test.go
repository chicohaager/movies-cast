package server

import (
	"bytes"
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
	checkNone(t, "tv.js", string(js), map[string]string{
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
