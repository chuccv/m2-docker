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
	if s.Hostnames[0] != "ee249-qa.md.site" || s.Hostnames[1] != "ee249-qa.ddev.site" || s.SearchVersion != "latest" || s.NodeVersion != "24" || s.Webserver != "nginx-fpm" {
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
	want := []string{"site-apache.md.site", "site-apache.ddev.site", "local.example.test"}
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
	if strings.Join(s.Hostnames, ",") != "a.md.site,a.ddev.site,b.md.site,c.test" {
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
	if err := Supported(load(t, "site-es")); err != nil {
		t.Fatalf("site-es: %v", err)
	}
	qa := load(t, "ee249-qa")
	if err := Supported(qa); err == nil || !strings.Contains(err.Error(), "latest") {
		t.Fatalf("latest: %v", err)
	}
	qa.SearchVersion = "2.19.1"
	if err := Supported(qa); err != nil {
		t.Fatal(err)
	}
	if err := Supported(load(t, "site-apache")); err != nil {
		t.Fatal(err)
	}
	if err := Supported(Site{Type: "magento2", Webserver: "generic", DBType: "mariadb"}); err == nil || !strings.Contains(err.Error(), "generic") {
		t.Fatalf("generic: %v", err)
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
		if s.Name != "my-shop" || s.Hostnames[0] != "my-shop.md.site" || s.Hostnames[1] != "my-shop.ddev.site" {
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

func TestLoadRabbitMQFromEnvPHP(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".ddev"), 0o755)
	os.MkdirAll(filepath.Join(root, "app", "etc"), 0o755)
	os.WriteFile(filepath.Join(root, ".ddev", "config.yaml"), []byte("name: rq\ntype: magento2\n"), 0o644)
	os.WriteFile(filepath.Join(root, "app", "etc", "env.php"), []byte("<?php\nreturn [\n    'queue' => [\n        'amqp' => [\n            'host' => 'rabbitmq',\n            'port' => '5672',\n            'user' => 'magento',\n            'password' => 'secret',\n            'virtualhost' => '/'\n        ],\n    ],\n];\n"), 0o644)
	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasAddon("rabbitmq") || s.RabbitMQUser != "magento" || s.RabbitMQPass != "secret" || s.RabbitMQVhost != "/" {
		t.Fatalf("%+v", s)
	}
	os.Remove(filepath.Join(root, "app", "etc", "env.php"))
	if s, _ := Load(root); s.HasAddon("rabbitmq") {
		t.Fatalf("addons %v", s.Addons)
	}
	os.WriteFile(filepath.Join(root, ".ddev", "docker-compose.rabbitmq.yaml"), []byte("x"), 0o644)
	if s, _ := Load(root); s.RabbitMQUser != "rabbitmq" || s.RabbitMQPass != "rabbitmq" {
		t.Fatalf("%+v", s)
	}
}

func TestEnvAMQP(t *testing.T) {
	cases := []struct {
		name, php string
		want      map[string]string
	}{
		{"array syntax", `<?php return array('queue' => array('amqp' => array('host' => 'rabbitmq', 'user' => 'u', 'password' => 'p(x]', 'virtualhost' => '/v')));`,
			map[string]string{"host": "rabbitmq", "user": "u", "password": "p(x]", "virtualhost": "/v"}},
		{"nested and double quotes", "<?php return ['queue' => [\"amqp\" => ['host' => \"rabbitmq\", 'ssl_options' => ['verify' => false], 'user' => 'it\\'s', 'virtualhost' => 'v']]];",
			map[string]string{"host": "rabbitmq", "user": "it's", "virtualhost": "v"}},
		{"commented out", "<?php return [\n'queue' => [\n// 'amqp' => ['host' => 'rabbitmq'],\n/* 'amqp' => ['host' => 'rabbitmq'], */\n'consumers_wait_for_messages' => 1]];", map[string]string{}},
		{"outside queue", `<?php return ['other' => ['amqp' => ['host' => 'rabbitmq']], 'queue' => []];`, map[string]string{}},
	}
	for _, c := range cases {
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "app", "etc"), 0o755)
		os.WriteFile(filepath.Join(root, "app", "etc", "env.php"), []byte(c.php), 0o644)
		got := envAMQP(root)
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: %s = %q, want %q (%v)", c.name, k, got[k], v, got)
			}
		}
		if len(c.want) == 0 && len(got) != 0 {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestLoadIgnoresRemoteAMQP(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".ddev"), 0o755)
	os.MkdirAll(filepath.Join(root, "app", "etc"), 0o755)
	os.WriteFile(filepath.Join(root, ".ddev", "config.yaml"), []byte("name: rq\ntype: magento2\n"), 0o644)
	os.WriteFile(filepath.Join(root, "app", "etc", "env.php"), []byte(`<?php return ['queue' => ['amqp' => ['host' => 'mq.example.com', 'user' => 'x']]];`), 0o644)
	if s, _ := Load(root); s.HasAddon("rabbitmq") {
		t.Fatalf("addons %v", s.Addons)
	}
}
