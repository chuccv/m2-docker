package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chuccv/magento2-docker/internal/run"
	"github.com/chuccv/magento2-docker/internal/site"
)

var s = site.Site{Name: "ee249-qa", Hostnames: []string{"ee249-qa.ddev.site", "shop.example.test"}}

func writeCert(t *testing.T, home string, hosts []string, notAfter time.Time) {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "t"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     hosts,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "certs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(filepath.Join(dir, s.Name+".crt"), pemBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, s.Name+".key"), []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureCertGenerates(t *testing.T) {
	home := t.TempDir()
	r := &run.FakeRunner{}
	crt, key, err := EnsureCert(s, home, r)
	if err != nil {
		t.Fatal(err)
	}
	if crt != filepath.Join(home, "certs", "ee249-qa.crt") || key != filepath.Join(home, "certs", "ee249-qa.key") {
		t.Fatalf("paths %s %s", crt, key)
	}
	want := "mkcert -cert-file " + crt + " -key-file " + key + " ee249-qa.ddev.site shop.example.test"
	if len(r.Calls) != 1 || strings.Join(r.Calls[0], " ") != want {
		t.Fatalf("calls %v", r.Calls)
	}
	if st, err := os.Stat(filepath.Join(home, "certs")); err != nil || !st.IsDir() {
		t.Fatal("certs dir missing")
	}
}

func TestEnsureCertSkipsValid(t *testing.T) {
	home := t.TempDir()
	writeCert(t, home, s.Hostnames, time.Now().Add(90*24*time.Hour))
	r := &run.FakeRunner{}
	if _, _, err := EnsureCert(s, home, r); err != nil {
		t.Fatal(err)
	}
	if len(r.Calls) != 0 {
		t.Fatalf("expected skip, calls %v", r.Calls)
	}
}

func TestEnsureCertRegeneratesMissingSAN(t *testing.T) {
	home := t.TempDir()
	writeCert(t, home, s.Hostnames[:1], time.Now().Add(90*24*time.Hour))
	r := &run.FakeRunner{}
	if _, _, err := EnsureCert(s, home, r); err != nil {
		t.Fatal(err)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("expected mkcert, calls %v", r.Calls)
	}
}

func TestEnsureCertRegeneratesExpiring(t *testing.T) {
	home := t.TempDir()
	writeCert(t, home, s.Hostnames, time.Now().Add(10*24*time.Hour))
	r := &run.FakeRunner{}
	if _, _, err := EnsureCert(s, home, r); err != nil {
		t.Fatal(err)
	}
	if len(r.Calls) != 1 {
		t.Fatalf("expected mkcert, calls %v", r.Calls)
	}
}

func TestEnsureDefaultCert(t *testing.T) {
	home := t.TempDir()
	r := &run.FakeRunner{}
	crt, key, err := EnsureDefaultCert(home, r)
	if err != nil {
		t.Fatal(err)
	}
	if crt != filepath.Join(home, "certs", "_default.crt") || key != filepath.Join(home, "certs", "_default.key") {
		t.Fatalf("paths %s %s", crt, key)
	}
	want := "mkcert -cert-file " + crt + " -key-file " + key + " *.md.site md.site *.ddev.site ddev.site"
	if len(r.Calls) != 1 || strings.Join(r.Calls[0], " ") != want {
		t.Fatalf("calls %v", r.Calls)
	}
}

func TestEnsureDefaultCertSkipsValid(t *testing.T) {
	home := t.TempDir()
	old := s
	s = site.Site{Name: "_default"}
	writeCert(t, home, []string{"*.md.site", "md.site", "*.ddev.site", "ddev.site"}, time.Now().Add(90*24*time.Hour))
	s = old
	r := &run.FakeRunner{}
	if _, _, err := EnsureDefaultCert(home, r); err != nil {
		t.Fatal(err)
	}
	if len(r.Calls) != 0 {
		t.Fatalf("expected skip, calls %v", r.Calls)
	}
}
