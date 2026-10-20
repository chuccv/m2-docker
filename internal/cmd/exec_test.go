package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/chuccv/m2-docker/internal/run"
)

func testEnv(t *testing.T, tty bool) (*env, *run.FakeRunner, string) {
	t.Helper()
	root := t.TempDir()
	sd := filepath.Join(root, "ee249-qa", ".ddev")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "name: ee249-qa\ntype: magento2\nphp_version: \"8.5\"\nadditional_hostnames:\n  - shop\n"
	if err := os.WriteFile(filepath.Join(sd, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	f := &run.FakeRunner{}
	e := &env{
		runner:    f,
		tty:       func() bool { return tty },
		lookup:    func(string) ([]string, error) { return []string{"127.0.0.1"}, nil },
		errOut:    &strings.Builder{},
		scanRoots: []string{root},
		getwd:     func() (string, error) { return filepath.Join(root, "ee249-qa"), nil },
		home:      func() (string, error) { return home, nil },
	}
	return e, f, root
}

func exec(t *testing.T, e *env, args ...string) error {
	t.Helper()
	root := &cobra.Command{Use: "md", SilenceUsage: true, SilenceErrors: true}
	register(root, e)
	root.SetArgs(args)
	root.SetOut(&strings.Builder{})
	return root.Execute()
}

func want(t *testing.T, f *run.FakeRunner, w ...string) {
	t.Helper()
	if len(f.Calls) != 1 || !reflect.DeepEqual(f.Calls[0], w) {
		t.Fatalf("calls %v\nwant %v", f.Calls, w)
	}
}

func TestMagentoFromCwd(t *testing.T) {
	e, f, _ := testEnv(t, false)
	if err := exec(t, e, "magento", "cache:flush"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-i", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "php", "bin/magento", "cache:flush")
}

func TestMagentoTTY(t *testing.T) {
	e, f, _ := testEnv(t, true)
	if err := exec(t, e, "magento", "cache:flush"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-i", "-t", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "php", "bin/magento", "cache:flush")
}

func TestMagentoKeepsFlags(t *testing.T) {
	e, f, _ := testEnv(t, false)
	if err := exec(t, e, "magento", "setup:upgrade", "--keep-generated"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-i", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "php", "bin/magento", "setup:upgrade", "--keep-generated")
	e, f, _ = testEnv(t, false)
	if err := exec(t, e, "magento", "cache:clean", "--help"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-i", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "php", "bin/magento", "cache:clean", "--help")
}

func TestSiteFlagBeforeAndAfterCommand(t *testing.T) {
	for _, args := range [][]string{
		{"-s", "ee249-qa", "php", "-v"},
		{"php", "-s", "ee249-qa", "-v"},
		{"php", "--site=ee249-qa", "-v"},
	} {
		e, f, root := testEnv(t, false)
		e.getwd = func() (string, error) { return root, nil }
		if err := exec(t, e, args...); err != nil {
			t.Fatal(args, err)
		}
		want(t, f, "docker", "exec", "-i", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "php", "-v")
	}
}

func TestPhpAndComposer(t *testing.T) {
	e, f, _ := testEnv(t, false)
	if err := exec(t, e, "composer", "install", "-n"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-i", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "composer", "install", "-n")
}

func TestExecDashDash(t *testing.T) {
	e, f, _ := testEnv(t, false)
	if err := exec(t, e, "exec", "--", "ls", "-la"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-i", "-u", "1000", "-w", "/var/www/html", "ddev-ee249-qa-web", "ls", "-la")
}

func TestExecPropagatesError(t *testing.T) {
	e, f, _ := testEnv(t, false)
	boom := errors.New("exit 3")
	f.Err = map[string]error{"docker exec -i -u 1000 -w /var/www/html ddev-ee249-qa-web false": boom}
	if err := exec(t, e, "exec", "false"); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestMysql(t *testing.T) {
	e, f, _ := testEnv(t, false)
	if err := exec(t, e, "mysql"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-i", "ddev-ee249-qa-db", "mariadb", "-udb", "-pdb", "db")
	e, f, _ = testEnv(t, true)
	if err := exec(t, e, "mysql", "-e", "select 1"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-i", "-t", "ddev-ee249-qa-db", "mariadb", "-udb", "-pdb", "db", "-e", "select 1")
}

func TestXdebugToggle(t *testing.T) {
	e, f, _ := testEnv(t, false)
	if err := exec(t, e, "xdebug", "on"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-u", "1000", "ddev-ee249-qa-web", "sh", "-c", toggleScript("xdebug", true))
	e, f, _ = testEnv(t, false)
	if err := exec(t, e, "xdebug", "off"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-u", "1000", "ddev-ee249-qa-web", "sh", "-c", toggleScript("xdebug", false))
}

func TestToggleScriptIdempotent(t *testing.T) {
	s := toggleScript("xdebug", true)
	for _, w := range []string{"/usr/local/etc/php/conf.d/xdebug.ini.off", "/usr/local/etc/php/conf.d/xdebug.ini", "pkill -USR2 -o php-fpm"} {
		if !strings.Contains(s, w) {
			t.Fatalf("missing %q in %s", w, s)
		}
	}
}

func TestXhprofOnPrintsURL(t *testing.T) {
	e, f, _ := testEnv(t, false)
	var out strings.Builder
	e.out = &out
	if err := exec(t, e, "xhprof", "on"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-u", "1000", "ddev-ee249-qa-web", "sh", "-c", toggleScript("xhprof", true))
	if got := out.String(); got != "https://ee249-qa.md.site:8142\n" {
		t.Fatalf("got %q", got)
	}
}

func TestXhprofOffNoURL(t *testing.T) {
	e, _, _ := testEnv(t, false)
	var out strings.Builder
	e.out = &out
	if err := exec(t, e, "xhprof", "off"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "" {
		t.Fatalf("got %q", out.String())
	}
}

func TestToggleRejectsBadArg(t *testing.T) {
	e, _, _ := testEnv(t, false)
	if err := exec(t, e, "xdebug", "maybe"); err == nil {
		t.Fatal("want error")
	}
}

func TestMysqlClientByDBType(t *testing.T) {
	e, f, root := testEnv(t, false)
	cfg := "name: ee249-qa\ntype: magento2\nphp_version: \"8.5\"\ndatabase:\n  type: mysql\n  version: \"8.0\"\n"
	os.WriteFile(filepath.Join(root, "ee249-qa", ".ddev", "config.yaml"), []byte(cfg), 0o644)
	if err := exec(t, e, "mysql"); err != nil {
		t.Fatal(err)
	}
	want(t, f, "docker", "exec", "-i", "ddev-ee249-qa-db", "mysql", "-udb", "-pdb", "db")
}
