// Package dlna finds UPnP MediaRenderers on the LAN and drives their
// AVTransport and RenderingControl services.
//
// Measured on an LG OLED55B97LA (webOS 4.x, 2026-10-28), and the reason for
// the defensive parts below:
//   - Seek while STOPPED gets no reply at all; every call needs a hard timeout.
//   - GetVolume answers UPnP error 606 "Action not authorized" while nothing
//     plays; while a film plays (2026-10-29) it answers CurrentVolume 0, as
//     GetMute answers 0. Volume is optional.
//   - After Stop the TV still hands out the last TrackURI to anyone asking
//     GetPositionInfo, so a URL given to a renderer must not carry a credential.
package dlna

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	svcAVTransport       = "urn:schemas-upnp-org:service:AVTransport:1"
	svcRenderingControl  = "urn:schemas-upnp-org:service:RenderingControl:1"
	svcConnectionManager = "urn:schemas-upnp-org:service:ConnectionManager:1"
	ssdpAddr             = "239.255.255.250:1900"
	descriptionMaxBytes  = 1 << 20
	soapResponseMaxBytes = 1 << 20
)

// multicastTarget is where Search sends its multicast query; tests point it
// at a local socket so real devices on the LAN stay out of the result.
var multicastTarget = ssdpAddr

// Renderer is one MediaRenderer that offers AVTransport.
type Renderer struct {
	ID           string `json:"id"` // UDN
	Name         string `json:"name"`
	Model        string `json:"model"`
	Location     string `json:"-"`
	AVTransport  string `json:"-"` // absolute control URL
	RenderingCtl string `json:"-"` // absolute control URL, may be empty
	ConnMgr      string `json:"-"` // ConnectionManager of the same device, may be empty
}

// Host returns the renderer's IP or hostname.
func (r Renderer) Host() string {
	u, err := url.Parse(r.Location)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

type deviceDesc struct {
	URLBase string `xml:"URLBase"`
	Device  device `xml:"device"`
}

type device struct {
	UDN          string    `xml:"UDN"`
	FriendlyName string    `xml:"friendlyName"`
	ModelName    string    `xml:"modelName"`
	Services     []service `xml:"serviceList>service"`
	Devices      []device  `xml:"deviceList>device"`
}

type service struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

// ParseDescription reads a UPnP device description fetched from location.
// It returns an error when the device has no AVTransport service.
func ParseDescription(location string, body []byte) (Renderer, error) {
	var d deviceDesc
	if err := xml.Unmarshal(body, &d); err != nil {
		return Renderer{}, fmt.Errorf("device description: %w", err)
	}
	base := location
	if strings.TrimSpace(d.URLBase) != "" {
		base = strings.TrimSpace(d.URLBase)
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return Renderer{}, fmt.Errorf("base url %q: %w", base, err)
	}
	var walk func(device) (Renderer, bool)
	walk = func(dv device) (Renderer, bool) {
		r := Renderer{ID: strings.TrimSpace(dv.UDN), Name: strings.TrimSpace(dv.FriendlyName), Model: strings.TrimSpace(dv.ModelName), Location: location}
		for _, s := range dv.Services {
			ref, err := url.Parse(strings.TrimSpace(s.ControlURL))
			if err != nil {
				continue
			}
			abs := baseURL.ResolveReference(ref).String()
			switch strings.TrimSpace(s.ServiceType) {
			case svcAVTransport:
				r.AVTransport = abs
			case svcRenderingControl:
				r.RenderingCtl = abs
			case svcConnectionManager:
				r.ConnMgr = abs
			}
		}
		if r.AVTransport != "" {
			return r, true
		}
		for _, sub := range dv.Devices {
			if rr, ok := walk(sub); ok {
				return rr, true
			}
		}
		return Renderer{}, false
	}
	r, ok := walk(d.Device)
	if !ok {
		return Renderer{}, errors.New("device has no AVTransport service")
	}
	// Control URLs must stay on the device that answered: a description
	// (or URLBase) pointing elsewhere would make this server POST to any
	// host it can reach, its own loopback services included (host network).
	locHost := r.Host()
	for _, cu := range []*string{&r.AVTransport, &r.RenderingCtl, &r.ConnMgr} {
		if *cu == "" {
			continue
		}
		u, err := url.Parse(*cu)
		if err != nil || u.Scheme != "http" || u.Hostname() != locHost {
			if cu == &r.AVTransport {
				return Renderer{}, fmt.Errorf("AVTransport control URL %q is not on %s", *cu, locHost)
			}
			*cu = ""
		}
	}
	// The ID carries the host: another device announcing the TV's UDN gets
	// its own entry instead of taking over the TV's.
	if r.ID == "" {
		r.ID = location
	} else {
		r.ID += "@" + locHost
	}
	if r.Name == "" {
		r.Name = r.Host()
	}
	return r, nil
}

// FetchRenderer loads and parses the description at location.
func FetchRenderer(ctx context.Context, hc *http.Client, location string) (Renderer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return Renderer{}, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Renderer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Renderer{}, fmt.Errorf("description %s: HTTP %d", location, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, descriptionMaxBytes))
	if err != nil {
		return Renderer{}, err
	}
	return ParseDescription(location, body)
}

