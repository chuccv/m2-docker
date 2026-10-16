package guard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chuccv/m2-docker/internal/config"
	"github.com/chuccv/m2-docker/internal/run"
	"github.com/chuccv/m2-docker/internal/site"
)

var (
	defaultPorts = config.Ports{HTTP: 80, HTTPS: 443, MailpitHTTP: 8025, MailpitHTTPS: 8026, XHGuiHTTP: 8143, XHGuiHTTPS: 8142}
	devPorts     = config.Ports{HTTP: 9080, HTTPS: 9443, MailpitHTTP: 9025, MailpitHTTPS: 9026, XHGuiHTTP: 9143, XHGuiHTTPS: 9142}
)

const (
	psKey       = "docker ps --format {{.Names}}"
	leftoverKey = "docker ps -a --filter label=com.docker.compose.project=ddev-ee249-qa --format {{.Names}}|{{.Label \"com.md.managed\"}}"
)

func qa(t *testing.T) site.Site {
	t.Helper()
	s, err := site.Load(filepath.Join("..", "site", "testdata", "ee249-qa"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCheckNoDDEVRouterRunning(t *testing.T) {
	r := &run.FakeRunner{Out: map[string]string{psKey: "ddev-router\n"}}
	err := CheckNoDDEV(r, qa(t), defaultPorts)
	if err == nil || !strings.Contains(err.Error(), "ddev poweroff") {
		t.Fatalf("got %v", err)
	}
}

func TestCheckNoDDEVRouterRunningDevPorts(t *testing.T) {
	r := &run.FakeRunner{Out: map[string]string{psKey: "ddev-router\n"}}
	if err := CheckNoDDEV(r, qa(t), devPorts); err != nil {
		t.Fatal(err)
	}
}

func TestCheckNoDDEVLeftoverContainer(t *testing.T) {
	r := &run.FakeRunner{Out: map[string]string{leftoverKey: "ddev-ee249-qa-web|\n"}}
	err := CheckNoDDEV(r, qa(t), devPorts)
	if err == nil || !strings.Contains(err.Error(), "ddev stop ee249-qa") {
		t.Fatalf("got %v", err)
	}
}

func versionKey(file string) string {
	return "docker run --rm -u 1000:1000 --entrypoint cat -v ee249-qa-mariadb:/v:ro mariadb:10.6 /v/" + file
}

func TestCheckDBVersion(t *testing.T) {
	cases := []struct {
		name    string
		out     map[string]string
		wantErr []string
	}{
		{"txt match", map[string]string{versionKey("db_mariadb_version.txt"): "mariadb_10.6\n"}, nil},
		{"txt mismatch", map[string]string{versionKey("db_mariadb_version.txt"): "mariadb_10.11\n"}, []string{"10.11", "10.6"}},
		{"fallback mismatch", map[string]string{versionKey("mysql_upgrade_info"): "10.11.8-MariaDB\n"}, []string{"10.11", "10.6"}},
		{"fallback match", map[string]string{versionKey("mysql_upgrade_info"): "10.6.21-MariaDB\n"}, nil},
		{"both missing", map[string]string{}, []string{"cannot be determined"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckDBVersion(&run.FakeRunner{Out: c.out}, qa(t))
			if c.wantErr == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("want error")
			}
			for _, w := range c.wantErr {
				if !strings.Contains(err.Error(), w) {
					t.Fatalf("%q missing %q", err, w)
				}
			}
		})
	}
}

func TestCheckExtraComposeFixture(t *testing.T) {
	if err := CheckExtraCompose(qa(t)); err != nil {
		t.Fatal(err)
	}
}

func TestCheckExtraComposeUnknown(t *testing.T) {
	root := t.TempDir()
	ddev := filepath.Join(root, ".ddev")
	if err := os.MkdirAll(ddev, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ddev, "docker-compose.solr.yaml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := CheckExtraCompose(site.Site{Root: root})
	if err == nil || !strings.Contains(err.Error(), "docker-compose.solr.yaml") {
		t.Fatalf("got %v", err)
	}
}

func TestCheckNoDDEVIgnoresMdManagedContainers(t *testing.T) {
	r := &run.FakeRunner{Out: map[string]string{leftoverKey: "ddev-ee249-qa-web|true\nddev-ee249-qa-db|true\n"}}
	if err := CheckNoDDEV(r, qa(t), devPorts); err != nil {
		t.Fatal(err)
	}
}

func TestCheckNoDDEVSiteNameIsNotPrefixOfOther(t *testing.T) {
	r := &run.FakeRunner{Out: map[string]string{}}
	s := qa(t)
	s.Name = "ee249"
	if err := CheckNoDDEV(r, s, devPorts); err != nil {
		t.Fatal(err)
	}
	key := "docker ps -a --filter label=com.docker.compose.project=ddev-ee249 --format {{.Names}}|{{.Label \"com.md.managed\"}}"
	if !hasCall(r, key) {
		t.Fatalf("filter must be by exact compose project, calls: %v", r.Calls)
	}
}

func hasCall(r *run.FakeRunner, key string) bool {
	for _, c := range r.Calls {
		if strings.Join(c, " ") == key {
			return true
		}
	}
	return false
}

func TestCheckDBVersionMissingVolumeDoesNotRunDocker(t *testing.T) {
	r := &run.FakeRunner{Err: map[string]error{"docker volume inspect ee249-qa-mariadb": errors.New("no such volume")}}
	err := CheckDBVersion(r, qa(t))
	if err == nil || !strings.Contains(err.Error(), "DB volume ee249-qa-mariadb not found") {
		t.Fatalf("got %v", err)
	}
	for _, c := range r.Calls {
		if c[1] == "run" {
			t.Fatalf("docker run executed: %v", c)
		}
	}
}
