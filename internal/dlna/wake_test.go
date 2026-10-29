package dlna

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestMACFromARP(t *testing.T) {
	arp := `IP address       HW type     Flags       HW address            Mask     Device
192.0.2.10       0x1         0x2         02:00:00:00:00:01     *        eth0
192.0.2.20        0x1         0x0         00:00:00:00:00:00     *        eth0
192.0.2.30         0x1         0x2         02:00:00:00:00:09     *        eth0
`
	cases := map[string]string{"192.0.2.10": "02:00:00:00:00:01", "192.0.2.20": "", "192.0.2.2": "", "192.0.2.1": ""}
	for ip, want := range cases {
		if got := macFromARP(strings.NewReader(arp), ip); got != want {
			t.Errorf("%s: %q, want %q", ip, got, want)
		}
	}
}

func TestMagicPacket(t *testing.T) {
	p, err := MagicPacket("02:00:00:00:00:01")
	if err != nil || len(p) != 102 || !bytes.Equal(p[:6], bytes.Repeat([]byte{0xff}, 6)) {
		t.Fatalf("%d bytes, %v", len(p), err)
	}
	for i := 6; i < 102; i += 6 {
		if !bytes.Equal(p[i:i+6], []byte{2, 0, 0, 0, 0, 1}) {
			t.Fatalf("repeat at %d: % x", i, p[i:i+6])
		}
	}
	if _, err := MagicPacket("not a mac"); err == nil {
		t.Fatal("bad MAC accepted")
	}
}

// Wake sends the packet to every target; one unusable target does not stop it.
func TestWakeSends(t *testing.T) {
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := Wake(context.Background(), "02:00:00:00:00:01", []string{"bad target", c.LocalAddr().String()}); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 200)
	n, _, err := c.ReadFrom(buf)
	want, _ := MagicPacket("02:00:00:00:00:01")
	if err != nil || !bytes.Equal(buf[:n], want) {
		t.Fatalf("got %d bytes, %v", n, err)
	}
	if err := Wake(context.Background(), "02:00:00:00:00:01", []string{"bad target"}); err == nil {
		t.Fatal("no packet sent, but no error")
	}
}

func TestWakeTargets(t *testing.T) {
	got := WakeTargets("127.0.0.5")
	if got[0] != "255.255.255.255:9" || len(got) < 2 || got[1] != "127.255.255.255:9" {
		t.Fatalf("%v", got)
	}
}
