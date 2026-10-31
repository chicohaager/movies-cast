// Package cast sends films to Google Cast devices (Chromecast, Google TV,
// Nvidia Shield, TVs with Chromecast built-in) — roadmap 17.
//
// Cast v2: TLS to port 8009 (the device's certificate is self-signed, so it
// is not verified, like every Cast sender does), frames of a 4-byte length
// and a protobuf CastMessage carrying JSON in four namespaces: connection,
// heartbeat, receiver (launch the Default Media Receiver CC1AD845, volume)
// and media (LOAD, PLAY, PAUSE, SEEK, STOP, GET_STATUS).
//
// No Cast device was reachable while this was written (2026-10-31, mDNS
// query for _googlecast._tcp unanswered on the owner's LAN): the client is
// tested against a receiver that speaks the same framing and messages
// (cast_test.go), not against a device.
package cast

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	nsConnection = "urn:x-cast:com.google.cast.tp.connection"
	nsHeartbeat  = "urn:x-cast:com.google.cast.tp.heartbeat"
	nsReceiver   = "urn:x-cast:com.google.cast.receiver"
	nsMedia      = "urn:x-cast:com.google.cast.media"
	// DefaultMediaReceiver is Google's receiver app for plain media URLs.
	DefaultMediaReceiver = "CC1AD845"
	senderID             = "sender-mc"
	receiverID           = "receiver-0"
	maxFrame             = 64 << 10
)

// message is a CastMessage (cast_channel.proto) with a UTF-8 payload.
type message struct {
	Source, Destination, Namespace, Payload string
}

// encode writes the protobuf by hand: field 1 protocol_version (0),
// 2 source_id, 3 destination_id, 4 namespace, 5 payload_type (0 = STRING),
// 6 payload_utf8.
func (m message) encode() []byte {
	var b []byte
	b = append(b, 0x08, 0x00)
	for _, f := range []struct {
		tag byte
		s   string
	}{{0x12, m.Source}, {0x1a, m.Destination}, {0x22, m.Namespace}} {
		b = append(b, f.tag)
		b = binary.AppendUvarint(b, uint64(len(f.s)))
		b = append(b, f.s...)
	}
	b = append(b, 0x28, 0x00, 0x32)
	b = binary.AppendUvarint(b, uint64(len(m.Payload)))
	return append(b, m.Payload...)
}

// decode reads the fields encode writes; others are skipped.
func decode(b []byte) (message, error) {
	var m message
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return m, errors.New("cast: bad field key")
		}
		b = b[n:]
		switch key & 7 {
		case 0:
			_, n = binary.Uvarint(b)
			if n <= 0 {
				return m, errors.New("cast: bad varint")
			}
			b = b[n:]
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || uint64(len(b)-n) < l {
				return m, errors.New("cast: bad length")
			}
			v := string(b[n : n+int(l)])
			b = b[n+int(l):]
			switch key >> 3 {
			case 2:
				m.Source = v
			case 3:
				m.Destination = v
			case 4:
				m.Namespace = v
			case 6:
				m.Payload = v
			}
		default:
			return m, fmt.Errorf("cast: wire type %d", key&7)
		}
	}
	return m, nil
}

func writeFrame(w io.Writer, m message) error {
	b := m.encode()
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(b)))
	_, err := w.Write(append(h[:], b...))
	return err
}

func readFrame(r io.Reader) (message, error) {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return message{}, err
	}
	n := binary.BigEndian.Uint32(h[:])
	if n > maxFrame {
		return message{}, fmt.Errorf("cast: frame of %d bytes", n)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return message{}, err
	}
	return decode(b)
}

// MediaStatus is the part of a MEDIA_STATUS the server needs.
type MediaStatus struct {
	SessionID   int
	PlayerState string // IDLE, PLAYING, PAUSED, BUFFERING
	IdleReason  string // FINISHED, CANCELLED, INTERRUPTED, ERROR
	CurrentTime float64
	Duration    float64
	ContentID   string
}

// Device is one connection to a Cast device, opened on first use and opened
// again after an error.
type Device struct {
	Addr string // host:port
	// Dial is replaced in tests; nil means TLS over TCP without verifying
	// the device's self-signed certificate.
	Dial func(ctx context.Context, addr string) (net.Conn, error)

	mu        sync.Mutex
	conn      net.Conn
	reqID     int
	waiting   map[int]chan map[string]any
	transport string // transportId of the launched receiver app
	media     MediaStatus
	hasMedia  bool
	volume    float64
	closed    chan struct{}
}

