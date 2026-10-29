package dlna

// Wake-on-LAN for a TV in standby. A TV that is off answers neither SSDP
// nor its description URL, so its MAC address has to be learned while it
// is on: the host's ARP table holds it right after the description was
// fetched (on .143 the LG was there as REACHABLE, 2026-10-28). Whether a
// TV wakes from a magic packet depends on the TV and its settings (LG:
// "Mobile TV On"); that is measured per TV, not assumed here.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

// ARPFile is the kernel's ARP table; a container on the host network sees the host's.
var ARPFile = "/proc/net/arp"

// MACOf returns the MAC address the ARP table holds for ip, or "".
func MACOf(ip string) string {
	f, err := os.Open(ARPFile)
	if err != nil {
		return ""
	}
	defer f.Close()
	return macFromARP(f, ip)
}

// macFromARP reads /proc/net/arp lines:
// "IP address  HW type  Flags  HW address  Mask  Device". Flags 0x0 is an
// incomplete entry (no answer), its address is all zeros.
func macFromARP(r io.Reader, ip string) string {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 4 || f[0] != ip || f[2] == "0x0" {
			continue
		}
		if hw, err := net.ParseMAC(f[3]); err == nil && len(hw) == 6 && !bytes.Equal(hw, make([]byte, 6)) {
			return hw.String()
		}
	}
	return ""
}

// MagicPacket is 6×0xFF followed by the MAC 16 times.
func MagicPacket(mac string) ([]byte, error) {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 {
		return nil, fmt.Errorf("wake: bad MAC address %q", mac)
	}
	p := bytes.Repeat([]byte{0xff}, 6)
	for range 16 {
		p = append(p, hw...)
	}
	return p, nil
}

// WakeTargets are where the packet goes for a TV at host: the limited
// broadcast and the directed broadcast of every own IPv4 network that holds
// the host (some routers and TVs see only one of the two), port 9.
func WakeTargets(host string) []string {
	out := []string{"255.255.255.255:9"}
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return out
	}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.To4() == nil || !n.Contains(ip) {
			continue
		}
		b := make(net.IP, 4)
		for i := range 4 {
			b[i] = n.IP.To4()[i] | ^n.Mask[len(n.Mask)-4+i]
		}
		out = append(out, net.JoinHostPort(b.String(), "9"))
	}
	return out
}

// Wake sends the magic packet for mac to every target. It succeeds when at
// least one send went out; sent is not woken — the caller searches again.
func Wake(ctx context.Context, mac string, targets []string) error {
	p, err := MagicPacket(mac)
	if err != nil {
		return err
	}
	var lc net.ListenConfig
	c, err := lc.ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return err
	}
	defer c.Close()
	var errs []error
	sent := 0
	for _, t := range targets {
		a, err := net.ResolveUDPAddr("udp4", t)
		if err == nil {
			_, err = c.WriteTo(p, a)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t, err))
			continue
		}
		sent++
	}
	if sent == 0 {
		return errors.Join(append([]error{errors.New("wake: no packet sent")}, errs...)...)
	}
	return nil
}
