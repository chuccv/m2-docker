package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chuccv/magento2-docker/internal/run"
)

func mdSiteEnv(t *testing.T) (*env, *run.FakeRunner, string) {
	t.Helper()
	e, f, root := testEnv(t, false)
	dir := filepath.Join(root, "ee249-qa", ".md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("name: ee249-qa\nphp: \"8.4\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return e, f, filepath.Join(dir, "config.yaml")
}

const psKey = "docker ps --filter name=^ddev-ee249-qa-web$ --filter status=running --format {{.Names}}"

func TestCronOnRunning(t *testing.T) {
	e, f, cfg := mdSiteEnv(t)
	f.Out = map[string]string{psKey: "ddev-ee249-qa-web\n"}
	if err := exec(t, e, "cron", "on"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfg)
	if !strings.Contains(string(data), "cron: true") {
		t.Fatalf("config:\n%s", data)
	}
	last := f.Calls[len(f.Calls)-1]
	if !reflect.DeepEqual(last, []string{"docker", "exec", "-u", "1000", "ddev-ee249-qa-web", "sh", "-c", cronScript(true)}) {
		t.Fatalf("calls %v", f.Calls)
	}
}

func TestCronOffRunning(t *testing.T) {
	e, f, cfg := mdSiteEnv(t)
	os.WriteFile(cfg, []byte("name: ee249-qa\ncron: true\n"), 0o644)
	f.Out = map[string]string{psKey: "ddev-ee249-qa-web\n"}
	if err := exec(t, e, "cron", "off"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfg)
	if strings.Contains(string(data), "cron") {
		t.Fatalf("config:\n%s", data)
	}
	last := f.Calls[len(f.Calls)-1]
	if !reflect.DeepEqual(last, []string{"docker", "exec", "-u", "1000", "ddev-ee249-qa-web", "sh", "-c", cronScript(false)}) {
		t.Fatalf("calls %v", f.Calls)
	}
}

func TestCronNotRunning(t *testing.T) {
	e, f, cfg := mdSiteEnv(t)
	if err := exec(t, e, "cron", "on"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfg)
	if !strings.Contains(string(data), "cron: true") {
		t.Fatalf("config:\n%s", data)
	}
	for _, c := range f.Calls {
		if c[1] == "exec" {
			t.Fatalf("unexpected exec %v", c)
		}
	}
}

func TestCronNeedsMDConfig(t *testing.T) {
	e, f, _ := testEnv(t, false)
	err := exec(t, e, "cron", "on")
	if err == nil || !strings.Contains(err.Error(), "md migrate") {
		t.Fatalf("got %v", err)
	}
	if len(f.Calls) != 0 {
		t.Fatalf("calls %v", f.Calls)
	}
}

func TestMCPCron(t *testing.T) {
	e, f, cfg := mdSiteEnv(t)
	f.Out = map[string]string{psKey: "ddev-ee249-qa-web\n"}
	_, isErr := callTool(t, mcpSession(t, e), "cron", map[string]any{"site": "ee249-qa", "on": true})
	if isErr {
		t.Fatal("tool error")
	}
	data, _ := os.ReadFile(cfg)
	if !strings.Contains(string(data), "cron: true") {
		t.Fatalf("config:\n%s", data)
	}
}