// Search sends an SSDP M-SEARCH for AVTransport to the multicast group and,
// once each, to the given hosts, and returns the LOCATION headers that
// answered within wait.
//
// port fixes the local UDP port (0 = any). Behind a default-deny INPUT chain
// (ZFW on .143) a random port gets nothing back: the LG answers from a random
// source port, so conntrack ties the reply neither to a multicast nor to a
// unicast query (measured 2026-10-28, 0 replies on .143, replies on .250).
// A fixed port can be allowed in the firewall. The Samsung UE50MU6179 answers
// the multicast query only, not a unicast one (measured 2026-10-28).
func Search(ctx context.Context, wait time.Duration, port int, hosts []string) ([]string, int, error) {
	conn, err := ListenSSDP(ctx, port)
	if err != nil {
		return nil, 0, err
	}
	defer conn.Close()
	return SearchOn(conn, wait, hosts)
}

// ListenSSDP opens the UDP socket a search sends from and reads answers on.
func ListenSSDP(ctx context.Context, port int) (net.PacketConn, error) {
	var lc net.ListenConfig
	return lc.ListenPacket(ctx, "udp4", ":"+strconv.Itoa(port))
}

// SearchOn searches on an open socket. The Finder keeps its socket open for
// good: ZimaOS ZFW opens a new host port to the LAN only while something
// listens on it — a socket open for the 2 s of a search was seen at 18:28:36
// and closed again at 18:28:55, and the Samsung's answers fell in between
// (measured 2026-10-28).
//
// answers counts every datagram that came back, also from devices that are
// not TVs: zero answers on a LAN full of SSDP devices means the answers are
// being dropped (a firewall), not that there is no TV.
func SearchOn(conn net.PacketConn, wait time.Duration, hosts []string) (locs []string, answers int, err error) {
	dst, err := net.ResolveUDPAddr("udp4", multicastTarget)
	if err != nil {
		return nil, 0, err
	}
	mx := int(wait / time.Second)
	if mx < 1 {
		mx = 1
	}
	msg := func(host string) []byte {
		return []byte("M-SEARCH * HTTP/1.1\r\nHOST: " + host + "\r\nMAN: \"ssdp:discover\"\r\nMX: " + strconv.Itoa(mx) + "\r\nST: " + svcAVTransport + "\r\n\r\n")
	}
	// Two sends: SSDP is UDP and a single datagram gets lost on busy Wi-Fi.
	for i := 0; i < 2; i++ {
		if _, err := conn.WriteTo(msg(ssdpAddr), dst); err != nil {
			return nil, 0, fmt.Errorf("ssdp send: %w", err)
		}
	}
	for _, h := range hosts {
		ua, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(h, "1900"))
		if err != nil {
			log.Printf("ssdp: %s: %v", h, err)
			continue
		}
		if _, err := conn.WriteTo(msg(ua.String()), ua); err != nil {
			log.Printf("ssdp: unicast to %s: %v", h, err)
		}
	}
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	seen := map[string]bool{}
	buf := make([]byte, 4096)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return locs, answers, nil
			}
			return locs, answers, err
		}
		answers++
		loc := ParseSSDPLocation(buf[:n])
		if loc == "" || seen[loc] || !sameHost(from, loc) {
			continue
		}
		seen[loc] = true
		locs = append(locs, loc)
	}
}

// sameHost reports whether an SSDP answer came from the host its LOCATION
// names; anyone on the LAN can send an answer that points somewhere else.
func sameHost(from net.Addr, loc string) bool {
	ua, ok := from.(*net.UDPAddr)
	if !ok {
		return false
	}
	u, err := url.Parse(loc)
	if err != nil {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.Equal(ua.IP)
}

// LANLocation accepts a description URL only if it is plain http on a
// private LAN address that is not this machine: with host networking a
// loopback or own address would reach the NAS's internal services.
func LANLocation(loc string, own []net.IP) bool {
	u, err := url.Parse(loc)
	if err != nil || u.Scheme != "http" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, o := range own {
		if o.Equal(ip) {
			return false
		}
	}
	return true
}

// OwnIPs lists the addresses of this machine's interfaces.
func OwnIPs() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			out = append(out, n.IP)
		}
	}
	return out
}

