package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
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
		web + "composer create-project --repository-url=https://repo.magento.com/ magento/project-community-edition=2.4.8 /tmp/md-install",
		web + "cp -a /tmp/md-install/. /var/www/html/",
		web + "php bin/magento setup:install",
		web + "php bin/magento config:set --lock-env web/unsecure/base_url https://shop.ddev.site/",
		web + "php bin/magento config:set --lock-env web/secure/base_url https://shop.ddev.site/",
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

func TestInstallExistingCodeSkipsCreateProject(t *testing.T) {
	fx := setup(t)
	os.MkdirAll(filepath.Join(fx.root, "vendor"), 0o755)
	os.WriteFile(filepath.Join(fx.root, "composer.json"), []byte("{}"), 0o644)
	cs, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	if idx(cs, "create-project") >= 0 || idx(cs, "composer install") >= 0 {
		t.Fatalf("unexpected composer: %v", cs)
	}
}

func TestInstallComposerInstallWhenNoVendor(t *testing.T) {
	fx := setup(t)
	os.WriteFile(filepath.Join(fx.root, "composer.json"), []byte("{}"), 0o644)
	cs, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	if idx(cs, web+"composer install") < 0 || idx(cs, "create-project") >= 0 {
		t.Fatalf("calls: %v", cs)
	}
}

func TestInstallRefusesInstalledSite(t *testing.T) {
	fx := setup(t)
	os.MkdirAll(filepath.Join(fx.root, ".ddev"), 0o755)
	os.WriteFile(filepath.Join(fx.root, ".ddev", "config.yaml"), []byte("name: shop\n"), 0o644)
	os.MkdirAll(filepath.Join(fx.root, "app", "etc"), 0o755)
	os.WriteFile(filepath.Join(fx.root, "app", "etc", "env.php"), []byte("<?php"), 0o644)
	cs, err := fx.run()
	if err == nil || !strings.Contains(err.Error(), "md up") {
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
