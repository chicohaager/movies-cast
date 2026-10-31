package tlsconf

import (
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSelfSignedIsCreatedOnceWithAPrivateKey(t *testing.T) {
	dir := t.TempDir()
	c1, err := Load("", "", dir, []net.IP{net.ParseIP("192.0.2.10")}) // RFC 5737
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(c1.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.VerifyHostname("192.0.2.10"); err != nil {
		t.Errorf("not valid for the box's address: %v", err)
	}
	if err := leaf.VerifyHostname("localhost"); err != nil {
		t.Errorf("not valid for localhost: %v", err)
	}
	st, err := os.Stat(filepath.Join(dir, "tls-key.pem"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v, want 0600", err, st)
	}
	c2, err := Load("", "", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(c2.Certificate[0]) != string(c1.Certificate[0]) {
		t.Error("a second start made a new certificate; phones would have to trust it again")
	}
}

func TestOnlyCertOrOnlyKeyIsRefused(t *testing.T) {
	if _, err := Load("cert.pem", "", t.TempDir(), nil); err == nil {
		t.Fatal("TLS_CERT without TLS_KEY accepted")
	}
}
