package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	for in, want := range map[string]string{"EE249": "ee249", "demo_store": "demo-store", "shop_a": "shop-a", "ee249-qa": "ee249-qa"} {
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
	dir := filepath.Join(t.TempDir(), "Shop_A")
	writeFile(t, filepath.Join(dir, ".ddev", "config.yaml"), "type: magento2\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "shop-a" {
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

func TestLoadWebsites(t *testing.T) {
	s := load(t, "md-multi")
	if strings.Join(s.Hostnames, ",") != "shop.ddev.site,b2b.shop.ddev.site,fr.shop.ddev.site" {
		t.Fatalf("hostnames %v", s.Hostnames)
	}
	if len(s.Websites) != 3 || s.Websites[1] != (Website{Host: "b2b.shop.ddev.site", Code: "b2b", Type: "website"}) || s.Websites[2].Type != "store" {
		t.Fatalf("websites %+v", s.Websites)
	}
}

func TestLoadWebsitesInvalidType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".md", "config.yaml"), "name: a\nwebsites:\n  - host: x.test\n    code: x\n    type: group\n")
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "type") {
		t.Fatalf("err %v", err)
	}
}

func loadWebsites(t *testing.T, entries string) (Site, error) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".md", "config.yaml"), "name: a\nwebsites:\n"+entries)
	return Load(dir)
}

func TestLoadWebsitesValidation(t *testing.T) {
	bad := map[string]string{
		"semicolon host": "  - host: a;b.ddev.site\n    code: x\n",
		"space host":     "  - host: \"a b.ddev.site\"\n    code: x\n",
		"no dot host":    "  - host: localhost\n    code: x\n",
		"leading dash":   "  - host: -a.ddev.site\n    code: x\n",
		"quote host":     "  - host: 'a\"b.ddev.site'\n    code: x\n",
		"bad code":       "  - host: a.ddev.site\n    code: \"x;y\"\n",
		"upper code":     "  - host: a.ddev.site\n    code: Main\n",
		"dash code":      "  - host: a.ddev.site\n    code: a-b\n",
		"duplicate":      "  - host: a.ddev.site\n    code: x\n  - host: A.ddev.site\n    code: y\n",
	}
	for name, e := range bad {
		_, err := loadWebsites(t, e)
		if err == nil || !strings.Contains(err.Error(), "websites[") {
			t.Errorf("%s: err %v", name, err)
		}
	}
}

func TestLoadWebsitesNamesOffendingEntry(t *testing.T) {
	_, err := loadWebsites(t, "  - host: ok.ddev.site\n    code: x\n  - host: a;b.ddev.site\n    code: y\n")
	if err == nil || !strings.Contains(err.Error(), "websites[1]") || !strings.Contains(err.Error(), "a;b.ddev.site") {
		t.Fatalf("%v", err)
	}
}

func TestLoadWebsitesLowercasesHost(t *testing.T) {
	s, err := loadWebsites(t, "  - host: Shop.DDEV.site\n    code: shop_2\n")
	if err != nil || s.Websites[0].Host != "shop.ddev.site" || s.Hostnames[1] != "shop.ddev.site" {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestLoadMDVarnishForms(t *testing.T) {
	for body, want := range map[string]string{"varnish: true\n": "6.0", "varnish: \"7.4\"\n": "7.4", "varnish: 6.0\n": "6.0", "varnish: false\n": "", "php: \"8.3\"\n": ""} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".md", "config.yaml"), body)
		s, err := Load(dir)
		if err != nil || s.Varnish != want {
			t.Fatalf("%q: %v %q", body, err, s.Varnish)
		}
	}
}

func TestLoadMDElasticsearch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".md", "config.yaml"), "search_engine: elasticsearch\nsearch: \"7.17.14\"\n")
	s, err := Load(dir)
	if err != nil || s.SearchEngine != "elasticsearch" || s.SearchKey() != "elasticsearch:7.17.14" {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestLoadMDPhpMyAdmin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".md", "config.yaml"), "phpmyadmin: true\n")
	s, err := Load(dir)
	if err != nil || !s.PhpMyAdmin {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestLoadMDElasticsearchDefaultVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".md", "config.yaml"), "search_engine: elasticsearch\n")
	s, err := Load(dir)
	if err != nil || s.SearchVersion != "7.17.14" || s.SearchKey() != "elasticsearch:7.17.14" {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestLoadMDSeesLeftoverDDEVMutagen(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".md", "config.yaml"), "php: \"8.3\"\n")
	if s, _ := Load(dir); s.HasAddon("mutagen") {
		t.Fatal("mutagen without ddev")
	}
	writeFile(t, filepath.Join(dir, ".ddev", "config.yaml"), "name: a\nperformance_mode: mutagen\n")
	if s, _ := Load(dir); !s.HasAddon("mutagen") {
		t.Fatal("mutagen not detected")
	}
}
