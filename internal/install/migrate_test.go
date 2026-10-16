package install

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

const (
	mWeb      = "docker exec -u 1000 -w /var/www/html ddev-mig-web "
	mVolLabel = `docker volume inspect -f {{index .Labels "com.md.db"}} mig-mariadb`
	mLeftover = `docker ps -a --filter label=com.docker.compose.project=ddev-mig --format {{.Names}}|{{.Label "com.md.managed"}}`
)

type mfixture struct {
	root, home string
	f          *run.FakeRunner
	ports      config.Ports
	upSites    []site.Site
	upSawMD    bool
	upErr      error
}

func msetup(t *testing.T) *mfixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "mig")
	ddev := filepath.Join(root, ".ddev")
	if err := os.MkdirAll(ddev, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "name: mig\ntype: magento2\nphp_version: \"8.4\"\nwebserver_type: nginx-fpm\nadditional_hostnames:\n  - shop\ndatabase:\n  type: mariadb\n  version: \"11.4\"\nnodejs_version: \"22\"\n"
	os.WriteFile(filepath.Join(ddev, "config.yaml"), []byte(cfg), 0o644)
	os.WriteFile(filepath.Join(ddev, "docker-compose.opensearch.yaml"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(ddev, "docker-compose.redis.yaml"), []byte("x"), 0o644)
	fx := &mfixture{root: root, home: t.TempDir(), f: &run.FakeRunner{Out: map[string]string{}, Err: map[string]error{}}, ports: config.Ports{HTTP: 80, HTTPS: 443}}
	fx.f.Out[mVolLabel] = "mariadb_11.4"
	return fx
}

func (fx *mfixture) migrate() ([]string, error) {
	up := func(s site.Site) error {
		fx.upSites = append(fx.upSites, s)
		fx.upSawMD = site.HasMDConfig(fx.root)
		return fx.upErr
	}
	err := Migrate(fx.root, fx.f, fx.home, fx.ports, up)
	var cs []string
	for _, c := range fx.f.Calls {
		cs = append(cs, strings.Join(c, " "))
	}
	return cs, err
}

func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		info, _ := d.Info()
		if d.IsDir() {
			out[p] = info.ModTime().String()
			return nil
		}
		data, _ := os.ReadFile(p)
		out[p] = info.ModTime().String() + string(data)
		return nil
	})
	return out
}

func TestFromDDEV(t *testing.T) {
	s, err := site.Load(filepath.Join("..", "site", "testdata", "ee249-qa"))
	if err != nil {
		t.Fatal(err)
	}
	c := FromDDEV(s)
	if c.Name != "ee249-qa" || c.Type != "magento2" || c.PHP != "8.5" || c.Database.Type != "mariadb" || c.Database.Version != "10.6" ||
		c.Search != "latest" || c.Node != "24" || !c.Redis || len(c.Hostnames) != 0 {
		t.Fatalf("%+v", c)
	}
}

func TestFromDDEVAdditionalHostnames(t *testing.T) {
	fx := msetup(t)
	s, _ := site.Load(fx.root)
	c := FromDDEV(s)
	if !reflect.DeepEqual(c.Hostnames, []string{"shop.ddev.site"}) {
		t.Fatalf("%v", c.Hostnames)
	}
}

func TestMigrateOrder(t *testing.T) {
	fx := msetup(t)
	cs, err := fx.migrate()
	if err != nil {
		t.Fatal(err)
	}
	if !site.HasMDConfig(fx.root) {
		t.Fatal("md config missing")
	}
	if len(fx.upSites) != 1 || !fx.upSawMD || fx.upSites[0].Name != "mig" || fx.upSites[0].SearchVersion != "latest" {
		t.Fatalf("up %+v md=%v", fx.upSites, fx.upSawMD)
	}
	order := []string{
		"docker ps --format {{.Names}}",
		"docker volume inspect mig-mariadb",
		mWeb + "php bin/magento config:set --lock-env catalog/search/opensearch_index_prefix mig",
		mWeb + "php bin/magento indexer:reindex catalogsearch_fulltext",
		"docker exec ddev-mig-web php bin/magento config:set --lock-env --scope=default web/unsecure/base_url https://mig.ddev.site/",
		"docker exec ddev-mig-web php bin/magento config:set --lock-env --scope=default web/secure/base_url https://mig.ddev.site/",
	}
	last := -1
	for _, o := range order {
		i := idx(cs, o)
		if i <= last {
			t.Fatalf("%q at %d, previous %d\n%s", o, i, last, strings.Join(cs, "\n"))
		}
		last = i
	}
	if _, err := os.Stat(filepath.Join(fx.home, "sites", "mig", ".md-migrated")); err != nil {
		t.Errorf("marker: %v", err)
	}
}