// ParseSSDPLocation returns the LOCATION header of an SSDP response.
func ParseSSDPLocation(b []byte) string {
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), "location") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Fault is a UPnP error returned by the renderer.
type Fault struct {
	HTTPStatus  int
	Code        int
	Description string
}

func (f *Fault) Error() string {
	return fmt.Sprintf("UPnP error %d (%s), HTTP %d", f.Code, f.Description, f.HTTPStatus)
}

// Client sends SOAP actions. Each renderer gets its own lock: the LG answers
// overlapping actions out of order or not at all.
type Client struct {
	HTTP  *http.Client
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// Timeouts per SOAP call, set in send by context. Variables for the tests.
var (
	soapTimeout = 6 * time.Second
	// The LG answers Play only once it has opened the file: 1.7–4.0 s for
	// 3–15 MB test clips from a LAN server (measured 2026-10-28). A 6 s limit
	// gave up on large films, and giving up revokes the URL under the TV.
	playTimeout = 30 * time.Second
)

// NewClient has no overall client timeout: it would cut every call at its
// value, Play's 30 s included (0.18 set http.Client.Timeout to 6 s, and a
// converted Play was cut after 6.2 s on the LG, 2026-10-28).
func NewClient() *Client {
	return &Client{HTTP: &http.Client{}, locks: map[string]*sync.Mutex{}}
}

func (c *Client) lock(controlURL string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.locks[controlURL]
	if !ok {
		l = &sync.Mutex{}
		c.locks[controlURL] = l
	}
	return l
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// callNoInstance is call without the InstanceID argument.
func (c *Client) callNoInstance(ctx context.Context, controlURL, svc, action string) ([]byte, error) {
	return c.send(ctx, controlURL, svc, action, "")
}

// call sends action with args (name, value pairs, already in protocol order)
// and returns the raw response body.
func (c *Client) call(ctx context.Context, controlURL, svc, action string, args ...string) ([]byte, error) {
	if len(args)%2 != 0 {
		return nil, errors.New("dlna: odd argument list")
	}
	var inner strings.Builder
	inner.WriteString("<InstanceID>0</InstanceID>")
	for i := 0; i < len(args); i += 2 {
		inner.WriteString("<" + args[i] + ">" + xmlEscape(args[i+1]) + "</" + args[i] + ">")
	}
	return c.send(ctx, controlURL, svc, action, inner.String())
}

func (c *Client) send(ctx context.Context, controlURL, svc, action, inner string) ([]byte, error) {
	body := `<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` +
		`<u:` + action + ` xmlns:u="` + svc + `">` + inner + `</u:` + action + `></s:Body></s:Envelope>`

	l := c.lock(controlURL)
	l.Lock()
	defer l.Unlock()

	timeout := soapTimeout
	if action == "Play" {
		timeout = playTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+svc+`#`+action+`"`)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, soapResponseMaxBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	if resp.StatusCode != http.StatusOK {
		f := ParseFault(raw)
		f.HTTPStatus = resp.StatusCode
		return nil, fmt.Errorf("%s: %w", action, f)
	}
	return raw, nil
}

// ParseFault extracts errorCode/errorDescription from a SOAP fault body.
func ParseFault(raw []byte) *Fault {
	f := &Fault{}
	if v := xmlField(raw, "errorCode"); v != "" {
		f.Code, _ = strconv.Atoi(v)
	}
	f.Description = xmlField(raw, "errorDescription")
	return f
}

// xmlField returns the text of the first element with the given local name.
func xmlField(raw []byte, name string) string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == name {
			var s string
			if err := dec.DecodeElement(&s, &se); err != nil {
				return ""
			}
			return strings.TrimSpace(s)
		}
	}
}

// Media describes what to hand to the renderer.
type Media struct {
	URL      string
	Title    string
	MimeType string // e.g. video/x-matroska
	Size     int64
	Duration time.Duration
	// Converted is a stream joined from Movies' conversion segments: no size,
	// no byte ranges; the TV may ask for a start time (TimeSeekRange).
	Converted bool
}

// convFlags marks a converted stream: time-seekable (OP=10), converted (CI=1).
const convFlags = "DLNA.ORG_OP=10;DLNA.ORG_CI=1;DLNA.ORG_FLAGS=01700000000000000000000000000000"

