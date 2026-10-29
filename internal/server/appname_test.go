package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The app is called "Movies-Cast" wherever a person sees its name: the ZimaOS
// tile, the browser tab, the page heading, the home-screen icon, the TV page
// and every text that names the app (asked for 29.10.2026). Before, the tile said
// "Movies auf dem Fernseher" in German and "Movies Cast" in English, and the
// texts said "movies-cast-App". "Movies" alone stays IceWhale's app.
const appName = "Movies-Cast"

func TestAppNameIsMoviesCast(t *testing.T) {
	b, err := os.ReadFile("../../deploy/zimaos/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^  title:\n    en_us: (.*)\n    de_de: (.*)\n`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatal("compose: x-casaos title with en_us and de_de not found")
	}
	if m[1] != appName || m[2] != appName {
		t.Errorf("tile title en_us %q, de_de %q, want %q", m[1], m[2], appName)
	}

	var man struct {
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
	}
	mb, err := webFS.ReadFile("web/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mb, &man); err != nil {
		t.Fatal(err)
	}
	if man.Name != appName || man.ShortName != appName {
		t.Errorf("manifest name %q, short_name %q, want %q", man.Name, man.ShortName, appName)
	}

	files, _ := filepath.Glob("web/i18n/*.json")
	if len(files) < 18 {
		t.Fatalf("only %d language files", len(files))
	}
	for _, f := range files {
		var s map[string]string
		jb, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(jb, &s); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if s["title"] != appName {
			t.Errorf("%s: title %q, want %q", f, s["title"], appName)
		}
		for k, v := range s {
			if strings.Contains(v, "movies-cast") {
				t.Errorf("%s: %q names the app %q", f, k, v)
			}
		}
	}

	for _, page := range []string{"web/index.html", "web/tv/index.html"} {
		pb, err := webFS.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		p := string(pb)
		if !strings.Contains(p, "<title>"+appName+"</title>") {
			t.Errorf("%s: <title> is not %q", page, appName)
		}
		for _, h := range regexp.MustCompile(`<h1>(?:<img[^>]*>)?([^<]*)`).FindAllStringSubmatch(p, -1) {
			if h[1] != "" && h[1] != appName {
				t.Errorf("%s: heading %q, want %q", page, h[1], appName)
			}
		}
		if strings.Contains(p, "movies-cast-App") {
			t.Errorf("%s still says movies-cast-App", page)
		}
	}

	for msg, en := range errorsEn {
		if strings.Contains(msg+en, "movies-cast") {
			t.Errorf("server message %q / %q names the app in lower case", msg, en)
		}
	}
}
