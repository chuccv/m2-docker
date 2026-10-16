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
	}{{"site-apache", "apache"}, {"site-es", "rabbitmq"}}
	for _, c := range cases {
		err := Supported(load(t, c.name))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
	if err := Supported(load(t, "ee249-qa")); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"rabbitmq"} {
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

func TestSupportedMySQLAndUnknownDB(t *testing.T) {
	if err := Supported(Site{Type: "magento2", Webserver: "nginx-fpm", DBType: "mysql", DBVersion: "8.0"}); err != nil {
		t.Fatal(err)
	}
	err := Supported(Site{Type: "magento2", Webserver: "nginx-fpm", DBType: "postgres"})
	if err == nil || !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("got %v", err)
	}
}

func TestDBHelpers(t *testing.T) {
	my := load(t, "site-mysql")
	if my.DBType != "mysql" || my.DBVolume() != "site-mysql-mysql" || my.DBClient() != "mysql" || my.DBDump() != "mysqldump" || my.DBVersionFile() != "db_mysql_version.txt" || my.DBLabel() != "mysql_8.0" {
		t.Fatalf("%+v", my)
	}
	ma := load(t, "ee249-qa")
	if ma.DBVolume() != "ee249-qa-mariadb" || ma.DBClient() != "mariadb" || ma.DBDump() != "mariadb-dump" || ma.DBVersionFile() != "db_mariadb_version.txt" || ma.DBLabel() != "mariadb_10.6" {
		t.Fatalf("%+v", ma)
	}
	if (Site{Name: "x"}).DBClient() != "mariadb" {
		t.Fatal("empty type must default to mariadb")
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

func TestLoadVarnishFromAddonImage(t *testing.T) {
	s := load(t, "site-varnish")
	if s.Varnish != "7.4" {
		t.Fatalf("got %q", s.Varnish)
	}
	if load(t, "ee249-qa").Varnish != "" {
		t.Fatal("varnish must be off by default")
	}
}

func TestLoadVarnishDefaultVersion(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ddev"), 0o755)
	os.WriteFile(filepath.Join(dir, ".ddev", "config.yaml"), []byte("name: a\ntype: magento2\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".ddev", "docker-compose.varnish.yaml"), []byte("services: {}\n"), 0o644)
	s, err := Load(dir)
	if err != nil || s.Varnish != "6.0" {
		t.Fatalf("%v %q", err, s.Varnish)
	}
}

func TestLoadElasticsearchFromAddon(t *testing.T) {
	s := load(t, "site-es")
	if s.SearchEngine != "elasticsearch" || s.SearchVersion != "7.17.14" || s.SearchKey() != "elasticsearch:7.17.14" {
		t.Fatalf("%+v", s)
	}
	o := load(t, "ee249-qa")
	if o.SearchEngine != "" || o.SearchKey() != "latest" {
		t.Fatalf("%+v", o)
	}
}

func TestSearchContainerAndAlias(t *testing.T) {
	for key, want := range map[string][2]string{"2.19.1": {"md-opensearch-2.19.1", "opensearch"}, "elasticsearch:7.17.14": {"md-elasticsearch-7.17.14", "elasticsearch"}} {
		if SearchContainer(key) != want[0] || SearchAlias(key) != want[1] {
			t.Fatalf("%s: %s %s", key, SearchContainer(key), SearchAlias(key))
		}
	}
}

func TestSupportedRejectsUnknownSearchEngine(t *testing.T) {
	err := Supported(Site{Type: "magento2", Webserver: "nginx-fpm", DBType: "mariadb", SearchEngine: "solr"})
	if err == nil || !strings.Contains(err.Error(), "solr") {
		t.Fatalf("%v", err)
	}
}

func TestLoadPhpMyAdminFromAddon(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ddev"), 0o755)
	os.WriteFile(filepath.Join(dir, ".ddev", "config.yaml"), []byte("name: a\ntype: magento2\n"), 0o644)
	s, _ := Load(dir)
	if s.PhpMyAdmin {
		t.Fatal("off by default")
	}
	os.WriteFile(filepath.Join(dir, ".ddev", "docker-compose.phpmyadmin.yaml"), []byte("services: {}\n"), 0o644)
	s, _ = Load(dir)
	if !s.PhpMyAdmin {
		t.Fatal("addon not detected")
	}
}
