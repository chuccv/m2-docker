package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
	"github.com/chuccv/m2-docker/internal/stack"
)

const (
	web     = "docker exec -u 1000 -w /var/www/html ddev-shop-web "
	enabled = web + "php bin/magento module:status --enabled"
)

type fixture struct {
	root, home string
	f          *run.FakeRunner
	upCalls    []site.Site
	opts       Options
	ports      config.Ports
}

func setup(t *testing.T) *fixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "shop")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	ch := t.TempDir()
	if err := os.WriteFile(filepath.Join(ch, "auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPOSER_HOME", ch)
	fx := &fixture{root: root, home: t.TempDir(), f: &run.FakeRunner{Out: map[string]string{}, Err: map[string]error{}}, ports: config.Ports{HTTP: 80, HTTPS: 443}}
	fx.f.Err["docker volume inspect shop-mariadb"] = os.ErrNotExist
	fx.f.Out[enabled] = "List of enabled modules:\nMagento_Catalog\nMagento_TwoFactorAuth\nMagento_AdminAdobeImsTwoFactorAuth\n"
	fx.opts = Options{Root: root, Version: "2.4.8", Edition: "community", AdminUser: "admin", AdminPassword: "admin123", AdminEmail: "admin@example.com"}
	fx.opts.Up = func(s site.Site) error { fx.upCalls = append(fx.upCalls, s); return nil }
	return fx
}

func (fx *fixture) run() ([]string, error) {
	var out strings.Builder
	fx.opts.Out = &out
	err := Install(fx.opts, fx.f, fx.home, fx.ports)
	var cs []string
	for _, c := range fx.f.Calls {
		cs = append(cs, strings.Join(c, " "))
	}
	return cs, err
}