// Flags returns the DLNA flags for m, for the DIDL and the stream's contentFeatures.
func (m Media) Flags() string {
	if m.Converted {
		return convFlags
	}
	return dlnaFlags
}

// dlnaFlags marks the resource as byte-seekable (OP=01). Measured on the LG
// 2026-10-28: without OP=01, size and duration in the DIDL it answers Seek
// with 200 and keeps playing from where it was; with them it jumps.
const dlnaFlags = "DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000"

// DIDL builds the CurrentURIMetaData document. The LG plays without it, but
// only seeks with the DLNA flags, size and duration in it.
func DIDL(m Media) string {
	mime := m.MimeType
	if mime == "" {
		mime = "video/x-matroska"
	}
	return `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">` +
		`<item id="0" parentID="-1" restricted="1"><dc:title>` + xmlEscape(m.Title) + `</dc:title>` +
		`<upnp:class>object.item.videoItem.movie</upnp:class>` +
		`<res protocolInfo="http-get:*:` + xmlEscape(mime) + `:` + m.Flags() + `"` + resAttrs(m) + `>` + xmlEscape(m.URL) + `</res></item></DIDL-Lite>`
}

func resAttrs(m Media) string {
	var a string
	if m.Size > 0 {
		a += ` size="` + strconv.FormatInt(m.Size, 10) + `"`
	}
	if m.Duration > 0 {
		a += ` duration="` + FormatHMS(m.Duration) + `.000"`
	}
	return a
}

func (c *Client) SetURI(ctx context.Context, r Renderer, m Media) error {
	_, err := c.call(ctx, r.AVTransport, svcAVTransport, "SetAVTransportURI", "CurrentURI", m.URL, "CurrentURIMetaData", DIDL(m))
	return err
}

func (c *Client) Play(ctx context.Context, r Renderer) error {
	_, err := c.call(ctx, r.AVTransport, svcAVTransport, "Play", "Speed", "1")
	return err
}

func (c *Client) Pause(ctx context.Context, r Renderer) error {
	_, err := c.call(ctx, r.AVTransport, svcAVTransport, "Pause")
	return err
}

func (c *Client) Stop(ctx context.Context, r Renderer) error {
	_, err := c.call(ctx, r.AVTransport, svcAVTransport, "Stop")
	return err
}

// Seek jumps to an absolute position. Only call it while PLAYING or
// PAUSED_PLAYBACK; the LG does not answer it while STOPPED.
func (c *Client) Seek(ctx context.Context, r Renderer, pos time.Duration) error {
	_, err := c.call(ctx, r.AVTransport, svcAVTransport, "Seek", "Unit", "REL_TIME", "Target", FormatHMS(pos))
	return err
}

// Status is the renderer's transport state and position.
type Status struct {
	State    string        `json:"state"` // PLAYING, PAUSED_PLAYBACK, STOPPED, TRANSITIONING, NO_MEDIA_PRESENT
	Position time.Duration `json:"-"`
	Duration time.Duration `json:"-"`
	TrackURI string        `json:"-"`
}

func (c *Client) Status(ctx context.Context, r Renderer) (Status, error) {
	ti, err := c.call(ctx, r.AVTransport, svcAVTransport, "GetTransportInfo")
	if err != nil {
		return Status{}, err
	}
	pi, err := c.call(ctx, r.AVTransport, svcAVTransport, "GetPositionInfo")
	if err != nil {
		return Status{}, err
	}
	return ParseStatus(ti, pi), nil
}

// ParseStatus combines GetTransportInfo and GetPositionInfo responses.
func ParseStatus(transportInfo, positionInfo []byte) Status {
	pos, _ := ParseHMS(xmlField(positionInfo, "RelTime"))
	dur, _ := ParseHMS(xmlField(positionInfo, "TrackDuration"))
	return Status{
		State:    xmlField(transportInfo, "CurrentTransportState"),
		Position: pos,
		Duration: dur,
		TrackURI: xmlField(positionInfo, "TrackURI"),
	}
}

// PlaysVideo asks the renderer's ConnectionManager whether its sink accepts
// any video/* type. Sonos speakers answer with audio types only (measured).
func (c *Client) PlaysVideo(ctx context.Context, r Renderer) (bool, error) {
	if r.ConnMgr == "" {
		return false, errors.New("renderer has no ConnectionManager")
	}
	raw, err := c.callNoInstance(ctx, r.ConnMgr, svcConnectionManager, "GetProtocolInfo")
	if err != nil {
		return false, err
	}
	return SinkHasVideo(raw), nil
}

