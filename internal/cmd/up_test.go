package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/chuccv/magento2-docker/internal/run"
)

const siteCfg = "name: ee249-qa\ntype: magento2\nphp_version: \"8.5\"\nnodejs_version: \"22.11.0\"\nadditional_hostnames:\n  - shop\ndatabase:\n  type: mariadb\n  version: \"11.4\"\n"

const (
	dbKey     = "docker run --rm -u 1000:1000 --entrypoint cat -v ee249-qa-mariadb:/v:ro mariadb:11.4 /v/db_mariadb_version.txt"
	imgKey    = "docker image inspect -f {{index .Config.Labels \"com.md.hash\"}} md/php:8.5"
	volKey    = "docker volume inspect md-node-22"
	sharedUp  = "docker compose -p md-shared -f %s/shared/compose.yaml up -d --wait"
	siteUpFmt = "docker compose -p md-ee249-qa -f %s/sites/ee249-qa/compose.yaml up -d --wait"
)

func upEnv(t *testing.T) (*env, *run.FakeRunner, string) {
	t.Helper()
	e, f, home, dir := ddevEnv(t)
	mdCfg := "name: ee249-qa\ntype: magento2\nphp: \"8.5\"\nnode: \"22.11.0\"\nsearch: 2.19.1\nhostnames:\n  - shop.ddev.site\ndatabase:\n  type: mariadb\n  version: \"11.4\"\n"
	if err := os.MkdirAll(filepath.Join(dir, ".md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".md", "config.yaml"), []byte(mdCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return e, f, home
}

func ddevEnv(t *testing.T) (*env, *run.FakeRunner, string, string) {
	t.Helper()
	e, f, root := testEnv(t, false)
	sd := filepath.Join(root, "ee249-qa", ".ddev")
	if err := os.WriteFile(filepath.Join(sd, "config.yaml"), []byte(siteCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, ".env.opensearch"), []byte("OPENSEARCH_TAG=2.19.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home, _ := e.home()
	if err := os.MkdirAll(filepath.Join(home, "ca"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "ca", "rootCA.pem"), []byte("ca"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.out = &strings.Builder{}
	h, err := imageHash()
	if err != nil {
		t.Fatal(err)
	}
	f.Out = map[string]string{dbKey: "11.4.2", imgKey: h + "\n"}
	f.Err = map[string]error{}
	return e, f, home, filepath.Join(root, "ee249-qa")
}

func calls(f *run.FakeRunner) []string {
	var out []string
	for _, c := range f.Calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func index(cs []string, sub string) int {
	for i, c := range cs {
		if strings.Contains(c, sub) {
			return i
		}
	}
	return -1
}

func hasPrefixCall(cs []string, p string) bool {
	for _, c := range cs {
		if strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}

func TestUpCallOrder(t *testing.T) {
	e, f, home := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	order := []string{
		"docker ps --format",
		"--entrypoint cat",
		"docker image inspect",
		"docker volume inspect md-node-22",
		"docker compose -p md-shared",
		"mkcert -cert-file",
		"docker compose -p md-ee249-qa",
		"docker network connect --alias opensearch md-ee249-qa_default md-opensearch-2.19.1",
		"docker network connect --alias xhgui md-ee249-qa_default md-xhgui",
		"docker network connect --alias ee249-qa.md.site --alias ee249-qa.ddev.site --alias shop.ddev.site md-ee249-qa_default md-router",
	}
	last := -1
	for _, o := range order {
		i := index(cs, o)
		if i < 0 {
			t.Fatalf("missing call %q in\n%s", o, strings.Join(cs, "\n"))
		}
		if i <= last {
			t.Fatalf("call %q out of order in\n%s", o, strings.Join(cs, "\n"))
		}
		last = i
	}
	if index(cs, strings.Replace(sharedUp, "%s", home, 1)) < 0 || index(cs, strings.Replace(siteUpFmt, "%s", home, 1)) < 0 {
		t.Fatalf("compose up args wrong:\n%s", strings.Join(cs, "\n"))
	}
	for _, p := range []string{"shared/compose.yaml", "sites/ee249-qa/compose.yaml", "router/dynamic/ee249-qa.yaml", "state.json"} {
		if _, err := os.Stat(filepath.Join(home, p)); err != nil {
			t.Fatalf("missing %s", p)
		}
	}
	st, _ := os.ReadFile(filepath.Join(home, "state.json"))
	if !strings.Contains(string(st), "2.19.1") {
		t.Fatalf("state: %s", st)
	}
	if !strings.Contains(e.out.(*strings.Builder).String(), "https://ee249-qa.md.site\n") {
		t.Fatalf("urls: %s", e.out.(*strings.Builder).String())
	}
}

func TestUpGuardFailNoCompose(t *testing.T) {
	e, f, _ := upEnv(t)
	f.Out[dbKey] = "10.6.1"
	if err := exec(t, e, "up"); err == nil {
		t.Fatal("expected guard error")
	}
	for _, c := range calls(f) {
		if strings.HasPrefix(c, "docker volume inspect ") {
			continue
		}
		if strings.HasPrefix(c, "docker compose") || strings.HasPrefix(c, "docker build") || strings.HasPrefix(c, "docker volume") || strings.HasPrefix(c, "docker image") {
			t.Fatalf("unexpected call after guard failure: %s", c)
		}
	}
}

func TestStopNoVolumesFlag(t *testing.T) {
	e, f, home := upEnv(t)
	if err := exec(t, e, "stop"); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	down := index(cs, "docker compose -p md-ee249-qa -f "+home+"/sites/ee249-qa/compose.yaml down")
	if down < 0 {
		t.Fatalf("no down: %v", cs)
	}
	if dis := index(cs, "docker network disconnect"); dis < 0 || dis > down {
		t.Fatalf("disconnect must precede down: %v", cs)
	}
	for _, a := range f.Calls[down] {
		if a == "-v" || a == "--volumes" {
			t.Fatalf("down has %s", a)
		}
	}
}

func TestUpStartsOnlyOwnSearchContainer(t *testing.T) {
	e, f, home := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), strings.Replace(sharedUp, "%s", home, 1)+" md-router md-xhgui md-opensearch-2.19.1") < 0 {
		t.Fatalf("%s", strings.Join(calls(f), "\n"))
	}
}

func TestUpReconnectsSharedToOtherRunningSites(t *testing.T) {
	e, f, home := upEnv(t)
	other := filepath.Join(t.TempDir(), "other")
	os.MkdirAll(filepath.Join(other, ".md"), 0o755)
	os.WriteFile(filepath.Join(other, ".md", "config.yaml"), []byte("name: other\ntype: magento2\nphp: \"8.4\"\nsearch: 2.19.1\nhostnames:\n  - other.ddev.site\ndatabase:\n  type: mariadb\n  version: \"11.4\"\n"), 0o644)
	if err := saveState(home, state{Sites: []siteRef{{Name: "other", Root: other}}}); err != nil {
		t.Fatal(err)
	}
	f.Out["docker compose ls --format json"] = `[{"Name":"md-other","Status":"running(2)"}]`
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	for _, want := range []string{"docker network connect --alias opensearch md-other_default md-opensearch-2.19.1", "docker network connect --alias other.md.site --alias other.ddev.site md-other_default md-router"} {
		if index(cs, want) < 0 {
			t.Fatalf("missing %q:\n%s", want, strings.Join(cs, "\n"))
		}
	}
}

func TestStopStopsSearchWhenUnused(t *testing.T) {
	e, f, _ := upEnv(t)
	if err := exec(t, e, "stop"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), "docker stop md-opensearch-2.19.1") < 0 {
		t.Fatalf("%s", strings.Join(calls(f), "\n"))
	}
}

func TestStopKeepsSearchUsedByRunningSite(t *testing.T) {
	e, f, home := upEnv(t)
	other := filepath.Join(t.TempDir(), "other")
	os.MkdirAll(filepath.Join(other, ".md"), 0o755)
	os.WriteFile(filepath.Join(other, ".md", "config.yaml"), []byte("name: other\ntype: magento2\nphp: \"8.4\"\nsearch: 2.19.1\ndatabase:\n  type: mariadb\n  version: \"11.4\"\n"), 0o644)
	if err := saveState(home, state{Sites: []siteRef{{Name: "other", Root: other}}}); err != nil {
		t.Fatal(err)
	}
	f.Out["docker compose ls --format json"] = `[{"Name":"md-other","Status":"running(2)"}]`
	if err := exec(t, e, "stop"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), "docker stop md-opensearch") >= 0 {
		t.Fatalf("%s", strings.Join(calls(f), "\n"))
	}
}

func TestRestartKeepsSearchRunning(t *testing.T) {
	e, f, _ := upEnv(t)
	if err := exec(t, e, "restart"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), "docker stop md-opensearch") >= 0 {
		t.Fatalf("%s", strings.Join(calls(f), "\n"))
	}
}

func TestUpRebuildsOnHashMismatch(t *testing.T) {
	e, f, _ := upEnv(t)
	f.Out[imgKey] = "stale"
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	h, _ := imageHash()
	b := index(cs, "docker build --label com.md.hash="+h+" --build-arg PHP_VERSION=8.5 -t md/php:8.5 ")
	if b < 0 {
		t.Fatalf("no build:\n%s", strings.Join(cs, "\n"))
	}
	if index(cs, "docker image prune -f --filter label=com.md.image=php") < index(cs, "/sites/ee249-qa/compose.yaml up -d --wait") {
		t.Fatalf("prune must follow site up:\n%s", strings.Join(cs, "\n"))
	}
}

func TestUpSkipsBuildOnHashMatch(t *testing.T) {
	e, f, _ := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if hasPrefixCall(calls(f), "docker build") {
		t.Fatal("unexpected build")
	}
	if index(calls(f), "docker image prune -f --filter label=com.md.image=php") < 0 {
		t.Fatal("old md/php images must be pruned after up")
	}
}

func TestUpCreatesNodeVolume(t *testing.T) {
	e, f, _ := upEnv(t)
	f.Err[volKey] = os.ErrNotExist
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	c := index(cs, "docker volume create md-node-22")
	r := index(cs, "docker run --rm -v md-node-22:/out node:22-slim cp -a /usr/local/. /out/")
	if c < 0 || r < c {
		t.Fatalf("volume create/populate wrong:\n%s", strings.Join(cs, "\n"))
	}
}

func TestUpCreatesMissingRedisVolume(t *testing.T) {
	e, f, _ := upEnv(t)
	s, err := e.findSite("")
	if err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(s.Root, ".md", "config.yaml")
	body, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, append(body, "redis: true\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	f.Err["docker volume inspect ddev-ee249-qa_redis"] = os.ErrNotExist
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	c := index(cs, "docker volume create ddev-ee249-qa_redis")
	u := index(cs, "docker compose -p md-ee249-qa")
	if c < 0 || u < c {
		t.Fatalf("redis volume not created before compose up:\n%s", strings.Join(cs, "\n"))
	}
}

func TestEmptyNodeVersionVolumeMatchesCompose(t *testing.T) {
	e, f, home := upEnv(t)
	s, err := e.findSite("")
	if err != nil {
		t.Fatal(err)
	}
	sd := filepath.Join(s.Root, ".ddev", "config.yaml")
	cfg := strings.Replace(siteCfg, "nodejs_version: \"22.11.0\"\n", "", 1)
	if err := os.WriteFile(sd, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	f.Err[volKey] = os.ErrNotExist
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), "docker volume create md-node-22") < 0 {
		t.Fatalf("volume not created: %v", calls(f))
	}
	body, err := os.ReadFile(filepath.Join(home, "sites", "ee249-qa", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "md-node-22") || strings.Contains(string(body), "md-node-\n") || strings.Contains(string(body), "md-node-:") {
		t.Fatalf("compose volume mismatch:\n%s", body)
	}
}

func TestPoweroffContinuesPastFailure(t *testing.T) {
	e, f, home := upEnv(t)
	st := state{Sites: []siteRef{{Name: "a", Root: "/x"}, {Name: "b", Root: "/y"}}}
	if err := saveState(home, st); err != nil {
		t.Fatal(err)
	}
	f.Out["docker compose ls --format json"] = `[{"Name":"md-a","Status":"running(1)"},{"Name":"md-b","Status":"running(1)"},{"Name":"md-shared","Status":"running(2)"}]`
	f.Err["docker compose -p md-a -f "+home+"/sites/a/compose.yaml down"] = os.ErrInvalid
	err := exec(t, e, "poweroff")
	if err == nil || !strings.Contains(err.Error(), "stop a") {
		t.Fatalf("err %v", err)
	}
	cs := calls(f)
	if index(cs, "-p md-b -f "+home+"/sites/b/compose.yaml down") < 0 || index(cs, "-p md-shared -f "+home+"/shared/compose.yaml down") < 0 {
		t.Fatalf("not resilient:\n%s", strings.Join(cs, "\n"))
	}
}

func TestUpCreatesSharedMountDirs(t *testing.T) {
	e, _, home := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"certs", "router/dynamic"} {
		if _, err := os.Stat(filepath.Join(home, d)); err != nil {
			t.Errorf("%s missing", d)
		}
	}
}

func TestUpRouterUsesContainerCertPaths(t *testing.T) {
	e, _, home := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, "router", "dynamic", "ee249-qa.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "certFile: /certs/ee249-qa.crt") || !strings.Contains(string(b), "keyFile: /certs/ee249-qa.key") {
		t.Fatalf("cert paths not container paths:\n%s", b)
	}
}

func TestUpWritesXhguiConfig(t *testing.T) {
	e, _, home := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, "shared", "xhgui-config.php"))
	if err != nil || !strings.Contains(string(b), "sqlite:/data/xhgui.sqlite") {
		t.Fatalf("xhgui config: %v %s", err, b)
	}
}