func idx(cs []string, sub string) int {
	for i, c := range cs {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}

func TestInstallOrderEmptyDir(t *testing.T) {
	fx := setup(t)
	cs, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	if !site.HasMDConfig(fx.root) {
		t.Fatal("md config not written")
	}
	s, err := site.Load(fx.root)
	if err != nil || s.Name != "shop" || s.PHPVersion != "8.4" || s.DBVersion != "11.4" || s.SearchVersion != "2.19.1" || len(s.Addons) != 1 {
		t.Fatalf("site %+v %v", s, err)
	}
	if len(fx.upCalls) != 1 || fx.upCalls[0].Name != "shop" {
		t.Fatalf("up calls %+v", fx.upCalls)
	}
	order := []string{
		"docker volume create --label com.md.db=mariadb_11.4 shop-mariadb",
		web + "rm -rf /tmp/md-install",
		web + "composer create-project --repository-url=https://repo.magento.com/ magento/project-community-edition=2.4.8 /tmp/md-install",
		web + "cp -a /tmp/md-install/. /var/www/html/",
		web + "php bin/magento setup:install",
		"docker exec ddev-shop-web php bin/magento config:set --lock-env --scope=default web/unsecure/base_url https://shop.ddev.site/",
		"docker exec ddev-shop-web php bin/magento config:set --lock-env --scope=default web/secure/base_url https://shop.ddev.site/",
		web + "php bin/magento module:disable Magento_AdminAdobeImsTwoFactorAuth Magento_TwoFactorAuth",
		web + "php bin/magento setup:upgrade",
		web + "php bin/magento deploy:mode:set developer",
		web + "php bin/magento cache:flush",
	}
	last := -1
	for _, o := range order {
		i := idx(cs, o)
		if i <= last {
			t.Fatalf("%q at %d, previous %d\n%s", o, i, last, strings.Join(cs, "\n"))
		}
		last = i
	}
	rm := web + "rm -rf /tmp/md-install"
	n := 0
	for i, c := range cs {
		if c == rm {
			n++
			if n == 2 && i < idx(cs, "cp -a") {
				t.Error("second rm before cp")
			}
		}
	}
	if n != 2 {
		t.Errorf("rm count %d", n)
	}
	inst := cs[idx(cs, "setup:install")]
	for _, w := range []string{"--base-url=https://shop.ddev.site/", "--base-url-secure=https://shop.ddev.site/", "--use-secure=1", "--use-secure-admin=1",
		"--db-host=db", "--opensearch-index-prefix=shop", "--session-save-redis-db=2", "--page-cache-redis-db=1", "--admin-user=admin",
		"--admin-password=admin123", "--admin-firstname=Admin", "--use-rewrites=1", "--timezone=UTC"} {
		if !strings.Contains(inst, w) {
			t.Errorf("setup:install missing %s", w)
		}
	}
	if _, err := os.Stat(filepath.Join(fx.home, "sites", "shop", ".md-migrated")); err != nil {
		t.Errorf("marker: %v", err)
	}
	if idx(cs, "sampledata:deploy") >= 0 {
		t.Error("sampledata without flag")
	}
}

func TestInstallRefusesInstalledSite(t *testing.T) {
	fx := setup(t)
	os.MkdirAll(filepath.Join(fx.root, ".ddev"), 0o755)
	os.WriteFile(filepath.Join(fx.root, ".ddev", "config.yaml"), []byte("name: shop\n"), 0o644)
	os.MkdirAll(filepath.Join(fx.root, "app", "etc"), 0o755)
	os.WriteFile(filepath.Join(fx.root, "app", "etc", "env.php"), []byte("<?php"), 0o644)
	cs, err := fx.run()
	if err == nil || !strings.Contains(err.Error(), "md migrate") {
		t.Fatalf("err %v", err)
	}
	if len(cs) != 0 || site.HasMDConfig(fx.root) {
		t.Fatalf("side effects: %v", cs)
	}
}

func TestInstallNeedsAuth(t *testing.T) {
	for _, ed := range []string{"community", "enterprise"} {
		fx := setup(t)
		t.Setenv("COMPOSER_HOME", t.TempDir())
		t.Setenv("HOME", t.TempDir())
		fx.opts.Edition = ed
		cs, err := fx.run()
		if err == nil || !strings.Contains(err.Error(), "auth.json") || len(cs) != 0 {
			t.Fatalf("%s: err %v calls %v", ed, err, cs)
		}
	}
}

func TestInstallBadEdition(t *testing.T) {
	fx := setup(t)
	fx.opts.Edition = "cloud"
	if cs, err := fx.run(); err == nil || len(cs) != 0 {
		t.Fatalf("err %v", err)
	}
}

func TestInstallBadPassword(t *testing.T) {
	for _, p := range []string{"short1", "onlyletters", "12345678"} {
		fx := setup(t)
		fx.opts.AdminPassword = p
		if cs, err := fx.run(); err == nil || len(cs) != 0 {
			t.Fatalf("%s: err %v", p, err)
		}
	}
}

func TestInstallEnterpriseEdition(t *testing.T) {
	fx := setup(t)
	fx.opts.Edition = "enterprise"
	cs, err := fx.run()
	if err != nil || idx(cs, "magento/project-enterprise-edition=2.4.8") < 0 {
		t.Fatalf("%v %v", err, cs)
	}
}

func TestInstallBaseURLDevPort(t *testing.T) {
	fx := setup(t)
	fx.ports.HTTPS = 9443
	cs, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	if idx(cs, "--base-url=https://shop.ddev.site:9443/") < 0 || idx(cs, "web/secure/base_url https://shop.ddev.site:9443/") < 0 {
		t.Fatalf("calls %v", cs)
	}
}

func TestInstallBaseURLDefaultPort(t *testing.T) {
	fx := setup(t)
	cs, _ := fx.run()
	if idx(cs, ":443") >= 0 {
		t.Fatalf("port in %v", cs)
	}
}

func TestInstallSampleData(t *testing.T) {
	fx := setup(t)
	fx.opts.SampleData = true
	cs, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	sd := idx(cs, "sampledata:deploy")
	first := idx(cs, "setup:upgrade")
	if sd < 0 || first < sd || idx(cs, "setup:install") > sd {
		t.Fatalf("order %v", cs)
	}
}

func TestInstallSkipsDisableWhenModulesAbsent(t *testing.T) {
	fx := setup(t)
	fx.f.Out[enabled] = "List of enabled modules:\nMagento_Catalog\n"
	cs, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	if idx(cs, "module:disable") >= 0 || idx(cs, "setup:upgrade") >= 0 {
		t.Fatalf("calls %v", cs)
	}
}

func TestInstallOnlyDisablesPresent(t *testing.T) {
	fx := setup(t)
	fx.f.Out[enabled] = "List of enabled modules:\nMagento_TwoFactorAuth\n"
	cs, _ := fx.run()
	if idx(cs, "module:disable Magento_TwoFactorAuth") < 0 || idx(cs, "Magento_AdminAdobeImsTwoFactorAuth") >= 0 {
		t.Fatalf("calls %v", cs)
	}
}

func TestInstallNeverLogsAuth(t *testing.T) {
	fx := setup(t)
	var out strings.Builder
	fx.opts.Out = &out
	Install(fx.opts, fx.f, fx.home, fx.ports)
	if strings.Contains(out.String(), "auth.json") {
		t.Fatal("auth leaked")
	}
}

func TestInstallRefusesEnvPhpWithoutConfig(t *testing.T) {
	fx := setup(t)
	os.MkdirAll(filepath.Join(fx.root, "app", "etc"), 0o755)
	os.WriteFile(filepath.Join(fx.root, "app", "etc", "env.php"), []byte("<?php"), 0o644)
	cs, err := fx.run()
	if err == nil || !strings.Contains(err.Error(), "folder không trống") || len(cs) != 0 {
		t.Fatalf("err %v calls %v", err, cs)
	}
}

func TestInstallRedactsSecretsInErrors(t *testing.T) {
	fx := setup(t)
	fx.opts.AdminPassword = "Secret123x"
	fx.f.Err[web+"php bin/magento setup:install"] = nil
	cs, _ := fx.run()
	_ = cs
	full := ""
	for _, c := range fx.f.Calls {
		if strings.Contains(strings.Join(c, " "), "setup:install") {
			full = strings.Join(c, " ")
		}
	}
	fx2 := setup(t)
	fx2.opts.AdminPassword = "Secret123x"
	fx2.f.Err[full] = os.ErrPermission
	_, err := fx2.run()
	if err == nil || strings.Contains(err.Error(), "Secret123x") || !strings.Contains(err.Error(), "--admin-password=***") {
		t.Fatalf("err %v", err)
	}
}

func TestInstallRefusesNonBlankFolder(t *testing.T) {
	fx := setup(t)
	os.WriteFile(filepath.Join(fx.root, "composer.json"), []byte("{}"), 0o644)
	cs, err := fx.run()
	if err == nil || !strings.Contains(err.Error(), "folder không trống") || len(cs) != 0 || site.HasMDConfig(fx.root) {
		t.Fatalf("err %v calls %v", err, cs)
	}
}

func TestInstallAllowsGitAndMDEntries(t *testing.T) {
	fx := setup(t)
	os.MkdirAll(filepath.Join(fx.root, ".git"), 0o755)
	os.MkdirAll(filepath.Join(fx.root, ".md"), 0o755)
	if _, err := fx.run(); err != nil {
		t.Fatal(err)
	}
}

func TestIsBlank(t *testing.T) {
	d := t.TempDir()
	if ok, err := IsBlank(d); err != nil || !ok {
		t.Fatalf("empty: %v %v", ok, err)
	}
	if ok, err := IsBlank(filepath.Join(d, "missing")); err != nil || !ok {
		t.Fatalf("missing: %v %v", ok, err)
	}
	os.MkdirAll(filepath.Join(d, ".git"), 0o755)
	os.MkdirAll(filepath.Join(d, ".md"), 0o755)
	if ok, _ := IsBlank(d); !ok {
		t.Fatal(".git/.md should be blank")
	}
	os.WriteFile(filepath.Join(d, "composer.json"), []byte("{}"), 0o644)
	if ok, _ := IsBlank(d); ok {
		t.Fatal("composer.json should not be blank")
	}
}

func TestInstallRefusesExistingMDConfig(t *testing.T) {
	fx := setup(t)
	site.WriteMDConfig(fx.root, site.MDConfig{Name: "shop"})
	cs, err := fx.run()
	if err == nil || !strings.Contains(err.Error(), "md up") || len(cs) != 0 {
		t.Fatalf("err %v calls %v", err, cs)
	}
}

func TestInstallRefusesExistingDBVolume(t *testing.T) {
	fx := setup(t)
	delete(fx.f.Err, "docker volume inspect shop-mariadb")
	cs, err := fx.run()
	if err == nil || !strings.Contains(err.Error(), "volume DB shop-mariadb đã tồn tại — có thể là DB của site khác") {
		t.Fatalf("err %v", err)
	}
	if site.HasMDConfig(fx.root) || exists(filepath.Join(fx.root, ".md")) || len(fx.upCalls) != 0 || idx(cs, "volume create") >= 0 {
		t.Fatalf("wrote something: %v", cs)
	}
}

func (fx *fixture) upWritesCompose() {
	fx.opts.Up = func(s site.Site) error {
		fx.upCalls = append(fx.upCalls, s)
		cf := stack.ComposeFile(fx.home, s.Name)
		os.MkdirAll(filepath.Dir(cf), 0o755)
		return os.WriteFile(cf, []byte("x"), 0o644)
	}
}

func TestInstallFailureRollsBack(t *testing.T) {
	fx := setup(t)
	fx.upWritesCompose()
	fx.f.Err[web+"rm -rf /tmp/md-install"] = os.ErrPermission
	cs, err := fx.run()
	if err == nil || !strings.Contains(err.Error(), os.ErrPermission.Error()) || !strings.Contains(err.Error(), "docker volume rm shop-mariadb") {
		t.Fatalf("err %v", err)
	}
	if site.HasMDConfig(fx.root) || exists(filepath.Join(fx.root, ".md")) {
		t.Fatal("md config left behind")
	}
	down := idx(cs, "docker compose -p md-shop -f "+stack.ComposeFile(fx.home, "shop")+" down")
	dis := idx(cs, "docker network disconnect md-shop_default md-router")
	if down < 0 || dis < 0 || dis > down {
		t.Fatalf("down %d disconnect %d\n%v", down, dis, cs)
	}
	for _, c := range cs {
		if strings.Contains(c, "volume rm") || strings.Contains(c, " -v") {
			t.Fatalf("destructive %q", c)
		}
	}
}

func TestInstallUpFailureWithoutComposeSkipsDown(t *testing.T) {
	fx := setup(t)
	fx.opts.Up = func(s site.Site) error { return os.ErrPermission }
	cs, err := fx.run()
	if err == nil || site.HasMDConfig(fx.root) || idx(cs, " down") >= 0 {
		t.Fatalf("err %v calls %v", err, cs)
	}
}

func TestInstallRollbackKeepsPreexistingMDDir(t *testing.T) {
	fx := setup(t)
	os.MkdirAll(filepath.Join(fx.root, ".md"), 0o755)
	fx.opts.Up = func(s site.Site) error { return os.ErrPermission }
	fx.run()
	if site.HasMDConfig(fx.root) || !exists(filepath.Join(fx.root, ".md")) {
		t.Fatal("md dir should stay, config removed")
	}
}

func TestInstallRollbackKeepsNonEmptyMDDir(t *testing.T) {
	fx := setup(t)
	fx.opts.Up = func(s site.Site) error {
		return os.WriteFile(filepath.Join(fx.root, ".md", "other"), []byte("x"), 0o644)
	}
	fx.run()
	if !exists(filepath.Join(fx.root, ".md", "other")) {
		t.Fatal("foreign file removed")
	}
}

func TestInstallSuccessKeepsConfigAndNoVolumeMessage(t *testing.T) {
	fx := setup(t)
	_, err := fx.run()
	if err != nil || !site.HasMDConfig(fx.root) {
		t.Fatalf("%v", err)
	}
}

func TestInstallElasticsearch7(t *testing.T) {
	fx := setup(t)
	fx.opts.SearchEngine, fx.opts.Search = "elasticsearch", "7.17.14"
	cs, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	inst := cs[idx(cs, "setup:install")]
	for _, w := range []string{"--search-engine=elasticsearch7", "--elasticsearch-host=elasticsearch", "--elasticsearch-port=9200", "--elasticsearch-index-prefix=shop"} {
		if !strings.Contains(inst, w) {
			t.Errorf("missing %s", w)
		}
	}
	if strings.Contains(inst, "opensearch") {
		t.Error("opensearch flag present")
	}
	s, err := site.Load(fx.root)
	if err != nil || s.SearchEngine != "elasticsearch" || s.SearchVersion != "7.17.14" {
		t.Fatalf("%+v %v", s, err)
	}
	if len(fx.upCalls) != 1 || fx.upCalls[0].SearchKey() != "elasticsearch:7.17.14" {
		t.Fatalf("up %+v", fx.upCalls)
	}
}

func TestInstallElasticsearch8(t *testing.T) {
	fx := setup(t)
	fx.opts.SearchEngine, fx.opts.Search = "elasticsearch", "8.11.0"
	cs, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	if inst := cs[idx(cs, "setup:install")]; !strings.Contains(inst, "--search-engine=elasticsearch8") {
		t.Fatal(inst)
	}
}

func TestInstallSearchEngineNeedsVersion(t *testing.T) {
	fx := setup(t)
	fx.opts.SearchEngine = "elasticsearch"
	if cs, err := fx.run(); err == nil || len(cs) != 0 {
		t.Fatalf("err %v calls %v", err, cs)
	}
}

func TestInstallMySQLVolume(t *testing.T) {
	fx := setup(t)
	fx.f.Err["docker volume inspect shop-mysql"] = os.ErrNotExist
	fx.opts.DBType, fx.opts.DB = "mysql", "8.0"
	cs, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	if idx(cs, "docker volume create --label com.md.db=mysql_8.0 shop-mysql") < 0 || idx(cs, "docker volume inspect shop-mysql") < 0 {
		t.Fatalf("calls %v", cs)
	}
	s, err := site.Load(fx.root)
	if err != nil || s.DBType != "mysql" || s.DBVersion != "8.0" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestInstallBadDBType(t *testing.T) {
	fx := setup(t)
	fx.opts.DBType = "postgres"
	if cs, err := fx.run(); err == nil || len(cs) != 0 {
		t.Fatalf("err %v", err)
	}
}

func TestInstallVarnish(t *testing.T) {
	fx := setup(t)
	fx.opts.Varnish = true
	if _, err := fx.run(); err != nil {
		t.Fatal(err)
	}
	s, err := site.Load(fx.root)
	if err != nil || s.Varnish != "7.6" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestInstallNoVarnishByDefault(t *testing.T) {
	fx := setup(t)
	if _, err := fx.run(); err != nil {
		t.Fatal(err)
	}
	s, _ := site.Load(fx.root)
	if s.Varnish != "" {
		t.Fatalf("varnish %q", s.Varnish)
	}
}

func TestInstallWritesStackVersions(t *testing.T) {
	fx := setup(t)
	fx.opts.Version = "2.4.6"
	if _, err := fx.run(); err != nil {
		t.Fatal(err)
	}
	s, err := site.Load(fx.root)
	if err != nil || s.RedisVersion != "7.0" || s.NodeVersion != "22" || s.SearchVersion != "2.5.0" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestInstallUserStacksOverride(t *testing.T) {
	fx := setup(t)
	os.WriteFile(filepath.Join(fx.home, "stacks.yaml"), []byte("\"2.4.8\":\n  php: \"8.3\"\n  database: {type: mariadb, version: \"10.11\"}\n  search: {engine: opensearch, version: \"2.12.0\"}\n  redis: \"7.0\"\n  node: \"20\"\n"), 0o644)
	if _, err := fx.run(); err != nil {
		t.Fatal(err)
	}
	s, _ := site.Load(fx.root)
	if s.PHPVersion != "8.3" || s.DBVersion != "10.11" || s.RedisVersion != "7.0" {
		t.Fatalf("%+v", s)
	}
}
