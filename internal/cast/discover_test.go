package cast

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
)

func name(n string) []byte {
	var b []byte
	for _, l := range strings.Split(n, ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	return append(b, 0)
}

func record(n string, typ uint16, rdata []byte) []byte {
	b := name(n)
	var h [10]byte
	binary.BigEndian.PutUint16(h[0:], typ)
	binary.BigEndian.PutUint16(h[2:], 0x8001)
	binary.BigEndian.PutUint32(h[4:], 120)
	binary.BigEndian.PutUint16(h[8:], uint16(len(rdata)))
	return append(append(b, h[:]...), rdata...)
}

func txt(kv ...string) []byte {
	var b []byte
	for _, s := range kv {
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	return b
}

// An answer shaped like a Chromecast's: PTR to the instance, SRV with port
// and target, TXT with id/fn/md, A for the target (values invented, address
// from RFC 5737).
func TestParseAnswerOfACastDevice(t *testing.T) {
	inst := "Chromecast-abc123._googlecast._tcp.local"
	srv := []byte{0, 0, 0, 0, 0x1f, 0x49} // priority, weight, port 8009
	srv = append(srv, name("abc123.local")...)
	var p []byte
	h := make([]byte, 12)
	binary.BigEndian.PutUint16(h[2:], 0x8400)
	binary.BigEndian.PutUint16(h[6:], 4)
	p = append(p, h...)
	p = append(p, record(service, 12, name(inst))...)
	p = append(p, record(inst, 33, srv)...)
	p = append(p, record(inst, 16, txt("id=abc123", "fn=Wohnzimmer", "md=Chromecast Ultra", "ve=05"))...)
	p = append(p, record("abc123.local", 1, []byte{192, 0, 2, 77})...)
	got := parseAnswer(p, net.ParseIP("192.0.2.60"))
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	want := Found{ID: "abc123", Name: "Wohnzimmer", Model: "Chromecast Ultra", Addr: "192.0.2.77:8009"}
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
}

func TestParseAnswerIgnoresOtherServicesAndBrokenPackets(t *testing.T) {
	inst := "Speaker._airplay._tcp.local"
	var p []byte
	h := make([]byte, 12)
	binary.BigEndian.PutUint16(h[6:], 2)
	p = append(p, h...)
	p = append(p, record("_airplay._tcp.local", 12, name(inst))...)
	p = append(p, record(inst, 16, txt("id=x"))...)
	if got := parseAnswer(p, net.ParseIP("192.0.2.5")); len(got) != 0 {
		t.Fatalf("AirPlay answer taken as Cast: %+v", got)
	}
	for i := 0; i < len(p); i++ {
		parseAnswer(p[:i], nil) // must not panic on a cut packet
	}
}
