package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chuccv/m2-docker/internal/run"
)

const siteCfg = "name: ee249-qa\ntype: magento2\nphp_version: \"8.5\"\nnodejs_version: \"22.11.0\"\nadditional_hostnames:\n  - shop\ndatabase:\n  type: mariadb\n  version: \"11.4\"\n"

const (
	dbKey     = "docker run --rm -u 1000:1000 --entrypoint cat -v ee249-qa-mariadb:/v:ro mariadb:11.4 /v/db_mariadb_version.txt"
	imgKey    = "docker image inspect -f {{index .Config.Labels \"com.md.hash\"}} md/php:8.5"
	volKey    = "docker volume inspect md-node-22"
	sharedUp  = "docker compose -p md-shared -f %s/shared/compose.yaml up -d"
	siteUpFmt = "docker compose -p md-ee249-qa -f %s/sites/ee249-qa/compose.yaml up -d --wait"
)

func upEnv(t *testing.T) (*env, *run.FakeRunner, string) {
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
	return e, f, home
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
		"docker network connect --alias ee249-qa.ddev.site --alias shop.ddev.site md-ee249-qa_default md-router",
		"bin/magento config:set",
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
	if !strings.Contains(e.out.(*strings.Builder).String(), "https://ee249-qa.ddev.site\n") {
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
	if index(cs, "docker image prune -f --filter label=com.md.image=php") < b {
		t.Fatalf("prune must follow build")
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

func TestUpSkipMigrate(t *testing.T) {
	e, f, _ := upEnv(t)
	if err := exec(t, e, "up", "--skip-migrate"); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	if index(cs, "config:set") >= 0 || index(cs, "indexer:reindex") >= 0 {
		t.Fatalf("migrate ran with --skip-migrate:\n%s", strings.Join(cs, "\n"))
	}
	if index(cs, "docker compose -p md-ee249-qa") < 0 {
		t.Fatalf("site not started:\n%s", strings.Join(cs, "\n"))
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

func TestUpWithoutSkipMigrateRunsMigrate(t *testing.T) {
	e, f, _ := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), "config:set") < 0 {
		t.Fatalf("migrate did not run:\n%s", strings.Join(calls(f), "\n"))
	}
}
