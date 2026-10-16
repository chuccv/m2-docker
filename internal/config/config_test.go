package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileDefaults(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142}
	if c.Ports != want {
		t.Fatalf("got %+v", c.Ports)
	}
}

func TestLoadPartialFile(t *testing.T) {
	d := t.TempDir()
	y := "ports:\n  http: 9080\n  xhgui_https: 9142\n"
	if err := os.WriteFile(filepath.Join(d, "config.yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(d)
	if err != nil {
		t.Fatal(err)
	}
	want := Ports{HTTP: 9080, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 9142}
	if c.Ports != want {
		t.Fatalf("got %+v", c.Ports)
	}
}

func TestLoadFullFile(t *testing.T) {
	d := t.TempDir()
	y := "ports:\n  http: 9080\n  https: 9443\n  mailpit_http: 9025\n  mailpit_https: 9026\n  xhgui_http: 9143\n  xhgui_https: 9142\n"
	if err := os.WriteFile(filepath.Join(d, "config.yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(d)
	if err != nil {
		t.Fatal(err)
	}
	want := Ports{HTTP: 9080, HTTPS: 9443, MailpitHTTP: 9025, MailpitHTTPS: 9026, XHGuiHTTP: 9143, XHGuiHTTPS: 9142}
	if c.Ports != want {
		t.Fatalf("got %+v", c.Ports)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "config.yaml"), []byte("ports: [1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(d); err == nil {
		t.Fatal("want error")
	}
}

func TestHome(t *testing.T) {
	t.Setenv("HOME", "/tmp/hx")
	h, err := Home()
	if err != nil || h != "/tmp/hx/.md" {
		t.Fatalf("got %q %v", h, err)
	}
}