func TestUpNodePopulateFailureRemovesVolume(t *testing.T) {
	e, f, _ := upEnv(t)
	f.Err[volKey] = errors.New("no such volume")
	cp := "docker run --rm -v md-node-22:/out node:22-slim cp -a /usr/local/. /out/"
	f.Err[cp] = errors.New("cp failed")
	err := exec(t, e, "up")
	if err == nil || !strings.Contains(err.Error(), "cp failed") {
		t.Fatalf("got %v", err)
	}
	if !hasPrefixCall(calls(f), "docker volume rm md-node-22") {
		t.Fatalf("volume not removed:\n%s", strings.Join(calls(f), "\n"))
	}
}

func TestUpComposeFailureLeavesStateUntouched(t *testing.T) {
	e, f, home := upEnv(t)
	f.Err[strings.Replace(siteUpFmt, "%s", home, 1)] = errors.New("compose boom")
	if err := exec(t, e, "up"); err == nil {
		t.Fatal("want error")
	}
	st, _ := os.ReadFile(filepath.Join(home, "state.json"))
	if strings.Contains(string(st), "ee249-qa") {
		t.Fatalf("state has site: %s", st)
	}
}

func TestInstallCmdRegistered(t *testing.T) {
	e, _, _ := testEnv(t, false)
	root := &cobra.Command{Use: "md"}
	register(root, e)
	c, _, err := root.Find([]string{"install"})
	if err != nil || c.Name() != "install" || c.Flags().Lookup("sample-data") == nil || c.Flags().Lookup("admin-password").DefValue != "admin123" {
		t.Fatalf("install cmd: %v", err)
	}
}

