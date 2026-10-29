package dlna

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Description captured from the LG OLED55B97LA at 10.0.0.252:1795.
func TestParseDescriptionLG(t *testing.T) {
	r, err := ParseDescription("http://10.0.0.252:1795/", fixture(t, "lg-oled55b9-desc.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "http://10.0.0.252:1795/AVTransport/e5f75f8e-cf7f-e018-da71-5df259164b42/control.xml"; r.AVTransport != want {
		t.Errorf("AVTransport = %q, want %q", r.AVTransport, want)
	}
	if want := "http://10.0.0.252:1795/RenderingControl/e5f75f8e-cf7f-e018-da71-5df259164b42/control.xml"; r.RenderingCtl != want {
		t.Errorf("RenderingCtl = %q, want %q", r.RenderingCtl, want)
	}
	if r.Name != "[LG] webOS TV OLED55B97LA" || r.Host() != "10.0.0.252" || r.ID != "uuid:e5f75f8e-cf7f-e018-da71-5df259164b42@10.0.0.252" {
		t.Errorf("got name=%q host=%q id=%q", r.Name, r.Host(), r.ID)
	}
}

func TestParseDescriptionRelativeWithURLBaseAndNested(t *testing.T) {
	desc := `<root><URLBase>http://10.0.0.9:8200/base/</URLBase><device><UDN>uuid:root</UDN><friendlyName>Box</friendlyName>
<deviceList><device><UDN>uuid:inner</UDN><friendlyName>Inner</friendlyName><serviceList><service>
<serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>ctl/av</controlURL></service></serviceList></device></deviceList></device></root>`
	r, err := ParseDescription("http://10.0.0.9:8200/desc.xml", []byte(desc))
	if err != nil {
		t.Fatal(err)
	}
	if r.AVTransport != "http://10.0.0.9:8200/base/ctl/av" || r.ID != "uuid:inner@10.0.0.9" {
		t.Errorf("got %+v", r)
	}
}

func TestParseDescriptionWithoutAVTransport(t *testing.T) {
	_, err := ParseDescription("http://x/", []byte(`<root><device><UDN>uuid:a</UDN></device></root>`))
	if err == nil {
		t.Fatal("want error for device without AVTransport")
	}
}

func TestParseStatusLGStopped(t *testing.T) {
	st := ParseStatus(fixture(t, "lg-transportinfo-stopped.xml"), fixture(t, "lg-positioninfo-stopped.xml"))
	if st.State != "STOPPED" || st.Duration != time.Hour+47*time.Minute+30*time.Second || st.Position != 0 {
		t.Errorf("got %+v", st)
	}
	// The TV keeps the last URI after Stop and tells anyone who asks.
	if !strings.Contains(st.TrackURI, "api_key=DUMMYTOKEN0000") {
		t.Errorf("TrackURI = %q", st.TrackURI)
	}
}

func TestParseFaultLGGetVolume(t *testing.T) {
	f := ParseFault(fixture(t, "lg-getvolume.xml"))
	if f.Code != 606 || f.Description != "Action not authorized" {
		t.Errorf("got %+v", f)
	}
}

func TestHMS(t *testing.T) {
	for _, c := range []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"01:47:30", time.Hour + 47*time.Minute + 30*time.Second, true},
		{"0:00:06", 6 * time.Second, true},
		{"00:00:01.500", 1500 * time.Millisecond, true},
		{"NOT_IMPLEMENTED", 0, false},
		{"", 0, false},
	} {
		got, err := ParseHMS(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("ParseHMS(%q) = %v, %v", c.in, got, err)
		}
	}
	if s := FormatHMS(2*time.Hour + 3*time.Minute + 4*time.Second + 900*time.Millisecond); s != "2:03:04" {
		t.Errorf("FormatHMS = %q", s)
	}
	if s := FormatHMS(-time.Second); s != "0:00:00" {
		t.Errorf("FormatHMS(neg) = %q", s)
	}
}

func TestDIDLEscapes(t *testing.T) {
	d := DIDL(Media{URL: "http://h/s/a.mkv?x=1&y=2", Title: `Tom & Jerry <"3">`})
	if !strings.Contains(d, "x=1&amp;y=2") || !strings.Contains(d, "Tom &amp; Jerry &lt;&#34;3&#34;&gt;") {
		t.Errorf("DIDL not escaped: %s", d)
	}
}

// The LG only seeks when the DIDL carries OP=01, size and duration (measured).
func TestDIDLMarksSeekable(t *testing.T) {
	d := DIDL(Media{URL: "http://h/s/a.mkv", Title: "x", Size: 10228471838, Duration: time.Hour + 47*time.Minute + 30*time.Second})
	for _, want := range []string{
		`protocolInfo="http-get:*:video/x-matroska:DLNA.ORG_OP=01;`,
		` size="10228471838"`,
		` duration="1:47:30.000"`,
	} {
		if !strings.Contains(d, want) {
			t.Errorf("missing %s in %s", want, d)
		}
	}
}

func TestSSDPLocation(t *testing.T) {
	resp := "HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=1800\r\nLocation: http://10.0.0.252:1795/\r\nST: urn:schemas-upnp-org:service:AVTransport:1\r\n\r\n"
	if got := ParseSSDPLocation([]byte(resp)); got != "http://10.0.0.252:1795/" {
		t.Errorf("got %q", got)
	}
	if got := ParseSSDPLocation([]byte("HTTP/1.1 200 OK\r\n\r\n")); got != "" {
		t.Errorf("got %q", got)
	}
}