func (d *Device) dial(ctx context.Context) (net.Conn, error) {
	if d.Dial != nil {
		return d.Dial(ctx, d.Addr)
	}
	td := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // Cast devices present self-signed certificates
	return td.DialContext(ctx, "tcp", d.Addr)
}

// connect opens the channel to receiver-0 if none is open. Caller holds d.mu.
func (d *Device) connectLocked(ctx context.Context) error {
	if d.conn != nil {
		return nil
	}
	c, err := d.dial(ctx)
	if err != nil {
		return err
	}
	d.conn, d.waiting, d.transport, d.closed = c, map[int]chan map[string]any{}, "", make(chan struct{})
	if err := d.sendLocked(receiverID, nsConnection, map[string]any{"type": "CONNECT"}); err != nil {
		d.dropLocked()
		return err
	}
	go d.read(c, d.closed)
	go d.ping(c, d.closed)
	return nil
}

func (d *Device) dropLocked() {
	if d.conn != nil {
		_ = d.conn.Close()
		close(d.closed)
	}
	d.conn, d.transport = nil, ""
	for id, ch := range d.waiting {
		close(ch)
		delete(d.waiting, id)
	}
}

func (d *Device) sendLocked(dest, ns string, payload map[string]any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_ = d.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return writeFrame(d.conn, message{Source: senderID, Destination: dest, Namespace: ns, Payload: string(b)})
}

func (d *Device) ping(c net.Conn, closed chan struct{}) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-closed:
			return
		case <-t.C:
			d.mu.Lock()
			if d.conn == c {
				if err := d.sendLocked(receiverID, nsHeartbeat, map[string]any{"type": "PING"}); err != nil {
					d.dropLocked()
				}
			}
			d.mu.Unlock()
		}
	}
}

// read dispatches answers to waiting requests and keeps the last media and
// volume status.
func (d *Device) read(c net.Conn, closed chan struct{}) {
	for {
		m, err := readFrame(c)
		if err != nil {
			d.mu.Lock()
			if d.conn == c {
				d.dropLocked()
			}
			d.mu.Unlock()
			return
		}
		var p map[string]any
		if json.Unmarshal([]byte(m.Payload), &p) != nil {
			continue
		}
		d.mu.Lock()
		switch p["type"] {
		case "PING":
			if d.conn == c {
				_ = d.sendLocked(m.Source, nsHeartbeat, map[string]any{"type": "PONG"})
			}
		case "MEDIA_STATUS":
			d.takeMediaLocked(p)
		case "RECEIVER_STATUS":
			d.takeReceiverLocked(p)
		case "CLOSE":
			if m.Source != receiverID {
				d.transport = ""
			}
		}
		if id, ok := p["requestId"].(float64); ok {
			if ch := d.waiting[int(id)]; ch != nil {
				ch <- p
				delete(d.waiting, int(id))
			}
		}
		d.mu.Unlock()
		select {
		case <-closed:
			return
		default:
		}
	}
}

func (d *Device) takeMediaLocked(p map[string]any) {
	list, _ := p["status"].([]any)
	if len(list) == 0 {
		d.hasMedia = false
		return
	}
	st, _ := list[0].(map[string]any)
	ms := MediaStatus{ContentID: d.media.ContentID, Duration: d.media.Duration}
	if v, ok := st["mediaSessionId"].(float64); ok {
		ms.SessionID = int(v)
	}
	ms.PlayerState, _ = st["playerState"].(string)
	ms.IdleReason, _ = st["idleReason"].(string)
	ms.CurrentTime, _ = st["currentTime"].(float64)
	if media, ok := st["media"].(map[string]any); ok {
		ms.ContentID, _ = media["contentId"].(string)
		if v, ok := media["duration"].(float64); ok {
			ms.Duration = v
		}
	}
	d.media, d.hasMedia = ms, true
}

func (d *Device) takeReceiverLocked(p map[string]any) {
	st, _ := p["status"].(map[string]any)
	if v, ok := st["volume"].(map[string]any); ok {
		if l, ok := v["level"].(float64); ok {
			d.volume = l
		}
	}
	apps, _ := st["applications"].([]any)
	d.transport = ""
	for _, a := range apps {
		app, _ := a.(map[string]any)
		if app["appId"] == DefaultMediaReceiver {
			d.transport, _ = app["transportId"].(string)
		}
	}
}