func TestUpRefusesDDEVOnlySite(t *testing.T) {
	e, f, _, _ := ddevEnv(t)
	err := exec(t, e, "up")
	if err == nil || !strings.Contains(err.Error(), "md migrate") {
		t.Fatalf("err %v", err)
	}
	if hasPrefixCall(calls(f), "docker compose") {
		t.Fatalf("compose ran:\n%s", strings.Join(calls(f), "\n"))
	}
}

func TestUpHasNoSkipMigrateFlag(t *testing.T) {
	e, _, _ := upEnv(t)
	if err := exec(t, e, "up", "--skip-migrate"); err == nil {
		t.Fatal("flag should not exist")
	}
}

func TestUpSetsSearchPrefixOnceForInstalledSite(t *testing.T) {
	e, f, _ := upEnv(t)
	s, err := e.findSite("")
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(s.Root, "app", "etc"), 0o755)
	os.WriteFile(filepath.Join(s.Root, "app", "etc", "env.php"), []byte("<?php return [];"), 0o644)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	if index(cs, "opensearch_index_prefix ee249_qa") < 0 || index(cs, "indexer:reindex catalogsearch_fulltext") < 0 {
		t.Fatalf("no prefix migration:\n%s", strings.Join(cs, "\n"))
	}
	f.Calls = nil
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), "indexer:reindex") >= 0 {
		t.Fatal("reindex ran twice")
	}
}

