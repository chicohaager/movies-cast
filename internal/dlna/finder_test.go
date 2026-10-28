package dlna

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// device serves a captured description and GetProtocolInfo answer.
func fakeDevice(t *testing.T, desc, protocolInfo string) *httptest.Server {
	d, p := fixture(t, desc), fixture(t, protocolInfo)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.Header.Get("SOAPAction"), "GetProtocolInfo") {
			_, _ = w.Write(p)
			return
		}
		_, _ = w.Write(d)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSinkHasVideo(t *testing.T) {
	if !SinkHasVideo(fixture(t, "lg-protocolinfo.xml")) {
		t.Error("LG sink should list video")
	}
	if SinkHasVideo(fixture(t, "sonos-protocolinfo.xml")) {
		t.Error("Sonos sink should not list video")
	}
}

func TestParseSonosUsesRendererSubdeviceConnectionManager(t *testing.T) {
	r, err := ParseDescription("http://10.0.0.65:1400/xml/device_description.xml", fixture(t, "sonos-desc.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if r.ConnMgr != "http://10.0.0.65:1400/MediaRenderer/ConnectionManager/Control" {
		t.Errorf("ConnMgr = %q (must be the MediaRenderer's, not the MediaServer's)", r.ConnMgr)
	}
}

func TestFinderKeepsTVDropsSpeakerIgnoresCallerCancel(t *testing.T) {
	lgSrv := fakeDevice(t, "lg-oled55b9-desc.xml", "lg-protocolinfo.xml")
	sonos := fakeDevice(t, "sonos-desc.xml", "sonos-protocolinfo.xml")
	f := NewFinder(nil, true, 0, NewClient())
	f.search = func(context.Context, time.Duration) ([]string, int, error) {
		return []string{lgSrv.URL + "/", sonos.URL + "/xml/device_description.xml"}, 2, nil
	}
	f.allow = func(string) bool { return true } // httptest listens on loopback
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the browser went away before the search finished
	rs, err := f.Find(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Name != "[LG] webOS TV OLED55B97LA" {
		t.Fatalf("got %+v", rs)
	}
}

func TestFinderDoesNotCacheEmptyResult(t *testing.T) {
	lgSrv := fakeDevice(t, "lg-oled55b9-desc.xml", "lg-protocolinfo.xml")
	f := NewFinder(nil, true, 0, NewClient())
	answers := [][]string{nil, {lgSrv.URL + "/"}}
	f.allow = func(string) bool { return true }
	f.search = func(context.Context, time.Duration) ([]string, int, error) {
		a := answers[0]
		answers = answers[1:]
		return a, len(a), nil
	}
	if rs, _ := f.Find(context.Background(), false); len(rs) != 0 {
		t.Fatalf("first search: %+v", rs)
	}
	// No refresh flag: an empty first answer must not stick for the TTL.
	if rs, _ := f.Find(context.Background(), false); len(rs) != 1 {
		t.Fatalf("second search: %+v", rs)
	}
}

func TestLANLocation(t *testing.T) {
	own := []net.IP{net.ParseIP("10.0.0.143")}
	for loc, want := range map[string]bool{
		"http://10.0.0.252:1795/":                         true,
		"http://10.0.0.5:1400/xml/device_description.xml": true,
		"http://127.0.0.1:8080/":                          false, // loopback services of the NAS
		"http://10.0.0.143:5000/v2/":                      false, // the NAS itself
		"http://169.254.169.254/latest/":                  false, // link-local
		"http://8.8.8.8/":                                 false, // not LAN
		"https://10.0.0.252/":                             false,
		"http://tv.local:1795/":                           false, // names could resolve anywhere
		"file:///etc/passwd":                              false,
	} {
		if got := LANLocation(loc, own); got != want {
			t.Errorf("LANLocation(%q) = %v, want %v", loc, got, want)
		}
	}
}

func TestFinderDropsLocationsOutsideLAN(t *testing.T) {
	f := NewFinder(nil, true, 0, NewClient())
	f.search = func(context.Context, time.Duration) ([]string, int, error) {
		return []string{"http://127.0.0.1:1/"}, 1, nil
	}
	fetched := false
	f.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		fetched = true
		return nil, errors.New("must not be called")
	})}
	if rs, _ := f.Find(context.Background(), true); len(rs) != 0 || fetched {
		t.Fatalf("loopback location was fetched: %v %v", rs, fetched)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestControlURLMustStayOnDevice(t *testing.T) {
	desc := `<root><device><UDN>uuid:evil</UDN><serviceList><service>
<serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>http://127.0.0.1:5000/v2/</controlURL></service></serviceList></device></root>`
	if _, err := ParseDescription("http://10.0.0.50:1400/d.xml", []byte(desc)); err == nil {
		t.Fatal("AVTransport on another host was accepted")
	}
	r, err := ParseDescription("http://10.0.0.252:1795/", fixture(t, "lg-oled55b9-desc.xml"))
	if err != nil || r.ID != "uuid:e5f75f8e-cf7f-e018-da71-5df259164b42@10.0.0.252" {
		t.Fatalf("id %q err %v", r.ID, err)
	}
}

func TestSearchIgnoresAnswerFromOtherHost(t *testing.T) {
	tv, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tv.Close()
	old := multicastTarget
	multicastTarget = tv.LocalAddr().String() // the "group" is a fake TV on loopback
	defer func() { multicastTarget = old }()
	go func() {
		buf := make([]byte, 2048)
		_, from, err := tv.ReadFrom(buf)
		if err != nil {
			return
		}
		// Answers from 127.0.0.1 but names another host.
		_, _ = tv.WriteTo([]byte("HTTP/1.1 200 OK\r\nLOCATION: http://10.0.0.99:1795/\r\n\r\n"), from)
	}()
	locs, _, err := Search(context.Background(), 400*time.Millisecond, 0, nil)
	if err != nil || len(locs) != 0 {
		t.Fatalf("got %v %v", locs, err)
	}
}

// A host entry gets its URL from SSDP; when the TV misses a later search,
// the last URL stands in. Without SSDP=true, other answers are not listed.
func TestFinderHostEntryFollowsSSDPAndKeepsLastURL(t *testing.T) {
	lgSrv := fakeDevice(t, "lg-oled55b9-desc.xml", "lg-protocolinfo.xml")
	other := fakeDevice(t, "lg-oled55b9-desc.xml", "lg-protocolinfo.xml")
	otherURL := strings.Replace(other.URL, "127.0.0.1", "localhost", 1) + "/"
	f := NewFinder([]string{"127.0.0.1"}, false, 0, NewClient())
	if len(f.Hosts) != 1 || len(f.Static) != 0 {
		t.Fatalf("entries split as hosts=%v static=%v", f.Hosts, f.Static)
	}
	f.allow = func(string) bool { return true }
	f.TTL = 0 // no cache carry-over: only the remembered URL may keep the TV listed
	answers := [][]string{{lgSrv.URL + "/", otherURL}, nil}
	f.search = func(context.Context, time.Duration) ([]string, int, error) {
		a := answers[0]
		answers = answers[1:]
		return a, len(a), nil
	}
	rs, err := f.Find(context.Background(), true)
	if err != nil || len(rs) != 1 || rs[0].Host() != "127.0.0.1" {
		t.Fatalf("first search: %+v %v", rs, err)
	}
	rs, err = f.Find(context.Background(), true)
	if err != nil || len(rs) != 1 || rs[0].Host() != "127.0.0.1" {
		t.Fatalf("TV missed the search, last URL should stand in: %+v %v", rs, err)
	}
}

// The query leaves from the fixed port, so a firewall rule for that port lets
// the answer in; the answer to it is read.
func TestSearchSendsFromFixedPort(t *testing.T) {
	probe, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	tv, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tv.Close()
	old := multicastTarget
	multicastTarget = tv.LocalAddr().String()
	defer func() { multicastTarget = old }()
	fromPort := make(chan int, 1)
	go func() {
		buf := make([]byte, 2048)
		_, from, err := tv.ReadFrom(buf)
		if err != nil {
			return
		}
		fromPort <- from.(*net.UDPAddr).Port
		_, _ = tv.WriteTo([]byte("HTTP/1.1 200 OK\r\nLOCATION: http://127.0.0.1:1626/\r\n\r\n"), from)
	}()
	locs, n, err := Search(context.Background(), 400*time.Millisecond, port, nil)
	if n != 1 {
		t.Errorf("answers = %d, want 1", n)
	}
	if err != nil || len(locs) != 1 || locs[0] != "http://127.0.0.1:1626/" {
		t.Fatalf("got %v %v", locs, err)
	}
	if got := <-fromPort; got != port {
		t.Fatalf("query left from port %d, want %d", got, port)
	}
}

// With a fixed port the socket stays open between searches, so a firewall
// that opens a host port only while it is listened on keeps it open.
func TestFinderKeepsFixedPortOpen(t *testing.T) {
	probe, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	old := multicastTarget
	multicastTarget = "127.0.0.1:9" // discard; no answer needed
	defer func() { multicastTarget = old }()
	f := NewFinder(nil, true, port, NewClient())
	f.Wait = 100 * time.Millisecond
	if err := f.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c, err := net.ListenPacket("udp4", ":"+strconv.Itoa(port)); err == nil {
		c.Close()
		t.Fatalf("port %d free right after Open", port)
	}
	_, _ = f.Find(context.Background(), true)
	if c, err := net.ListenPacket("udp4", ":"+strconv.Itoa(port)); err == nil {
		c.Close()
		t.Fatalf("port %d was free after the search, want it held open", port)
	}
	if f.conn == nil {
		t.Fatal("no socket kept")
	}
	f.conn.Close()
}

// Hosts added in the app are searched like RENDERERS hosts; the stats count
// every answer, so zero answers can be told apart from "no TV among them".
func TestFinderExtraHostsAndStats(t *testing.T) {
	lgSrv := fakeDevice(t, "lg-oled55b9-desc.xml", "lg-protocolinfo.xml")
	other := fakeDevice(t, "lg-oled55b9-desc.xml", "lg-protocolinfo.xml")
	otherURL := strings.Replace(other.URL, "127.0.0.1", "localhost", 1) + "/"
	f := NewFinder(nil, false, 0, NewClient())
	f.allow = func(string) bool { return true }
	answers := [][]string{{lgSrv.URL + "/", otherURL}, nil}
	f.search = func(context.Context, time.Duration) ([]string, int, error) {
		a := answers[0]
		answers = answers[1:]
		return a, len(a), nil
	}
	if rs, _ := f.Find(context.Background(), true); len(rs) != 0 {
		t.Fatalf("no hosts, SSDP off: %+v", rs)
	}
	answers = [][]string{{lgSrv.URL + "/", otherURL}}
	f.SetExtraHosts([]string{"127.0.0.1"})
	rs, _ := f.Find(context.Background(), false) // SetExtraHosts forces a new search
	if len(rs) != 1 || rs[0].Host() != "127.0.0.1" {
		t.Fatalf("with app host: %+v", rs)
	}
	if st := f.LastSearch(); st.Answers != 2 || st.Renderers != 1 {
		t.Fatalf("stats %+v, want 2 answers, 1 renderer", st)
	}
}