func TestMigrateBaseURLPort(t *testing.T) {
	fx := msetup(t)
	fx.ports.HTTPS = 9443
	cs, err := fx.migrate()
	if err != nil || idx(cs, "web/secure/base_url https://mig.ddev.site:9443/") < 0 {
		t.Fatalf("%v %v", err, cs)
	}
}

func TestMigrateRefusesWhenMDConfigExists(t *testing.T) {
	fx := msetup(t)
	site.WriteMDConfig(fx.root, site.MDConfig{Name: "mig"})
	cs, err := fx.migrate()
	if err == nil || !strings.Contains(err.Error(), "đã migrate") || len(cs) != 0 || len(fx.upSites) != 0 {
		t.Fatalf("err %v calls %v", err, cs)
	}
}

func TestMigrateRefusesWithoutDDEV(t *testing.T) {
	fx := msetup(t)
	os.RemoveAll(filepath.Join(fx.root, ".ddev"))
	cs, err := fx.migrate()
	if err == nil || len(cs) != 0 || site.HasMDConfig(fx.root) {
		t.Fatalf("err %v calls %v", err, cs)
	}
}

func TestMigrateUnsupportedWritesNothing(t *testing.T) {
	fx := msetup(t)
	os.WriteFile(filepath.Join(fx.root, ".ddev", "docker-compose.varnish.yaml"), []byte("x"), 0o644)
	cs, err := fx.migrate()
	if err == nil || !strings.Contains(err.Error(), "varnish") || len(cs) != 0 || site.HasMDConfig(fx.root) || len(fx.upSites) != 0 {
		t.Fatalf("err %v calls %v", err, cs)
	}
}

func TestMigrateGuardFailuresWriteNothing(t *testing.T) {
	cases := map[string]func(*mfixture){
		"leftover ddev": func(fx *mfixture) { fx.f.Out[mLeftover] = "ddev-mig-web|" },
		"no volume":     func(fx *mfixture) { fx.f.Err["docker volume inspect mig-mariadb"] = os.ErrNotExist },
		"db mismatch":   func(fx *mfixture) { fx.f.Out[mVolLabel] = "mariadb_10.6" },
	}
	for name, mod := range cases {
		fx := msetup(t)
		mod(fx)
		_, err := fx.migrate()
		if err == nil || site.HasMDConfig(fx.root) || len(fx.upSites) != 0 || exists(filepath.Join(fx.root, ".md")) {
			t.Fatalf("%s: err %v", name, err)
		}
	}
}

func TestMigrateUpFailureStops(t *testing.T) {
	fx := msetup(t)
	fx.upErr = os.ErrPermission
	cs, err := fx.migrate()
	if err == nil || idx(cs, "indexer:reindex") >= 0 || idx(cs, "base_url") >= 0 {
		t.Fatalf("err %v calls %v", err, cs)
	}
}

func TestMigrateNeverWritesDDEV(t *testing.T) {
	fx := msetup(t)
	ddev := filepath.Join(fx.root, ".ddev")
	before := snapshotTree(t, ddev)
	if _, err := fx.migrate(); err != nil {
		t.Fatal(err)
	}
	if after := snapshotTree(t, ddev); !reflect.DeepEqual(before, after) {
		t.Fatalf("ddev changed\n%v\n%v", before, after)
	}
}

