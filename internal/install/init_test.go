package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

type initFixture struct {
	root, home string
	f          *run.FakeRunner
	upCalls    []site.Site
	upErr      error
	opts       InitOptions
}

func initSetup(t *testing.T, lock string) *initFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "shop")
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if lock != "" {
		if err := os.WriteFile(filepath.Join(root, "composer.lock"), []byte(lock), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fx := &initFixture{root: root, home: t.TempDir(), f: &run.FakeRunner{Out: map[string]string{}, Err: map[string]error{}}}
	fx.f.Err["docker volume inspect shop-mariadb"] = os.ErrNotExist
	fx.opts = InitOptions{Root: root}
	fx.opts.Up = func(s site.Site) error { fx.upCalls = append(fx.upCalls, s); return fx.upErr }
	return fx
}

const (
	ceLock = `{"packages":[{"name":"x/y","version":"1"},{"name":"magento/product-community-edition","version":"2.4.7-p3"}]}`
	eeLock = `{"packages":[{"name":"magento/product-enterprise-edition","version":"2.4.8"}]}`
)

func (fx *initFixture) calls() string {
	var cs []string
	for _, c := range fx.f.Calls {
		cs = append(cs, strings.Join(c, " "))
	}
	return strings.Join(cs, "\n")
}

func TestInitDetectsVersionAndEdition(t *testing.T) {
	for lock, want := range map[string][2]string{ceLock: {"2.4.7-p3", "community"}, eeLock: {"2.4.8", "enterprise"}} {
		fx := initSetup(t, lock)
		s, err := Init(fx.opts, fx.f, fx.home)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := os.ReadFile(filepath.Join(fx.root, ".md", "config.yaml"))
		if err != nil || !strings.Contains(string(cfg), want[0]) || !strings.Contains(string(cfg), want[1]) {
			t.Fatalf("config %s %v", cfg, err)
		}
		if s.Name != "shop" || len(fx.upCalls) != 1 || !strings.Contains(fx.calls(), "docker volume create --label com.md.db=") {
			t.Fatalf("site %+v up %d calls %s", s, len(fx.upCalls), fx.calls())
		}
	}
}

func TestInitOverrides(t *testing.T) {
	fx := initSetup(t, ceLock)
	fx.opts.PHP, fx.opts.DBType, fx.opts.DB, fx.opts.Varnish = "8.2", "mysql", "8.0", true
	fx.f.Err["docker volume inspect shop-mysql"] = os.ErrNotExist
	s, err := Init(fx.opts, fx.f, fx.home)
	if err != nil {
		t.Fatal(err)
	}
	if s.PHPVersion != "8.2" || s.DBType != "mysql" || s.DBVersion != "8.0" || s.Varnish == "" {
		t.Fatalf("site %+v", s)
	}
}

func TestInitInvalidOverride(t *testing.T) {
	fx := initSetup(t, ceLock)
	fx.opts.SearchEngine = "elasticsearch"
	if _, err := Init(fx.opts, fx.f, fx.home); err == nil || !strings.Contains(err.Error(), "--search") {
		t.Fatalf("err %v", err)
	}
	fx.opts.SearchEngine, fx.opts.DBType = "", "oracle"
	if _, err := Init(fx.opts, fx.f, fx.home); err == nil || !strings.Contains(err.Error(), "database type") {
		t.Fatalf("err %v", err)
	}
}

func TestInitRefusals(t *testing.T) {
	fx := initSetup(t, ceLock)
	os.MkdirAll(filepath.Join(fx.root, ".md"), 0o755)
	os.WriteFile(filepath.Join(fx.root, ".md", "config.yaml"), []byte("name: shop\n"), 0o644)
	if _, err := Init(fx.opts, fx.f, fx.home); err == nil || !strings.Contains(err.Error(), "md up") {
		t.Fatalf("err %v", err)
	}
	fx = initSetup(t, ceLock)
	os.MkdirAll(filepath.Join(fx.root, ".ddev"), 0o755)
	if _, err := Init(fx.opts, fx.f, fx.home); err == nil || !strings.Contains(err.Error(), "md migrate") {
		t.Fatalf("err %v", err)
	}
	blank := filepath.Join(t.TempDir(), "blank")
	os.MkdirAll(blank, 0o755)
	fx.opts.Root = blank
	if _, err := Init(fx.opts, fx.f, fx.home); err == nil || !strings.Contains(err.Error(), "md install") {
		t.Fatalf("err %v", err)
	}
	fx = initSetup(t, ceLock)
	fx.f.Err["docker volume inspect shop-mariadb"] = nil
	if _, err := Init(fx.opts, fx.f, fx.home); err == nil || !strings.Contains(err.Error(), "đã tồn tại") {
		t.Fatalf("err %v", err)
	}
}

func TestInitNeedsVersionWithoutLock(t *testing.T) {
	fx := initSetup(t, "")
	if _, err := Init(fx.opts, fx.f, fx.home); err == nil || !strings.Contains(err.Error(), "--version") {
		t.Fatalf("err %v", err)
	}
	fx.opts.Version = "2.4.8"
	if _, err := Init(fx.opts, fx.f, fx.home); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(filepath.Join(fx.root, ".md", "config.yaml"))
	if !strings.Contains(string(cfg), "community") {
		t.Fatalf("config %s", cfg)
	}
}

func TestInitRollbackOnUpFailure(t *testing.T) {
	fx := initSetup(t, ceLock)
	fx.upErr = errors.New("boom")
	if _, err := Init(fx.opts, fx.f, fx.home); err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "docker volume rm shop-mariadb") {
		t.Fatalf("err %v", err)
	}
	if site.HasMDConfig(fx.root) || exists(filepath.Join(fx.root, ".md")) {
		t.Fatal("config not rolled back")
	}
}
