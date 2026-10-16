package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mdWebsitesCfg = "name: ee249-qa\nphp: \"8.5\"\ndatabase:\n  type: mariadb\n  version: \"11.4\"\nsearch: \"2.19.1\"\nwebsites:\n  - host: b2b.ee249-qa.ddev.site\n    code: b2b\n    type: website\n"

func mdSite(t *testing.T, withEnvPHP bool) (string, func() []string) {
	t.Helper()
	e, f, home := upEnv(t)
	root := filepath.Join(filepath.Dir(mustCwd(t, e)), "ee249-qa")
	if err := os.MkdirAll(filepath.Join(root, ".md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".md", "config.yaml"), []byte(mdWebsitesCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if withEnvPHP {
		if err := os.MkdirAll(filepath.Join(root, "app", "etc"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "app", "etc", "env.php"), []byte("<?php"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	return home, func() []string { return calls(f) }
}

func mustCwd(t *testing.T, e *env) string {
	t.Helper()
	d, err := e.getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestUpWritesNginxRunWithWebsites(t *testing.T) {
	home, _ := mdSite(t, false)
	data, err := os.ReadFile(filepath.Join(home, "sites", "ee249-qa", "nginx-run.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `b2b.ee249-qa.ddev.site "b2b";`) {
		t.Fatalf("got %s", data)
	}
}

func TestUpWritesEmptyNginxRunForDDEVSite(t *testing.T) {
	e, _, home := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, "sites", "ee249-qa", "nginx-run.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "b2b") || !strings.Contains(string(data), "map $host $md_run_code") {
		t.Fatalf("got %s", data)
	}
}

func TestUpSyncsBaseURLsForMDSiteWithEnvPHP(t *testing.T) {
	_, cs := mdSite(t, true)
	all := cs()
	i := index(all, "config:show --scope=websites --scope-code=b2b web/secure/base_url")
	if i < 0 || i < index(all, "docker compose -p md-ee249-qa") {
		t.Fatalf("missing sync in\n%s", strings.Join(all, "\n"))
	}
}

func TestUpNoSyncWithoutEnvPHP(t *testing.T) {
	_, cs := mdSite(t, false)
	if index(cs(), "config:show") >= 0 {
		t.Fatal("sync ran without env.php")
	}
}

func TestUpNoSyncForDDEVSite(t *testing.T) {
	e, f, _ := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), "config:show") >= 0 {
		t.Fatal("sync ran for ddev site")
	}
}

const reloadCall = "docker exec ddev-ee249-qa-web nginx -s reload"

func TestUpReloadsNginxWhenRunConfChanged(t *testing.T) {
	e, f, home := upEnv(t)
	p := filepath.Join(home, "sites", "ee249-qa", "nginx-run.conf")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	cs := calls(f)
	r := index(cs, reloadCall)
	if r < 0 || r < index(cs, "docker compose -p md-ee249-qa") {
		t.Fatalf("reload missing or early:\n%s", strings.Join(cs, "\n"))
	}
}

func TestUpNoReloadWhenRunConfUnchanged(t *testing.T) {
	e, f, _ := upEnv(t)
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, e, "up"); err != nil {
		t.Fatal(err)
	}
	if index(calls(f), reloadCall) >= 0 {
		t.Fatal("reload ran")
	}
}
