package server

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// The ZimaOS tile shows the movies-cast logo: the compose carries web/logo.svg
// as a data: URI (no CDN, no host address — the repo is private, a tester's
// NAS has its own address, and an http:// icon is blocked as mixed content on
// an HTTPS dashboard). Before, the tile showed Jellyfin's logo from a CDN.
func TestComposeIconIsTheLogo(t *testing.T) {
	logo, err := webFS.ReadFile("web/logo.svg")
	if err != nil {
		t.Fatal(err)
	}
	want := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(logo)
	b, err := os.ReadFile("../../deploy/zimaos/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(b)
	if n := strings.Count(compose, "icon: "+want+"\n"); n != 2 {
		t.Errorf("compose has the logo as icon %d times, want 2 (labels.icon and x-casaos.icon)", n)
	}
	if n := strings.Count(compose, "icon:"); n != 2 {
		t.Errorf("compose has %d icon entries, want 2", n)
	}
}

// The time zone comes from the host's /etc/localtime, read-only, not from
// TZ=$TZ: ZimaOS fills $TZ in on the first import only, and after an update
// in "Verwalten" the program ran in UTC (log 09:04:30 at 10:04 CET, .184,
// 29.10.2026). Go reads /etc/localtime when TZ is unset; a set but empty or
// unknown TZ would mean UTC, so TZ must not be there at all.
func TestComposeTimeZoneFromHost(t *testing.T) {
	b, err := os.ReadFile("../../deploy/zimaos/docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(b)
	for i, line := range strings.Split(compose, "\n") {
		if l := strings.TrimSpace(line); !strings.HasPrefix(l, "#") && (strings.Contains(l, "TZ=") || strings.Contains(l, "TZ:")) {
			t.Errorf("compose line %d still sets TZ: %s", i+1, l)
		}
	}
	want := "      - type: bind\n        source: /etc/localtime\n        target: /etc/localtime\n        read_only: true\n"
	if !strings.Contains(compose, want) {
		t.Errorf("compose does not bind /etc/localtime read-only:\n%s", want)
	}
}