func TestUpSkipsSearchMigrationWithoutEnvPHP(t *testing.T) {
	e, f, _ := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), "indexer:reindex") >= 0 {
		t.Fatalf("migration in up:\n%s", strings.Join(calls(f), "\n"))
	}
}

func TestMigrateCmd(t *testing.T) {
	e, f, home, dir := ddevEnv(t)
	os.MkdirAll(filepath.Join(dir, "app", "etc"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "etc", "env.php"), []byte("<?php"), 0o644)
	if err := exec(t, e, "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".md", "config.yaml")); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	up := index(cs, "docker compose -p md-ee249-qa")
	re := index(cs, "indexer:reindex")
	lock := index(cs, "web/secure/base_url https://ee249-qa.md.site")
	if up < 0 || re < up || lock < up {
		t.Fatalf("order up=%d reindex=%d lock=%d\n%s", up, re, lock, strings.Join(cs, "\n"))
	}
	n := 0
	for _, c := range cs {
		if strings.Contains(c, "config:set --lock-env --scope=default web/secure/base_url") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("base_url set %d times", n)
	}
	if _, err := os.Stat(filepath.Join(home, "sites", "ee249-qa", ".md-migrated")); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateCmdRefusesMigratedSite(t *testing.T) {
	e, f, _ := upEnv(t)
	err := exec(t, e, "migrate")
	if err == nil || !strings.Contains(err.Error(), "already migrated") || len(f.Calls) != 0 {
		t.Fatalf("err %v calls %v", err, f.Calls)
	}
}

func TestUpWritesVCLWhenVarnish(t *testing.T) {
	e, _, home, dir := ddevEnv(t)
	cfg := "name: ee249-qa\ntype: magento2\nphp: \"8.5\"\nnode: \"22.11.0\"\nvarnish: true\ndatabase:\n  type: mariadb\n  version: \"11.4\"\n"
	os.MkdirAll(filepath.Join(dir, ".md"), 0o755)
	os.WriteFile(filepath.Join(dir, ".md", "config.yaml"), []byte(cfg), 0o644)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, "sites", "ee249-qa", "default.vcl"))
	if err != nil || !strings.Contains(string(b), `.host = "web"`) {
		t.Fatalf("vcl: %v %s", err, b)
	}
}

