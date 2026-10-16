package router

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/site"
)

var update = flag.Bool("update", false, "update golden files")

var ports = config.Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142}

var testSite = site.Site{Name: "ee249-qa", Hostnames: []string{"ee249-qa.ddev.site", "shop.example.test"}}

func TestRenderTraefikSite(t *testing.T) {
	got := RenderTraefikSite(testSite, "/certs/ee249-qa.crt", "/certs/ee249-qa.key", ports)
	path := filepath.Join("testdata", "site-ee249-qa.yaml")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatalf("mismatch\n%s", got)
	}
}

func TestWriteSite(t *testing.T) {
	home := t.TempDir()
	if err := WriteSite(testSite, "/certs/a.crt", "/certs/a.key", home, ports); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, "router", "dynamic", "ee249-qa.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(RenderTraefikSite(testSite, "/certs/a.crt", "/certs/a.key", ports)) {
		t.Fatal("content mismatch")
	}
}

func TestRenderDefaultCert(t *testing.T) {
	got := RenderDefaultCert("/certs/_default.crt", "/certs/_default.key")
	path := filepath.Join("testdata", "default-cert.yaml")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatalf("mismatch\n%s", got)
	}
}

func TestWriteDefaultCert(t *testing.T) {
	home := t.TempDir()
	if err := WriteDefaultCert("/certs/_default.crt", "/certs/_default.key", home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "router", "dynamic", "_default.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestRenderTraefikSiteVarnish(t *testing.T) {
	s := testSite
	s.Varnish = "6.0"
	got := RenderTraefikSite(s, "/certs/ee249-qa.crt", "/certs/ee249-qa.key", ports)
	path := filepath.Join("testdata", "site-varnish.yaml")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatalf("mismatch\n%s", got)
	}
}
