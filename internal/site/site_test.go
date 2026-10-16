package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, name string) Site {
	t.Helper()
	s, err := Load(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLoadEE249QA(t *testing.T) {
	s := load(t, "ee249-qa")
	if s.Name != "ee249-qa" || s.PHPVersion != "8.5" || s.DBType != "mariadb" || s.DBVersion != "10.6" {
		t.Fatalf("got %+v", s)
	}
	if s.Hostnames[0] != "ee249-qa.ddev.site" || s.SearchVersion != "latest" || s.NodeVersion != "24" || s.Webserver != "nginx-fpm" {
		t.Fatalf("got %+v", s)
	}
	if !has(s.Addons, "redis") || !has(s.Addons, "opensearch") {
		t.Fatalf("addons %v", s.Addons)
	}
}

func TestLoadPfsm248(t *testing.T) {
	s := load(t, "pfsm248")
	if s.DBVersion != "11.4" || s.SearchVersion != "3.0.0" {
		t.Fatalf("got %+v", s)
	}
}

func TestLoadHostnames(t *testing.T) {
	s := load(t, "site-apache")
	want := []string{"site-apache.ddev.site", "local.example.test"}
	if strings.Join(s.Hostnames, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v", s.Hostnames)
	}
	if s.SearchVersion != "" {
		t.Fatalf("search %q", s.SearchVersion)
	}
}

func TestLoadAdditionalHostnames(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ddev"), 0o755)
	os.WriteFile(filepath.Join(dir, ".ddev", "config.yaml"), []byte("name: a\ntype: magento2\nadditional_hostnames:\n  - b\nadditional_fqdns:\n  - c.test\nperformance_mode: mutagen\n"), 0o644)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.Hostnames, ",") != "a.ddev.site,b.ddev.site,c.test" {
		t.Fatalf("got %v", s.Hostnames)
	}
	if !has(s.Addons, "mutagen") {
		t.Fatalf("addons %v", s.Addons)
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("want error")
	}
}

func TestSupported(t *testing.T) {
	cases := []struct {
		name, want string
	}{{"site-apache", "apache"}, {"site-es", "elasticsearch"}}
	for _, c := range cases {
		err := Supported(load(t, c.name))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
	if err := Supported(load(t, "ee249-qa")); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"varnish", "rabbitmq"} {
		err := Supported(Site{Type: "magento2", Webserver: "nginx-fpm", Addons: []string{a}})
		if err == nil || !strings.Contains(err.Error(), a) {
			t.Fatalf("%s: %v", a, err)
		}
	}
	if err := Supported(Site{Type: "drupal10", Webserver: "nginx-fpm"}); err == nil {
		t.Fatal("want type error")
	}
}

func TestFindByCwd(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("testdata", "ee249-qa"))
	cwd := filepath.Join(root, "app", "code")
	s, err := Find("", cwd, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Root != root {
		t.Fatalf("root %q", s.Root)
	}
}

func TestFindByCwdNotFound(t *testing.T) {
	if _, err := Find("", t.TempDir(), nil); err == nil {
		t.Fatal("want error")
	}
}

func TestFindByName(t *testing.T) {
	www, _ := filepath.Abs(filepath.Join("testdata", "www"))
	s, err := Find("demo-store", "/", []string{www})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "demo-store" || !strings.HasSuffix(s.Root, "DemoSetup/demo_store") {
		t.Fatalf("got %+v", s)
	}
	_, err = Find("nope", "/", []string{www})
	if err == nil || !strings.Contains(err.Error(), www) {
		t.Fatalf("err %v", err)
	}
}

func has(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

func TestSupportedAllowsMutagen(t *testing.T) {
	if err := Supported(Site{Type: "magento2", Webserver: "nginx-fpm", DBType: "mariadb", Addons: []string{"mutagen"}}); err != nil {
		t.Fatal(err)
	}
}

func TestSupportedRejectsMySQL(t *testing.T) {
	err := Supported(Site{Type: "magento2", Webserver: "nginx-fpm", DBType: "mysql", DBVersion: "8.0"})
	if err == nil || !strings.Contains(err.Error(), "mysql") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadNameDefaultsToDirBase(t *testing.T) {
	for _, cfg := range []string{"type: magento2\n", "name: \"\"\ntype: magento2\n"} {
		root := filepath.Join(t.TempDir(), "my-shop")
		if err := os.MkdirAll(filepath.Join(root, ".ddev"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".ddev", "config.yaml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if s.Name != "my-shop" || s.Hostnames[0] != "my-shop.ddev.site" {
			t.Fatalf("got %+v", s)
		}
	}
}

func TestSupportedRejectsSharedName(t *testing.T) {
	err := Supported(Site{Name: "shared", Type: "magento2", Webserver: "nginx-fpm", DBType: "mariadb", DBVersion: "11.4"})
	if err == nil || !strings.Contains(err.Error(), "shared") {
		t.Fatalf("got %v", err)
	}
}