func TestUpConnectsElasticsearchAlias(t *testing.T) {
	e, f, _, dir := ddevEnv(t)
	cfg := "name: ee249-qa\ntype: magento2\nphp: \"8.5\"\nnode: \"22.11.0\"\nsearch_engine: elasticsearch\nsearch: \"7.17.14\"\ndatabase:\n  type: mariadb\n  version: \"11.4\"\n"
	os.MkdirAll(filepath.Join(dir, ".md"), 0o755)
	os.WriteFile(filepath.Join(dir, ".md", "config.yaml"), []byte(cfg), 0o644)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), "docker network connect --alias elasticsearch md-ee249-qa_default md-elasticsearch-7.17.14") < 0 {
		t.Fatalf("%v", calls(f))
	}
}

func TestMigrateCmdNotesMutagenOnce(t *testing.T) {
	e, _, _, dir := ddevEnv(t)
	os.WriteFile(filepath.Join(dir, ".ddev", "docker-compose.mutagen.yaml"), []byte("x"), 0o644)
	if err := exec(t, e, "migrate"); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(e.out.(*strings.Builder).String(), "mutagen ignored on Linux"); n != 1 {
		t.Fatalf("note printed %d times", n)
	}
}

func TestStateChangingCommandsWaitForLock(t *testing.T) {
	for _, cmd := range []string{"up", "stop", "restart", "poweroff"} {
		t.Run(cmd, func(t *testing.T) {
			e, _, home := upEnv(t)
			unlock, err := lockHome(home, io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- exec(t, e, cmd) }()
			select {
			case <-done:
				t.Fatal("ran while the lock was held")
			case <-time.After(200 * time.Millisecond):
			}
			unlock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("still blocked after unlock")
			}
		})
	}
}
