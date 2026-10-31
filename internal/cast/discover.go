package cast

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

// Found is a Cast device that answered the mDNS query.
type Found struct {
	ID    string // TXT id
	Name  string // TXT fn, the name the owner gave it
	Model string // TXT md
	Addr  string // host:port
}

const service = "_googlecast._tcp.local"

// query builds an mDNS PTR question for the Cast service (QU bit set, so
// devices may answer unicast to the port the query came from).
func query() []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[4:], 1) // one question
	for _, l := range strings.Split(service, ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	return append(b, 0, 0, 12, 0x80, 1) // type PTR, class IN | QU
}

// Discover asks the LAN for Cast devices for wait and returns what answered.
func Discover(ctx context.Context, wait time.Duration) ([]Found, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	if _, err := conn.WriteToUDP(query(), group); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetReadDeadline(deadline)
	byID := map[string]Found{}
	var order []string
	buf := make([]byte, 9000)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				break
			}
			return nil, err
		}
		for _, f := range parseAnswer(buf[:n], from.IP) {
			if _, seen := byID[f.ID]; !seen {
				order = append(order, f.ID)
			}
			byID[f.ID] = f
		}
	}
	out := make([]Found, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, nil
}

type rr struct {
	name  string
	typ   uint16
	rdata []byte
	off   int // offset of rdata in the packet, for names in it
}

// readName reads a DNS name at off (with compression), returning it and the
// offset after it in the original position.
func readName(p []byte, off int) (string, int, error) {
	var labels []string
	end := -1
	for hops := 0; hops < 32; hops++ {
		if off >= len(p) {
			return "", 0, errors.New("dns: name out of range")
		}
		l := int(p[off])
		switch {
		case l == 0:
			if end < 0 {
				end = off + 1
			}
			return strings.Join(labels, "."), end, nil
		case l&0xc0 == 0xc0:
			if off+1 >= len(p) {
				return "", 0, errors.New("dns: bad pointer")
			}
			if end < 0 {
				end = off + 2
			}
			off = int(binary.BigEndian.Uint16(p[off:]) & 0x3fff)
		default:
			if off+1+l > len(p) {
				return "", 0, errors.New("dns: label out of range")
			}
			labels = append(labels, string(p[off+1:off+1+l]))
			off += 1 + l
		}
	}
	return "", 0, errors.New("dns: too many pointers")
}

func parseRecords(p []byte) ([]rr, error) {
	if len(p) < 12 {
		return nil, errors.New("dns: short")
	}
	qd := int(binary.BigEndian.Uint16(p[4:]))
	n := int(binary.BigEndian.Uint16(p[6:])) + int(binary.BigEndian.Uint16(p[8:])) + int(binary.BigEndian.Uint16(p[10:]))
	off := 12
	for i := 0; i < qd; i++ {
		_, o, err := readName(p, off)
		if err != nil {
			return nil, err
		}
		off = o + 4
	}
	var out []rr
	for i := 0; i < n; i++ {
		name, o, err := readName(p, off)
		if err != nil {
			return out, err
		}
		if o+10 > len(p) {
			return out, errors.New("dns: short record")
		}
		typ := binary.BigEndian.Uint16(p[o:])
		l := int(binary.BigEndian.Uint16(p[o+8:]))
		if o+10+l > len(p) {
			return out, errors.New("dns: short rdata")
		}
		out = append(out, rr{name: strings.ToLower(name), typ: typ, rdata: p[o+10 : o+10+l], off: o + 10})
		off = o + 10 + l
	}
	return out, nil
}

// parseAnswer turns one mDNS answer into the Cast devices it describes; a
// device without an A record in the answer is reached at the sender's IP.
func parseAnswer(p []byte, from net.IP) []Found {
	recs, _ := parseRecords(p)
	instances := map[string]bool{}
	srv := map[string]struct {
		target string
		port   int
	}{}
	txt := map[string]map[string]string{}
	addr := map[string]net.IP{}
	for _, r := range recs {
		switch r.typ {
		case 12: // PTR
			if r.name == service {
				if n, _, err := readName(p, r.off); err == nil {
					instances[strings.ToLower(n)] = true
				}
			}
		case 33: // SRV
			if len(r.rdata) >= 7 {
				t, _, err := readName(p, r.off+6)
				if err == nil {
					srv[r.name] = struct {
						target string
						port   int
					}{strings.ToLower(t), int(binary.BigEndian.Uint16(r.rdata[4:]))}
				}
			}
		case 16: // TXT
			kv := map[string]string{}
			for b := r.rdata; len(b) > 0; {
				l := int(b[0])
				if 1+l > len(b) {
					break
				}
				if k, v, ok := strings.Cut(string(b[1:1+l]), "="); ok {
					kv[k] = v
				}
				b = b[1+l:]
			}
			txt[r.name] = kv
		case 1: // A
			if len(r.rdata) == 4 {
				addr[r.name] = net.IP(append([]byte(nil), r.rdata...))
			}
		}
	}
	for name := range srv {
		if strings.HasSuffix(name, "."+service) {
			instances[name] = true
		}
	}
	var out []Found
	for inst := range instances {
		kv := txt[inst]
		id := kv["id"]
		if id == "" {
			continue
		}
		port := 8009
		ip := from
		if s, ok := srv[inst]; ok {
			port = s.port
			if a, ok := addr[s.target]; ok {
				ip = a
			}
		}
		name := kv["fn"]
		if name == "" {
			name = "Chromecast"
		}
		out = append(out, Found{ID: id, Name: name, Model: kv["md"], Addr: net.JoinHostPort(ip.String(), strconv.Itoa(port))})
	}
	return out
}
