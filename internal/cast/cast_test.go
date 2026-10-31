package cast

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"movies-cast/internal/tlsconf"
)

// fakeReceiver speaks Cast v2 like a device: TLS, length-prefixed
// CastMessages, the receiver and media namespaces of the Default Media
// Receiver (message shapes from Google's Cast documentation).
type fakeReceiver struct {
	t        *testing.T
	ln       net.Listener
	mu       sync.Mutex
	launched bool
	volume   float64
	state    string
	idle     string
	at       float64
	content  string
	pongs    int
	mediaTo  []string // destinations of media messages
	commands []string
}

func newFakeReceiver(t *testing.T) *fakeReceiver {
	cert, err := tlsconf.Load("", "", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeReceiver{t: t, ln: ln, volume: 0.3}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeReceiver) send(c net.Conn, src, ns string, p map[string]any) {
	b, _ := json.Marshal(p)
	_ = writeFrame(c, message{Source: src, Destination: senderID, Namespace: ns, Payload: string(b)})
}

func (f *fakeReceiver) receiverStatus(id any) map[string]any {
	st := map[string]any{"volume": map[string]any{"level": f.volume, "muted": false}}
	if f.launched {
		st["applications"] = []any{map[string]any{"appId": DefaultMediaReceiver, "transportId": "web-7", "sessionId": "s-1"}}
	}
	return map[string]any{"type": "RECEIVER_STATUS", "requestId": id, "status": st}
}

func (f *fakeReceiver) mediaStatus(id any) map[string]any {
	if f.content == "" {
		return map[string]any{"type": "MEDIA_STATUS", "requestId": id, "status": []any{}}
	}
	st := map[string]any{"mediaSessionId": 4, "playerState": f.state, "currentTime": f.at,
		"media": map[string]any{"contentId": f.content, "duration": 120.5}}
	if f.idle != "" {
		st["idleReason"] = f.idle
	}
	return map[string]any{"type": "MEDIA_STATUS", "requestId": id, "status": []any{st}}
}

func (f *fakeReceiver) serve(c net.Conn) {
	defer c.Close()
	// A device pings the sender too; the sender must answer PONG.
	f.send(c, receiverID, nsHeartbeat, map[string]any{"type": "PING"})
	for {
		m, err := readFrame(c)
		if err != nil {
			return
		}
		var p map[string]any
		_ = json.Unmarshal([]byte(m.Payload), &p)
		id := p["requestId"]
		f.mu.Lock()
		switch m.Namespace {
		case nsHeartbeat:
			if p["type"] == "PONG" {
				f.pongs++
			}
		case nsReceiver:
			switch p["type"] {
			case "LAUNCH":
				f.launched = p["appId"] == DefaultMediaReceiver
			case "SET_VOLUME":
				v, _ := p["volume"].(map[string]any)
				f.volume, _ = v["level"].(float64)
			}
			f.send(c, receiverID, nsReceiver, f.receiverStatus(id))
		case nsMedia:
			f.mediaTo = append(f.mediaTo, m.Destination)
			typ, _ := p["type"].(string)
			f.commands = append(f.commands, typ)
			switch typ {
			case "LOAD":
				media, _ := p["media"].(map[string]any)
				f.content, _ = media["contentId"].(string)
				f.at, _ = p["currentTime"].(float64)
				f.state, f.idle = "PLAYING", ""
			case "PAUSE":
				f.state = "PAUSED"
			case "PLAY":
				f.state = "PLAYING"
			case "SEEK":
				f.at, _ = p["currentTime"].(float64)
			case "STOP":
				f.state, f.idle = "IDLE", "CANCELLED"
			}
			f.send(c, "web-7", nsMedia, f.mediaStatus(id))
		}
		f.mu.Unlock()
	}
}

func TestFramesRoundTrip(t *testing.T) {
	in := message{Source: "a", Destination: "receiver-0", Namespace: nsMedia, Payload: `{"type":"PING","x":"` + string(make([]byte, 300)) + `"}`}
	got, err := decode(in.encode())
	if err != nil || got != in {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestLoadControlAndStatusAgainstAReceiver(t *testing.T) {
	f := newFakeReceiver(t)
	d := &Device{Addr: f.ln.Addr().String()}
	defer d.Close()
	ctx := context.Background()
	if err := d.Load(ctx, "http://192.0.2.1:8097/s/p1/movie.mp4", "video/mp4", "Film", 90*time.Second); err != nil {
		t.Fatal(err)
	}
	st, err := d.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.PlayerState != "PLAYING" || st.ContentID != "http://192.0.2.1:8097/s/p1/movie.mp4" || st.CurrentTime != 90 || st.Duration != 120.5 || st.SessionID != 4 {
		t.Fatalf("status %+v", st)
	}
	for _, step := range []struct {
		do   func() error
		want string
	}{
		{func() error { return d.Pause(ctx) }, "PAUSED"},
		{func() error { return d.Play(ctx) }, "PLAYING"},
		{func() error { return d.Seek(ctx, 30*time.Second) }, "PLAYING"},
		{func() error { return d.Stop(ctx) }, "IDLE"},
	} {
		if err := step.do(); err != nil {
			t.Fatal(err)
		}
		if st, _ = d.Status(ctx); st.PlayerState != step.want {
			t.Fatalf("after a command: %+v, want %s", st, step.want)
		}
	}
	if st.IdleReason != "CANCELLED" || st.CurrentTime != 30 {
		t.Fatalf("after seek and stop: %+v", st)
	}
	if err := d.SetVolume(ctx, 55); err != nil {
		t.Fatal(err)
	}
	if v, err := d.Volume(ctx); err != nil || v != 55 {
		t.Fatalf("volume %d %v", v, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.launched {
		t.Error("the Default Media Receiver was not launched")
	}
	for i, dest := range f.mediaTo {
		if dest != "web-7" {
			t.Errorf("media message %d (%s) went to %q, not the app's transportId", i, f.commands[i], dest)
		}
	}
	if f.pongs == 0 {
		t.Error("the receiver's PING got no PONG")
	}
}

func TestAFailedLoadIsAnError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		for {
			m, err := readFrame(c)
			if err != nil {
				return
			}
			var p map[string]any
			_ = json.Unmarshal([]byte(m.Payload), &p)
			var out map[string]any
			switch p["type"] {
			case "GET_STATUS", "LAUNCH":
				out = map[string]any{"type": "RECEIVER_STATUS", "requestId": p["requestId"], "status": map[string]any{"applications": []any{map[string]any{"appId": DefaultMediaReceiver, "transportId": "t"}}}}
			case "LOAD":
				out = map[string]any{"type": "LOAD_FAILED", "requestId": p["requestId"]}
			default:
				continue
			}
			b, _ := json.Marshal(out)
			_ = writeFrame(c, message{Source: "x", Destination: senderID, Namespace: m.Namespace, Payload: string(b)})
		}
	}()
	d := &Device{Addr: ln.Addr().String(), Dial: func(ctx context.Context, addr string) (net.Conn, error) {
		var nd net.Dialer
		return nd.DialContext(ctx, "tcp", addr)
	}}
	defer d.Close()
	if err := d.Load(context.Background(), "http://192.0.2.1/x.mkv", "video/x-matroska", "x", 0); err == nil {
		t.Fatal("LOAD_FAILED was not reported")
	}
}
