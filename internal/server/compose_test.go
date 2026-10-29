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
