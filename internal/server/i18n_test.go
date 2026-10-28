package server

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Every message the server sends to a page has an English text: the English
// page shows errorEn, and a missing entry would leave German there.
func TestEveryFailMessageHasEnglish(t *testing.T) {
	re := regexp.MustCompile(`fail\(w, [^,]+, "([^"]+)"`)
	n := 0
	for _, f := range []string{"server.go", "tv.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			n++
			if _, ok := errorsEn[m[1]]; !ok {
				t.Errorf("%s: no English for %q", f, m[1])
			}
		}
	}
	if n < 30 { // 33 calls on 2026-10-28; fewer means the pattern stopped matching
		t.Fatalf("found only %d fail() messages", n)
	}
	if got := english("Fernseher startet nicht (UPnP-Fehler 701)"); got != "the TV does not start (UPnP error 701)" {
		t.Errorf("suffix: %q", got)
	}
	if got := english("etwas Neues"); got != "etwas Neues" {
		t.Errorf("unknown text: %q", got)
	}
}

// Both languages have the same keys, and every key the pages use exists.
func TestI18nKeysComplete(t *testing.T) {
	js, err := webFS.ReadFile("web/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(js)
	block := func(name string) map[string]bool {
		i := strings.Index(src, "var "+name+" = {")
		j := strings.Index(src[i:], "};")
		keys := map[string]bool{}
		for _, m := range regexp.MustCompile(`"([\w.]+)":`).FindAllStringSubmatch(src[i:i+j], -1) {
			keys[m[1]] = true
		}
		return keys
	}
	de, en := block("DE"), block("EN")
	if len(de) < 40 {
		t.Fatalf("DE has only %d keys", len(de))
	}
	for k := range de {
		if !en[k] {
			t.Errorf("EN lacks %q", k)
		}
	}
	for k := range en {
		if !de[k] {
			t.Errorf("DE lacks %q", k)
		}
	}
	used := map[string]string{}
	for _, f := range []string{"web/index.html", "web/app.js", "web/tv/index.html", "web/tv/tv.js"} {
		b, err := webFS.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, re := range []string{`data-i18n="([\w.]+)"`, `[:;]([\w.]+)[";]`, `\bt\("([\w.]+)"`, `\btr\("([\w.]+)"`} {
			for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(string(b), -1) {
				if strings.Contains(m[1], ".") || de[m[1]] {
					used[m[1]] = f
				}
			}
		}
	}
	for _, s := range []string{"PLAYING", "PAUSED_PLAYBACK", "STOPPED", "TRANSITIONING", "NO_MEDIA_PRESENT"} {
		used["st."+s] = "app.js (st. + state)"
	}
	if len(used) < 40 {
		t.Fatalf("only %d keys found in the pages", len(used))
	}
	for k, f := range used {
		if strings.HasSuffix(k, ".") || strings.HasPrefix(k, "data-") || strings.HasSuffix(k, ".js") || strings.HasSuffix(k, ".css") || strings.HasSuffix(k, ".svg") {
			continue
		}
		if !de[k] {
			t.Errorf("%s uses %q, which i18n.js does not define", f, k)
		}
	}
}