// fakeRenderer records actions and can hang on one of them, like the LG on Seek.
func fakeRenderer(t *testing.T, hangOn string) (*httptest.Server, *[]string) {
	var actions []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := r.Header.Get("SOAPAction")
		body, _ := io.ReadAll(r.Body)
		actions = append(actions, a+" "+string(body))
		if strings.HasSuffix(strings.Trim(a, `"`), "#"+hangOn) {
			<-r.Context().Done()
			return
		}
		if strings.Contains(a, "GetVolume") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write(fixture(t, "lg-getvolume.xml"))
			return
		}
		_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:R/></s:Body></s:Envelope>`))
	}))
	t.Cleanup(srv.Close)
	return srv, &actions
}

func TestCallSendsActionAndEscapedArgs(t *testing.T) {
	srv, actions := fakeRenderer(t, "")
	c := NewClient()
	r := Renderer{AVTransport: srv.URL + "/av", RenderingCtl: srv.URL + "/rc"}
	if err := c.SetURI(context.Background(), r, Media{URL: "http://h/s/x.mkv?a=1&b=2", Title: "A&B"}); err != nil {
		t.Fatal(err)
	}
	got := (*actions)[0]
	if !strings.HasPrefix(got, `"urn:schemas-upnp-org:service:AVTransport:1#SetAVTransportURI"`) {
		t.Errorf("SOAPAction: %s", got)
	}
	// URL escaped once in CurrentURI, the DIDL escaped as a whole in CurrentURIMetaData.
	if !strings.Contains(got, "<CurrentURI>http://h/s/x.mkv?a=1&amp;b=2</CurrentURI>") || !strings.Contains(got, "a=1&amp;amp;b=2") {
		t.Errorf("escaping: %s", got)
	}
}

func TestVolumeFaultIsTyped(t *testing.T) {
	srv, _ := fakeRenderer(t, "")
	c := NewClient()
	_, err := c.Volume(context.Background(), Renderer{AVTransport: srv.URL, RenderingCtl: srv.URL})
	var f *Fault
	if !errors.As(err, &f) || f.Code != 606 || f.HTTPStatus != 500 {
		t.Fatalf("want 606 fault, got %v", err)
	}
}

func TestSeekThatNeverAnswersTimesOut(t *testing.T) {
	srv, _ := fakeRenderer(t, "Seek")
	c := NewClient()
	c.HTTP.Timeout = 300 * time.Millisecond
	start := time.Now()
	err := c.Seek(context.Background(), Renderer{AVTransport: srv.URL}, 10*time.Second)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("want timeout error within 2s, got %v after %v", err, time.Since(start))
	}
}

// Sink lists captured 2026-10-28 from the three TVs in the house.
func TestUnlistedRealTVs(t *testing.T) {
	sink := func(name string) string { return xmlField(fixture(t, name), "Sink") }
	cases := []struct {
		name, fixture, container, codec string
		want                            string
	}{
		{"LG lists matroska, no HEVC", "lg-protocolinfo.xml", "mkv", "hevc", "HEVC (H.265)"},
		{"LG with H.264", "lg-protocolinfo.xml", "mkv", "h264", ""},
		{"Samsung 2017 lists x-mkv and video/hevc", "sink-samsung-ue50mu6179.xml", "mkv", "hevc", ""},
		{"Samsung 2012 lists x-mkv, no HEVC", "sink-samsung-ue32eh5300.xml", "mkv", "hevc", "HEVC (H.265)"},
		{"Samsung 2012 with H.264", "sink-samsung-ue32eh5300.xml", "mkv", "h264", ""},
		{"Samsung 2012 mp4", "sink-samsung-ue32eh5300.xml", "mp4", "h264", ""},
		{"Sonos has no video container", "sonos-protocolinfo.xml", "mkv", "h264", "MKV"},
		{"unknown container is not judged", "sonos-protocolinfo.xml", "xyz", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(Unlisted(sink(c.fixture), c.container, c.codec), ", "); got != c.want {
				t.Errorf("Unlisted = %q, want %q", got, c.want)
			}
		})
	}
	if got := Unlisted("http-get:*:*:*", "mkv", "hevc"); len(got) != 0 {
		t.Errorf("wildcard sink: Unlisted = %v, want nothing", got)
	}
}

// Play may take longer than other calls (the LG opens the file first), and
// nothing else may cut it shorter: 0.18 had a 6 s client timeout that cut
// Play at 6.2 s on the LG although send gave it 30 s.
func TestPlayGetsItsOwnLongerTimeout(t *testing.T) {
	oldSOAP, oldPlay := soapTimeout, playTimeout
	soapTimeout, playTimeout = 50*time.Millisecond, 2*time.Second
	t.Cleanup(func() { soapTimeout, playTimeout = oldSOAP, oldPlay })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond) // longer than soapTimeout, shorter than playTimeout
		_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:R/></s:Body></s:Envelope>`))
	}))
	t.Cleanup(srv.Close)
	c := NewClient()
	r := Renderer{AVTransport: srv.URL + "/av"}
	if err := c.Play(context.Background(), r); err != nil {
		t.Fatalf("Play after 300 ms: %v", err)
	}
	if err := c.Pause(context.Background(), r); err == nil {
		t.Fatal("Pause after 300 ms with a 50 ms limit: no error")
	}
	if NewClient().HTTP.Timeout != 0 {
		t.Fatal("NewClient sets an overall timeout; it would cut Play")
	}
}
