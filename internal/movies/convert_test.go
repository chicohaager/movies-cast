package movies

import (
	"strings"
	"testing"
	"time"
)

func TestParseMediaPlaylist(t *testing.T) {
	txt := "#EXTM3U\n#EXT-X-TARGETDURATION:5\n#EXTINF:5.000000, nodesc\nhls/0.ts?a=1\n#EXTINF:2.5,\nhls/1.ts\n#EXT-X-ENDLIST\n"
	segs, err := parseMediaPlaylist(txt, func(r string) (string, error) { return "http://h/" + r, nil })
	if err != nil || len(segs) != 2 || segs[0].URL != "http://h/hls/0.ts?a=1" || segs[1].Duration != 2500*time.Millisecond {
		t.Fatalf("%v %+v", err, segs)
	}
	cv := Conversion{Segments: segs}
	if i, at := cv.At(6 * time.Second); i != 1 || at != 5*time.Second {
		t.Fatalf("At(6s) = %d %v", i, at)
	}
	if i, at := cv.At(time.Hour); i != 1 || at != 5*time.Second {
		t.Fatalf("At past the end = %d %v", i, at)
	}
	for _, bad := range []string{"#EXTM3U\n", "#EXTM3U\nhls/0.ts\n", "#EXTINF:x,\nhls/0.ts\n"} {
		if _, err := parseMediaPlaylist(bad, func(r string) (string, error) { return r, nil }); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// The token rides along with every playlist and segment URL, so they must
// stay on the Movies server.
func TestConversionURLStaysOnMovies(t *testing.T) {
	c := New("http://192.0.2.40/movies", "t")
	if u, err := c.absolute("/movies/Videos/x/master.m3u8?api_key=k"); err != nil || u != "http://192.0.2.40/movies/Videos/x/master.m3u8?api_key=k" {
		t.Fatalf("%q %v", u, err)
	}
	for _, other := range []string{"http://evil.example/x.m3u8", "//evil.example/x", "https://192.0.2.40/movies/x"} {
		if _, err := c.absolute(other); err == nil || !strings.Contains(err.Error(), "another server") {
			t.Errorf("%s: %v", other, err)
		}
	}
}
