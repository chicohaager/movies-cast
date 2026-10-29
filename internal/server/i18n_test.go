package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every message the server sends to a page has an English text: the English
// page shows errorEn, and a missing entry would leave German there.
func TestEveryFailMessageHasEnglish(t *testing.T) {
	// Every fail( call, in every source file: tvs.go and convert.go were not
	// read, and a built message (Sprintf) has no entry to look up.
	call := regexp.MustCompile(`fail\(w, [^,]+, (.*)\)\s*$`)
	lit := regexp.MustCompile(`^"([^"]+)"(?:\+faultCode\(err\))?$`) // errorBody splits the suffix off
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			c := call.FindStringSubmatch(line)
			if c == nil {
				continue
			}
			n++
			m := lit.FindStringSubmatch(c[1])
			if m == nil {
				t.Errorf("%s:%d: message is not a plain string, no English can be looked up: %s", f, i+1, c[1])
				continue
			}
			if _, ok := errorsEn[m[1]]; !ok {
				t.Errorf("%s:%d: no English for %q", f, i+1, m[1])
			}
		}
	}
	if n < 45 { // 47 calls in 4 files on 2026-10-29; fewer means the pattern stopped matching
		t.Fatalf("found only %d fail() messages", n)
	}
	if got := english("Fernseher startet nicht (UPnP-Fehler 701)"); got != "the TV does not start (UPnP error 701)" {
		t.Errorf("suffix: %q", got)
	}
	if got := english("etwas Neues"); got != "etwas Neues" {
		t.Errorf("unknown text: %q", got)
	}
}

// zimaosLanguages is the set the ZimaOS UI ships (assets/<lang>-*.js on .143,
// 2026-10-28; en_GB folded into en). A language file outside it is a typo.
var zimaosLanguages = map[string]bool{
	"en": true, "de": true, "fr": true, "es": true, "it": true, "nl": true, "pl": true, "cs": true, "hu": true,
	"sv": true, "tr": true, "ru": true, "ja": true, "ko": true, "zh_CN": true, "zh_TW": true, "pt_BR": true, "pt_PT": true,
}

func langFiles(t *testing.T) map[string]map[string]string {
	t.Helper()
	files, err := fs.Glob(webFS, "web/i18n/*.json")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]string{}
	for _, f := range files {
		b, err := webFS.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var d map[string]string
		if err := json.Unmarshal(b, &d); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out[strings.TrimSuffix(path.Base(f), ".json")] = d
	}
	return out
}

var placeholder = regexp.MustCompile(`\{\d\}`)

func placeholders(s string) string {
	p := placeholder.FindAllString(s, -1)
	sort.Strings(p)
	return strings.Join(p, "")
}

// Every language has exactly English's keys, no empty text, the same
// placeholders per text, and is one ZimaOS ships.
func TestStringsComplete(t *testing.T) {
	langs := langFiles(t)
	en := langs["en"]
	if len(en) < 120 {
		t.Fatalf("en.json has only %d keys", len(en))
	}
	if len(langs) < 2 {
		t.Fatalf("only %d language files", len(langs))
	}
	for code, d := range langs {
		if !zimaosLanguages[code] {
			t.Errorf("%s.json: not a ZimaOS language", code)
		}
		for k, v := range en {
			tr, ok := d[k]
			switch {
			case !ok:
				t.Errorf("%s.json lacks %q", code, k)
			case strings.TrimSpace(tr) == "":
				t.Errorf("%s.json: %q is empty", code, k)
			case placeholders(tr) != placeholders(v):
				t.Errorf("%s.json: %q has placeholders %q, English %q", code, k, placeholders(tr), placeholders(v))
			}
		}
		for k := range d {
			if _, ok := en[k]; !ok {
				t.Errorf("%s.json has %q, which en.json does not", code, k)
			}
		}
	}
	// Every server message has its key, and de.json says what the server says.
	for de, e := range errorsEn {
		if en["srv:"+e] != e {
			t.Errorf("en.json: srv:%s missing or different", e)
		}
		if d := langs["de"]; d != nil && d["srv:"+e] != de {
			t.Errorf("de.json: srv:%s is %q, the server says %q", e, d["srv:"+e], de)
		}
	}
	// Keys the Go code sends.
	for _, f := range []string{"server.go", "tvs.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range regexp.MustCompile(`"((?:format|tvs|sfx|tv)\.[\w]+)"`).FindAllStringSubmatch(string(src), -1) {
			if strings.HasSuffix(m[1], ".json") {
				continue // file names (tvs.json)
			}
			if _, ok := en[m[1]]; !ok {
				t.Errorf("%s sends key %q, which en.json does not define", f, m[1])
			}
		}
	}
	// Keys the pages use.
	used := map[string]string{}
	for _, f := range []string{"web/index.html", "web/app.js", "web/tv/index.html", "web/tv/tv.js"} {
		b, err := webFS.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, re := range []string{`data-i18n="([\w.]+)"`, `[:;]([\w.]+)[";]`, `\bt\("([\w.]+)"`, `\btr\("([\w.]+)"`} {
			for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(string(b), -1) {
				if strings.Contains(m[1], ".") || en[m[1]] != "" {
					used[m[1]] = f
				}
			}
		}
	}
	for _, st := range []string{"PLAYING", "PAUSED_PLAYBACK", "STOPPED", "TRANSITIONING", "NO_MEDIA_PRESENT"} {
		used["st."+st] = "app.js (st. + state)"
	}
	if len(used) < 40 {
		t.Fatalf("only %d keys found in the pages", len(used))
	}
	for k, f := range used {
		if strings.HasSuffix(k, ".") || strings.HasPrefix(k, "data-") || strings.HasSuffix(k, ".js") || strings.HasSuffix(k, ".css") || strings.HasSuffix(k, ".svg") || strings.HasSuffix(k, ".json") {
			continue
		}
		if _, ok := en[k]; !ok {
			t.Errorf("%s uses %q, which en.json does not define", f, k)
		}
	}
}

// /strings.js carries every language as one ES5 statement.
func TestStringsJS(t *testing.T) {
	r := newRig(t)
	code, body := r.get("/strings.js")
	if code != 200 || !strings.HasPrefix(body, "var I18N_STRINGS = {") || !strings.Contains(body, `"de":{`) {
		t.Fatalf("%d %.80s", code, body)
	}
}

// A server error carries the key and the suffix of a TV error, so every
// language can show it; English no longer keeps the German "keine Antwort".
func TestErrorBodyKeys(t *testing.T) {
	b := errorBody("Fernseher startet nicht (keine Antwort)")
	if b["key"] != "srv:the TV does not start" || b["errorEn"] != "the TV does not start (no answer)" {
		t.Fatalf("%v", b)
	}
	if sfx, _ := b["suffix"].(map[string]any); sfx["key"] != "sfx.noAnswer" {
		t.Fatalf("suffix %v", b["suffix"])
	}
	b = errorBody("Fernseher startet nicht (UPnP-Fehler 701)")
	if sfx, _ := b["suffix"].(map[string]any); sfx["key"] != "sfx.upnp" || fmt.Sprint(sfx["args"]) != "[701]" {
		t.Fatalf("upnp suffix %v", b["suffix"])
	}
}