// request sends a message that the device answers with the same requestId.
func (d *Device) request(ctx context.Context, dest, ns string, payload map[string]any) (map[string]any, error) {
	d.mu.Lock()
	if err := d.connectLocked(ctx); err != nil {
		d.mu.Unlock()
		return nil, err
	}
	if dest == "" {
		dest = d.transport
	}
	d.reqID++
	id := d.reqID
	payload["requestId"] = id
	ch := make(chan map[string]any, 1)
	d.waiting[id] = ch
	err := d.sendLocked(dest, ns, payload)
	if err != nil {
		d.dropLocked()
	}
	d.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case p, ok := <-ch:
		if !ok {
			return nil, errors.New("cast: connection closed")
		}
		switch p["type"] {
		case "LOAD_FAILED", "LOAD_CANCELLED", "INVALID_REQUEST", "LAUNCH_ERROR", "INVALID_PLAYER_STATE":
			return p, fmt.Errorf("cast: %v (%v)", p["type"], p["reason"])
		}
		return p, nil
	case <-ctx.Done():
		d.mu.Lock()
		delete(d.waiting, id)
		d.mu.Unlock()
		return nil, fmt.Errorf("cast: no answer: %w", ctx.Err())
	}
}

// app makes sure the Default Media Receiver runs and is connected.
func (d *Device) app(ctx context.Context) error {
	d.mu.Lock()
	running := d.conn != nil && d.transport != ""
	d.mu.Unlock()
	if running {
		return nil
	}
	if _, err := d.request(ctx, receiverID, nsReceiver, map[string]any{"type": "GET_STATUS"}); err != nil {
		return err
	}
	d.mu.Lock()
	running = d.transport != ""
	d.mu.Unlock()
	if !running {
		if _, err := d.request(ctx, receiverID, nsReceiver, map[string]any{"type": "LAUNCH", "appId": DefaultMediaReceiver}); err != nil {
			return err
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.transport == "" {
		return errors.New("cast: the media receiver did not start")
	}
	return d.sendLocked(d.transport, nsConnection, map[string]any{"type": "CONNECT"})
}

// Load starts url on the device.
func (d *Device) Load(ctx context.Context, url, mime, title string, at time.Duration) error {
	if err := d.app(ctx); err != nil {
		return err
	}
	_, err := d.request(ctx, "", nsMedia, map[string]any{
		"type": "LOAD", "autoplay": true, "currentTime": at.Seconds(),
		"media": map[string]any{"contentId": url, "contentType": mime, "streamType": "BUFFERED",
			"metadata": map[string]any{"metadataType": 0, "title": title}},
	})
	return err
}

func (d *Device) mediaCmd(ctx context.Context, typ string, extra map[string]any) error {
	d.mu.Lock()
	sid, ok := d.media.SessionID, d.hasMedia
	d.mu.Unlock()
	if !ok {
		if _, err := d.Status(ctx); err != nil {
			return err
		}
		d.mu.Lock()
		sid, ok = d.media.SessionID, d.hasMedia
		d.mu.Unlock()
		if !ok {
			return errors.New("cast: nothing is playing")
		}
	}
	p := map[string]any{"type": typ, "mediaSessionId": sid}
	for k, v := range extra {
		p[k] = v
	}
	_, err := d.request(ctx, "", nsMedia, p)
	return err
}

func (d *Device) Play(ctx context.Context) error  { return d.mediaCmd(ctx, "PLAY", nil) }
func (d *Device) Pause(ctx context.Context) error { return d.mediaCmd(ctx, "PAUSE", nil) }
func (d *Device) Stop(ctx context.Context) error  { return d.mediaCmd(ctx, "STOP", nil) }
func (d *Device) Seek(ctx context.Context, to time.Duration) error {
	return d.mediaCmd(ctx, "SEEK", map[string]any{"currentTime": to.Seconds()})
}

// Status asks for the media status; ok is false when nothing is loaded.
func (d *Device) Status(ctx context.Context) (MediaStatus, error) {
	if err := d.app(ctx); err != nil {
		return MediaStatus{}, err
	}
	if _, err := d.request(ctx, "", nsMedia, map[string]any{"type": "GET_STATUS"}); err != nil {
		return MediaStatus{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.hasMedia {
		return MediaStatus{}, nil
	}
	return d.media, nil
}

// Volume returns 0–100.
func (d *Device) Volume(ctx context.Context) (int, error) {
	if _, err := d.request(ctx, receiverID, nsReceiver, map[string]any{"type": "GET_STATUS"}); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return int(d.volume*100 + 0.5), nil
}

func (d *Device) SetVolume(ctx context.Context, v int) error {
	v = max(0, min(100, v))
	_, err := d.request(ctx, receiverID, nsReceiver, map[string]any{"type": "SET_VOLUME", "volume": map[string]any{"level": float64(v) / 100}})
	return err
}

// Close ends the connection.
func (d *Device) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropLocked()
}