// SinkHasVideo reports whether a GetProtocolInfo answer lists a video/* sink.
func SinkHasVideo(raw []byte) bool {
	for _, pi := range strings.Split(xmlField(raw, "Sink"), ",") {
		// protocol:network:contentFormat:additionalInfo
		if parts := strings.SplitN(strings.TrimSpace(pi), ":", 4); len(parts) >= 3 && strings.HasPrefix(parts[2], "video/") {
			return true
		}
	}
	return false
}

// Sink returns the renderer's GetProtocolInfo sink list as sent.
func (c *Client) Sink(ctx context.Context, r Renderer) (string, error) {
	if r.ConnMgr == "" {
		return "", errors.New("renderer has no ConnectionManager")
	}
	raw, err := c.callNoInstance(ctx, r.ConnMgr, svcConnectionManager, "GetProtocolInfo")
	if err != nil {
		return "", err
	}
	return xmlField(raw, "Sink"), nil
}

// containerFormats maps a file extension to the contentFormat spellings TVs
// use for it: the LG lists "video/x-matroska", both Samsungs "video/x-mkv"
// (measured 2026-10-28).
var containerFormats = map[string][]string{
	"mkv":  {"matroska", "x-mkv", "video/mkv"},
	"mp4":  {"video/mp4"},
	"m4v":  {"video/mp4", "x-m4v"},
	"mov":  {"quicktime", "video/mp4"},
	"avi":  {"avi", "x-msvideo", "divx"},
	"webm": {"webm"},
	"ts":   {"mp2t", "mpeg-tts", "mpeg2"},
	"m2ts": {"mp2t", "mpeg-tts", "mpeg2"},
}

// Unlisted names what a sink list does not mention for this file: the
// container and, for HEVC, the codec. It is a hint, not a verdict: the LG
// OLED55B9 lists no HEVC at all and still decodes 4K HEVC (measured
// 2026-10-28); the 2012 Samsung UE32EH5300 lists none either. Codecs other
// than HEVC are not checked — sink lists name containers, rarely codecs.
func Unlisted(sink, container, codec string) []string {
	s := strings.ToLower(sink)
	// Only http-get counts: movies-cast serves over HTTP, and Sonos lists
	// "x-rincon:*:*:*" wildcards for its own protocols (measured).
	var formats []string
	for _, pi := range strings.Split(s, ",") {
		if parts := strings.SplitN(strings.TrimSpace(pi), ":", 4); len(parts) >= 3 && parts[0] == "http-get" {
			formats = append(formats, parts[2])
		}
	}
	listed := func(names []string) bool {
		for _, f := range formats {
			if f == "*" {
				return true
			}
			for _, n := range names {
				if strings.Contains(f, n) {
					return true
				}
			}
		}
		return false
	}
	var out []string
	if names, known := containerFormats[strings.ToLower(container)]; known && !listed(names) {
		out = append(out, strings.ToUpper(container))
	}
	if c := strings.ToLower(codec); (c == "hevc" || c == "h265") && !listed(nil) && !strings.Contains(s, "hevc") && !strings.Contains(s, "h265") {
		out = append(out, "HEVC (H.265)")
	}
	return out
}

// Volume returns 0–100, or an error (the LG answers 606).
func (c *Client) Volume(ctx context.Context, r Renderer) (int, error) {
	if r.RenderingCtl == "" {
		return 0, errors.New("renderer has no RenderingControl")
	}
	raw, err := c.call(ctx, r.RenderingCtl, svcRenderingControl, "GetVolume", "Channel", "Master")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(xmlField(raw, "CurrentVolume"))
}

func (c *Client) SetVolume(ctx context.Context, r Renderer, v int) error {
	if r.RenderingCtl == "" {
		return errors.New("renderer has no RenderingControl")
	}
	v = max(0, min(100, v))
	_, err := c.call(ctx, r.RenderingCtl, svcRenderingControl, "SetVolume", "Channel", "Master", "DesiredVolume", strconv.Itoa(v))
	return err
}

// FormatHMS renders a duration as H:MM:SS as AVTransport expects.
func FormatHMS(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int64(d / time.Second)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

// ParseHMS reads H+:MM:SS[.fff]; "NOT_IMPLEMENTED" and empty give an error.
func ParseHMS(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("not H:MM:SS: %q", s)
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err := errors.Join(err1, err2, err3); err != nil {
		return 0, fmt.Errorf("not H:MM:SS: %q", s)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec*float64(time.Second)), nil
}