func TestMigrateRollsBackMDConfigOnFailure(t *testing.T) {
	cases := map[string]func(*mfixture){
		"up": func(fx *mfixture) { fx.upErr = os.ErrPermission },
		"reindex": func(fx *mfixture) {
			fx.f.Err[mWeb+"php bin/magento indexer:reindex catalogsearch_fulltext"] = os.ErrPermission
		},
		"lock": func(fx *mfixture) {
			fx.f.Err["docker exec ddev-mig-web php bin/magento config:set --lock-env --scope=default web/secure/base_url https://mig.ddev.site/"] = os.ErrPermission
		},
	}
	for name, mod := range cases {
		fx := msetup(t)
		mod(fx)
		if _, err := fx.migrate(); err == nil {
			t.Fatalf("%s: expected error", name)
		}
		if site.HasMDConfig(fx.root) || exists(filepath.Join(fx.root, ".md")) {
			t.Fatalf("%s: md config left behind", name)
		}
		if !exists(filepath.Join(fx.root, ".ddev", "config.yaml")) {
			t.Fatalf("%s: .ddev touched", name)
		}
		fx.upErr = nil
		fx.f.Err = map[string]error{}
		if _, err := fx.migrate(); err != nil || !site.HasMDConfig(fx.root) {
			t.Fatalf("%s: second migrate: %v", name, err)
		}
	}
}

func TestMigrateUpFailureBringsStackDown(t *testing.T) {
	fx := msetup(t)
	fx.upErr = os.ErrPermission
	cs, err := fx.migrate()
	down := "docker compose -p md-mig -f " + filepath.Join(fx.home, "sites", "mig", "compose.yaml") + " down"
	if err == nil || idx(cs, down) < 0 || site.HasMDConfig(fx.root) {
		t.Fatalf("err %v calls %v", err, cs)
	}
	for _, c := range cs {
		if strings.Contains(c, " -v") || strings.Contains(c, "volume rm") {
			t.Fatalf("destructive call %q", c)
		}
	}
}

func TestMigrateDownFailureIncludedInError(t *testing.T) {
	fx := msetup(t)
	fx.upErr = os.ErrPermission
	fx.f.Err["docker compose -p md-mig -f "+filepath.Join(fx.home, "sites", "mig", "compose.yaml")+" down"] = os.ErrClosed
	_, err := fx.migrate()
	if err == nil || !strings.Contains(err.Error(), os.ErrClosed.Error()) || !strings.Contains(err.Error(), os.ErrPermission.Error()) {
		t.Fatalf("%v", err)
	}
}

func TestMigrateWriteFailureLeavesNothing(t *testing.T) {
	fx := msetup(t)
	os.WriteFile(filepath.Join(fx.root, ".md"), []byte("x"), 0o644)
	cs, err := fx.migrate()
	if err == nil || len(fx.upSites) != 0 || idx(cs, " down") >= 0 {
		t.Fatalf("err %v calls %v", err, cs)
	}
	if data, _ := os.ReadFile(filepath.Join(fx.root, ".md")); string(data) != "x" {
		t.Fatal(".md file altered")
	}
}

func TestMigrateWarnsDDEVNginx(t *testing.T) {
	fx := msetup(t)
	os.MkdirAll(filepath.Join(fx.root, ".ddev", "nginx_full"), 0o755)
	os.WriteFile(filepath.Join(fx.root, ".ddev", "nginx_full", "x.conf"), []byte("x"), 0o644)
	rd, wr, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = wr
	_, err := fx.migrate()
	os.Stderr = old
	wr.Close()
	out, _ := io.ReadAll(rd)
	if err != nil || !strings.Contains(string(out), "nginx custom của DDEV") {
		t.Fatalf("err %v out %q", err, out)
	}
}

func TestMigrateRollbackKeepsPreexistingMDDir(t *testing.T) {
	fx := msetup(t)
	os.MkdirAll(filepath.Join(fx.root, ".md"), 0o755)
	fx.upErr = os.ErrPermission
	fx.migrate()
	if site.HasMDConfig(fx.root) || !exists(filepath.Join(fx.root, ".md")) {
		t.Fatal("md dir should stay, config removed")
	}
}
