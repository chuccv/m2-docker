package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	for in, want := range map[string]string{"EE249": "ee249", "demo_store": "demo-store", "zhik_m2": "zhik-m2", "ee249-qa": "ee249-qa"} {
		if got := NormalizeName(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPrefersMD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".ddev", "config.yaml"), "name: a\ntype: magento2\nphp_version: \"8.1\"\n")
	writeFile(t, filepath.Join(dir, ".ddev", "docker-compose.varnish.yaml"), "")
	writeFile(t, filepath.Join(dir, ".md", "config.yaml"), "name: b\nphp: \"8.4\"\nredis: true\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "b" || s.PHPVersion != "8.4" || strings.Join(s.Addons, ",") != "redis" || s.Webserver != "nginx-fpm" {
		t.Fatalf("got %+v", s)
	}
}

func TestLoadMDDefaults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Demo_Store")
	writeFile(t, filepath.Join(dir, ".md", "config.yaml"), "php: \"8.3\"\nhostnames:\n  - x.test\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "demo-store" || s.NodeVersion != "22" || s.Type != "magento2" {
		t.Fatalf("got %+v", s)
	}
	if strings.Join(s.Hostnames, ",") != "demo-store.ddev.site,x.test" {
		t.Fatalf("hostnames %v", s.Hostnames)
	}
	if len(s.Addons) != 0 || s.SearchVersion != "" {
		t.Fatalf("got %+v", s)
	}
}

func TestLoadMDFixture(t *testing.T) {
	s := load(t, "md-site")
	if s.Name != "md-site" || s.DBType != "mariadb" || s.DBVersion != "10.6" || s.SearchVersion != "2.19.1" || s.NodeVersion != "20" {
		t.Fatalf("got %+v", s)
	}
}

func TestLoadDDEVEmptyNameNormalized(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Zhik_M2")
	writeFile(t, filepath.Join(dir, ".ddev", "config.yaml"), "type: magento2\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "zhik-m2" {
		t.Fatalf("got %q", s.Name)
	}
}

func TestWriteMDConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := MDConfig{Name: "w", Type: "magento2", PHP: "8.3", Search: "2.19.1", Node: "22", Redis: true, Hostnames: []string{"w.test"}}
	c.Database.Type, c.Database.Version = "mariadb", "10.6"
	if err := WriteMDConfig(dir, c); err != nil {
		t.Fatal(err)
	}
	if !HasMDConfig(dir) {
		t.Fatal("HasMDConfig false")
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "w" || s.DBVersion != "10.6" || s.SearchVersion != "2.19.1" || strings.Join(s.Addons, ",") != "redis" {
		t.Fatalf("got %+v", s)
	}
}

func TestFindByCwdMDOnly(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "s1")
	writeFile(t, filepath.Join(dir, ".md", "config.yaml"), "name: s1\n")
	s, err := Find("", filepath.Join(dir, "sub"), nil)
	if err != nil || s.Name != "s1" {
		t.Fatalf("cwd: %v %+v", err, s)
	}
	s, err = Find("s1", "", []string{root})
	if err != nil || s.Name != "s1" {
		t.Fatalf("scan: %v %+v", err, s)
	}
}
