// Package casttest is a Cast v2 receiver for tests of code that uses
// package cast: plain TCP (the sender's Dial is replaced), its own frame
// coding (so it checks the sender's wire format independently), the receiver
// and media namespaces of the Default Media Receiver.
package casttest

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"sync"
	"testing"
)

type Receiver struct {
	Addr string

	mu       sync.Mutex
	launched bool
	State    string // PLAYING, PAUSED, IDLE
	Idle     string
	At       float64
	Content  string
	Commands []string
}

// New starts a receiver that runs until the test ends.
func New(t *testing.T) *Receiver {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	r := &Receiver{Addr: ln.Addr().String()}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go r.serve(c)
		}
	}()
	return r
}

// Set changes the player state, e.g. to IDLE/FINISHED at the end of a film.
func (r *Receiver) Set(state, idle string, at float64) {
	r.mu.Lock()
	r.State, r.Idle, r.At = state, idle, at
	r.mu.Unlock()
}

// Snapshot returns the content loaded last and the commands so far.
func (r *Receiver) Snapshot() (content string, commands []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Content, append([]string(nil), r.Commands...)
}

// protobuf strings of a CastMessage: 2 source, 3 destination, 4 namespace, 6 payload.
func readMsg(c net.Conn) (ns, payload string, err error) {
	var h [4]byte
	if _, err = io.ReadFull(c, h[:]); err != nil {
		return
	}
	b := make([]byte, binary.BigEndian.Uint32(h[:]))
	if _, err = io.ReadFull(c, b); err != nil {
		return
	}
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		b = b[n:]
		if key&7 == 0 {
			_, n = binary.Uvarint(b)
			b = b[n:]
			continue
		}
		l, n := binary.Uvarint(b)
		v := string(b[n : n+int(l)])
		b = b[n+int(l):]
		switch key >> 3 {
		case 4:
			ns = v
		case 6:
			payload = v
		}
	}
	return
}

func writeMsg(c net.Conn, src, ns string, p any) {
	pl, _ := json.Marshal(p)
	var b []byte
	b = append(b, 0x08, 0)
	for _, f := range []struct {
		tag byte
		s   string
	}{{0x12, src}, {0x1a, "sender-0"}, {0x22, ns}} {
		b = append(b, f.tag)
		b = binary.AppendUvarint(b, uint64(len(f.s)))
		b = append(b, f.s...)
	}
	b = append(b, 0x28, 0, 0x32)
	b = binary.AppendUvarint(b, uint64(len(pl)))
	b = append(b, pl...)
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(b)))
	_, _ = c.Write(append(h[:], b...))
}

func (r *Receiver) serve(c net.Conn) {
	defer c.Close()
	for {
		ns, payload, err := readMsg(c)
		if err != nil {
			return
		}
		var p map[string]any
		_ = json.Unmarshal([]byte(payload), &p)
		id := p["requestId"]
		r.mu.Lock()
		switch ns {
		case "urn:x-cast:com.google.cast.receiver":
			if p["type"] == "LAUNCH" {
				r.launched = true
			}
			st := map[string]any{"volume": map[string]any{"level": 0.5}}
			if r.launched {
				st["applications"] = []any{map[string]any{"appId": "CC1AD845", "transportId": "web-1"}}
			}
			writeMsg(c, "receiver-0", ns, map[string]any{"type": "RECEIVER_STATUS", "requestId": id, "status": st})
		case "urn:x-cast:com.google.cast.media":
			typ, _ := p["type"].(string)
			r.Commands = append(r.Commands, typ)
			switch typ {
			case "LOAD":
				m, _ := p["media"].(map[string]any)
				r.Content, _ = m["contentId"].(string)
				r.State, r.Idle, r.At = "PLAYING", "", 0
			case "PAUSE":
				r.State = "PAUSED"
			case "PLAY":
				r.State = "PLAYING"
			case "SEEK":
				r.At, _ = p["currentTime"].(float64)
			case "STOP":
				r.State, r.Idle = "IDLE", "CANCELLED"
			}
			var list []any
			if r.Content != "" {
				st := map[string]any{"mediaSessionId": 1, "playerState": r.State, "currentTime": r.At, "media": map[string]any{"contentId": r.Content, "duration": 1200.0}}
				if r.Idle != "" {
					st["idleReason"] = r.Idle
				}
				list = append(list, st)
			}
			writeMsg(c, "web-1", ns, map[string]any{"type": "MEDIA_STATUS", "requestId": id, "status": list})
		}
		r.mu.Unlock()
	}
}
